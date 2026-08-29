package product

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

type productTestValidator struct{ validate *validator.Validate }

func (v productTestValidator) Validate(value any) error { return v.validate.Struct(value) }

func TestCreateProductRequiresDepartmentEmployee(t *testing.T) {
	db, current, employeeID := openProductTestDB(t)
	handler := NewHandler(db)

	missing := newProductTestContext(t, current, `{"name":"外壳","code":"P-1"}`)
	if status := productErrorStatus(handler.CreateProduct(missing)); status != http.StatusBadRequest {
		t.Fatalf("missing operator status=%d", status)
	}
	var count int64
	if err := db.Model(&model.Product{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("missing operator wrote product count=%d err=%v", count, err)
	}

	valid := newProductTestContext(t, current, `{"name":"外壳","code":"P-1","operator_employee_id":`+strconv.FormatUint(uint64(employeeID), 10)+`}`)
	if err := handler.CreateProduct(valid); err != nil {
		t.Fatalf("valid product: %v", err)
	}
	if err := db.Model(&model.Product{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("valid product count=%d err=%v", count, err)
	}
}

func openProductTestDB(t *testing.T) (*gorm.DB, *auth.CurrentUser, uint) {
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
	if err := db.AutoMigrate(&model.Organization{}, &model.Department{}, &model.Employee{}, &model.EmployeeDepartment{}, &model.User{}, &model.Product{}); err != nil {
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

func newProductTestContext(t *testing.T, current *auth.CurrentUser, body string) *echo.Context {
	t.Helper()
	e := echo.New()
	e.Validator = productTestValidator{validate: validator.New()}
	req := httptest.NewRequest(http.MethodPost, "/products", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	c := e.NewContext(req, httptest.NewRecorder())
	c.Set(auth.ContextUserKey, current)
	return c
}

func productErrorStatus(err error) int {
	if httpErr, ok := err.(*echo.HTTPError); ok {
		return httpErr.Code
	}
	if err != nil {
		return http.StatusInternalServerError
	}
	return http.StatusOK
}
