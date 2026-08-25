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
	Manager *Manager
}

// NewHandler 创建版本更新处理器。
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{Config: cfg, Manager: NewManager(cfg.Update)}
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
	system.POST("/updates/check", h.CheckRemoteUpdates, require("/api/v1/system/updates", "write"))
}

// Version 返回当前服务端、客户端版本和已缓存客户端升级包状态。
func (h *Handler) Version(c *echo.Context) error {
	return c.JSON(http.StatusOK, VersionResponse{
		AppName:       h.Config.App.Name,
		ServerVersion: h.Config.App.Version,
		ClientVersion: h.Config.Update.ClientVersion,
		UpdateEnabled: h.Config.Update.Enabled,
		ClientUpdate:  h.Manager.CachedClientStatus(),
	})
}

// ClientStatus 返回服务端已缓存的客户端升级包状态。
func (h *Handler) ClientStatus(c *echo.Context) error {
	return c.JSON(http.StatusOK, h.Manager.CachedClientStatus())
}

// DownloadClientPackage 把服务端缓存的客户端升级包分发给员工电脑。
func (h *Handler) DownloadClientPackage(c *echo.Context) error {
	path := h.Manager.CachedClientPackagePath()
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return echo.NewHTTPError(http.StatusNotFound, "暂无可下载的客户端升级包")
	}
	return c.Attachment(path, clientPackageName)
}

// CheckRemoteUpdates 由管理员触发服务端检查 GitHub、Gitee 或内网 manifest。
func (h *Handler) CheckRemoteUpdates(c *echo.Context) error {
	status, err := h.Manager.CheckAndCacheClientUpdate()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.JSON(http.StatusOK, status)
}
