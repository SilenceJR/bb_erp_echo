package contact

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

// testValidator 是联系人模块测试使用的 Echo 校验适配器。
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

// TestCreateContactWithPhones 验证联系人通过 customer_id 关联客户，
// 并能同时写入多条联系人电话明细。
func TestCreateContactWithPhones(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)

	customer := model.Customer{Name: "测试客户", Code: "CUST-001"}
	if err := db.Create(&customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	body := map[string]any{
		"customer_id": customer.ID,
		"name":        "张三",
		"phones": []map[string]any{
			{"phone": "13800000000", "label": "手机", "primary": true},
			{"phone": "0755-88888888", "label": "座机"},
		},
	}
	rec := performJSON(t, handler.CreateContact, http.MethodPost, "/api/v1/contacts", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create contact status = %d body=%s", rec.Code, rec.Body.String())
	}

	var contact model.Contact
	if err := db.Preload("Phones").Where("customer_id = ?", customer.ID).First(&contact).Error; err != nil {
		t.Fatalf("find contact: %v", err)
	}
	if len(contact.Phones) != 2 {
		t.Fatalf("phones length = %d", len(contact.Phones))
	}
	if !contact.Phones[0].Primary {
		t.Fatalf("first phone should be primary")
	}
}

// TestCreateContactRejectsMissingCustomer 验证创建联系人时必须绑定已存在客户。
func TestCreateContactRejectsMissingCustomer(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)

	body := map[string]any{
		"customer_id": 999,
		"name":        "不存在客户联系人",
	}
	rec := performJSON(t, handler.CreateContact, http.MethodPost, "/api/v1/contacts", body)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("create contact missing customer status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateContactRejectsMultiplePrimaryPhones(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)
	customer := model.Customer{Name: "主电话规则客户", Code: "CUST-PRIMARY"}
	if err := db.Create(&customer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	body := map[string]any{
		"customer_id": customer.ID,
		"name":        "重复主电话联系人",
		"phones": []map[string]any{
			{"phone": "13800000000", "primary": true},
			{"phone": "13900000000", "primary": true},
		},
	}
	rec := performJSON(t, handler.CreateContact, http.MethodPost, "/api/v1/contacts", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("multiple primary phones status = %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestListContactsPreloadsPhones 验证联系人列表会返回电话明细。
func TestListContactsPreloadsPhones(t *testing.T) {
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

	rec := performJSON(t, handler.ListContacts, http.MethodGet, "/api/v1/contacts", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list contacts status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("13900000000")) {
		t.Fatalf("list response should contain contact phone: %s", rec.Body.String())
	}
}

// TestGetContactByID 验证通过 ID 查询联系人详情并预加载电话明细。
func TestGetContactByID(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)

	contact := seedContactWithPhone(t, db)
	rec := performJSONWithParams(t, handler.GetContact, http.MethodGet, "/api/v1/contacts/:id", nil, map[string]string{
		"id": strconv.FormatUint(uint64(contact.ID), 10),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("get contact status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("13900000000")) {
		t.Fatalf("get response should contain contact phone: %s", rec.Body.String())
	}
}

// TestUpdateContactReplacesPhones 验证更新联系人时会整体替换电话明细。
func TestUpdateContactReplacesPhones(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)

	contact := seedContactWithPhone(t, db)
	body := map[string]any{
		"customer_id": contact.CustomerID,
		"name":        "李四-更新",
		"phones": []map[string]any{
			{"phone": "13600000000", "label": "新手机", "primary": true},
		},
	}
	rec := performJSONWithParams(t, handler.UpdateContact, http.MethodPatch, "/api/v1/contacts/:id", body, map[string]string{
		"id": strconv.FormatUint(uint64(contact.ID), 10),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update contact status = %d body=%s", rec.Code, rec.Body.String())
	}

	var updated model.Contact
	if err := db.Preload("Phones").First(&updated, contact.ID).Error; err != nil {
		t.Fatalf("find updated contact: %v", err)
	}
	if updated.Name != "李四-更新" {
		t.Fatalf("updated contact name = %q", updated.Name)
	}
	if len(updated.Phones) != 1 || updated.Phones[0].Phone != "13600000000" {
		t.Fatalf("updated phones mismatch: %+v", updated.Phones)
	}
}

// TestDeleteContactByID 验证删除联系人时同步软删除其电话明细。
func TestDeleteContactByID(t *testing.T) {
	db := openTestDB(t)
	handler := NewHandler(db)

	contact := seedContactWithPhone(t, db)
	rec := performJSONWithParams(t, handler.DeleteContact, http.MethodDelete, "/api/v1/contacts/:id", nil, map[string]string{
		"id": strconv.FormatUint(uint64(contact.ID), 10),
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete contact status = %d body=%s", rec.Code, rec.Body.String())
	}

	assertVisibleCount(t, db, &model.Contact{}, 0)
	assertVisibleCount(t, db, &model.ContactPhone{}, 0)
}

// openTestDB 创建联系人模块测试数据库并执行自动迁移。
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

// seedContactWithPhone 创建一个带主电话的联系人测试数据。
//
// 参数说明：
// - db：测试数据库。
func seedContactWithPhone(t *testing.T, db *gorm.DB) model.Contact {
	t.Helper()

	customer := model.Customer{
		Name: "已有客户",
		Code: "CUST-SEED",
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
	return customer.Contacts[0]
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
