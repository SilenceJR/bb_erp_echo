package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/role"

	"golang.org/x/crypto/bcrypt"
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

// TestRefreshRotatesAndLogoutRevokes 验证 refresh token 轮换、旧令牌失效和退出撤销。
func TestRefreshRotatesAndLogoutRevokes(t *testing.T) {
	erp := newTestApp(t)
	session := erp.loginSession(t, "admin", "admin123456")

	rec := erp.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{
		"refresh_token": session.RefreshToken,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d body=%s", rec.Code, rec.Body.String())
	}
	var refreshed loginSession
	var body map[string]any
	decodeJSON(t, rec, &body)
	refreshed.AccessToken, _ = body["access_token"].(string)
	refreshed.RefreshToken, _ = body["refresh_token"].(string)
	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatalf("refresh response missing tokens: %v", body)
	}
	if refreshed.RefreshToken == session.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	if rec = erp.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{
		"refresh_token": session.RefreshToken,
	}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old refresh status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec = erp.request(http.MethodGet, "/api/v1/auth/me", refreshed.AccessToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("refreshed access token status = %d body=%s", rec.Code, rec.Body.String())
	}

	if rec = erp.request(http.MethodPost, "/api/v1/auth/logout", "", map[string]any{
		"refresh_token": refreshed.RefreshToken,
	}); rec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec = erp.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{
		"refresh_token": refreshed.RefreshToken,
	}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked refresh status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestRefreshRejectsExpiredAndDisabledSessions 验证 refresh token 的过期和账号状态边界。
func TestRefreshRejectsExpiredAndDisabledSessions(t *testing.T) {
	erp := newTestApp(t)
	session := erp.loginSession(t, "admin", "admin123456")
	if err := erp.DB.Model(&model.RefreshSession{}).
		Where("user_id = ?", 1).
		Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("expire refresh session: %v", err)
	}
	if rec := erp.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{
		"refresh_token": session.RefreshToken,
	}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired refresh status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	disabled := erp.createLimitedUserAndLoginSession(t)
	var user model.User
	if err := erp.DB.Where("username = ?", "limited").First(&user).Error; err != nil {
		t.Fatalf("find limited user: %v", err)
	}
	if err := erp.DB.Model(&user).Update("status", model.StatusDisabled).Error; err != nil {
		t.Fatalf("disable limited user: %v", err)
	}
	if rec := erp.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{
		"refresh_token": disabled.RefreshToken,
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("disabled refresh status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// TestRefreshAllowsOnlyOneConcurrentRotation 验证同一个 refresh token 并发使用时只允许一次成功。
func TestRefreshAllowsOnlyOneConcurrentRotation(t *testing.T) {
	erp := newTestApp(t)
	session := erp.loginSession(t, "admin", "admin123456")
	statuses := make(chan int, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			rec := erp.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{
				"refresh_token": session.RefreshToken,
			})
			statuses <- rec.Code
		}()
	}
	group.Wait()
	close(statuses)

	var success, unauthorized int
	for status := range statuses {
		switch status {
		case http.StatusOK:
			success++
		case http.StatusUnauthorized:
			unauthorized++
		}
	}
	if success != 1 || unauthorized != 1 {
		t.Fatalf("concurrent refresh statuses = success:%d unauthorized:%d", success, unauthorized)
	}
}

// TestChangePasswordInvalidatesOldToken 验证修改密码接口的认证、密码校验、
// bcrypt 长度限制，以及密码版本递增后旧 JWT 立即失效。
func TestChangePasswordInvalidatesOldToken(t *testing.T) {
	erp := newTestApp(t)
	if rec := erp.request(http.MethodPost, "/api/v1/auth/change-password", "", map[string]any{
		"current_password": "admin123456",
		"new_password":     "newAdmin123456",
	}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous change password status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	token := erp.login(t, "admin", "admin123456")
	secondSession := erp.loginSession(t, "admin", "admin123456")

	tests := []struct {
		name       string
		current    string
		new        string
		wantStatus int
	}{
		{name: "当前密码错误", current: "wrong-current", new: "newAdmin123456", wantStatus: http.StatusUnauthorized},
		{name: "新密码过短", current: "admin123456", new: "short", wantStatus: http.StatusBadRequest},
		{name: "新密码超过 bcrypt 字节限制", current: "admin123456", new: strings.Repeat("a", auth.MaxPasswordBytes+1), wantStatus: http.StatusBadRequest},
		{name: "新密码与旧密码相同", current: "admin123456", new: "admin123456", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := erp.request(http.MethodPost, "/api/v1/auth/change-password", token, map[string]any{
				"current_password": tt.current,
				"new_password":     tt.new,
			})
			if rec.Code != tt.wantStatus {
				t.Fatalf("change password status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}

	rec := erp.request(http.MethodPost, "/api/v1/auth/change-password", token, map[string]any{
		"current_password": "admin123456",
		"new_password":     "newAdmin123456",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("change password status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("change password response body = %q, want empty", rec.Body.String())
	}

	var admin model.User
	if err := erp.DB.Where("username = ?", "admin").First(&admin).Error; err != nil {
		t.Fatalf("find admin after password change: %v", err)
	}
	if admin.PasswordVersion != 2 {
		t.Fatalf("password_version = %d, want 2", admin.PasswordVersion)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("newAdmin123456")); err != nil {
		t.Fatalf("new password hash does not match: %v", err)
	}

	rec = erp.request(http.MethodGet, "/api/v1/auth/me", token, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old token status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if rec = erp.request(http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{
		"refresh_token": secondSession.RefreshToken,
	}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh token after password change status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec = erp.request(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "admin",
		"password": "admin123456",
	}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	newToken := erp.login(t, "admin", "newAdmin123456")
	if rec = erp.request(http.MethodGet, "/api/v1/auth/me", newToken, nil); rec.Code != http.StatusOK {
		t.Fatalf("new token status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestUpdateStatusRoutesAndClientVersionParameter(t *testing.T) {
	erp := newTestApp(t)
	token := erp.login(t, "admin", "admin123456")

	rec := erp.request(http.MethodGet, "/api/v1/system/updates/status", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET update status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = erp.request(http.MethodPost, "/api/v1/system/updates/check", token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST update check = %d body=%s", rec.Code, rec.Body.String())
	}
	var checkStatus map[string]any
	decodeJSON(t, rec, &checkStatus)
	if checkStatus["last_error"] == "" {
		t.Fatal("disabled update check should return its state and error detail")
	}

	rec = erp.request(http.MethodGet, "/api/v1/updates/client/status?current_version=1.2.3-beta.1", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET client update status = %d", rec.Code)
	}
	var clientStatus map[string]any
	decodeJSON(t, rec, &clientStatus)
	if clientStatus["current_version"] != "1.2.3-beta.1" {
		t.Fatalf("client current_version = %v", clientStatus["current_version"])
	}
	rec = erp.request(http.MethodGet, "/api/v1/updates/client/download", "", nil)
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusNotFound {
		t.Fatalf("legacy public recovery download = %d, want 401 or 404", rec.Code)
	}
	rec = erp.request(http.MethodGet, "/api/v1/system/updates/client/download", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated recovery download = %d, want 401", rec.Code)
	}
	rec = erp.request(http.MethodGet, "/api/v1/system/updates/client/download", token, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("authorized recovery download without package = %d, want 404", rec.Code)
	}

	limitedToken := erp.createLimitedUserAndLogin(t)
	rec = erp.request(http.MethodGet, "/api/v1/system/updates/status", limitedToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("limited update status access = %d", rec.Code)
	}
	rec = erp.request(http.MethodGet, "/api/v1/system/updates/client/download", limitedToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("limited recovery download access = %d", rec.Code)
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

	product := model.Product{
		Name:   "审计测试产品",
		Code:   "AUDIT-PRODUCT-001",
		Unit:   "个",
		Status: model.StatusActive,
	}
	if err := erp.DB.Create(&product).Error; err != nil {
		t.Fatalf("create audit product: %v", err)
	}
	birth := time.Date(1990, 1, 1, 0, 0, 0, 0, time.Local)
	operator := model.Employee{OrganizationID: 1, Name: "审计操作员工", HireDate: birth, BirthDate: birth, Status: model.StatusActive}
	if err := erp.DB.Create(&operator).Error; err != nil {
		t.Fatalf("create audit operator: %v", err)
	}
	if err := erp.DB.Create(&model.EmployeeDepartment{EmployeeID: operator.ID, DepartmentID: 1}).Error; err != nil {
		t.Fatalf("link audit operator: %v", err)
	}

	terminalToken := erp.createTerminalUserAndLogin(t)
	rec = erp.request(http.MethodPost, "/api/v1/tasks", terminalToken, map[string]any{
		"code":                  "AUDIT-TASK-001",
		"type":                  "production",
		"product_id":            product.ID,
		"planned_quantity":      int64(10000),
		"target_department_ids": []uint{1},
		"operator_employee_id":  operator.ID,
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

type loginSession struct {
	AccessToken  string
	RefreshToken string
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
	return a.loginSession(t, username, password).AccessToken
}

// loginSession 登录并返回 access/refresh token 对，供会话轮换测试使用。
func (a *testApp) loginSession(t *testing.T, username, password string) loginSession {
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
	refreshToken, ok := body["refresh_token"].(string)
	if !ok || refreshToken == "" {
		t.Fatalf("missing refresh_token in %v", body)
	}
	return loginSession{AccessToken: token, RefreshToken: refreshToken}
}

// createLimitedUserAndLogin 创建没有角色权限的普通账号并登录。
//
// 参数说明：
// - t：当前测试对象。
func (a *testApp) createLimitedUserAndLogin(t *testing.T) string {
	t.Helper()
	return a.createLimitedUserAndLoginSession(t).AccessToken
}

func (a *testApp) createLimitedUserAndLoginSession(t *testing.T) loginSession {
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
	return a.loginSession(t, "limited", "limited123")
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
