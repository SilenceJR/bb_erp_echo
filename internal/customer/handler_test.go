package customer

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"bb_erp_echo/internal/model"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// testValidator 是客户模块测试使用的 Echo 校验适配器。
type testValidator struct {
	validate *validator.Validate
}

// Validate 执行请求结构体 tag 校验。
//
// 参数说明：
// - i：需要校验的请求结构体。
func (v *testValidator) Validate(i any) error {
	return v.validate.Struct(i)
}

// TestCreateCustomer 验证客户档案创建时只保存客户名称和编码。
func TestCreateCustomer(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)

	customerBody := map[string]any{
		"name": "测试客户",
		"code": "CUST-001",
	}
	rec := performJSON(t, handler.CreateCustomer, http.MethodPost, "/api/v1/customers", customerBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create customer status = %d body=%s", rec.Code, rec.Body.String())
	}

	var created model.Customer
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created customer: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("created customer id should not be zero")
	}
	if created.Name != "测试客户" || created.Code != "CUST-001" {
		t.Fatalf("created customer mismatch: %+v", created)
	}
}

// TestListCustomersPreloadsContactPhones 验证客户列表会预加载联系人电话明细。
func TestListCustomersPreloadsContactPhones(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)

	customer := model.Customer{
		Name: "已有客户",
		Code: "CUST-002",
		Contacts: []model.Contact{
			{
				Name: "李四",
				Phones: []model.ContactPhone{
					{Phone: "13900000000", Label: "手机", Primary: true},
				},
			},
		},
	}
	if err := db.Create(&customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	rec := performJSON(t, handler.ListCustomers, http.MethodGet, "/api/v1/customers", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list customer status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("13900000000")) {
		t.Fatalf("list response should contain contact phone: %s", rec.Body.String())
	}
}

// TestUpdateCustomerByID 验证通过路径 ID 更新客户名称、编码和地址。
func TestUpdateCustomerByID(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)

	customer := model.Customer{Name: "旧客户", Code: "CUST-OLD"}
	if err := db.Create(&customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	body := map[string]any{
		"name":    "新客户",
		"code":    "CUST-NEW",
		"address": "深圳市宝安区",
	}
	rec := performJSONWithParams(t, handler.UpdateCustomer, http.MethodPatch, "/api/v1/customers/:id", body, map[string]string{
		"id": strconv.FormatUint(uint64(customer.ID), 10),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update customer status = %d body=%s", rec.Code, rec.Body.String())
	}

	var updated model.Customer
	if err := db.First(&updated, customer.ID).Error; err != nil {
		t.Fatalf("find updated customer: %v", err)
	}
	if updated.Name != "新客户" || updated.Code != "CUST-NEW" || updated.Address != "深圳市宝安区" {
		t.Fatalf("updated customer mismatch: %+v", updated)
	}
}

// TestUpdateCustomerRejectsMissingID 验证更新不存在客户时返回 404。
func TestUpdateCustomerRejectsMissingID(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)

	body := map[string]any{
		"name": "不存在客户",
		"code": "CUST-MISSING",
	}
	rec := performJSONWithParams(t, handler.UpdateCustomer, http.MethodPatch, "/api/v1/customers/:id", body, map[string]string{
		"id": "999",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("update missing customer status = %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestDeleteCustomerByID 验证通过路径 ID 删除客户时，只软删除客户本体，
// 不删除联系人和电话明细。
func TestDeleteCustomerByID(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)

	customer := model.Customer{
		Name: "待删除客户",
		Code: "CUST-DEL",
		Contacts: []model.Contact{
			{
				Name: "王五",
				Phones: []model.ContactPhone{
					{Phone: "13700000000", Label: "手机", Primary: true},
				},
			},
		},
	}
	if err := db.Create(&customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	rec := performJSONWithParams(t, handler.DeleteCustomer, http.MethodDelete, "/api/v1/customers/:id", nil, map[string]string{
		"id": strconv.FormatUint(uint64(customer.ID), 10),
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete customer status = %d body=%s", rec.Code, rec.Body.String())
	}

	assertVisibleCount(t, db, &model.Customer{}, 0)
	assertVisibleCount(t, db, &model.Contact{}, 1)
	assertVisibleCount(t, db, &model.ContactPhone{}, 1)
}

// TestDeleteCustomerRejectsMissingID 验证删除不存在客户时返回 404。
func TestDeleteCustomerRejectsMissingID(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)

	rec := performJSONWithParams(t, handler.DeleteCustomer, http.MethodDelete, "/api/v1/customers/:id", nil, map[string]string{
		"id": "999",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing customer status = %d body=%s", rec.Code, rec.Body.String())
	}
}

// openTestDB 创建客户模块测试数据库并执行自动迁移。
func openTestDB(t *testing.T) *gorm.DB {
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

	if err := db.AutoMigrate(&model.Customer{}, &model.Contact{}, &model.ContactPhone{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

// performJSON 执行 JSON 请求并返回响应记录器。
//
// 参数说明：
// - handler：待测试的 Echo Handler。
// - method：HTTP 方法。
// - path：请求路径。
// - body：请求体，nil 表示无请求体。
func performJSON(t *testing.T, handler echo.HandlerFunc, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return performJSONWithParams(t, handler, method, path, body, nil)
}

// performJSONWithParams 执行 JSON 请求并设置 Echo 路径参数。
//
// 参数说明：
// - handler：待测试的 Echo Handler。
// - method：HTTP 方法。
// - path：请求路径。
// - body：请求体，nil 表示无请求体。
// - params：路径参数键值，例如 {"id": "1"}。
func performJSONWithParams(t *testing.T, handler echo.HandlerFunc, method string, path string, body any, params map[string]string) *httptest.ResponseRecorder {
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
	if err := handler(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}

// assertVisibleCount 断言软删除过滤后的可见记录数。
//
// 参数说明：
// - db：测试数据库。
// - modelValue：待统计的 GORM 模型指针。
// - want：期望数量。
func assertVisibleCount(t *testing.T, db *gorm.DB, modelValue any, want int64) {
	t.Helper()

	var count int64
	if err := db.Model(modelValue).Count(&count).Error; err != nil {
		t.Fatalf("count %T: %v", modelValue, err)
	}
	if count != want {
		t.Fatalf("count %T = %d, want %d", modelValue, count, want)
	}
}
