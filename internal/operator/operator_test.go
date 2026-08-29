package operator

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"

	"github.com/labstack/echo/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveRequiresActiveDepartmentEmployeeMembership(t *testing.T) {
	db := openOperatorTestDB(t)
	orgA := model.Organization{Name: "组织 A", Code: "ORG-A", Status: model.StatusActive}
	orgB := model.Organization{Name: "组织 B", Code: "ORG-B", Status: model.StatusActive}
	if err := db.Create(&orgA).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&orgB).Error; err != nil {
		t.Fatal(err)
	}
	dept := model.Department{OrganizationID: orgA.ID, Name: "生产部", Code: "PROD", Status: model.StatusActive}
	otherDept := model.Department{OrganizationID: orgA.ID, Name: "仓库部", Code: "WH", Status: model.StatusActive}
	if err := db.Create(&dept).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherDept).Error; err != nil {
		t.Fatal(err)
	}
	birth := time.Date(1990, 1, 1, 0, 0, 0, 0, time.Local)
	active := model.Employee{OrganizationID: orgA.ID, Name: "在职", HireDate: birth, BirthDate: birth, Status: model.StatusActive}
	disabled := model.Employee{OrganizationID: orgA.ID, Name: "停用", HireDate: birth, BirthDate: birth, Status: model.StatusDisabled}
	foreign := model.Employee{OrganizationID: orgB.ID, Name: "跨组织", HireDate: birth, BirthDate: birth, Status: model.StatusActive}
	for _, item := range []*model.Employee{&active, &disabled, &foreign} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.EmployeeDepartment{EmployeeID: active.ID, DepartmentID: dept.ID}).Error; err != nil {
		t.Fatal(err)
	}
	account := model.User{Username: "tester", AccountType: model.AccountTypePersonal, Name: "测试账号", OrganizationID: orgA.ID, DepartmentID: &dept.ID, Status: model.StatusActive, PasswordHash: "test"}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	current := &auth.CurrentUser{ID: account.ID, Username: "tester", OrganizationID: orgA.ID, DepartmentID: &dept.ID}
	c := newOperatorContext(current)
	identity, err := Resolve(c, db, active.ID)
	if err != nil || identity.EmployeeName != "在职" || identity.DepartmentName != "生产部" {
		t.Fatalf("identity=%+v err=%v", identity, err)
	}
	if _, ok := Get(c); !ok {
		t.Fatal("resolved identity was not saved in context")
	}
	dept.Status = model.StatusDisabled
	if err := db.Save(&dept).Error; err != nil {
		t.Fatal(err)
	}
	if status := operatorErrorStatus(func() error {
		_, err := Resolve(newOperatorContext(current), db, active.ID)
		return err
	}()); status != http.StatusConflict {
		t.Fatalf("disabled department status=%d want=%d", status, http.StatusConflict)
	}
	dept.Status = model.StatusActive
	if err := db.Save(&dept).Error; err != nil {
		t.Fatal(err)
	}
	account.DepartmentID = &otherDept.ID
	if err := db.Save(&account).Error; err != nil {
		t.Fatal(err)
	}
	staleCurrent := &auth.CurrentUser{ID: account.ID, Username: "tester", OrganizationID: orgA.ID, DepartmentID: &dept.ID}
	if status := operatorErrorStatus(func() error {
		_, err := Resolve(newOperatorContext(staleCurrent), db, active.ID)
		return err
	}()); status != http.StatusConflict {
		t.Fatalf("changed affiliation status=%d want=%d", status, http.StatusConflict)
	}
	account.DepartmentID = &dept.ID
	if err := db.Save(&account).Error; err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name       string
		employeeID uint
		wantStatus int
	}{
		{name: "missing", employeeID: 0, wantStatus: http.StatusBadRequest},
		{name: "disabled", employeeID: disabled.ID, wantStatus: http.StatusConflict},
		{name: "foreign", employeeID: foreign.ID, wantStatus: http.StatusForbidden},
		{name: "not member", employeeID: active.ID + 100, wantStatus: http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Resolve(newOperatorContext(current), db, test.employeeID)
			if status := operatorErrorStatus(err); status != test.wantStatus {
				t.Fatalf("status=%d want=%d err=%v", status, test.wantStatus, err)
			}
		})
	}
}

func openOperatorTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&model.Organization{}, &model.Department{}, &model.Employee{}, &model.EmployeeDepartment{}, &model.User{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func newOperatorContext(current *auth.CurrentUser) *echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserKey, current)
	return c
}

func operatorErrorStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if httpErr, ok := err.(*echo.HTTPError); ok {
		return httpErr.Code
	}
	return http.StatusInternalServerError
}
