// Package request 提供 HTTP 请求绑定、校验和路径参数解析工具。
package request

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
)

// BindAndValidate 绑定 JSON 请求体并执行 validator 校验。
//
// 参数说明：
// - c：Echo 请求上下文。
// - dst：目标请求结构体指针。
func BindAndValidate(c *echo.Context, dst any) error {
	if err := c.Bind(dst); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请求 JSON 格式错误")
	}
	if err := c.Validate(dst); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请求参数校验失败")
	}
	return nil
}

// ParamID 读取并校验路径参数 id。
//
// 参数说明：
// - c：Echo 请求上下文。
func ParamID(c *echo.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "无效 ID")
	}
	return uint(id), nil
}
