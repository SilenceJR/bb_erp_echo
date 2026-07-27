// Package statistics 注册统计报表模块骨架。
package statistics

import (
	"net/http"

	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
)

// RegisterRoutes 注册统计报表模块占位路由。
func RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	path := "statistics"
	name := "统计报表"
	group := v1.Group("/"+path, audit)
	group.GET("", func(c *echo.Context) error {
		return response.Skeleton(c, http.StatusOK, path, name, "模块骨架已注册，业务接口待后续迭代。")
	}, require("/api/v1/"+path, "read"))
	group.POST("", func(c *echo.Context) error {
		return response.Skeleton(c, http.StatusAccepted, path, name, "模块写入入口已预留，业务流程待后续迭代。")
	}, require("/api/v1/"+path, "write"))
}
