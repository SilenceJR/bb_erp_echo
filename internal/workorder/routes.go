// Package workorder 注册任务单与部门子任务模块骨架。
package workorder

import (
	"net/http"

	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
)

// RegisterRoutes 注册任务单模块占位路由。
func RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	register(v1, "workorder", "任务单与部门子任务", require, audit)

	// 兼容第一版 /api/v1/tasks 路径，后续前端稳定后可统一收敛到 workorder。
	register(v1, "tasks", "任务单与部门子任务", require, audit)
}

func register(v1 *echo.Group, path string, name string, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/"+path, audit)
	group.GET("", func(c *echo.Context) error {
		return response.Skeleton(c, http.StatusOK, path, name, "模块骨架已注册，业务接口待后续迭代。")
	}, require("/api/v1/"+path, "read"))
	group.POST("", func(c *echo.Context) error {
		return response.Skeleton(c, http.StatusAccepted, path, name, "模块写入入口已预留，业务流程待后续迭代。")
	}, require("/api/v1/"+path, "write"))
}
