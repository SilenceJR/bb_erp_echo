package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/role"
)

// TestInitializationHealthReadyAndSQLiteWAL 验证应用能完成初始化，并确认健康检查、
// 就绪检查和 SQLite WAL 模式都按后台框架要求工作。
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

// TestLoginMeAndInvalidCredentials 验证登录成功、密码错误和当前用户信息返回。
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
	if body["account_type"] != model.AccountTypePersonal {
		t.Fatalf("account_type = %v", body["account_type"])
	}
	if len(body["permissions"].([]any)) == 0 {
		t.Fatalf("permissions should not be empty")
	}
}

// TestJWTCasbinAndSingleOrganizationDepartmentCreate 验证 JWT 必填、Casbin 权限拒绝，
// 以及单组织模式下部门创建会自动落到当前默认组织。
func TestJWTCasbinAndSingleOrganizationDepartmentCreate(t *testing.T) {
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

	rec = erp.request(http.MethodPost, "/api/v1/system/departments", token, map[string]any{
		"name": "包装部",
		"code": "PACK",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("single-org create department status = %d body=%s", rec.Code, rec.Body.String())
	}
	var department model.Department
	decodeJSON(t, rec, &department)
	if department.OrganizationID != 1 {
		t.Fatalf("department organization_id = %d", department.OrganizationID)
	}
}

// TestAuditPersonalAndDepartmentTerminalAccounts 验证两类账号的审计差异：
// 个人账号记录具体人员，部门终端账号记录部门和终端且人员为“未知”。
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

	var personalAudit model.AuditLog
	if err := erp.DB.Where("actor_username = ?", "admin").Order("id desc").First(&personalAudit).Error; err != nil {
		t.Fatalf("find personal audit: %v", err)
	}
	if personalAudit.PersonName != "系统管理员" {
		t.Fatalf("personal audit person = %q", personalAudit.PersonName)
	}

	terminalToken := erp.createTerminalUserAndLogin(t)
	rec = erp.request(http.MethodPost, "/api/v1/tasks", terminalToken, map[string]any{
		"code":                  "AUDIT-TASK-001",
		"type":                  "production",
		"product_name":          "审计测试产品",
		"planned_quantity":      int64(10000),
		"unit":                  "个",
		"target_department_ids": []uint{1},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("terminal task create status = %d body=%s", rec.Code, rec.Body.String())
	}

	var terminalAudit model.AuditLog
	if err := erp.DB.Where("actor_username = ?", "injection-terminal-01").Order("id desc").First(&terminalAudit).Error; err != nil {
		t.Fatalf("find terminal audit: %v", err)
	}
	if terminalAudit.AccountType != model.AccountTypeDepartmentTerminal {
		t.Fatalf("terminal audit account_type = %q", terminalAudit.AccountType)
	}
	if terminalAudit.PersonName != model.UnknownPerson {
		t.Fatalf("terminal audit person = %q", terminalAudit.PersonName)
	}
	if terminalAudit.DepartmentID == nil || terminalAudit.TerminalID == nil {
		t.Fatalf("terminal audit should record department and terminal")
	}
}

// TestWebStaticServedByEcho 验证 Echo 可以随着后端服务一起托管 Web 管理端静态文件。
func TestWebStaticServedByEcho(t *testing.T) {
	erp := newTestApp(t)

	rec := erp.request(http.MethodGet, "/", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("博邦 ERP Web")) {
		t.Fatalf("index content mismatch: %s", rec.Body.String())
	}

	rec = erp.request(http.MethodGet, "/assets/app.js", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /assets/app.js status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("console.log")) {
		t.Fatalf("asset content mismatch: %s", rec.Body.String())
	}

	rec = erp.request(http.MethodGet, "/dashboard", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /dashboard status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("博邦 ERP Web")) {
		t.Fatalf("spa fallback content mismatch: %s", rec.Body.String())
	}

	rec = erp.request(http.MethodGet, "/api/not-found", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/not-found status = %d body=%s", rec.Code, rec.Body.String())
	}
}

type testApp struct {
	*App
}

// newTestApp 创建隔离测试应用。
//
// 参数说明：
// - t：当前测试对象，用于注册清理函数和失败输出。
//
// 返回说明：返回使用临时 SQLite 数据库和测试 JWT 密钥的应用实例。
func newTestApp(t *testing.T) *testApp {
	t.Helper()
	webDir := filepath.Join(t.TempDir(), "web-dist")
	if err := os.MkdirAll(filepath.Join(webDir, "assets"), 0o755); err != nil {
		t.Fatalf("create web assets dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<html><body>博邦 ERP Web</body></html>"), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "assets", "app.js"), []byte("console.log('bb erp')"), 0o644); err != nil {
		t.Fatalf("write app.js: %v", err)
	}

	t.Setenv("BB_ERP_DATABASE_PATH", filepath.Join(t.TempDir(), "erp.sqlite3"))
	t.Setenv("BB_ERP_LOG_DIR", filepath.Join(t.TempDir(), "logs"))
	t.Setenv("BB_ERP_LOG_CONSOLE", "false")
	t.Setenv("BB_ERP_WEB_DIST_DIR", webDir)
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

// request 通过 httptest 向 Echo 发起请求。
//
// 参数说明：
// - method：HTTP 方法。
// - path：请求路径。
// - token：Bearer token；为空表示匿名请求。
// - body：JSON 请求体；为 nil 时发送空请求体。
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

// login 使用指定账号密码登录并返回 access token。
//
// 参数说明：
// - t：当前测试对象。
// - username：登录账号。
// - password：登录密码。
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

// createLimitedUserAndLogin 创建没有角色权限的普通账号并登录。
//
// 参数说明：
// - t：当前测试对象。
func (a *testApp) createLimitedUserAndLogin(t *testing.T) string {
	t.Helper()
	hash, err := auth.HashPassword("limited123")
	if err != nil {
		t.Fatalf("hash limited password: %v", err)
	}
	user := model.User{
		Username:       "limited",
		AccountType:    model.AccountTypePersonal,
		Name:           "普通用户",
		OrganizationID: 1,
		Status:         model.StatusActive,
		PasswordHash:   hash,
	}
	if err := a.DB.Create(&user).Error; err != nil {
		t.Fatalf("create limited user: %v", err)
	}
	return a.login(t, "limited", "limited123")
}

// createTerminalUserAndLogin 创建部门终端账号并登录。
//
// 参数说明：
// - t：当前测试对象。
//
// 返回说明：返回部门终端账号的 access token。
func (a *testApp) createTerminalUserAndLogin(t *testing.T) string {
	t.Helper()
	var terminal model.Terminal
	if err := a.DB.Where("code = ?", "injection-terminal-01").First(&terminal).Error; err != nil {
		t.Fatalf("find seeded terminal: %v", err)
	}
	hash, err := auth.HashPassword("terminal123")
	if err != nil {
		t.Fatalf("hash terminal password: %v", err)
	}
	user := model.User{
		Username:       "injection-terminal-01",
		AccountType:    model.AccountTypeDepartmentTerminal,
		Name:           "注塑车间电脑01",
		OrganizationID: 1,
		DepartmentID:   &terminal.DepartmentID,
		TerminalID:     &terminal.ID,
		Status:         model.StatusActive,
		PasswordHash:   hash,
	}
	if err := a.DB.Create(&user).Error; err != nil {
		t.Fatalf("create terminal user: %v", err)
	}
	if err := a.RoleService.AssignRoleCodes(user.ID, []string{role.TerminalOperatorCode}); err != nil {
		t.Fatalf("assign terminal role: %v", err)
	}
	if err := a.RoleService.ReloadPolicies(); err != nil {
		t.Fatalf("reload policies: %v", err)
	}
	return a.login(t, "injection-terminal-01", "terminal123")
}

// decodeJSON 解析测试响应 JSON。
//
// 参数说明：
// - t：当前测试对象。
// - rec：HTTP 响应记录器。
// - dst：JSON 解析目标指针。
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode json %q: %v", rec.Body.String(), err)
	}
}
