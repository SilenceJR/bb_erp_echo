package inventory

import (
	"bytes"
	"encoding/json"
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
	if err := db.AutoMigrate(&model.Warehouse{}, &model.Material{}, &model.Product{}, &model.InventoryDocument{}, &model.InventoryDocumentLine{}, &model.InventoryBalance{}, &model.InventoryLedger{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
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
