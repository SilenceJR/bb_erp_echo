package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bb_erp_echo/internal/config"
	"bb_erp_echo/internal/database"
	"bb_erp_echo/internal/domain"

	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	contextUserKey = "current_user"

	roleSuperAdmin = "super_admin"
	roleTerminal   = "department_terminal_operator"
)

type App struct {
	Config   *config.Config
	Logger   *slog.Logger
	DB       *gorm.DB
	Echo     *echo.Echo
	Server   *http.Server
	Enforcer *casbin.Enforcer
}

type Validator struct {
	validate *validator.Validate
}

func (v *Validator) Validate(i any) error {
	return v.validate.Struct(i)
}

type CurrentUser struct {
	ID             uint
	Username       string
	AccountType    string
	Name           string
	OrganizationID uint
	DepartmentID   *uint
	TerminalID     *uint
	Permissions    []string
	Roles          []string
}

type Claims struct {
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	AccountType string `json:"account_type"`
	jwt.RegisteredClaims
}

func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	logger := newLogger(cfg)

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(domain.AllModels()...); err != nil {
		return nil, fmt.Errorf("auto migrate database: %w", err)
	}

	enforcer, err := newEnforcer()
	if err != nil {
		return nil, err
	}

	app := &App{
		Config:   cfg,
		Logger:   logger,
		DB:       db,
		Echo:     echo.New(),
		Enforcer: enforcer,
	}

	app.configureEcho()

	if err := app.seedSystemData(); err != nil {
		return nil, err
	}

	if err := app.reloadPolicies(); err != nil {
		return nil, err
	}

	app.registerRoutes()

	return app, nil
}

func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.Log.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

func newEnforcer() (*casbin.Enforcer, error) {
	m, err := casbinmodel.NewModelFromString(`
[request_definition]
r = sub, obj, act, org, dept

[policy_definition]
p = sub, obj, act, org, dept

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && regexMatch(r.act, p.act) && (p.org == "*" || p.org == r.org) && (p.dept == "*" || p.dept == r.dept)
`)
	if err != nil {
		return nil, fmt.Errorf("create casbin model: %w", err)
	}

	return casbin.NewEnforcer(m)
}

func (a *App) configureEcho() {
	a.Echo.Validator = &Validator{validate: validator.New()}

	a.Echo.Use(middleware.RequestID())
	a.Echo.Use(middleware.Recover())
	a.Echo.Use(middleware.Secure())
	a.Echo.Use(middleware.BodyLimit(10 * 1024 * 1024))
	a.Echo.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: a.Config.HTTP.AllowedOrigins,
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			echo.HeaderXRequestID,
		},
		AllowCredentials: true,
	}))
	a.Echo.Use(a.requestLogger())
	a.Echo.HTTPErrorHandler = a.httpErrorHandler
}

func (a *App) registerRoutes() {
	a.Echo.GET("/health", a.health)
	a.Echo.GET("/ready", a.ready)

	v1 := a.Echo.Group("/api/v1")

	auth := v1.Group("/auth")
	auth.POST("/login", a.login)
	auth.GET("/me", a.me, a.jwtMiddleware())

	system := v1.Group("/system", a.jwtMiddleware(), a.auditMiddleware())
	system.GET("/organizations", a.listOrganizations, a.requirePermission("/api/v1/system/organizations", "read"))
	system.POST("/organizations", a.createOrganization, a.requirePermission("/api/v1/system/organizations", "write"))
	system.GET("/departments", a.listDepartments, a.requirePermission("/api/v1/system/departments", "read"))
	system.POST("/departments", a.createDepartment, a.requirePermission("/api/v1/system/departments", "write"))
	system.GET("/terminals", a.listTerminals, a.requirePermission("/api/v1/system/terminals", "read"))
	system.POST("/terminals", a.createTerminal, a.requirePermission("/api/v1/system/terminals", "write"))
	system.GET("/users", a.listUsers, a.requirePermission("/api/v1/system/users", "read"))
	system.POST("/users", a.createUser, a.requirePermission("/api/v1/system/users", "write"))
	system.PATCH("/users/:id/status", a.updateUserStatus, a.requirePermission("/api/v1/system/users", "write"))
	system.POST("/users/:id/reset-password", a.resetUserPassword, a.requirePermission("/api/v1/system/users", "write"))
	system.POST("/users/:id/roles", a.assignUserRoles, a.requirePermission("/api/v1/system/users", "write"))
	system.GET("/roles", a.listRoles, a.requirePermission("/api/v1/system/roles", "read"))
	system.POST("/roles", a.createRole, a.requirePermission("/api/v1/system/roles", "write"))
	system.POST("/roles/:id/permissions", a.assignRolePermissions, a.requirePermission("/api/v1/system/roles", "write"))
	system.GET("/permissions", a.listPermissions, a.requirePermission("/api/v1/system/permissions", "read"))
	system.GET("/audits", a.listAudits, a.requirePermission("/api/v1/system/audits", "read"))

	a.registerModule(v1, "customers", "客户与联系人")
	a.registerModule(v1, "inventory", "仓库与库存")
	a.registerModule(v1, "materials", "物料与产品")
	a.registerModule(v1, "molds", "模具管理")
	a.registerModule(v1, "tasks", "任务单与部门子任务")
	a.registerModule(v1, "reports", "统计报表")
}

func (a *App) registerModule(v1 *echo.Group, path string, name string) {
	group := v1.Group("/"+path, a.jwtMiddleware(), a.auditMiddleware())
	group.GET("", func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{
			"module":  path,
			"name":    name,
			"status":  "skeleton",
			"message": "模块骨架已注册，业务接口待后续迭代。",
		})
	}, a.requirePermission("/api/v1/"+path, "read"))
	group.POST("", func(c *echo.Context) error {
		return c.JSON(http.StatusAccepted, map[string]any{
			"module":  path,
			"name":    name,
			"status":  "skeleton",
			"message": "模块写入入口已预留，业务流程待后续迭代。",
		})
	}, a.requirePermission("/api/v1/"+path, "write"))
}

func (a *App) Start() error {
	address := fmt.Sprintf("%s:%d", a.Config.HTTP.Host, a.Config.HTTP.Port)
	a.Logger.Info("ERP server starting", "address", address, "environment", a.Config.App.Environment, "database", a.Config.Database.Path)

	a.Server = &http.Server{
		Addr:    address,
		Handler: a.Echo,
	}

	err := a.Server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("start echo server: %w", err)
	}
	return nil
}

func (a *App) Run() error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Start()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.Shutdown(ctx)
	}
}

func (a *App) Shutdown(ctx context.Context) error {
	a.Logger.Info("ERP server shutting down")

	if a.Server != nil {
		if err := a.Server.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown echo server: %w", err)
		}
	}

	sqlDB, err := a.DB.DB()
	if err != nil {
		return fmt.Errorf("get sql database: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}

	a.Logger.Info("ERP server stopped")
	return nil
}

func (a *App) health(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{"status": "ok", "time": time.Now()})
}

func (a *App) ready(c *echo.Context) error {
	sqlDB, err := a.DB.DB()
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "message": "database unavailable"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "message": "database ping failed"})
	}

	return c.JSON(http.StatusOK, map[string]any{"status": "ready"})
}

func (a *App) login(c *echo.Context) error {
	var req struct {
		Username string `json:"username" validate:"required"`
		Password string `json:"password" validate:"required"`
	}
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}

	var user domain.User
	if err := a.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "账号或密码错误")
	}
	if user.Status != domain.StatusActive {
		return echo.NewHTTPError(http.StatusForbidden, "账号已停用")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "账号或密码错误")
	}

	now := time.Now()
	a.DB.Model(&user).Update("last_login_at", now)

	token, expiresAt, err := a.issueToken(user)
	if err != nil {
		return err
	}

	current, err := a.currentUserFromModel(user)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"access_token": token,
		"expires_at":   expiresAt,
		"user":         currentUserResponse(current),
	})
}

func (a *App) me(c *echo.Context) error {
	current := currentUser(c)
	if current == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	return c.JSON(http.StatusOK, currentUserResponse(current))
}

func (a *App) listOrganizations(c *echo.Context) error {
	var items []domain.Organization
	if err := a.DB.Order("id").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

func (a *App) createOrganization(c *echo.Context) error {
	var req struct {
		Name string `json:"name" validate:"required"`
		Code string `json:"code" validate:"required"`
	}
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}
	item := domain.Organization{Name: req.Name, Code: req.Code, Status: domain.StatusActive}
	if err := a.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

func (a *App) listDepartments(c *echo.Context) error {
	var items []domain.Department
	query := a.DB.Order("id")
	if current := currentUser(c); current != nil {
		query = query.Where("organization_id = ?", current.OrganizationID)
	}
	if err := query.Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

func (a *App) createDepartment(c *echo.Context) error {
	var req struct {
		OrganizationID uint   `json:"organization_id" validate:"required"`
		Name           string `json:"name" validate:"required"`
		Code           string `json:"code" validate:"required"`
	}
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}
	if !a.canAccessOrg(c, req.OrganizationID) {
		return echo.NewHTTPError(http.StatusForbidden, "无权访问该组织数据")
	}
	item := domain.Department{OrganizationID: req.OrganizationID, Name: req.Name, Code: req.Code, Status: domain.StatusActive}
	if err := a.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

func (a *App) listTerminals(c *echo.Context) error {
	var items []domain.Terminal
	query := a.DB.Model(&domain.Terminal{}).Order("terminals.id")
	if current := currentUser(c); current != nil {
		query = query.Joins("JOIN departments ON departments.id = terminals.department_id").Where("departments.organization_id = ?", current.OrganizationID)
	}
	if err := query.Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

func (a *App) createTerminal(c *echo.Context) error {
	var req struct {
		DepartmentID uint   `json:"department_id" validate:"required"`
		Code         string `json:"code" validate:"required"`
		Name         string `json:"name" validate:"required"`
		Location     string `json:"location"`
	}
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}
	if !a.canAccessDepartment(c, req.DepartmentID) {
		return echo.NewHTTPError(http.StatusForbidden, "无权访问该部门数据")
	}
	item := domain.Terminal{DepartmentID: req.DepartmentID, Code: req.Code, Name: req.Name, Location: req.Location, Status: domain.StatusActive}
	if err := a.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

func (a *App) listUsers(c *echo.Context) error {
	var items []domain.User
	query := a.DB.Order("id")
	if current := currentUser(c); current != nil {
		query = query.Where("organization_id = ?", current.OrganizationID)
	}
	if err := query.Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

func (a *App) createUser(c *echo.Context) error {
	var req struct {
		Username       string `json:"username" validate:"required"`
		Password       string `json:"password" validate:"required,min=8"`
		AccountType    string `json:"account_type" validate:"required,oneof=personal department_terminal"`
		Name           string `json:"name" validate:"required"`
		OrganizationID uint   `json:"organization_id" validate:"required"`
		DepartmentID   *uint  `json:"department_id"`
		TerminalID     *uint  `json:"terminal_id"`
	}
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}
	if !a.canAccessOrg(c, req.OrganizationID) {
		return echo.NewHTTPError(http.StatusForbidden, "无权访问该组织数据")
	}
	if req.AccountType == domain.AccountTypeDepartmentTerminal && (req.DepartmentID == nil || req.TerminalID == nil) {
		return echo.NewHTTPError(http.StatusBadRequest, "部门终端账号必须绑定部门和终端")
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		return err
	}

	item := domain.User{
		Username:       req.Username,
		AccountType:    req.AccountType,
		Name:           req.Name,
		OrganizationID: req.OrganizationID,
		DepartmentID:   req.DepartmentID,
		TerminalID:     req.TerminalID,
		Status:         domain.StatusActive,
		PasswordHash:   hash,
	}
	if err := a.DB.Create(&item).Error; err != nil {
		return err
	}
	if req.AccountType == domain.AccountTypeDepartmentTerminal {
		a.assignRoleCodes(item.ID, []string{roleTerminal})
	}
	return c.JSON(http.StatusCreated, item)
}

func (a *App) updateUserStatus(c *echo.Context) error {
	id, err := paramID(c)
	if err != nil {
		return err
	}
	var req struct {
		Status string `json:"status" validate:"required,oneof=active disabled"`
	}
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}
	if err := a.DB.Model(&domain.User{}).Where("id = ?", id).Update("status", req.Status).Error; err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) resetUserPassword(c *echo.Context) error {
	id, err := paramID(c)
	if err != nil {
		return err
	}
	var req struct {
		Password string `json:"password" validate:"required,min=8"`
	}
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return err
	}
	if err := a.DB.Model(&domain.User{}).Where("id = ?", id).Update("password_hash", hash).Error; err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) assignUserRoles(c *echo.Context) error {
	id, err := paramID(c)
	if err != nil {
		return err
	}
	var req struct {
		RoleIDs []uint `json:"role_ids" validate:"required"`
	}
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}
	var user domain.User
	if err := a.DB.First(&user, id).Error; err != nil {
		return err
	}
	if user.AccountType == domain.AccountTypeDepartmentTerminal && a.includesSystemRole(req.RoleIDs) {
		return echo.NewHTTPError(http.StatusBadRequest, "部门终端账号不能授予系统管理权限")
	}
	if err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&domain.UserRole{}).Error; err != nil {
			return err
		}
		for _, roleID := range req.RoleIDs {
			if err := tx.Create(&domain.UserRole{UserID: id, RoleID: roleID}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := a.reloadPolicies(); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) listRoles(c *echo.Context) error {
	var items []domain.Role
	if err := a.DB.Order("id").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

func (a *App) createRole(c *echo.Context) error {
	var req struct {
		Name        string `json:"name" validate:"required"`
		Code        string `json:"code" validate:"required"`
		Description string `json:"description"`
	}
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}
	item := domain.Role{Name: req.Name, Code: req.Code, Description: req.Description}
	if err := a.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

func (a *App) assignRolePermissions(c *echo.Context) error {
	id, err := paramID(c)
	if err != nil {
		return err
	}
	var req struct {
		PermissionIDs []uint `json:"permission_ids" validate:"required"`
	}
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}
	if err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", id).Delete(&domain.RolePermission{}).Error; err != nil {
			return err
		}
		for _, permissionID := range req.PermissionIDs {
			if err := tx.Create(&domain.RolePermission{RoleID: id, PermissionID: permissionID}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := a.reloadPolicies(); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (a *App) listPermissions(c *echo.Context) error {
	var items []domain.Permission
	if err := a.DB.Order("object, action").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

func (a *App) listAudits(c *echo.Context) error {
	var items []domain.AuditLog
	query := a.DB.Order("id desc").Limit(200)
	if current := currentUser(c); current != nil {
		query = query.Where("organization_id = ?", current.OrganizationID)
	}
	if err := query.Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

func (a *App) jwtMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			header := c.Request().Header.Get(echo.HeaderAuthorization)
			tokenText := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
			if tokenText == "" || tokenText == header {
				return echo.NewHTTPError(http.StatusUnauthorized, "缺少登录令牌")
			}

			token, err := jwt.ParseWithClaims(tokenText, &Claims{}, func(token *jwt.Token) (any, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method")
				}
				return []byte(a.Config.JWT.Secret), nil
			})
			if err != nil || !token.Valid {
				return echo.NewHTTPError(http.StatusUnauthorized, "登录令牌无效")
			}

			claims, ok := token.Claims.(*Claims)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "登录令牌无效")
			}

			var user domain.User
			if err := a.DB.First(&user, claims.UserID).Error; err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "账号不存在")
			}
			if user.Status != domain.StatusActive {
				return echo.NewHTTPError(http.StatusForbidden, "账号已停用")
			}

			current, err := a.currentUserFromModel(user)
			if err != nil {
				return err
			}
			c.Set(contextUserKey, current)
			return next(c)
		}
	}
}

func (a *App) requirePermission(object string, action string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			current := currentUser(c)
			if current == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
			}
			org := strconv.FormatUint(uint64(current.OrganizationID), 10)
			dept := "*"
			if current.DepartmentID != nil {
				dept = strconv.FormatUint(uint64(*current.DepartmentID), 10)
			}
			allowed, err := a.Enforcer.Enforce(current.Username, object, action, org, dept)
			if err != nil {
				return err
			}
			if !allowed {
				return echo.NewHTTPError(http.StatusForbidden, "没有操作权限")
			}
			return next(c)
		}
	}
}

func (a *App) auditMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			method := c.Request().Method
			if method == http.MethodGet || method == http.MethodOptions {
				return err
			}

			current := currentUser(c)
			log := domain.AuditLog{
				RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
				Object:    c.Path(),
				Action:    method,
				Method:    method,
				Path:      c.Request().URL.Path,
				Status:    responseStatus(c),
				RemoteIP:  c.RealIP(),
				UserAgent: c.Request().UserAgent(),
				Result:    "success",
			}
			if err != nil || responseStatus(c) >= http.StatusBadRequest {
				log.Result = "failed"
			}
			if current != nil {
				log.ActorUserID = &current.ID
				log.ActorUsername = current.Username
				log.AccountType = current.AccountType
				log.OrganizationID = &current.OrganizationID
				log.DepartmentID = current.DepartmentID
				log.TerminalID = current.TerminalID
				if current.AccountType == domain.AccountTypeDepartmentTerminal {
					log.PersonName = domain.UnknownPerson
				} else {
					log.PersonName = current.Name
				}
			}
			if createErr := a.DB.Create(&log).Error; createErr != nil {
				a.Logger.Error("create audit log failed", "error", createErr)
			}
			return err
		}
	}
}

func (a *App) requestLogger() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			startedAt := time.Now()
			err := next(c)

			current := currentUser(c)
			attrs := []any{
				"request_id", c.Response().Header().Get(echo.HeaderXRequestID),
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
				"query", c.Request().URL.RawQuery,
				"status", responseStatus(c),
				"duration", time.Since(startedAt).String(),
				"remote_ip", c.RealIP(),
				"user_agent", c.Request().UserAgent(),
			}
			if current != nil {
				attrs = append(attrs,
					"account", current.Username,
					"department_id", current.DepartmentID,
					"terminal_id", current.TerminalID,
				)
			}
			a.Logger.Info("HTTP request", attrs...)
			return err
		}
	}
}

func (a *App) httpErrorHandler(c *echo.Context, err error) {
	if responseCommitted(c) {
		return
	}

	status := http.StatusInternalServerError
	code := "INTERNAL_ERROR"
	message := "服务器内部错误"

	var httpError *echo.HTTPError
	if errors.As(err, &httpError) {
		status = httpError.Code
		code = strings.ToUpper(strings.ReplaceAll(http.StatusText(status), " ", "_"))
		if httpError.Message != "" {
			message = httpError.Message
		}
	}

	requestID := c.Response().Header().Get(echo.HeaderXRequestID)
	if status >= http.StatusInternalServerError {
		a.Logger.Error("HTTP request failed", "error", err, "request_id", requestID, "method", c.Request().Method, "path", c.Request().URL.Path, "status", status)
	} else {
		a.Logger.Warn("HTTP request rejected", "error", err, "request_id", requestID, "method", c.Request().Method, "path", c.Request().URL.Path, "status", status)
	}

	_ = c.JSON(status, map[string]any{"code": code, "message": message, "request_id": requestID})
}

func (a *App) issueToken(user domain.User) (string, time.Time, error) {
	expiresAt := time.Now().Add(a.Config.JWT.ExpiresIn)
	claims := Claims{
		UserID:      user.ID,
		Username:    user.Username,
		AccountType: user.AccountType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    a.Config.JWT.Issuer,
			Subject:   strconv.FormatUint(uint64(user.ID), 10),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.Config.JWT.Secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("issue jwt token: %w", err)
	}
	return token, expiresAt, nil
}

func (a *App) currentUserFromModel(user domain.User) (*CurrentUser, error) {
	roles, err := a.roleCodesForUser(user.ID)
	if err != nil {
		return nil, err
	}
	permissions, err := a.permissionCodesForUser(user.ID)
	if err != nil {
		return nil, err
	}
	return &CurrentUser{
		ID:             user.ID,
		Username:       user.Username,
		AccountType:    user.AccountType,
		Name:           user.Name,
		OrganizationID: user.OrganizationID,
		DepartmentID:   user.DepartmentID,
		TerminalID:     user.TerminalID,
		Permissions:    permissions,
		Roles:          roles,
	}, nil
}

func (a *App) seedSystemData() error {
	org := domain.Organization{Name: "博邦", Code: "BOBANG", Status: domain.StatusActive}
	if err := a.DB.FirstOrCreate(&org, domain.Organization{Code: org.Code}).Error; err != nil {
		return err
	}

	dept := domain.Department{OrganizationID: org.ID, Name: "总部", Code: "HQ", Status: domain.StatusActive}
	if err := a.DB.Where("organization_id = ? AND code = ?", org.ID, dept.Code).FirstOrCreate(&dept).Error; err != nil {
		return err
	}

	terminal := domain.Terminal{DepartmentID: dept.ID, Code: "injection-terminal-01", Name: "注塑车间电脑01", Location: "注塑车间", Status: domain.StatusActive}
	if err := a.DB.FirstOrCreate(&terminal, domain.Terminal{Code: terminal.Code}).Error; err != nil {
		return err
	}

	permissions := defaultPermissions()
	for _, permission := range permissions {
		if err := a.DB.FirstOrCreate(&permission, domain.Permission{Code: permission.Code}).Error; err != nil {
			return err
		}
	}

	super := domain.Role{Name: "超级管理员", Code: roleSuperAdmin, Description: "系统内置管理员角色", System: true}
	if err := a.DB.FirstOrCreate(&super, domain.Role{Code: super.Code}).Error; err != nil {
		return err
	}
	terminalRole := domain.Role{Name: "部门终端操作员", Code: roleTerminal, Description: "公共部门终端账号使用", System: true}
	if err := a.DB.FirstOrCreate(&terminalRole, domain.Role{Code: terminalRole.Code}).Error; err != nil {
		return err
	}

	if err := a.attachPermissions(super.ID, nil); err != nil {
		return err
	}
	if err := a.attachPermissionCodes(terminalRole.ID, []string{"tasks:read", "tasks:write", "inventory:read"}); err != nil {
		return err
	}

	hash, err := hashPassword(a.Config.Admin.Password)
	if err != nil {
		return err
	}
	admin := domain.User{
		Username:       a.Config.Admin.Username,
		AccountType:    domain.AccountTypePersonal,
		Name:           a.Config.Admin.Name,
		OrganizationID: org.ID,
		DepartmentID:   &dept.ID,
		Status:         domain.StatusActive,
		PasswordHash:   hash,
	}
	if err := a.DB.FirstOrCreate(&admin, domain.User{Username: admin.Username}).Error; err != nil {
		return err
	}
	return a.assignRoleCodes(admin.ID, []string{roleSuperAdmin})
}

func (a *App) attachPermissions(roleID uint, permissionIDs []uint) error {
	if len(permissionIDs) == 0 {
		var permissions []domain.Permission
		if err := a.DB.Find(&permissions).Error; err != nil {
			return err
		}
		for _, permission := range permissions {
			permissionIDs = append(permissionIDs, permission.ID)
		}
	}
	return a.DB.Transaction(func(tx *gorm.DB) error {
		for _, permissionID := range permissionIDs {
			row := domain.RolePermission{RoleID: roleID, PermissionID: permissionID}
			if err := tx.FirstOrCreate(&row, domain.RolePermission{RoleID: roleID, PermissionID: permissionID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *App) attachPermissionCodes(roleID uint, codes []string) error {
	var permissions []domain.Permission
	if err := a.DB.Where("code IN ?", codes).Find(&permissions).Error; err != nil {
		return err
	}
	var ids []uint
	for _, permission := range permissions {
		ids = append(ids, permission.ID)
	}
	return a.attachPermissions(roleID, ids)
}

func (a *App) assignRoleCodes(userID uint, codes []string) error {
	var roles []domain.Role
	if err := a.DB.Where("code IN ?", codes).Find(&roles).Error; err != nil {
		return err
	}
	return a.DB.Transaction(func(tx *gorm.DB) error {
		for _, role := range roles {
			row := domain.UserRole{UserID: userID, RoleID: role.ID}
			if err := tx.FirstOrCreate(&row, domain.UserRole{UserID: userID, RoleID: role.ID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *App) reloadPolicies() error {
	a.Enforcer.ClearPolicy()

	var policies []struct {
		Username string
		RoleCode string
	}
	if err := a.DB.Table("user_roles").
		Select("users.username, roles.code AS role_code").
		Joins("JOIN users ON users.id = user_roles.user_id").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Scan(&policies).Error; err != nil {
		return err
	}
	for _, policy := range policies {
		if _, err := a.Enforcer.AddGroupingPolicy(policy.Username, policy.RoleCode); err != nil {
			return err
		}
	}

	var permissions []struct {
		RoleCode string
		Object   string
		Action   string
	}
	if err := a.DB.Table("role_permissions").
		Select("roles.code AS role_code, permissions.object, permissions.action").
		Joins("JOIN roles ON roles.id = role_permissions.role_id").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Scan(&permissions).Error; err != nil {
		return err
	}
	for _, permission := range permissions {
		if _, err := a.Enforcer.AddPolicy(permission.RoleCode, permission.Object, permission.Action, "*", "*"); err != nil {
			return err
		}
	}
	return nil
}

func defaultPermissions() []domain.Permission {
	defs := []struct {
		name   string
		code   string
		object string
		action string
	}{
		{"组织查看", "system:organizations:read", "/api/v1/system/organizations", "read"},
		{"组织维护", "system:organizations:write", "/api/v1/system/organizations", "write"},
		{"部门查看", "system:departments:read", "/api/v1/system/departments", "read"},
		{"部门维护", "system:departments:write", "/api/v1/system/departments", "write"},
		{"终端查看", "system:terminals:read", "/api/v1/system/terminals", "read"},
		{"终端维护", "system:terminals:write", "/api/v1/system/terminals", "write"},
		{"用户查看", "system:users:read", "/api/v1/system/users", "read"},
		{"用户维护", "system:users:write", "/api/v1/system/users", "write"},
		{"角色查看", "system:roles:read", "/api/v1/system/roles", "read"},
		{"角色维护", "system:roles:write", "/api/v1/system/roles", "write"},
		{"权限查看", "system:permissions:read", "/api/v1/system/permissions", "read"},
		{"审计查看", "system:audits:read", "/api/v1/system/audits", "read"},
		{"客户查看", "customers:read", "/api/v1/customers", "read"},
		{"客户维护", "customers:write", "/api/v1/customers", "write"},
		{"库存查看", "inventory:read", "/api/v1/inventory", "read"},
		{"库存维护", "inventory:write", "/api/v1/inventory", "write"},
		{"物料查看", "materials:read", "/api/v1/materials", "read"},
		{"物料维护", "materials:write", "/api/v1/materials", "write"},
		{"模具查看", "molds:read", "/api/v1/molds", "read"},
		{"模具维护", "molds:write", "/api/v1/molds", "write"},
		{"任务查看", "tasks:read", "/api/v1/tasks", "read"},
		{"任务维护", "tasks:write", "/api/v1/tasks", "write"},
		{"报表查看", "reports:read", "/api/v1/reports", "read"},
		{"报表维护", "reports:write", "/api/v1/reports", "write"},
	}
	items := make([]domain.Permission, 0, len(defs))
	for _, def := range defs {
		items = append(items, domain.Permission{Name: def.name, Code: def.code, Object: def.object, Action: def.action})
	}
	return items
}

func (a *App) roleCodesForUser(userID uint) ([]string, error) {
	var roles []string
	err := a.DB.Table("user_roles").
		Select("roles.code").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ?", userID).
		Scan(&roles).Error
	return roles, err
}

func (a *App) permissionCodesForUser(userID uint) ([]string, error) {
	var codes []string
	err := a.DB.Table("user_roles").
		Select("DISTINCT permissions.code").
		Joins("JOIN role_permissions ON role_permissions.role_id = user_roles.role_id").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Where("user_roles.user_id = ?", userID).
		Order("permissions.code").
		Scan(&codes).Error
	return codes, err
}

func (a *App) includesSystemRole(roleIDs []uint) bool {
	var count int64
	a.DB.Model(&domain.Role{}).Where("id IN ? AND code = ?", roleIDs, roleSuperAdmin).Count(&count)
	return count > 0
}

func (a *App) canAccessOrg(c *echo.Context, orgID uint) bool {
	current := currentUser(c)
	return current == nil || current.OrganizationID == orgID
}

func (a *App) canAccessDepartment(c *echo.Context, departmentID uint) bool {
	current := currentUser(c)
	if current == nil {
		return true
	}
	var dept domain.Department
	if err := a.DB.First(&dept, departmentID).Error; err != nil {
		return false
	}
	return dept.OrganizationID == current.OrganizationID
}

func bindAndValidate(c *echo.Context, dst any) error {
	if err := c.Bind(dst); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请求 JSON 格式错误")
	}
	if err := c.Validate(dst); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请求参数校验失败")
	}
	return nil
}

func currentUser(c *echo.Context) *CurrentUser {
	value := c.Get(contextUserKey)
	current, _ := value.(*CurrentUser)
	return current
}

func currentUserResponse(current *CurrentUser) map[string]any {
	return map[string]any{
		"id":              current.ID,
		"username":        current.Username,
		"account_type":    current.AccountType,
		"name":            current.Name,
		"organization_id": current.OrganizationID,
		"department_id":   current.DepartmentID,
		"terminal_id":     current.TerminalID,
		"roles":           current.Roles,
		"permissions":     current.Permissions,
	}
}

func paramID(c *echo.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "无效 ID")
	}
	return uint(id), nil
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(bytes), nil
}

func responseStatus(c *echo.Context) int {
	if res, ok := c.Response().(*echo.Response); ok && res.Status != 0 {
		return res.Status
	}
	return http.StatusOK
}

func responseCommitted(c *echo.Context) bool {
	if res, ok := c.Response().(*echo.Response); ok {
		return res.Committed
	}
	return false
}
