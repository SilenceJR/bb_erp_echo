package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"bb_erp_echo/internal/domain"
)

func TestInitializationHealthReadyAndSQLiteWAL(t *testing.T) {
	erp := newTestApp(t)

	rec := erp.request(http.MethodGet, "/health", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d", rec.Code)
	}

	rec = erp.request(http.MethodGet, "/ready", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /ready status = %d", rec.Code)
	}

	var mode string
	if err := erp.DB.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

func TestLoginMeAndInvalidCredentials(t *testing.T) {
	erp := newTestApp(t)

	rec := erp.request(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "admin",
		"password": "wrong-password",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d", rec.Code)
	}

	token := erp.login(t, "admin", "admin123456")
	rec = erp.request(http.MethodGet, "/api/v1/auth/me", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/me status = %d body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	decodeJSON(t, rec, &body)
	if body["account_type"] != domain.AccountTypePersonal {
		t.Fatalf("account_type = %v", body["account_type"])
	}
	if len(body["permissions"].([]any)) == 0 {
		t.Fatalf("permissions should not be empty")
	}
}

func TestJWTCasbinAndOrganizationDataBoundary(t *testing.T) {
	erp := newTestApp(t)

	rec := erp.request(http.MethodGet, "/api/v1/system/users", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing jwt status = %d", rec.Code)
	}

	token := erp.login(t, "admin", "admin123456")
	rec = erp.request(http.MethodGet, "/api/v1/system/users", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin users list status = %d body=%s", rec.Code, rec.Body.String())
	}

	limitedToken := erp.createLimitedUserAndLogin(t)
	rec = erp.request(http.MethodGet, "/api/v1/customers", limitedToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("limited user customer access status = %d", rec.Code)
	}

	otherOrg := domain.Organization{Name: "外部组织", Code: "OTHER", Status: domain.StatusActive}
	if err := erp.DB.Create(&otherOrg).Error; err != nil {
		t.Fatalf("create other org: %v", err)
	}
	rec = erp.request(http.MethodPost, "/api/v1/system/departments", token, map[string]any{
		"organization_id": otherOrg.ID,
		"name":            "跨组织部门",
		"code":            "CROSS",
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-org create department status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuditPersonalAndDepartmentTerminalAccounts(t *testing.T) {
	erp := newTestApp(t)

	adminToken := erp.login(t, "admin", "admin123456")
	rec := erp.request(http.MethodPost, "/api/v1/system/departments", adminToken, map[string]any{
		"organization_id": uint(1),
		"name":            "办公室",
		"code":            "OFFICE",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create department status = %d body=%s", rec.Code, rec.Body.String())
	}

	var personalAudit domain.AuditLog
	if err := erp.DB.Where("actor_username = ?", "admin").Order("id desc").First(&personalAudit).Error; err != nil {
		t.Fatalf("find personal audit: %v", err)
	}
	if personalAudit.PersonName != "系统管理员" {
		t.Fatalf("personal audit person = %q", personalAudit.PersonName)
	}

	terminalToken := erp.createTerminalUserAndLogin(t)
	rec = erp.request(http.MethodPost, "/api/v1/tasks", terminalToken, map[string]any{"placeholder": true})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("terminal task skeleton status = %d body=%s", rec.Code, rec.Body.String())
	}

	var terminalAudit domain.AuditLog
	if err := erp.DB.Where("actor_username = ?", "injection-terminal-01").Order("id desc").First(&terminalAudit).Error; err != nil {
		t.Fatalf("find terminal audit: %v", err)
	}
	if terminalAudit.AccountType != domain.AccountTypeDepartmentTerminal {
		t.Fatalf("terminal audit account_type = %q", terminalAudit.AccountType)
	}
	if terminalAudit.PersonName != domain.UnknownPerson {
		t.Fatalf("terminal audit person = %q", terminalAudit.PersonName)
	}
	if terminalAudit.DepartmentID == nil || terminalAudit.TerminalID == nil {
		t.Fatalf("terminal audit should record department and terminal")
	}
}

type testApp struct {
	*App
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	t.Setenv("BB_ERP_DATABASE_PATH", filepath.Join(t.TempDir(), "erp.sqlite3"))
	t.Setenv("BB_ERP_JWT_SECRET", "test-secret")
	t.Setenv("BB_ERP_HTTP_ALLOWED_ORIGINS", "http://localhost")

	erp, err := New()
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() {
		_ = erp.Shutdown(context.Background())
	})
	return &testApp{App: erp}
}

func (a *testApp) request(method, path, token string, body any) *httptest.ResponseRecorder {
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(body)
		payload = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, payload)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	a.Echo.ServeHTTP(rec, req)
	return rec
}

func (a *testApp) login(t *testing.T, username, password string) string {
	t.Helper()
	rec := a.request(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": username,
		"password": password,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s status = %d body=%s", username, rec.Code, rec.Body.String())
	}
	var body map[string]any
	decodeJSON(t, rec, &body)
	token, ok := body["access_token"].(string)
	if !ok || token == "" {
		t.Fatalf("missing access_token in %v", body)
	}
	return token
}

func (a *testApp) createLimitedUserAndLogin(t *testing.T) string {
	t.Helper()
	hash, err := hashPassword("limited123")
	if err != nil {
		t.Fatalf("hash limited password: %v", err)
	}
	user := domain.User{
		Username:       "limited",
		AccountType:    domain.AccountTypePersonal,
		Name:           "普通用户",
		OrganizationID: 1,
		Status:         domain.StatusActive,
		PasswordHash:   hash,
	}
	if err := a.DB.Create(&user).Error; err != nil {
		t.Fatalf("create limited user: %v", err)
	}
	return a.login(t, "limited", "limited123")
}

func (a *testApp) createTerminalUserAndLogin(t *testing.T) string {
	t.Helper()
	var terminal domain.Terminal
	if err := a.DB.Where("code = ?", "injection-terminal-01").First(&terminal).Error; err != nil {
		t.Fatalf("find seeded terminal: %v", err)
	}
	hash, err := hashPassword("terminal123")
	if err != nil {
		t.Fatalf("hash terminal password: %v", err)
	}
	user := domain.User{
		Username:       "injection-terminal-01",
		AccountType:    domain.AccountTypeDepartmentTerminal,
		Name:           "注塑车间电脑01",
		OrganizationID: 1,
		DepartmentID:   &terminal.DepartmentID,
		TerminalID:     &terminal.ID,
		Status:         domain.StatusActive,
		PasswordHash:   hash,
	}
	if err := a.DB.Create(&user).Error; err != nil {
		t.Fatalf("create terminal user: %v", err)
	}
	if err := a.assignRoleCodes(user.ID, []string{roleTerminal}); err != nil {
		t.Fatalf("assign terminal role: %v", err)
	}
	if err := a.reloadPolicies(); err != nil {
		t.Fatalf("reload policies: %v", err)
	}
	return a.login(t, "injection-terminal-01", "terminal123")
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode json %q: %v", rec.Body.String(), err)
	}
}
