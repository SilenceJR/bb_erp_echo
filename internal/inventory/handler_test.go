package inventory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

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

func TestItemDetailAggregatesDefaultWarehouseLocations(t *testing.T) {
	db := openInventoryTestDB(t)
	handler := NewHandler(db)
	var warehouse model.Warehouse
	if err := db.Where("code = ?", "MAIN").First(&warehouse).Error; err != nil {
		t.Fatalf("find default warehouse: %v", err)
	}
	product := model.Product{Name: "聚合产品", Code: "P-AGGREGATE", Unit: "个", Status: model.StatusActive}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	locations := []model.Location{
		{WarehouseID: warehouse.ID, Code: "A-01", Name: "A 货架", Status: model.StatusActive},
		{WarehouseID: warehouse.ID, Code: "B-01", Name: "B 货架", Status: model.StatusActive},
	}
	if err := db.Create(&locations).Error; err != nil {
		t.Fatalf("seed locations: %v", err)
	}
	balances := []model.InventoryBalance{
		{WarehouseID: warehouse.ID, LocationID: &locations[0].ID, ItemType: itemProduct, ItemID: product.ID, Quantity: 10000, Amount: 2500},
		{WarehouseID: warehouse.ID, LocationID: &locations[1].ID, ItemType: itemProduct, ItemID: product.ID, Quantity: 30000, Amount: 9000},
	}
	if err := db.Create(&balances).Error; err != nil {
		t.Fatalf("seed balances: %v", err)
	}

	rec := performInventoryJSON(t, handler.GetItemDetail, http.MethodGet, "/api/v1/warehouse/items/:itemType/:itemID", nil, map[string]string{
		"itemType": itemProduct,
		"itemID":   strconv.FormatUint(uint64(product.ID), 10),
	}, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", rec.Code, rec.Body.String())
	}
	var withoutCost map[string]any
	decodeInventoryJSON(t, rec, &withoutCost)
	if withoutCost["quantity"].(float64) != 40000 {
		t.Fatalf("aggregated quantity = %v, want 40000", withoutCost["quantity"])
	}
	if _, ok := withoutCost["amount"]; ok {
		t.Fatalf("amount should be hidden without cost permission: %v", withoutCost)
	}

	rec = performInventoryJSON(t, handler.GetItemDetail, http.MethodGet, "/api/v1/warehouse/items/:itemType/:itemID", nil, map[string]string{
		"itemType": itemProduct,
		"itemID":   strconv.FormatUint(uint64(product.ID), 10),
	}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("cost detail status = %d body=%s", rec.Code, rec.Body.String())
	}
	var withCost map[string]any
	decodeInventoryJSON(t, rec, &withCost)
	if withCost["quantity"].(float64) != 40000 || withCost["amount"].(float64) != 11500 || withCost["avg_cost"].(float64) != 2875 {
		t.Fatalf("aggregated cost detail = %v", withCost)
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

func TestInventoryIdempotencyRejectsDifferentRequestAndScope(t *testing.T) {
	db := openInventoryTestDB(t)
	handler := NewHandler(db)
	material := model.Material{Name: "幂等物料", Code: "IDEMP-MAT", Unit: "个", Status: model.StatusActive}
	supplier := model.Supplier{Name: "幂等供应商", Code: "IDEMP-SUP", Status: model.StatusActive}
	if err := db.Create(&material).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&supplier).Error; err != nil {
		t.Fatal(err)
	}

	documentBody := map[string]any{
		"code":         "IDEMP-DOC",
		"type":         typeInbound,
		"warehouse_id": uint(1),
		"lines": []map[string]any{{
			"item_type": itemMaterial,
			"item_id":   material.ID,
			"quantity":  int64(10000),
		}},
	}
	const key = "same-business-key"
	if rec := performInventoryDocumentJSON(t, handler, documentBody, key, 1); rec.Code != http.StatusCreated {
		t.Fatalf("first document status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := performInventoryDocumentJSON(t, handler, documentBody, key, 1); rec.Code != http.StatusOK {
		t.Fatalf("same document status = %d body=%s", rec.Code, rec.Body.String())
	}
	documentBody["lines"].([]map[string]any)[0]["quantity"] = int64(20000)
	if rec := performInventoryDocumentJSON(t, handler, documentBody, key, 1); rec.Code != http.StatusConflict {
		t.Fatalf("different quantity status = %d body=%s", rec.Code, rec.Body.String())
	}

	movementBody := map[string]any{
		"business_type": businessPurchaseInbound,
		"quantity":      int64(10000),
		"supplier_id":   supplier.ID,
	}
	if rec := performItemMovementJSON(t, handler, material, movementBody, key, []string{"suppliers:read"}); rec.Code != http.StatusConflict {
		t.Fatalf("cross-scope status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInventoryDocumentIdempotencyKeyIsPartialUnique(t *testing.T) {
	db := openInventoryTestDB(t)
	first := model.InventoryDocument{Code: "IDEMP-EMPTY-1", Type: typeInbound, Status: documentDraft, WarehouseID: 1}
	second := model.InventoryDocument{Code: "IDEMP-EMPTY-2", Type: typeInbound, Status: documentDraft, WarehouseID: 1}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first empty key document: %v", err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("empty idempotency keys should remain reusable: %v", err)
	}

	first = model.InventoryDocument{Code: "IDEMP-KEY-1", Type: typeInbound, Status: documentDraft, WarehouseID: 1, IdempotencyKey: "same-key"}
	second = model.InventoryDocument{Code: "IDEMP-KEY-2", Type: typeInbound, Status: documentDraft, WarehouseID: 1, IdempotencyKey: "same-key"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create keyed document: %v", err)
	}
	if err := db.Create(&second).Error; err == nil || !isUniqueConstraintError(err) {
		t.Fatalf("duplicate non-empty idempotency key error = %v, want unique constraint", err)
	}
}

func TestInventoryBalanceLocationNilIsUnique(t *testing.T) {
	db := openInventoryTestDB(t)
	first := model.InventoryBalance{WarehouseID: 1, ItemType: itemMaterial, ItemID: 1}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first default-location balance: %v", err)
	}
	duplicateDefault := model.InventoryBalance{WarehouseID: 1, ItemType: itemMaterial, ItemID: 1}
	if err := db.Create(&duplicateDefault).Error; err == nil || !isUniqueConstraintError(err) {
		t.Fatalf("duplicate default-location balance error = %v, want unique constraint", err)
	}

	locationOne := model.Location{WarehouseID: 1, Code: "UNIQUE-A", Name: "库位 A", Status: model.StatusActive}
	locationTwo := model.Location{WarehouseID: 1, Code: "UNIQUE-B", Name: "库位 B", Status: model.StatusActive}
	if err := db.Create(&locationOne).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&locationTwo).Error; err != nil {
		t.Fatal(err)
	}
	withLocation := model.InventoryBalance{WarehouseID: 1, LocationID: &locationOne.ID, ItemType: itemMaterial, ItemID: 1}
	if err := db.Create(&withLocation).Error; err != nil {
		t.Fatalf("create explicit-location balance: %v", err)
	}
	duplicateLocation := model.InventoryBalance{WarehouseID: 1, LocationID: &locationOne.ID, ItemType: itemMaterial, ItemID: 1}
	if err := db.Create(&duplicateLocation).Error; err == nil || !isUniqueConstraintError(err) {
		t.Fatalf("duplicate explicit-location balance error = %v, want unique constraint", err)
	}
	differentLocation := model.InventoryBalance{WarehouseID: 1, LocationID: &locationTwo.ID, ItemType: itemMaterial, ItemID: 1}
	if err := db.Create(&differentLocation).Error; err != nil {
		t.Fatalf("different-location balance should be allowed: %v", err)
	}
}

func TestCreateDocumentIdempotencyIsSerializedForConcurrentSameKey(t *testing.T) {
	db := openInventoryTestDB(t)
	handler := NewHandler(db)
	body := map[string]any{
		"code":                 "CONCURRENT-IDEMP",
		"type":                 typeInbound,
		"warehouse_id":         1,
		"operator_employee_id": 1,
		"lines": []map[string]any{{
			"item_type": itemMaterial,
			"item_id":   1,
			"quantity":  10000,
		}},
	}
	const key = "concurrent-same-key"
	responses := make(chan *httptest.ResponseRecorder, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			responses <- performInventoryDocumentJSON(t, handler, body, key, 1)
		}()
	}
	wait.Wait()
	close(responses)

	created := 0
	returned := 0
	for rec := range responses {
		switch rec.Code {
		case http.StatusCreated:
			created++
		case http.StatusOK:
			returned++
		default:
			t.Fatalf("concurrent idempotent create status = %d body=%s", rec.Code, rec.Body.String())
		}
	}
	if created != 1 || returned != 1 {
		t.Fatalf("concurrent responses created=%d returned=%d, want one each", created, returned)
	}
	var count int64
	if err := db.Model(&model.InventoryDocument{}).Where("idempotency_key = ?", key).Count(&count).Error; err != nil {
		t.Fatalf("count keyed documents: %v", err)
	}
	if count != 1 {
		t.Fatalf("keyed document count = %d, want 1", count)
	}
}

// TestItemMovementAcceptsFourDecimalInboundQuantity 验证采购入库支持按四位定点精度
// 直接提交校准数量，避免界面输入的 999.0000 被后端截断或改变。
func TestItemMovementAcceptsFourDecimalInboundQuantity(t *testing.T) {
	db := openInventoryTestDB(t)
	handler := NewHandler(db)
	supplier := model.Supplier{Name: "校准供应商", Code: "SUP-CALIBRATE", Status: model.StatusActive}
	material := model.Material{Name: "校准物料", Code: "MAT-CALIBRATE", Unit: "个", Status: model.StatusActive}
	if err := db.Create(&supplier).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&material).Error; err != nil {
		t.Fatal(err)
	}

	const calibratedQuantity int64 = 9990000
	rec := performItemMovementJSON(t, handler, material, map[string]any{
		"business_type": businessPurchaseInbound,
		"quantity":      calibratedQuantity,
		"supplier_id":   supplier.ID,
	}, "movement-calibrated-quantity", []string{"suppliers:read"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("calibrated inbound status = %d body=%s", rec.Code, rec.Body.String())
	}

	var balance model.InventoryBalance
	if err := db.Where("item_type = ? AND item_id = ?", itemMaterial, material.ID).First(&balance).Error; err != nil {
		t.Fatal(err)
	}
	if balance.Quantity != calibratedQuantity {
		t.Fatalf("calibrated quantity = %d, want %d", balance.Quantity, calibratedQuantity)
	}

	rec = performItemMovementJSON(t, handler, material, map[string]any{
		"business_type": businessPurchaseInbound,
		"quantity":      maxMovementQuantity,
		"supplier_id":   supplier.ID,
	}, "movement-max-quantity", []string{"suppliers:read"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("maximum inbound status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = performItemMovementJSON(t, handler, material, map[string]any{
		"business_type": businessPurchaseInbound,
		"quantity":      maxMovementQuantity + 1,
		"supplier_id":   supplier.ID,
	}, "movement-over-max-quantity", []string{"suppliers:read"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("over maximum inbound status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = performItemMovementJSON(t, handler, material, map[string]any{
		"business_type": businessPurchaseInbound,
		"quantity":      int64(0),
		"supplier_id":   supplier.ID,
	}, "movement-zero-quantity", []string{"suppliers:read"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("zero inbound status = %d body=%s", rec.Code, rec.Body.String())
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
	if err := db.AutoMigrate(&model.Warehouse{}, &model.Location{}, &model.Material{}, &model.Product{}, &model.Supplier{}, &model.Customer{}, &model.Department{}, &model.Employee{}, &model.EmployeeDepartment{}, &model.User{}, &model.InventoryDocument{}, &model.InventoryDocumentLine{}, &model.InventoryBalance{}, &model.InventoryLedger{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	department := model.Department{Name: "测试部门", Code: "TEST-DEPT", OrganizationID: 1, Status: model.StatusActive}
	if err := db.Create(&department).Error; err != nil {
		t.Fatalf("seed department: %v", err)
	}
	birth := time.Date(1990, 1, 1, 0, 0, 0, 0, time.Local)
	operator := model.Employee{OrganizationID: 1, Name: "测试操作人", HireDate: birth, BirthDate: birth, Status: model.StatusActive}
	if err := db.Create(&operator).Error; err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	if err := db.Create(&model.EmployeeDepartment{EmployeeID: operator.ID, DepartmentID: department.ID}).Error; err != nil {
		t.Fatalf("link operator: %v", err)
	}
	account := model.User{Username: "tester", AccountType: model.AccountTypePersonal, Name: "测试账号", OrganizationID: 1, DepartmentID: &department.ID, Status: model.StatusActive, PasswordHash: "test"}
	if err := db.Create(&account).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Create(&model.Warehouse{Name: "默认仓库", Code: "MAIN", Status: model.StatusActive}).Error; err != nil {
		t.Fatalf("seed default warehouse: %v", err)
	}
	return db
}

func performItemMovementJSON(t *testing.T, handler *Handler, item any, body any, idempotencyKey string, permissions []string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.Validator = &testValidator{validate: validator.New()}
	raw, _ := json.Marshal(body)
	if requestBody, ok := body.(map[string]any); ok {
		if _, exists := requestBody["operator_employee_id"]; !exists {
			requestBody["operator_employee_id"] = uint(1)
		}
		raw, _ = json.Marshal(requestBody)
	}
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
	departmentID := uint(1)
	c.Set(auth.ContextUserKey, &auth.CurrentUser{ID: 1, Username: "tester", OrganizationID: 1, DepartmentID: &departmentID, Permissions: permissions})
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
		body = map[string]any{"operator_employee_id": uint(1)}
		raw, _ := json.Marshal(body)
		payload = bytes.NewReader(raw)
	} else {
		raw, _ := json.Marshal(body)
		if requestBody, ok := body.(map[string]any); ok {
			if _, exists := requestBody["operator_employee_id"]; !exists {
				requestBody["operator_employee_id"] = uint(1)
			}
			raw, _ = json.Marshal(requestBody)
		}
		payload = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, payload)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if len(params) > 0 {
		names := make([]string, 0, len(params))
		values := make([]string, 0, len(params))
		for key, value := range params {
			names = append(names, key)
			values = append(values, value)
		}
		pathValues := make(echo.PathValues, 0, len(names))
		for i := range names {
			pathValues = append(pathValues, echo.PathValue{Name: names[i], Value: values[i]})
		}
		c.SetPathValues(pathValues)
	}
	departmentID := uint(1)
	current := &auth.CurrentUser{ID: 1, Username: "tester", OrganizationID: 1, DepartmentID: &departmentID}
	if costView {
		current.Permissions = []string{role.CostViewCode}
	}
	c.Set(auth.ContextUserKey, current)
	if err := handler(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}

func performInventoryDocumentJSON(t *testing.T, handler *Handler, body map[string]any, idempotencyKey string, departmentID uint) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.Validator = &testValidator{validate: validator.New()}
	if _, exists := body["operator_employee_id"]; !exists {
		body["operator_employee_id"] = uint(1)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal document body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory-documents", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserKey, &auth.CurrentUser{ID: 1, Username: "tester", OrganizationID: 1, DepartmentID: &departmentID})
	if err := handler.CreateDocument(c); err != nil {
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
