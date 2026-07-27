// Package customer 注册客户与联系人模块骨架。
package customer

import (
	"net/http"

	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
)

// RegisterRoutes 注册客户模块占位路由。
func RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	registerSkeleton(v1, "customers", "客户与联系人", require, audit)
}

func registerSkeleton(v1 *echo.Group, path string, name string, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/"+path, audit)
	group.GET("", func(c *echo.Context) error {
		return response.Skeleton(c, http.StatusOK, path, name, "模块骨架已注册，业务接口待后续迭代。")
	}, require("/api/v1/"+path, "read"))
	group.POST("", func(c *echo.Context) error {
		return response.Skeleton(c, http.StatusAccepted, path, name, "模块写入入口已预留，业务流程待后续迭代。")
	}, require("/api/v1/"+path, "write"))
}
