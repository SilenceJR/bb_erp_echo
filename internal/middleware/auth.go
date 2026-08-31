// Package middleware 放置 Echo 全局和业务中间件。
package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/role"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// JWT 校验 Authorization Bearer 令牌并写入当前用户上下文。
//
// 参数说明：
// - service：认证服务，用于解析 JWT 配置和组装 CurrentUser。
// - db：GORM 数据库连接，用于校验用户是否仍然有效。
func JWT(service *auth.Service, db *gorm.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			header := c.Request().Header.Get(echo.HeaderAuthorization)
			tokenText := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
			if tokenText == "" || tokenText == header {
				return echo.NewHTTPError(http.StatusUnauthorized, "缺少登录令牌")
			}

			token, err := jwt.ParseWithClaims(tokenText, &auth.Claims{}, func(token *jwt.Token) (any, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method")
				}
				return []byte(service.Config.JWT.Secret), nil
			})
			if err != nil || !token.Valid {
				return echo.NewHTTPError(http.StatusUnauthorized, "登录令牌无效")
			}

			claims, ok := token.Claims.(*auth.Claims)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "登录令牌无效")
			}

			var user model.User
			if err := db.First(&user, claims.UserID).Error; err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "账号不存在")
			}
			if user.Status != model.StatusActive {
				return echo.NewHTTPError(http.StatusForbidden, "账号已停用")
			}
			if claims.PasswordVersion != user.PasswordVersion {
				return echo.NewHTTPError(http.StatusUnauthorized, "登录令牌已失效")
			}

			current, err := service.CurrentUserFromModel(user)
			if err != nil {
				return err
			}
			c.Set(auth.ContextUserKey, current)
			return next(c)
		}
	}
}

// RequirePermission 使用统一权限 provider 校验接口权限和组织/部门数据范围。
//
// 参数说明：
// - authorizer：统一权限快照 provider。
// - object：资源对象，当前使用 API 路径，例如 /api/v1/system/users。
// - action：动作，当前约定为 read 或 write。
func RequirePermission(authorizer role.Authorizer, object string, action string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			current := auth.GetCurrentUser(c)
			if current == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
			}
			org := strconv.FormatUint(uint64(current.OrganizationID), 10)
			dept := "*"
			if current.DepartmentID != nil {
				dept = strconv.FormatUint(uint64(*current.DepartmentID), 10)
			}
			allowed, err := authorizer.Enforce(current.Username, object, action, org, dept)
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
