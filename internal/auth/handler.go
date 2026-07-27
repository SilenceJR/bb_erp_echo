package auth

import (
	"net/http"
	"time"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Handler 处理登录认证接口。
type Handler struct {
	// DB 是用户查询和登录时间更新所需数据库连接。
	DB *gorm.DB
	// Service 是 JWT 签发和当前用户组装服务。
	Service *Service
}

// NewHandler 创建认证接口处理器。
//
// 参数说明：
// - db：GORM 数据库连接。
// - service：认证服务。
func NewHandler(db *gorm.DB, service *Service) *Handler {
	return &Handler{DB: db, Service: service}
}

// RegisterRoutes 注册认证路由。
//
// 参数说明：
// - v1：/api/v1 路由组。
// - jwtMiddleware：登录态校验中间件，用于 /auth/me。
func (h *Handler) RegisterRoutes(v1 *echo.Group, jwtMiddleware echo.MiddlewareFunc) {
	group := v1.Group("/auth")
	group.POST("/login", h.Login)
	group.GET("/me", h.Me, jwtMiddleware)
}

// Login 处理账号密码登录并签发 JWT。
//
// 请求参数：
// - username：登录账号。
// - password：登录密码。
func (h *Handler) Login(c *echo.Context) error {
	var req struct {
		// Username 是登录账号。
		Username string `json:"username" validate:"required"`
		// Password 是登录密码。
		Password string `json:"password" validate:"required"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}

	var user model.User
	if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "账号或密码错误")
	}
	if user.Status != model.StatusActive {
		return echo.NewHTTPError(http.StatusForbidden, "账号已停用")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "账号或密码错误")
	}

	now := time.Now()
	h.DB.Model(&user).Update("last_login_at", now)

	token, expiresAt, err := h.Service.IssueToken(user)
	if err != nil {
		return err
	}

	current, err := h.Service.CurrentUserFromModel(user)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"access_token": token,
		"expires_at":   expiresAt,
		"user":         CurrentUserResponse(current),
	})
}

// Me 返回当前登录用户信息。
func (h *Handler) Me(c *echo.Context) error {
	current := GetCurrentUser(c)
	if current == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	return c.JSON(http.StatusOK, CurrentUserResponse(current))
}
