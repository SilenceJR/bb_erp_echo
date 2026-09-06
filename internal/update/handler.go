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

// VersionResponse contains only the server identity and version. Client
// update discovery is intentionally separate so the version endpoint remains
// cheap and does not disclose or imply a configured client package.
type VersionResponse struct {
	AppName       string `json:"app_name"`
	ServerVersion string `json:"server_version"`
}

// Handler serves the anonymous server version and portable client update API.
type Handler struct {
	Config  *config.Config
	Service UpdateService
}

// NewHandler creates a handler with the fixed-directory service.
func NewHandler(cfg *config.Config) *Handler {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return NewHandlerWithService(cfg, NewService(cfg.Update, cfg.App.Version))
}

// NewHandlerWithService injects a service for the application and tests.
func NewHandlerWithService(cfg *config.Config, service UpdateService) *Handler {
	return &Handler{Config: cfg, Service: service}
}

// RegisterPublicRoutes registers endpoints used before login as well as by the
// shared Web/Tauri client.
func (h *Handler) RegisterPublicRoutes(v1 *echo.Group) {
	v1.GET("/version", h.Version)
	updates := v1.Group("/client-updates")
	updates.GET("/check", h.CheckClientUpdate)
	updates.GET("/artifacts/:sha256", h.DownloadClientArtifact)
}

// Version returns only the server application name and version.
//
// @Summary 查询服务端版本
// @Tags updates
// @Produce json
// @Success 200 {object} VersionResponse
// @Router /api/v1/version [get]
func (h *Handler) Version(c *echo.Context) error {
	if h.Config == nil {
		return c.JSON(http.StatusOK, VersionResponse{})
	}
	return c.JSON(http.StatusOK, VersionResponse{
		AppName:       h.Config.App.Name,
		ServerVersion: h.Config.App.Version,
	})
}

// CheckClientUpdate refreshes the fixed server-side client directory on every
// request. It returns 204 when no newer client is available. A missing package
// also returns 204 with a diagnostic header; malformed/invalid packages return
// 503 without exposing filesystem details.
//
// @Summary 检查内网客户端更新
// @Description 从服务端 ../client 目录读取并验证单文件 portable 客户端。
// @Tags updates
// @Produce json
// @Param current_version query string true "当前客户端 SemVer"
// @Success 200 {object} ClientUpdatePlan
// @Success 204
// @Failure 400 {object} map[string]any
// @Failure 503 {object} map[string]any
// @Router /api/v1/client-updates/check [get]
func (h *Handler) CheckClientUpdate(c *echo.Context) error {
	if h.Service == nil {
		return c.NoContent(http.StatusNoContent)
	}
	plan, available, err := h.Service.CheckClientUpdate(c.Request().Context(), c.QueryParam("current_version"))
	if err != nil {
		if errors.Is(err, ErrInvalidClientVersion) {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if errors.Is(err, ErrClientUpdateNotPublished) {
			c.Response().Header().Set("X-Client-Update-Status", "not_published")
			return c.NoContent(http.StatusNoContent)
		}
		return echo.NewHTTPError(http.StatusServiceUnavailable, "客户端更新暂不可用")
	}
	if !available {
		c.Response().Header().Set("X-Client-Update-Status", "up_to_date")
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, plan)
}

// DownloadClientArtifact distributes only the currently committed verified
// content-addressed executable. http.ServeContent supplies Range and ETag
// compatible behavior while the service controls the active digest allowlist.
//
// @Summary 下载已验签客户端更新
// @Tags updates
// @Produce application/octet-stream
// @Param sha256 path string true "已验签客户端 SHA-256"
// @Success 200 {file} binary
// @Success 206 {file} binary
// @Failure 404 {object} map[string]any
// @Router /api/v1/client-updates/artifacts/{sha256} [get]
func (h *Handler) DownloadClientArtifact(c *echo.Context) error {
	if h.Service == nil {
		return echo.NewHTTPError(http.StatusNotFound, "客户端更新资源不存在")
	}
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
	if err != nil || !info.Mode().IsRegular() || info.Size() != artifact.Size {
		return echo.NewHTTPError(http.StatusNotFound, "客户端更新资源不存在")
	}
	response := c.Response()
	response.Header().Set("ETag", `"`+strings.ToLower(artifact.SHA256)+`"`)
	response.Header().Set(echo.HeaderContentType, "application/octet-stream")
	response.Header().Set(echo.HeaderContentDisposition, mime.FormatMediaType("attachment", map[string]string{"filename": clientArtifactFileName}))
	http.ServeContent(response, c.Request(), clientArtifactFileName, info.ModTime(), file)
	return nil
}
