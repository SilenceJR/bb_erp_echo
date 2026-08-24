package warehouse

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"bb_erp_echo/internal/model"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
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
// 首次查询会自动创建并返回默认仓库。
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
	if err := db.AutoMigrate(&model.Warehouse{}, &model.Material{}, &model.Product{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func performWarehouseJSON(t *testing.T, handler echo.HandlerFunc, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
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
	if err := handler(c); err != nil {
		e.HTTPErrorHandler(err, c)
	}
	return rec
}

func decodeWarehouseJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode json %q: %v", rec.Body.String(), err)
	}
}
