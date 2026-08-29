package department

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type departmentTestValidator struct{ validate *validator.Validate }

func (v departmentTestValidator) Validate(value any) error { return v.validate.Struct(value) }

func TestDepartmentMembersCanBeRemovedAndReaddedAfterDisable(t *testing.T) {
	db := openDepartmentTestDB(t)
	h := NewHandler(db)
	department := model.Department{OrganizationID: 1, Name: "生产部", Code: "PROD", Status: model.StatusActive}
	if err := db.Create(&department).Error; err != nil {
		t.Fatal(err)
	}
	birth := time.Date(1990, 1, 1, 0, 0, 0, 0, time.Local)
	first := model.Employee{OrganizationID: 1, Name: "员工一", HireDate: birth, BirthDate: birth, Status: model.StatusActive}
	second := model.Employee{OrganizationID: 1, Name: "员工二", HireDate: birth, BirthDate: birth, Status: model.StatusActive}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	current := &auth.CurrentUser{ID: 1, Username: "admin", OrganizationID: 1}

	replaceDepartmentMembers(t, h, department.ID, []uint{first.ID, second.ID}, current, http.StatusOK)
	if err := db.Model(&model.Department{}).Where("id = ?", department.ID).Update("status", model.StatusDisabled).Error; err != nil {
		t.Fatal(err)
	}
	// 停用部门仍可移除一名已有成员。
	replaceDepartmentMembers(t, h, department.ID, []uint{first.ID}, current, http.StatusOK)
	var count int64
	if err := db.Model(&model.EmployeeDepartment{}).Where("department_id = ?", department.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("member count after removal=%d, want 1", count)
	}
	// 停用部门不能借 PUT 新增另一个成员。
	replaceDepartmentMembers(t, h, department.ID, []uint{first.ID, second.ID}, current, http.StatusConflict)
	if err := db.Model(&model.Department{}).Where("id = ?", department.ID).Update("status", model.StatusActive).Error; err != nil {
		t.Fatal(err)
	}
	// 关系表硬删除后可以再次加入同一员工。
	replaceDepartmentMembers(t, h, department.ID, []uint{first.ID, second.ID}, current, http.StatusOK)
	if err := db.Model(&model.EmployeeDepartment{}).Where("department_id = ?", department.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("member count after readd=%d, want 2", count)
	}
}

func replaceDepartmentMembers(t *testing.T, h *Handler, departmentID uint, employeeIDs []uint, current *auth.CurrentUser, wantStatus int) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"employee_ids": employeeIDs})
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Validator = departmentTestValidator{validate: validator.New()}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/departments/1/employees", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	// IDs in each test DB start at 1 for the sole department.
	c.SetPathValues(echo.PathValues{{Name: "id", Value: formatDepartmentID(departmentID)}})
	c.Set(auth.ContextUserKey, current)
	if err := h.ReplaceDepartmentEmployees(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	if rec.Code != wantStatus {
		t.Fatalf("replace members status=%d want=%d body=%s", rec.Code, wantStatus, rec.Body.String())
	}
}

func openDepartmentTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&model.Department{}, &model.Employee{}, &model.EmployeeDepartment{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func formatDepartmentID(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
