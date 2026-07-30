package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
)

// RequestLogger 记录每次 HTTP 请求的结构化访问日志。
//
// 记录范围：跳过 GET 和 OPTIONS，只记录会改变状态的请求。
// 参数说明：
// - logger：结构化日志器。
func RequestLogger(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			startedAt := time.Now()
			err := next(c)
			method := c.Request().Method
			if method == http.MethodGet || method == http.MethodOptions {
				return err
			}

			current := auth.GetCurrentUser(c)
			attrs := []any{
				"request_id", c.Response().Header().Get(echo.HeaderXRequestID),
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
				"query", c.Request().URL.RawQuery,
				"status", response.ResponseStatus(c),
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
			logger.Info("HTTP request", attrs...)
			return err
		}
	}
}
