package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "bb_erp_echo/docs"
	"bb_erp_echo/internal/audit"
	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/config"
	"bb_erp_echo/internal/contact"
	"bb_erp_echo/internal/customer"
	"bb_erp_echo/internal/database"
	"bb_erp_echo/internal/department"
	filemodule "bb_erp_echo/internal/file"
	"bb_erp_echo/internal/frontend"
	"bb_erp_echo/internal/inventory"
	erplogger "bb_erp_echo/internal/logger"
	"bb_erp_echo/internal/material"
	erpmiddleware "bb_erp_echo/internal/middleware"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/mold"
	"bb_erp_echo/internal/product"
	"bb_erp_echo/internal/role"
	"bb_erp_echo/internal/shared/response"
	"bb_erp_echo/internal/statistics"
	"bb_erp_echo/internal/supplier"
	"bb_erp_echo/internal/update"
	"bb_erp_echo/internal/user"
	"bb_erp_echo/internal/warehouse"
	"bb_erp_echo/internal/workorder"

	"github.com/casbin/casbin/v2"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
	"gorm.io/gorm"
)

// App 是 ERP 后台应用容器。
//
// 字段说明：
// - Config：运行配置，来自 Koanf 和环境变量。
// - Logger：slog 结构化日志。
// - DB：GORM 数据库连接。
// - Echo：HTTP 路由和中间件实例。
// - Server：标准库 HTTP Server，用于优雅关闭。
// - Enforcer：Casbin 权限引擎。
// - AuthService：登录、JWT 和当前用户组装服务。
// - RoleService：角色、权限和策略重载服务。
// - LogSystem：文件化日志系统，负责关闭文件句柄。
type App struct {
	Config        *config.Config
	Logger        *slog.Logger
	AccessLogger  *slog.Logger
	ErrorLogger   *slog.Logger
	DB            *gorm.DB
	Echo          *echo.Echo
	Server        *http.Server
	Enforcer      *casbin.Enforcer
	AuthService   *auth.Service
	RoleService   *role.Service
	UpdateService update.UpdateService
	LogSystem     *erplogger.System
}

// Validator 是 Echo 请求校验适配器。
type Validator struct {
	validate *validator.Validate
}

// HealthResponse 是健康检查响应。
//
// 参数说明：
// - Status：服务存活状态。
// - Time：服务端当前时间。
type HealthResponse struct {
	// Status 是服务存活状态。
	Status string `json:"status" example:"ok"`
	// Time 是服务端当前时间。
	Time time.Time `json:"time"`
}

// ReadyResponse 是就绪检查响应。
//
// 参数说明：
// - Status：ready 表示服务依赖可用，not_ready 表示依赖不可用。
// - Message：不可用时的说明。
type ReadyResponse struct {
	// Status 是服务就绪状态。
	Status string `json:"status" example:"ready"`
	// Message 是不可用时的说明。
	Message string `json:"message,omitempty" example:"database ping failed"`
}

// Validate 执行结构体 tag 校验。
//
// 参数说明：
// - i：需要校验的请求结构体。
//
// 返回说明：校验通过返回 nil，否则返回 validator 错误。
func (v *Validator) Validate(i any) error {
	return v.validate.Struct(i)
}

// New 创建完整 ERP 应用。
//
// 初始化顺序：
// 1. 加载配置。
// 2. 初始化日志、数据库和 GORM 自动迁移。
// 3. 初始化认证服务、角色权限服务和 Casbin 权限引擎。
// 4. 装配 Echo 中间件和路由。
// 5. 写入默认组织、角色、权限和管理员。
//
// 参数说明：无。
// 返回说明：返回可启动的 App；任一初始化步骤失败时返回错误。
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	logSystem, err := erplogger.New(cfg.Log)
	if err != nil {
		return nil, err
	}

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		return nil, fmt.Errorf("auto migrate database: %w", err)
	}

	enforcer, err := role.NewEnforcer()
	if err != nil {
		return nil, err
	}

	authService := auth.NewService(cfg, db)
	roleService := role.NewService(db, enforcer)
	updateService := update.NewService(cfg.Update, cfg.App.Version)

	app := &App{
		Config:        cfg,
		Logger:        logSystem.App,
		AccessLogger:  logSystem.Access,
		ErrorLogger:   logSystem.Error,
		DB:            db,
		Echo:          echo.New(),
		Enforcer:      enforcer,
		AuthService:   authService,
		RoleService:   roleService,
		UpdateService: updateService,
		LogSystem:     logSystem,
	}

	app.configureEcho()

	if err := roleService.SeedSystemData(cfg); err != nil {
		return nil, err
	}
	if err := roleService.ReloadPolicies(); err != nil {
		return nil, err
	}

	if err := app.registerRoutes(); err != nil {
		return nil, err
	}

	return app, nil
}

// configureEcho 注册全局中间件和统一错误处理。
//
// 中间件顺序说明：
// RequestID -> Recover -> Secure -> BodyLimit -> CORS -> 请求日志。
// 登录认证、权限和审计在具体路由组中按业务需要挂载。
func (a *App) configureEcho() {
	a.Echo.Validator = &Validator{validate: validator.New()}

	a.Echo.Use(echomiddleware.RequestID())
	a.Echo.Use(echomiddleware.Recover())
	a.Echo.Use(echomiddleware.Secure())
	a.Echo.Use(echomiddleware.BodyLimit(25 * 1024 * 1024))
	a.Echo.Use(echomiddleware.CORSWithConfig(echomiddleware.CORSConfig{
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
	a.Echo.Use(erpmiddleware.RequestLogger(a.AccessLogger))
	a.Echo.HTTPErrorHandler = response.ErrorHandler(a.ErrorLogger)
}

// registerRoutes 注册健康检查、认证、系统管理和业务骨架路由。
//
// 参数说明：无，直接修改 a.Echo 的路由表。
func (a *App) registerRoutes() error {
	a.Echo.GET("/health", a.health)
	a.Echo.GET("/ready", a.ready)

	v1 := a.Echo.Group("/api/v1")
	jwtMiddleware := erpmiddleware.JWT(a.AuthService, a.DB)
	auditMiddleware := erpmiddleware.Audit(a.DB, a.ErrorLogger)
	require := func(object string, action string) echo.MiddlewareFunc {
		return erpmiddleware.RequirePermission(a.Enforcer, object, action)
	}

	auth.NewHandler(a.DB, a.AuthService).RegisterRoutes(v1, jwtMiddleware)
	updateHandler := update.NewHandlerWithService(a.Config, a.UpdateService)
	updateHandler.RegisterPublicRoutes(v1)

	protected := v1.Group("", jwtMiddleware)

	system := protected.Group("/system", auditMiddleware)
	department.NewHandler(a.DB).RegisterRoutes(system, require)
	user.NewHandler(a.DB, a.RoleService).RegisterRoutes(system, require)
	role.NewHandler(a.DB, a.RoleService).RegisterRoutes(system, require)
	audit.NewHandler(a.DB).RegisterRoutes(system, require)
	updateHandler.RegisterSystemRoutes(system, require)

	customer.NewHandler(a.DB).RegisterRoutes(protected, require, auditMiddleware)
	supplier.NewHandler(a.DB).RegisterRoutes(protected, require, auditMiddleware)
	contact.NewHandler(a.DB).RegisterRoutes(protected, require, auditMiddleware)
	warehouse.NewHandler(a.DB).RegisterRoutes(protected, require, auditMiddleware)
	inventory.NewHandler(a.DB).RegisterRoutes(protected, require, auditMiddleware)
	material.NewHandler(a.DB).RegisterRoutes(protected, require, auditMiddleware)
	product.NewHandler(a.DB).RegisterRoutes(protected, require, auditMiddleware)
	mold.NewHandler(a.DB).RegisterRoutes(protected, require, auditMiddleware)
	workorder.RegisterRoutes(protected, a.DB, require, auditMiddleware)
	imageService := filemodule.NewService(a.Config.Files.RootDir, a.DB)
	if err := imageService.EnsureRoot(); err != nil {
		return fmt.Errorf("create image upload root: %w", err)
	}
	filemodule.NewHandler(imageService, a.DB, a.Enforcer).RegisterRoutes(protected, auditMiddleware)
	statistics.RegisterRoutes(protected, a.DB, require, auditMiddleware)

	// Swagger API 文档路由必须放在 Web 静态文件兜底之前，避免被前端路由覆盖。
	a.Echo.GET("/swagger/*", echo.WrapHandler(httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	)))

	// Web 静态文件必须最后注册，避免覆盖 API、健康检查等后端路由。
	frontend.RegisterStatic(a.Echo, a.Config.Web)
	return nil
}

// Start 启动 HTTP 服务。
//
// 参数说明：无，监听地址来自 Config.HTTP。
// 返回说明：正常关闭返回 nil，启动失败返回错误。
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

// Run 启动服务并监听系统中断信号。
//
// 参数说明：无。
// 返回说明：服务启动失败或优雅关闭失败时返回错误。
func (a *App) Run() error {
	updateContext, cancelUpdates := context.WithCancel(context.Background())
	defer cancelUpdates()
	if a.UpdateService != nil {
		a.UpdateService.Start(updateContext)
	}
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

// Shutdown 优雅关闭 HTTP 服务和数据库连接。
//
// 参数说明：
// - ctx：关闭超时控制上下文。
//
// 返回说明：HTTP 服务或数据库关闭失败时返回错误。
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
	if a.LogSystem != nil {
		if err := a.LogSystem.Close(); err != nil {
			return fmt.Errorf("close log files: %w", err)
		}
	}

	return nil
}

// health 返回进程存活状态，不依赖数据库。
//
// @Summary 健康检查
// @Description 返回进程存活状态，不依赖数据库。
// @Tags 公共接口
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /health [get]
func (a *App) health(c *echo.Context) error {
	return c.JSON(http.StatusOK, HealthResponse{Status: "ok", Time: time.Now()})
}

// ready 返回服务就绪状态，会检查数据库连接是否可用。
//
// @Summary 就绪检查
// @Description 检查数据库连接是否可用，用于服务启动和部署探活。
// @Tags 公共接口
// @Produce json
// @Success 200 {object} ReadyResponse
// @Failure 503 {object} ReadyResponse
// @Router /ready [get]
func (a *App) ready(c *echo.Context) error {
	sqlDB, err := a.DB.DB()
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, ReadyResponse{Status: "not_ready", Message: "database unavailable"})
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, ReadyResponse{Status: "not_ready", Message: "database ping failed"})
	}

	return c.JSON(http.StatusOK, ReadyResponse{Status: "ready"})
}
