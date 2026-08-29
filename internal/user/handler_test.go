package user

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// testRoleService 是用户处理器测试使用的最小角色服务替身。
type testRoleService struct{}

func (testRoleService) UserRoleIDs(userIDs []uint) (map[uint][]uint, error) {
	return make(map[uint][]uint, len(userIDs)), nil
}

func (testRoleService) ReplaceUserRoles(uint, []uint, bool) error { return nil }

func (testRoleService) AssignRoleCodes(uint, []string) error { return nil }

func userHandlerErrorStatus(err error) int {
	if httpErr, ok := err.(*echo.HTTPError); ok {
		return httpErr.Code
	}
	if err != nil {
		return http.StatusInternalServerError
	}
	return http.StatusOK
}

// TestCreateUserValidatesDepartmentTerminalAffiliations 验证创建账号时，
// 部门和终端必须都在目标组织内，且终端归属到所选部门；失败时不写入用户。
func TestCreateUserValidatesDepartmentTerminalAffiliations(t *testing.T) {
	db := openUserTestDB(t)
	handler := NewHandler(db, testRoleService{})

	organizationA := createUserTestOrganization(t, db, "组织 A", "ORG-A")
	organizationB := createUserTestOrganization(t, db, "组织 B", "ORG-B")
	departmentA := createUserTestDepartment(t, db, organizationA.ID, "部门 A", "DEPT-A")
	departmentA2 := createUserTestDepartment(t, db, organizationA.ID, "部门 A2", "DEPT-A2")
	departmentB := createUserTestDepartment(t, db, organizationB.ID, "部门 B", "DEPT-B")
	terminalA := createUserTestTerminal(t, db, departmentA.ID, "TERM-A")
	terminalA2 := createUserTestTerminal(t, db, departmentA2.ID, "TERM-A2")
	terminalB := createUserTestTerminal(t, db, departmentB.ID, "TERM-B")
	current := &auth.CurrentUser{OrganizationID: organizationA.ID}
	if status := userHandlerErrorStatus(handler.validateAffiliations(organizationA.ID, nil, &terminalA.ID)); status != http.StatusBadRequest {
		t.Fatalf("terminal without department status=%d want=%d", status, http.StatusBadRequest)
	}

	tests := []struct {
		name         string
		username     string
		departmentID uint
		terminalID   uint
		wantStatus   int
		wantUsers    int64
	}{
		{
			name:         "同组织且终端归属所选部门",
			username:     "terminal-valid",
			departmentID: departmentA.ID,
			terminalID:   terminalA.ID,
			wantStatus:   http.StatusCreated,
			wantUsers:    1,
		},
		{
			name:         "终端属于同组织的其他部门",
			username:     "terminal-mismatched",
			departmentID: departmentA.ID,
			terminalID:   terminalA2.ID,
			wantStatus:   http.StatusForbidden,
			wantUsers:    1,
		},
		{
			name:         "终端属于其他组织",
			username:     "terminal-cross-org",
			departmentID: departmentA.ID,
			terminalID:   terminalB.ID,
			wantStatus:   http.StatusForbidden,
			wantUsers:    1,
		},
		{
			name:         "部门属于其他组织",
			username:     "department-cross-org",
			departmentID: departmentB.ID,
			terminalID:   terminalB.ID,
			wantStatus:   http.StatusForbidden,
			wantUsers:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performUserJSON(t, handler.CreateUser, current, map[string]any{
				"username":        tt.username,
				"password":        "password123",
				"account_type":    model.AccountTypeDepartmentTerminal,
				"name":            "部门终端账号",
				"organization_id": organizationA.ID,
				"department_id":   tt.departmentID,
				"terminal_id":     tt.terminalID,
			})
			if rec.Code != tt.wantStatus {
				t.Fatalf("create user status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			var userCount int64
			if err := db.Model(&model.User{}).Count(&userCount).Error; err != nil {
				t.Fatalf("count users: %v", err)
			}
			if userCount != tt.wantUsers {
				t.Fatalf("user count = %d, want %d", userCount, tt.wantUsers)
			}
		})
	}

	var created model.User
	if err := db.Where("username = ?", "terminal-valid").First(&created).Error; err != nil {
		t.Fatalf("find valid user: %v", err)
	}
	if created.DepartmentID == nil || *created.DepartmentID != departmentA.ID || created.TerminalID == nil || *created.TerminalID != terminalA.ID {
		t.Fatalf("valid user affiliations = department %v terminal %v", created.DepartmentID, created.TerminalID)
	}
}

// TestResetUserPasswordEnforcesOrganizationAndSelfRestrictions 验证管理员重置密码
// 只能作用于同组织的其他账号，并且会递增目标账号的密码版本。
func TestResetUserPasswordEnforcesOrganizationAndSelfRestrictions(t *testing.T) {
	db := openUserTestDB(t)
	handler := NewHandler(db, testRoleService{})

	organizationA := createUserTestOrganization(t, db, "组织 A", "RESET-ORG-A")
	organizationB := createUserTestOrganization(t, db, "组织 B", "RESET-ORG-B")
	adminHash, err := auth.HashPassword("admin123456")
	if err != nil {
		t.Fatalf("hash admin password: %v", err)
	}
	targetHash, err := auth.HashPassword("target123456")
	if err != nil {
		t.Fatalf("hash target password: %v", err)
	}
	foreignHash, err := auth.HashPassword("foreign123456")
	if err != nil {
		t.Fatalf("hash foreign password: %v", err)
	}
	admin := model.User{
		Username:       "reset-admin",
		AccountType:    model.AccountTypePersonal,
		Name:           "重置管理员",
		OrganizationID: organizationA.ID,
		Status:         model.StatusActive,
		PasswordHash:   adminHash,
	}
	target := model.User{
		Username:       "reset-target",
		AccountType:    model.AccountTypePersonal,
		Name:           "目标用户",
		OrganizationID: organizationA.ID,
		Status:         model.StatusActive,
		PasswordHash:   targetHash,
	}
	foreign := model.User{
		Username:       "reset-foreign",
		AccountType:    model.AccountTypePersonal,
		Name:           "其他组织用户",
		OrganizationID: organizationB.ID,
		Status:         model.StatusActive,
		PasswordHash:   foreignHash,
	}
	for _, item := range []*model.User{&admin, &target, &foreign} {
		if err := db.Create(item).Error; err != nil {
			t.Fatalf("create user %s: %v", item.Username, err)
		}
	}
	current := &auth.CurrentUser{ID: admin.ID, OrganizationID: organizationA.ID}

	rec := performUserJSONAtPath(t, handler.ResetUserPassword, target.ID, current, map[string]any{
		"password": "resetTarget123",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("same-org reset status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	var updated model.User
	if err := db.First(&updated, target.ID).Error; err != nil {
		t.Fatalf("find reset target: %v", err)
	}
	if updated.PasswordVersion != 2 {
		t.Fatalf("target password_version = %d, want 2", updated.PasswordVersion)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("resetTarget123")); err != nil {
		t.Fatalf("target password was not reset: %v", err)
	}

	rec = performUserJSONAtPath(t, handler.ResetUserPassword, admin.ID, current, map[string]any{
		"password": "resetAdmin123",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("self reset status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	rec = performUserJSONAtPath(t, handler.ResetUserPassword, foreign.ID, current, map[string]any{
		"password": "resetForeign123",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-org reset status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestUpdateUserAffiliationRejectsDisabledDepartmentWithoutChangingTarget(t *testing.T) {
	db := openUserTestDB(t)
	handler := NewHandler(db, testRoleService{})
	organization := createUserTestOrganization(t, db, "归属组织", "AFFILIATION-ORG")
	activeDepartment := createUserTestDepartment(t, db, organization.ID, "启用部门", "AFFILIATION-ACTIVE")
	disabledDepartment := createUserTestDepartment(t, db, organization.ID, "停用部门", "AFFILIATION-DISABLED")
	activeTerminal := createUserTestTerminal(t, db, activeDepartment.ID, "AFFILIATION-TERM-ACTIVE")
	disabledTerminal := createUserTestTerminal(t, db, disabledDepartment.ID, "AFFILIATION-TERM-DISABLED")
	target := model.User{
		Username: "affiliation-target", AccountType: model.AccountTypeDepartmentTerminal, Name: "归属目标",
		OrganizationID: organization.ID, Status: model.StatusActive, PasswordHash: "hash", PasswordVersion: 1,
		DepartmentID: &activeDepartment.ID, TerminalID: &activeTerminal.ID,
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create target user: %v", err)
	}
	current := &auth.CurrentUser{ID: target.ID + 100, Username: "affiliation-admin", OrganizationID: organization.ID}

	if err := db.Model(&model.Department{}).Where("id = ?", disabledDepartment.ID).Update("status", model.StatusDisabled).Error; err != nil {
		t.Fatalf("disable department: %v", err)
	}
	rec := performUserAffiliationJSON(t, handler.UpdateUserAffiliation, target.ID, current, map[string]any{
		"department_id": disabledDepartment.ID,
		"terminal_id":   disabledTerminal.ID,
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("disabled department affiliation status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	var unchanged model.User
	if err := db.First(&unchanged, target.ID).Error; err != nil {
		t.Fatalf("reload target user: %v", err)
	}
	if unchanged.DepartmentID == nil || *unchanged.DepartmentID != activeDepartment.ID || unchanged.TerminalID == nil || *unchanged.TerminalID != activeTerminal.ID {
		t.Fatalf("failed affiliation update changed target: department=%v terminal=%v", unchanged.DepartmentID, unchanged.TerminalID)
	}
}

func TestUpdateUserAffiliationConcurrentUpdatesKeepDepartmentTerminalPair(t *testing.T) {
	db := openUserTestDB(t)
	handler := NewHandler(db, testRoleService{})
	organization := createUserTestOrganization(t, db, "并发归属组织", "AFFILIATION-CONCURRENT-ORG")
	firstDepartment := createUserTestDepartment(t, db, organization.ID, "并发部门一", "AFFILIATION-CONCURRENT-ONE")
	secondDepartment := createUserTestDepartment(t, db, organization.ID, "并发部门二", "AFFILIATION-CONCURRENT-TWO")
	firstTerminal := createUserTestTerminal(t, db, firstDepartment.ID, "AFFILIATION-CONCURRENT-TERM-ONE")
	secondTerminal := createUserTestTerminal(t, db, secondDepartment.ID, "AFFILIATION-CONCURRENT-TERM-TWO")
	target := model.User{
		Username: "affiliation-concurrent-target", AccountType: model.AccountTypeDepartmentTerminal, Name: "并发目标",
		OrganizationID: organization.ID, Status: model.StatusActive, PasswordHash: "hash", PasswordVersion: 1,
		DepartmentID: &firstDepartment.ID, TerminalID: &firstTerminal.ID,
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create concurrent target: %v", err)
	}
	current := &auth.CurrentUser{ID: target.ID + 100, Username: "concurrent-admin", OrganizationID: organization.ID}

	type result struct{ code int }
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for _, pair := range [][2]uint{{firstDepartment.ID, firstTerminal.ID}, {secondDepartment.ID, secondTerminal.ID}} {
		pair := pair
		wait.Add(1)
		go func() {
			defer wait.Done()
			rec := performUserAffiliationJSON(t, handler.UpdateUserAffiliation, target.ID, current, map[string]any{
				"department_id": pair[0],
				"terminal_id":   pair[1],
			})
			results <- result{code: rec.Code}
		}()
	}
	wait.Wait()
	close(results)
	for item := range results {
		if item.code != http.StatusOK {
			t.Fatalf("concurrent affiliation status = %d, want %d", item.code, http.StatusOK)
		}
	}
	var final model.User
	if err := db.First(&final, target.ID).Error; err != nil {
		t.Fatalf("reload concurrent target: %v", err)
	}
	if final.DepartmentID == nil || final.TerminalID == nil {
		t.Fatalf("concurrent affiliation cleared pair: department=%v terminal=%v", final.DepartmentID, final.TerminalID)
	}
	if !(((*final.DepartmentID == firstDepartment.ID) && (*final.TerminalID == firstTerminal.ID)) || ((*final.DepartmentID == secondDepartment.ID) && (*final.TerminalID == secondTerminal.ID))) {
		t.Fatalf("concurrent affiliation left mismatched pair: department=%d terminal=%d", *final.DepartmentID, *final.TerminalID)
	}
}

func openUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.Organization{}, &model.Department{}, &model.Terminal{}, &model.User{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return db
}

func createUserTestOrganization(t *testing.T, db *gorm.DB, name, code string) model.Organization {
	t.Helper()
	item := model.Organization{Name: name, Code: code, Status: model.StatusActive}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create organization %s: %v", code, err)
	}
	return item
}

func createUserTestDepartment(t *testing.T, db *gorm.DB, organizationID uint, name, code string) model.Department {
	t.Helper()
	item := model.Department{OrganizationID: organizationID, Name: name, Code: code, Status: model.StatusActive}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create department %s: %v", code, err)
	}
	return item
}

func createUserTestTerminal(t *testing.T, db *gorm.DB, departmentID uint, code string) model.Terminal {
	t.Helper()
	item := model.Terminal{DepartmentID: departmentID, Code: code, Name: code, Status: model.StatusActive}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create terminal %s: %v", code, err)
	}
	return item
}

func performUserJSON(t *testing.T, handler echo.HandlerFunc, current *auth.CurrentUser, body any) *httptest.ResponseRecorder {
	return performUserJSONAtPath(t, handler, 0, current, body)
}

func performUserJSONAtPath(t *testing.T, handler echo.HandlerFunc, id uint, current *auth.CurrentUser, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	e := echo.New()
	e.Validator = &userTestValidator{validate: validator.New()}
	path := "/api/v1/system/users"
	if id != 0 {
		path = "/api/v1/system/users/" + itoa(id) + "/reset-password"
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if id != 0 {
		c.SetPath("/api/v1/system/users/:id/reset-password")
		c.SetPathValues(echo.PathValues{{Name: "id", Value: itoa(id)}})
	}
	c.Set(auth.ContextUserKey, current)
	if err := handler(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}

func performUserAffiliationJSON(t *testing.T, handler echo.HandlerFunc, id uint, current *auth.CurrentUser, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal affiliation request: %v", err)
	}
	e := echo.New()
	e.Validator = &userTestValidator{validate: validator.New()}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system/users/"+itoa(id)+"/affiliation", bytes.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/system/users/:id/affiliation")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: itoa(id)}})
	c.Set(auth.ContextUserKey, current)
	if err := handler(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}

func itoa(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}

type userTestValidator struct {
	validate *validator.Validate
}

func (v *userTestValidator) Validate(i any) error {
	return v.validate.Struct(i)
}
