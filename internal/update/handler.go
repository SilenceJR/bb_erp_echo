package update

import (
	"net/http"
	"os"

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
}

// RegisterSystemRoutes 注册管理员触发的远端检查接口。
func (h *Handler) RegisterSystemRoutes(system *echo.Group, require func(string, string) echo.MiddlewareFunc) {
	system.GET("/updates/status", h.SystemStatus, require("/api/v1/system/updates", "read"))
	system.POST("/updates/check", h.CheckRemoteUpdates, require("/api/v1/system/updates", "write"))
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
