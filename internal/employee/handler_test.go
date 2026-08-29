package employee

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type handlerValidator struct{ validate *validator.Validate }

func (v handlerValidator) Validate(value any) error { return v.validate.Struct(value) }

func TestEmployeeCRUDAndAgeUsesShanghaiDate(t *testing.T) {
	db := openEmployeeTestDB(t)
	h := NewHandler(db)
	current := &auth.CurrentUser{ID: 7, Username: "admin", OrganizationID: 1}

	birth := time.Now().In(shanghaiLocation)
	birth = time.Date(birth.Year()-30, birth.Month(), birth.Day(), 0, 0, 0, 0, shanghaiLocation)
	rec := performEmployeeJSON(t, h.CreateEmployee, http.MethodPost, "/api/v1/system/employees", map[string]any{
		"name": "张三", "phone": "13800000000", "hire_date": "2026-01-02", "birthplace": "杭州",
		"residential_address": "滨江", "birth_date": birth.Format("2006-01-02"),
	}, current)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created EmployeeResponse
	decodeEmployeeJSON(t, rec, &created)
	if created.Age != 30 || created.HireDate != "2026-01-02" || created.BirthDate != birth.Format("2006-01-02") {
		t.Fatalf("created=%+v", created)
	}

	rec = performEmployeeJSON(t, h.UpdateEmployee, http.MethodPut, "/api/v1/system/employees/:id", map[string]any{
		"name": "张三改", "hire_date": "2026-01-03", "birth_date": birth.Format("2006-01-02"),
	}, current, map[string]string{"id": formatID(created.ID)})
	if rec.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = performEmployeeJSON(t, h.DeleteEmployee, http.MethodDelete, "/api/v1/system/employees/:id", nil, current, map[string]string{"id": formatID(created.ID)})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stored model.Employee
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.StatusDisabled {
		t.Fatalf("status=%s, want disabled", stored.Status)
	}
}

func TestOperatorEmployeesOnlyReturnsActiveMembers(t *testing.T) {
	db := openEmployeeTestDB(t)
	h := NewHandler(db)
	department := model.Department{OrganizationID: 1, Name: "生产部", Code: "PROD", Status: model.StatusActive}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	birth := time.Date(1990, 1, 1, 0, 0, 0, 0, shanghaiLocation)
	active := model.Employee{OrganizationID: 1, Name: "可选员工", HireDate: birth, BirthDate: birth, Status: model.StatusActive}
	disabled := model.Employee{OrganizationID: 1, Name: "停用员工", HireDate: birth, BirthDate: birth, Status: model.StatusDisabled}
	if err := db.Create(&active).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.EmployeeDepartment{EmployeeID: active.ID, DepartmentID: department.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.EmployeeDepartment{EmployeeID: disabled.ID, DepartmentID: department.ID}).Error; err != nil {
		t.Fatal(err)
	}
	current := &auth.CurrentUser{ID: 7, Username: "operator", OrganizationID: 1, DepartmentID: &department.ID}
	rec := performEmployeeJSON(t, h.ListOperatorEmployees, http.MethodGet, "/api/v1/operator-employees", nil, current, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var result OperatorEmployeesResponse
	decodeEmployeeJSON(t, rec, &result)
	if result.Department.ID != department.ID || len(result.Employees) != 1 || result.Employees[0].ID != active.ID {
		t.Fatalf("result=%+v", result)
	}
	if stringsContains(rec.Body.String(), "13800000000") || stringsContains(rec.Body.String(), "birth_date") {
		t.Fatalf("operator response exposed sensitive fields: %s", rec.Body.String())
	}
}

func TestEmployeeRejectsFutureBirthDateAndNonISODate(t *testing.T) {
	db := openEmployeeTestDB(t)
	h := NewHandler(db)
	current := &auth.CurrentUser{ID: 7, Username: "admin", OrganizationID: 1}
	cases := []map[string]any{
		{"name": "未来", "hire_date": "2026-01-01", "birth_date": "2999-01-01"},
		{"name": "格式", "hire_date": "20260101", "birth_date": "1990-01-01"},
	}
	for _, body := range cases {
		rec := performEmployeeJSON(t, h.CreateEmployee, http.MethodPost, "/api/v1/system/employees", body, current, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func openEmployeeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.Organization{}, &model.Department{}, &model.Employee{}, &model.EmployeeDepartment{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func performEmployeeJSON(t *testing.T, handler echo.HandlerFunc, method, path string, body any, current *auth.CurrentUser, params ...map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var payload bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = *bytes.NewReader(raw)
	}
	e := echo.New()
	e.Validator = handlerValidator{validate: validator.New()}
	req := httptest.NewRequest(method, path, &payload)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if len(params) > 0 {
		values := make(echo.PathValues, 0, len(params[0]))
		for key, value := range params[0] {
			values = append(values, echo.PathValue{Name: key, Value: value})
		}
		c.SetPathValues(values)
	}
	c.Set(auth.ContextUserKey, current)
	if err := handler(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}

func decodeEmployeeJSON(t *testing.T, rec *httptest.ResponseRecorder, value any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), value); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

func formatID(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

func stringsContains(value, part string) bool {
	return strings.Contains(value, part)
}
