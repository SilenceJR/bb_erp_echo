package auth

import (
	"errors"
	"net/http"
	"time"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

// LoginRequest 是账号密码登录请求体。
//
// 参数说明：
// - Username：登录账号，必填。
// - Password：登录密码，必填。
type LoginRequest struct {
	// Username 是登录账号。
	Username string `json:"username" validate:"required" example:"admin"`
	// Password 是登录密码。
	Password string `json:"password" validate:"required" example:"admin123456"`
}

// RefreshRequest 是刷新登录会话的请求体。
type RefreshRequest struct {
	// RefreshToken 是登录或上次续期返回的 refresh token。
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// ChangePasswordRequest 是当前登录用户修改密码的请求体。
type ChangePasswordRequest struct {
	// CurrentPassword 是当前密码。
	CurrentPassword string `json:"current_password" validate:"required" example:"admin123456"`
	// NewPassword 是待设置的新密码，至少 8 个字符且不超过 bcrypt 支持的长度。
	NewPassword string `json:"new_password" validate:"required" example:"newAdmin123456"`
}

// CurrentUserDTO 是当前登录身份响应结构。
//
// 参数说明：
// - AccountType：账号类型，personal 表示个人账号，department_terminal 表示部门终端账号。
// - OrganizationID：所属组织 ID。
// - DepartmentID：所属部门 ID，个人账号可为空。
// - TerminalID：所属终端 ID，部门终端账号必填。
// - Roles：角色编码列表。
// - Permissions：权限编码列表。
type CurrentUserDTO struct {
	// ID 是用户 ID。
	ID uint `json:"id" example:"1"`
	// Username 是登录账号。
	Username string `json:"username" example:"admin"`
	// AccountType 是账号类型。
	AccountType string `json:"account_type" example:"personal"`
	// Name 是账号显示名称。
	Name string `json:"name" example:"系统管理员"`
	// OrganizationID 是所属组织 ID。
	OrganizationID uint `json:"organization_id" example:"1"`
	// DepartmentID 是所属部门 ID。
	DepartmentID *uint `json:"department_id"`
	// TerminalID 是所属终端 ID。
	TerminalID *uint `json:"terminal_id"`
	// Roles 是当前账号拥有的角色编码列表。
	Roles []string `json:"roles"`
	// Permissions 是当前账号拥有的权限编码列表。
	Permissions []string `json:"permissions"`
}

// LoginResponse 是登录成功响应结构。
//
// 参数说明：
// - AccessToken：JWT 访问令牌。
// - ExpiresAt：令牌过期时间。
// - RefreshToken：用于自动续期的轮换令牌。
// - RefreshExpiresAt：本次 refresh token 的滚动失效时间。
// - User：当前登录身份快照。
type LoginResponse struct {
	// AccessToken 是 JWT 访问令牌。
	AccessToken string `json:"access_token"`
	// ExpiresAt 是令牌过期时间。
	ExpiresAt time.Time `json:"expires_at"`
	// RefreshToken 是用于静默续期的轮换令牌。
	RefreshToken string `json:"refresh_token"`
	// RefreshExpiresAt 是本次 refresh token 的滚动失效时间。
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	// User 是当前登录身份快照。
	User CurrentUserDTO `json:"user"`
}

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
	group.POST("/refresh", h.Refresh)
	group.POST("/logout", h.Logout)
	group.GET("/me", h.Me, jwtMiddleware)
	group.POST("/change-password", h.ChangePassword, jwtMiddleware)
}

// Login 处理账号密码登录并签发 JWT。
//
// 请求参数：
// - username：登录账号。
// - password：登录密码。
//
// @Summary 账号登录
// @Description 使用账号密码登录并签发 JWT；后续接口通过 Authorization: Bearer <token> 认证。
// @Tags 登录认证
// @Accept json
// @Produce json
// @Param body body LoginRequest true "登录参数"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/auth/login [post]
func (h *Handler) Login(c *echo.Context) error {
	var req LoginRequest
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

	pair, err := h.Service.IssueTokenPair(user)
	if err != nil {
		return err
	}

	current, err := h.Service.CurrentUserFromModel(user)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, LoginResponse{
		AccessToken:      pair.AccessToken,
		ExpiresAt:        pair.ExpiresAt,
		RefreshToken:     pair.RefreshToken,
		RefreshExpiresAt: pair.RefreshExpiresAt,
		User:             CurrentUserResponse(current),
	})
}

// Refresh 使用 refresh token 轮换登录会话并签发新的 JWT。
//
// @Summary 刷新登录会话
// @Description 轮换 refresh token 并签发新的 JWT；旧 refresh token 成功使用后立即失效。
// @Tags 登录认证
// @Accept json
// @Produce json
// @Param body body RefreshRequest true "刷新参数"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/auth/refresh [post]
func (h *Handler) Refresh(c *echo.Context) error {
	var req RefreshRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}

	user, pair, err := h.Service.RotateRefreshToken(req.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) {
			return echo.NewHTTPError(http.StatusUnauthorized, "刷新令牌无效或已过期")
		}
		if errors.Is(err, ErrRefreshAccountDisabled) {
			return echo.NewHTTPError(http.StatusForbidden, "账号已停用")
		}
		return err
	}
	current, err := h.Service.CurrentUserFromModel(user)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, LoginResponse{
		AccessToken:      pair.AccessToken,
		ExpiresAt:        pair.ExpiresAt,
		RefreshToken:     pair.RefreshToken,
		RefreshExpiresAt: pair.RefreshExpiresAt,
		User:             CurrentUserResponse(current),
	})
}

// Logout 撤销 refresh token；access token 会在其自身过期前继续保持服务端可验证，
// 客户端在退出时会立即清理本地会话。
//
// @Summary 退出登录
// @Description 撤销当前 refresh token，客户端应同时清理本地 access token。
// @Tags 登录认证
// @Accept json
// @Produce json
// @Param body body RefreshRequest true "退出参数"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/auth/logout [post]
func (h *Handler) Logout(c *echo.Context) error {
	var req RefreshRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if err := h.Service.RevokeRefreshToken(req.RefreshToken); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// Me 返回当前登录用户信息。
//
// @Summary 当前用户
// @Description 返回当前登录账号的账号类型、组织、部门、终端、角色和权限列表。
// @Tags 登录认证
// @Security BearerAuth
// @Produce json
// @Success 200 {object} CurrentUserDTO
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/auth/me [get]
func (h *Handler) Me(c *echo.Context) error {
	current := GetCurrentUser(c)
	if current == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	return c.JSON(http.StatusOK, CurrentUserResponse(current))
}

// ChangePassword 修改当前登录用户的密码，并立即递增密码版本使旧 JWT 失效。
//
// @Summary 修改当前用户密码
// @Description 校验当前密码后修改当前登录账号的密码；成功后原 JWT 立即失效，需要重新登录。
// @Tags 登录认证
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body ChangePasswordRequest true "密码修改参数"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/auth/change-password [post]
func (h *Handler) ChangePassword(c *echo.Context) error {
	current := GetCurrentUser(c)
	if current == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}

	var req ChangePasswordRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if err := ValidatePassword(req.NewPassword); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "新密码长度必须至少为 8 个字符且不超过 bcrypt 支持的 72 字节")
	}

	db := h.DB.WithContext(c.Request().Context())
	err := db.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.First(&user, current.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return echo.NewHTTPError(http.StatusUnauthorized, "账号不存在")
			}
			return err
		}
		if user.Status != model.StatusActive {
			return echo.NewHTTPError(http.StatusForbidden, "账号已停用")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "当前密码错误")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.NewPassword)); err == nil {
			return echo.NewHTTPError(http.StatusBadRequest, "新密码不能与当前密码相同")
		}

		hash, err := HashPassword(req.NewPassword)
		if err != nil {
			return err
		}

		currentVersion := user.PasswordVersion
		nextVersion := NormalizePasswordVersion(currentVersion) + 1
		result := tx.Model(&model.User{}).
			Where("id = ? AND password_version = ?", user.ID, currentVersion).
			Updates(map[string]any{
				"password_hash":    hash,
				"password_version": nextVersion,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return echo.NewHTTPError(http.StatusConflict, "密码已发生变化，请重新登录后重试")
		}
		return h.Service.RevokeRefreshTokensForUser(tx, user.ID, time.Now())
	})
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
