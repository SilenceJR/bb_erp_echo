package discovery

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// Handler 提供客户端启动阶段使用的匿名服务身份接口。
type Handler struct {
	identity IdentityResponse
}

// NewHandler 创建身份接口处理器。
func NewHandler(identity Identity) *Handler {
	return &Handler{identity: identity.Response()}
}

// RegisterRoutes 注册不需要登录的身份接口。
func (h *Handler) RegisterRoutes(v1 *echo.Group) {
	v1.GET("/discovery/identity", h.GetIdentity)
}

// GetIdentity 返回当前 ERP 服务的公开最小身份。
//
// @Summary 查询服务身份
// @Description 返回客户端验证连接所需的最小服务身份，不需要登录，不包含组织、账号、业务数据、更新地址或凭据。
// @Tags 局域网发现
// @Produce json
// @Success 200 {object} IdentityResponse
// @Header 200 {string} Cache-Control "no-store"
// @Router /api/v1/discovery/identity [get]
func (h *Handler) GetIdentity(c *echo.Context) error {
	if err := ValidateIdentityResponse(h.identity); err != nil {
		// Do not expose a malformed identity DTO even if a future caller wires
		// the handler without going through LoadOrCreate.
		return echo.NewHTTPError(http.StatusInternalServerError, "discovery identity is invalid")
	}
	// The response contains the stable service identity used by startup
	// discovery. It is cheap to revalidate and must not be served from a
	// browser, proxy, or intermediary cache after a service restart/update.
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, h.identity)
}
