package warehouse

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"

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

// TestSingleWarehouseAlwaysReturnsDefault 验证仓库模块按单仓库使用，
// 启动种子已创建默认仓库，GET 只读取且始终返回这一条记录。
func TestSingleWarehouseAlwaysReturnsDefault(t *testing.T) {
	db := openWarehouseTestDB(t)
	handler := NewHandler(db)

	rec := performWarehouseJSON(t, handler.ListWarehouses, http.MethodGet, "/api/v1/warehouses", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list warehouses status = %d body=%s", rec.Code, rec.Body.String())
	}
	var items []model.Warehouse
	decodeWarehouseJSON(t, rec, &items)
	if len(items) != 1 || items[0].Code != "MAIN" {
		t.Fatalf("default warehouse mismatch: %+v", items)
	}
	var count int64
	if err := db.Model(&model.Warehouse{}).Count(&count).Error; err != nil {
		t.Fatalf("count warehouses: %v", err)
	}
	if count != 1 {
		t.Fatalf("warehouse count = %d, want 1", count)
	}
}

func TestListWarehousesDoesNotCreateMissingDefault(t *testing.T) {
	db := openWarehouseTestDB(t)
	if err := db.Where("code = ?", "MAIN").Delete(&model.Warehouse{}).Error; err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(db)
	rec := performWarehouseJSON(t, handler.ListWarehouses, http.MethodGet, "/api/v1/warehouses", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("missing default status=%d body=%s", rec.Code, rec.Body.String())
	}
	var count int64
	if err := db.Model(&model.Warehouse{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("GET recreated warehouse count=%d err=%v", count, err)
	}
}

func TestCreateWarehouseKeepsSystemCodeFixed(t *testing.T) {
	db := openWarehouseTestDB(t)
	handler := NewHandler(db)

	rec := performWarehouseJSON(t, handler.CreateWarehouse, http.MethodPost, "/api/v1/warehouses", map[string]any{
		"name": "更新后的主仓",
		"code": "WH-OTHER",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-MAIN code status = %d body=%s", rec.Code, rec.Body.String())
	}
	var warehouse model.Warehouse
	if err := db.Where("code = ?", model.DefaultWarehouseCode).First(&warehouse).Error; err != nil {
		t.Fatal(err)
	}
	if warehouse.Name != "默认仓库" || warehouse.Code != model.DefaultWarehouseCode {
		t.Fatalf("rejected update changed warehouse: %+v", warehouse)
	}

	rec = performWarehouseJSON(t, handler.CreateWarehouse, http.MethodPost, "/api/v1/warehouses", map[string]any{
		"name": "更新后的主仓",
		"code": model.DefaultWarehouseCode,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("MAIN code status = %d body=%s", rec.Code, rec.Body.String())
	}
	if err := db.First(&warehouse, warehouse.ID).Error; err != nil {
		t.Fatal(err)
	}
	if warehouse.Name != "更新后的主仓" || warehouse.Code != model.DefaultWarehouseCode {
		t.Fatalf("warehouse update result = %+v", warehouse)
	}
}

// TestWarehouseCatalogTabsCreateProductsAndMaterials 验证仓库标签策略：
// 产品写入产品表，生产物资等标签写入物料表并固定分类。
func TestWarehouseCatalogTabsCreateProductsAndMaterials(t *testing.T) {
	db := openWarehouseTestDB(t)
	handler := NewHandler(db)

	rec := performWarehouseJSON(t, handler.CreateItem, http.MethodPost, "/api/v1/warehouse/items", map[string]any{
		"tab":          tabProduct,
		"name":         "白壳成品",
		"code":         "P-001",
		"unit":         "个",
		"spec":         "标准",
		"safety_stock": int64(10000),
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create product tab item status = %d body=%s", rec.Code, rec.Body.String())
	}
	var productCount int64
	if err := db.Model(&model.Product{}).Count(&productCount).Error; err != nil {
		t.Fatalf("count products: %v", err)
	}
	if productCount != 1 {
		t.Fatalf("product count = %d", productCount)
	}
	var product model.Product
	if err := db.First(&product).Error; err != nil {
		t.Fatalf("find product: %v", err)
	}
	if product.OperatorEmployeeID == nil || *product.OperatorEmployeeID != 1 || product.OperatorEmployeeName != "测试操作人" {
		t.Fatalf("product operator snapshot = %+v", product.OperatorSnapshot)
	}

	rec = performWarehouseJSON(t, handler.CreateItem, http.MethodPost, "/api/v1/warehouse/items", map[string]any{
		"tab":  tabProductionMaterial,
		"name": "ABS 原料",
		"code": "ABS-001",
		"unit": "kg",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create material tab item status = %d body=%s", rec.Code, rec.Body.String())
	}
	var material model.Material
	if err := db.First(&material).Error; err != nil {
		t.Fatalf("find material: %v", err)
	}
	if material.Category != "生产物资" {
		t.Fatalf("material category = %q", material.Category)
	}
}

func openWarehouseTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&model.Warehouse{}, &model.Material{}, &model.Product{}, &model.Department{}, &model.Employee{}, &model.EmployeeDepartment{}, &model.User{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	warehouse := model.Warehouse{Name: "默认仓库", Code: "MAIN", Status: model.StatusActive}
	if err := db.Create(&warehouse).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
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
	return db
}

func performWarehouseJSON(t *testing.T, handler echo.HandlerFunc, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	if requestBody, ok := body.(map[string]any); ok {
		if _, exists := requestBody["operator_employee_id"]; !exists {
			requestBody["operator_employee_id"] = uint(1)
		}
	}
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	e := echo.New()
	e.Validator = &testValidator{validate: validator.New()}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	departmentID := uint(1)
	c.Set(auth.ContextUserKey, &auth.CurrentUser{ID: 1, Username: "tester", OrganizationID: 1, DepartmentID: &departmentID})
	if err := handler(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}

func decodeWarehouseJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode json %q: %v", rec.Body.String(), err)
	}
}
