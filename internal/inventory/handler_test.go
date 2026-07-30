package inventory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/role"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testValidator struct {
	validate *validator.Validate
}

func (v *testValidator) Validate(i any) error {
	return v.validate.Struct(i)
}

// TestInventoryInboundWeightedAverageAndCostTrim 验证入库过账会更新移动平均成本，
// 且无成本权限的响应不会返回金额字段。
func TestInventoryInboundWeightedAverageAndCostTrim(t *testing.T) {
	db := openInventoryTestDB(t)
	handler := NewHandler(db)
	warehouse := model.Warehouse{Name: "主仓", Code: "WH-001", Status: model.StatusActive}
	material := model.Material{Name: "ABS", Code: "ABS-001", Unit: "kg", Status: model.StatusActive}
	if err := db.Create(&warehouse).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Create(&material).Error; err != nil {
		t.Fatalf("seed material: %v", err)
	}

	body := map[string]any{
		"code":         "IN-001",
		"type":         typeInbound,
		"warehouse_id": warehouse.ID,
		"lines": []map[string]any{
			{
				"item_type": itemMaterial,
				"item_id":   material.ID,
				"quantity":  int64(1000000),
				"unit_cost": int64(250),
			},
		},
	}
	rec := performInventoryJSON(t, handler.CreateDocument, http.MethodPost, "/api/v1/inventory-documents", body, nil, false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create document status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	decodeInventoryJSON(t, rec, &created)
	id := uint(created["id"].(float64))

	rec = performInventoryJSON(t, handler.PostDocument, http.MethodPost, "/api/v1/inventory-documents/:id/post", nil, map[string]string{
		"id": strconv.FormatUint(uint64(id), 10),
	}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("post document status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performInventoryJSON(t, handler.ListBalances, http.MethodGet, "/api/v1/inventory-balances", nil, nil, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("list balances status = %d body=%s", rec.Code, rec.Body.String())
	}
	var noCost []map[string]any
	decodeInventoryJSON(t, rec, &noCost)
	if _, ok := noCost[0]["amount"]; ok {
		t.Fatalf("amount should be hidden without cost permission: %v", noCost[0])
	}

	rec = performInventoryJSON(t, handler.ListBalances, http.MethodGet, "/api/v1/inventory-balances", nil, nil, true)
	var withCost []map[string]any
	decodeInventoryJSON(t, rec, &withCost)
	if withCost[0]["avg_cost"].(float64) != 250 || withCost[0]["amount"].(float64) != 25000 {
		t.Fatalf("weighted cost mismatch: %v", withCost[0])
	}
}

// TestInventoryOutboundRejectsNegativeStock 验证默认禁止负库存。
func TestInventoryOutboundRejectsNegativeStock(t *testing.T) {
	db := openInventoryTestDB(t)
	handler := NewHandler(db)
	warehouse := model.Warehouse{Name: "主仓", Code: "WH-001", Status: model.StatusActive}
	material := model.Material{Name: "ABS", Code: "ABS-001", Unit: "kg", Status: model.StatusActive}
	if err := db.Create(&warehouse).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	if err := db.Create(&material).Error; err != nil {
		t.Fatalf("seed material: %v", err)
	}
	doc := model.InventoryDocument{
		Code:        "OUT-001",
		Type:        typeOutbound,
		Status:      documentDraft,
		WarehouseID: warehouse.ID,
		Lines: []model.InventoryDocumentLine{
			{ItemType: itemMaterial, ItemID: material.ID, Quantity: 10000},
		},
	}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	rec := performInventoryJSON(t, handler.PostDocument, http.MethodPost, "/api/v1/inventory-documents/:id/post", nil, map[string]string{
		"id": strconv.FormatUint(uint64(doc.ID), 10),
	}, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative stock status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestItemMovementsPostImmediatelyAndAreIdempotent(t *testing.T) {
	db := openInventoryTestDB(t)
	handler := NewHandler(db)
	supplier := model.Supplier{Name: "测试供应商", Code: "SUP-001", Status: model.StatusActive}
	material := model.Material{Name: "ABS", Code: "ABS-MOVE", Unit: "kg", Status: model.StatusActive}
	if err := db.Create(&supplier).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&material).Error; err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"business_type": businessPurchaseInbound,
		"quantity":      int64(50000),
		"unit_cost":     int64(250),
		"supplier_id":   supplier.ID,
	}
	permissions := []string{"suppliers:read", role.CostViewCode}
	rec := performItemMovementJSON(t, handler, material, body, "movement-001", permissions)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create movement status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	decodeInventoryJSON(t, rec, &created)
	if created["status"] != documentPosted || created["business_type"] != businessPurchaseInbound {
		t.Fatalf("movement was not posted: %v", created)
	}

	rec = performItemMovementJSON(t, handler, material, body, "movement-001", permissions)
	if rec.Code != http.StatusOK {
		t.Fatalf("idempotent movement status = %d body=%s", rec.Code, rec.Body.String())
	}
	var documentCount int64
	db.Model(&model.InventoryDocument{}).Count(&documentCount)
	if documentCount != 1 {
		t.Fatalf("document count = %d, want 1", documentCount)
	}
	var balance model.InventoryBalance
	if err := db.Where("item_type = ? AND item_id = ?", itemMaterial, material.ID).First(&balance).Error; err != nil {
		t.Fatal(err)
	}
	if balance.Quantity != 50000 {
		t.Fatalf("balance quantity = %d, want 50000", balance.Quantity)
	}
}

func TestItemMovementBusinessPartiesAndReturn(t *testing.T) {
	db := openInventoryTestDB(t)
	handler := NewHandler(db)
	supplier := model.Supplier{Name: "供应商", Code: "SUP-002", Status: model.StatusActive}
	customer := model.Customer{Name: "客户", Code: "CUST-MOVE"}
	department := model.Department{Name: "生产部", Code: "PROD", OrganizationID: 1, Status: model.StatusActive}
	product := model.Product{Name: "成品", Code: "PROD-MOVE", Unit: "个", Status: model.StatusActive}
	for _, value := range []any{&supplier, &customer, &department, &product} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	allPermissions := []string{"suppliers:read", "customers:read", "system:departments:read"}
	cases := []map[string]any{
		{"business_type": businessPurchaseInbound, "quantity": int64(50000), "supplier_id": supplier.ID},
		{"business_type": businessCustomerOutbound, "quantity": int64(10000), "customer_id": customer.ID},
		{"business_type": businessDepartmentOutbound, "quantity": int64(10000), "department_id": department.ID},
		{"business_type": businessReturnReworkInbound, "quantity": int64(5000), "customer_id": customer.ID, "reason": "客户返工"},
	}
	for index, body := range cases {
		rec := performItemMovementJSON(t, handler, product, body, fmt.Sprintf("party-%d", index), allPermissions)
		if rec.Code != http.StatusCreated {
			t.Fatalf("case %d status = %d body=%s", index, rec.Code, rec.Body.String())
		}
	}
	var balance model.InventoryBalance
	if err := db.Where("item_type = ? AND item_id = ?", itemProduct, product.ID).First(&balance).Error; err != nil {
		t.Fatal(err)
	}
	if balance.Quantity != 35000 {
		t.Fatalf("balance quantity = %d, want 35000", balance.Quantity)
	}
}

func openInventoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.Warehouse{}, &model.Material{}, &model.Product{}, &model.Supplier{}, &model.Customer{}, &model.Department{}, &model.InventoryDocument{}, &model.InventoryDocumentLine{}, &model.InventoryBalance{}, &model.InventoryLedger{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func performItemMovementJSON(t *testing.T, handler *Handler, item any, body any, idempotencyKey string, permissions []string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.Validator = &testValidator{validate: validator.New()}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/warehouse/items/:itemType/:itemID/movements", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	var itemTypeValue string
	var itemID uint
	switch value := item.(type) {
	case model.Material:
		itemTypeValue, itemID = itemMaterial, value.ID
	case model.Product:
		itemTypeValue, itemID = itemProduct, value.ID
	default:
		t.Fatalf("unsupported item type %T", item)
	}
	c.SetPathValues(echo.PathValues{
		{Name: "itemType", Value: itemTypeValue},
		{Name: "itemID", Value: strconv.FormatUint(uint64(itemID), 10)},
	})
	c.Set(auth.ContextUserKey, &auth.CurrentUser{ID: 1, Username: "tester", OrganizationID: 1, Permissions: permissions})
	if err := handler.CreateItemMovement(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}

func performInventoryJSON(t *testing.T, handler echo.HandlerFunc, method string, path string, body any, params map[string]string, costView bool) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.Validator = &testValidator{validate: validator.New()}
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(body)
		payload = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, payload)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if len(params) > 0 {
		pathValues := make(echo.PathValues, 0, len(params))
		for key, value := range params {
			pathValues = append(pathValues, echo.PathValue{Name: key, Value: value})
		}
		c.SetPathValues(pathValues)
	}
	current := &auth.CurrentUser{ID: 1, Username: "tester", OrganizationID: 1}
	if costView {
		current.Permissions = []string{role.CostViewCode}
	}
	c.Set(auth.ContextUserKey, current)
	if err := handler(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}

func decodeInventoryJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode json %q: %v", rec.Body.String(), err)
	}
}
