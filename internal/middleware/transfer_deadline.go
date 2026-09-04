package middleware

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

// TransferDeadline 为已经通过业务权限校验的大文件路由延长底层连接读写期限。
// 结束时清除连接 deadline，避免影响 HTTP keep-alive 上的后续普通请求。
func TransferDeadline(timeout time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			controller := http.NewResponseController(c.Response())
			deadline := time.Now().Add(timeout)
			if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return echo.NewHTTPError(http.StatusInternalServerError, "无法设置文件传输读取时限")
			}
			if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return echo.NewHTTPError(http.StatusInternalServerError, "无法设置文件传输写入时限")
			}
			defer func() {
				_ = controller.SetReadDeadline(time.Time{})
				_ = controller.SetWriteDeadline(time.Time{})
			}()
			return next(c)
		}
	}
}
