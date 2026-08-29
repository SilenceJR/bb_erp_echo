package update

import (
	"errors"
	"mime"
	"net/http"
	"os"
	"strings"

	"bb_erp_echo/internal/config"

	"github.com/labstack/echo/v5"
)

// VersionResponse 是服务端和客户端版本信息。
type VersionResponse struct {
	AppName       string             `json:"app_name"`
	ServerVersion string             `json:"server_version"`
	ClientVersion string             `json:"client_version"`
	UpdateEnabled bool               `json:"update_enabled"`
	ClientUpdate  ClientUpdateStatus `json:"client_update"`
}

// Handler 处理版本检查和客户端升级包分发。
type Handler struct {
	Config  *config.Config
	Service UpdateService
}

// NewHandler 创建版本更新处理器。
func NewHandler(cfg *config.Config) *Handler {
	return NewHandlerWithService(cfg, NewService(cfg.Update, cfg.App.Version))
}

// NewHandlerWithService 使用已装配的更新服务创建处理器，确保路由与调度共享状态。
func NewHandlerWithService(cfg *config.Config, service UpdateService) *Handler {
	return &Handler{Config: cfg, Service: service}
}

// RegisterPublicRoutes 注册客户端启动阶段也可访问的版本接口。
func (h *Handler) RegisterPublicRoutes(v1 *echo.Group) {
	v1.GET("/version", h.Version)
	updates := v1.Group("/updates/client")
	updates.GET("/status", h.ClientStatus)
	updates.GET("/download", h.DownloadClientPackage)
	updates.GET("/plan", h.ClientPlan)
	updates.GET("/tauri/:target/:arch/:current_version", h.TauriClientUpdate)
	updates.GET("/artifacts/:sha256", h.DownloadClientArtifact)
}

// RegisterSystemRoutes 注册管理员触发的远端检查接口。
func (h *Handler) RegisterSystemRoutes(system *echo.Group, require func(string, string) echo.MiddlewareFunc) {
	system.GET("/updates/status", h.SystemStatus, require("/api/v1/system/updates", "read"))
	system.POST("/updates/check", h.CheckRemoteUpdates, require("/api/v1/system/updates", "write"))
	system.GET("/updates/server/download", h.DownloadServerPackage, require("/api/v1/system/updates", "read"))
}

// Version 返回当前服务端、客户端版本和已缓存客户端升级包状态。
// @Summary 查询服务端与客户端版本
// @Tags updates
// @Produce json
// @Success 200 {object} VersionResponse
// @Router /api/v1/version [get]
func (h *Handler) Version(c *echo.Context) error {
	return c.JSON(http.StatusOK, VersionResponse{
		AppName:       h.Config.App.Name,
		ServerVersion: h.Config.App.Version,
		ClientVersion: h.Config.Update.ClientVersion,
		UpdateEnabled: h.Config.Update.Enabled,
		ClientUpdate:  h.Service.ClientStatus(h.Config.Update.ClientVersion),
	})
}

// ClientStatus 返回服务端已缓存的客户端升级包状态。
// @Summary 查询桌面客户端更新状态
// @Tags updates
// @Produce json
// @Param current_version query string false "Tauri 真实安装版本"
// @Success 200 {object} ClientUpdateStatus
// @Router /api/v1/updates/client/status [get]
func (h *Handler) ClientStatus(c *echo.Context) error {
	return c.JSON(http.StatusOK, h.Service.ClientStatus(c.QueryParam("current_version")))
}

// DownloadClientPackage 把服务端缓存的客户端升级包分发给员工电脑。
// @Summary 下载已校验缓存的桌面客户端包
// @Tags updates
// @Produce application/octet-stream
// @Success 200 {file} binary
// @Failure 404 {object} map[string]any
// @Router /api/v1/updates/client/download [get]
func (h *Handler) DownloadClientPackage(c *echo.Context) error {
	path := h.Service.CachedClientPackagePath()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return echo.NewHTTPError(http.StatusNotFound, "暂无可下载的客户端升级包")
	}
	return c.Attachment(path, clientPackageName)
}

// DownloadServerPackage 按当前成功清单下载、校验并缓存服务端升级包，再分发给管理员。
// @Summary 下载并校验服务端升级包
// @Tags system-updates
// @Produce application/octet-stream
// @Security BearerAuth
// @Success 200 {file} binary
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Failure 404 {object} map[string]any
// @Failure 502 {object} map[string]any
// @Router /api/v1/system/updates/server/download [get]
func (h *Handler) DownloadServerPackage(c *echo.Context) error {
	path, fileName, err := h.Service.ServerPackage(c.Request().Context())
	if err != nil {
		if errors.Is(err, ErrServerPackageUnavailable) {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return echo.NewHTTPError(http.StatusNotFound, "暂无可下载的服务端升级包")
	}
	file, err := os.Open(path)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "暂无可下载的服务端升级包")
	}
	defer file.Close()
	response := c.Response()
	response.Header().Set(echo.HeaderContentDisposition, mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
	response.Header().Set(echo.HeaderContentType, "application/octet-stream")
	http.ServeContent(response, c.Request(), fileName, info.ModTime(), file)
	return nil
}

// ClientPlan 返回桌面端应采用的增量或完整更新策略。
// @Summary 规划 Windows 客户端自动更新
// @Tags updates
// @Produce json
// @Param current_version query string true "当前客户端 SemVer"
// @Param current_sha256 query string true "当前客户端 EXE SHA-256"
// @Param target query string true "目标平台，固定 windows-x86_64"
// @Param install_mode query string true "安装模式：nsis 或 portable"
// @Success 200 {object} ClientUpdatePlan
// @Success 204
// @Failure 400 {object} map[string]any
// @Router /api/v1/updates/client/plan [get]
func (h *Handler) ClientPlan(c *echo.Context) error {
	plan, available, err := h.Service.ClientUpdatePlan(ClientUpdatePlanRequest{
		CurrentVersion: c.QueryParam("current_version"),
		CurrentSHA256:  c.QueryParam("current_sha256"),
		Target:         c.QueryParam("target"),
		InstallMode:    c.QueryParam("install_mode"),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !available {
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, plan)
}

// TauriClientUpdate 返回 tauri-plugin-updater 所需的完整 NSIS 更新信息。
// @Summary 查询 Tauri 完整客户端更新
// @Tags updates
// @Produce json
// @Param target path string true "Tauri target，固定 windows"
// @Param arch path string true "CPU 架构，固定 x86_64"
// @Param current_version path string true "当前客户端 SemVer"
// @Success 200 {object} TauriUpdateResponse
// @Success 204
// @Failure 400 {object} map[string]any
// @Router /api/v1/updates/client/tauri/{target}/{arch}/{current_version} [get]
func (h *Handler) TauriClientUpdate(c *echo.Context) error {
	target := strings.ToLower(strings.TrimSpace(c.Param("target"))) + "-" + strings.ToLower(strings.TrimSpace(c.Param("arch")))
	update, available, err := h.Service.TauriClientUpdate(target, c.Param("current_version"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if !available {
		return c.NoContent(http.StatusNoContent)
	}
	// tauri-plugin-updater requires an absolute artifact URL. The service keeps
	// artifact references origin-relative so the same signed manifest works on
	// every LAN deployment; bind it to the server address used by this request.
	if strings.HasPrefix(update.URL, "/") {
		scheme := "http"
		if c.Request().TLS != nil {
			scheme = "https"
		}
		update.URL = scheme + "://" + c.Request().Host + update.URL
	}
	return c.JSON(http.StatusOK, update)
}

// DownloadClientArtifact 从当前已验签 manifest 的内容寻址缓存分发资源，支持 ETag 和 Range。
// @Summary 下载已验签客户端更新资源
// @Tags updates
// @Produce application/octet-stream
// @Param sha256 path string true "当前 manifest 资源 SHA-256"
// @Success 200 {file} binary
// @Success 206 {file} binary
// @Failure 404 {object} map[string]any
// @Router /api/v1/updates/client/artifacts/{sha256} [get]
func (h *Handler) DownloadClientArtifact(c *echo.Context) error {
	path, artifact, ok := h.Service.ClientArtifact(c.Param("sha256"))
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "客户端更新资源不存在")
	}
	file, err := os.Open(path)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "客户端更新资源不存在")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return echo.NewHTTPError(http.StatusNotFound, "客户端更新资源不存在")
	}
	response := c.Response()
	response.Header().Set(http.CanonicalHeaderKey("ETag"), `"`+strings.ToLower(artifact.SHA256)+`"`)
	response.Header().Set(echo.HeaderContentType, "application/octet-stream")
	http.ServeContent(response, c.Request(), artifact.Kind, info.ModTime(), file)
	return nil
}

// CheckRemoteUpdates 由管理员触发服务端检查 GitHub、Gitee 或内网 manifest。
// @Summary 立即执行完整更新检查
// @Tags system-updates
// @Produce json
// @Security BearerAuth
// @Success 200 {object} SystemUpdateStatus
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Router /api/v1/system/updates/check [post]
func (h *Handler) CheckRemoteUpdates(c *echo.Context) error {
	status, _ := h.Service.Check(c.Request().Context())
	return c.JSON(http.StatusOK, status)
}

// SystemStatus 返回更新源连通性、版本、缓存和调度状态。
// @Summary 查询版本与更新状态
// @Tags system-updates
// @Produce json
// @Security BearerAuth
// @Success 200 {object} SystemUpdateStatus
// @Failure 401 {object} map[string]any
// @Failure 403 {object} map[string]any
// @Router /api/v1/system/updates/status [get]
func (h *Handler) SystemStatus(c *echo.Context) error {
	return c.JSON(http.StatusOK, h.Service.Status(""))
}
