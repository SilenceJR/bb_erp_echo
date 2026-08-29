package material

import (
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

type materialTestValidator struct{ validate *validator.Validate }

func (v materialTestValidator) Validate(value any) error { return v.validate.Struct(value) }

func TestCreateMaterialRequiresDepartmentEmployee(t *testing.T) {
	db, current, employeeID := openMaterialTestDB(t)
	handler := NewHandler(db)

	missing := newMaterialTestContext(t, current, `{"name":"ABS","code":"ABS-1"}`)
	if status := materialErrorStatus(handler.CreateMaterial(missing)); status != http.StatusBadRequest {
		t.Fatalf("missing operator status=%d", status)
	}
	var count int64
	if err := db.Model(&model.Material{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("missing operator wrote material count=%d err=%v", count, err)
	}

	valid := newMaterialTestContext(t, current, `{"name":"ABS","code":"ABS-1","operator_employee_id":`+strconv.FormatUint(uint64(employeeID), 10)+`}`)
	if err := handler.CreateMaterial(valid); err != nil {
		t.Fatalf("valid material: %v", err)
	}
	if err := db.Model(&model.Material{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("valid material count=%d err=%v", count, err)
	}
}

func openMaterialTestDB(t *testing.T) (*gorm.DB, *auth.CurrentUser, uint) {
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
	if err := db.AutoMigrate(&model.Organization{}, &model.Department{}, &model.Employee{}, &model.EmployeeDepartment{}, &model.User{}, &model.Material{}); err != nil {
		t.Fatal(err)
	}
	org := model.Organization{Name: "组织", Code: "ORG", Status: model.StatusActive}
	db.Create(&org)
	department := model.Department{OrganizationID: org.ID, Name: "仓库部", Code: "WH", Status: model.StatusActive}
	db.Create(&department)
	now := time.Now()
	employee := model.Employee{OrganizationID: org.ID, Name: "员工", HireDate: now, BirthDate: now, Status: model.StatusActive}
	db.Create(&employee)
	db.Create(&model.EmployeeDepartment{EmployeeID: employee.ID, DepartmentID: department.ID})
	account := model.User{Username: "admin", AccountType: model.AccountTypePersonal, Name: "管理员", OrganizationID: org.ID, DepartmentID: &department.ID, Status: model.StatusActive, PasswordHash: "test"}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	return db, &auth.CurrentUser{ID: account.ID, Username: "admin", OrganizationID: org.ID, DepartmentID: &department.ID}, employee.ID
}

func newMaterialTestContext(t *testing.T, current *auth.CurrentUser, body string) *echo.Context {
	t.Helper()
	e := echo.New()
	e.Validator = materialTestValidator{validate: validator.New()}
	req := httptest.NewRequest(http.MethodPost, "/materials", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := e.NewContext(req, httptest.NewRecorder())
	c.Set(auth.ContextUserKey, current)
	return c
}

func materialErrorStatus(err error) int {
	if httpErr, ok := err.(*echo.HTTPError); ok {
		return httpErr.Code
	}
	if err != nil {
		return http.StatusInternalServerError
	}
	return http.StatusOK
}
