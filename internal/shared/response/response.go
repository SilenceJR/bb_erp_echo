// Package response 提供统一 JSON 响应和错误响应处理。
package response

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// ErrorBody 是统一错误响应结构。
type ErrorBody struct {
	// Code 是错误码。
	Code string `json:"code"`
	// Message 是前端可展示错误信息。
	Message string `json:"message"`
	// RequestID 是请求 ID，用于关联日志。
	RequestID string `json:"request_id"`
}

// ErrorHandler 创建 Echo 统一错误处理函数。
//
// 参数说明：
// - logger：结构化日志器，用于记录错误上下文。
//
// 返回说明：返回可赋给 Echo.HTTPErrorHandler 的函数。
func ErrorHandler(logger *slog.Logger) echo.HTTPErrorHandler {
	return func(c *echo.Context, err error) {
		if ResponseCommitted(c) {
			return
		}

		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		message := "服务器内部错误"

		var httpError *echo.HTTPError
		if errors.As(err, &httpError) {
			status = httpError.Code
			code = strings.ToUpper(strings.ReplaceAll(http.StatusText(status), " ", "_"))
			if httpError.Message != "" {
				message = httpError.Message
			}
		}

		var businessError interface {
			error
			Code() string
			StatusCode() int
			PublicMessage() string
		}
		if errors.As(err, &businessError) {
			status = businessError.StatusCode()
			code = businessError.Code()
			message = businessError.PublicMessage()
		}

		requestID := c.Response().Header().Get(echo.HeaderXRequestID)
		if status >= http.StatusInternalServerError {
			logger.Error("HTTP request failed", "error", err, "request_id", requestID, "method", c.Request().Method, "path", c.Request().URL.Path, "status", status)
		} else {
			logger.Warn("HTTP request rejected", "error", err, "request_id", requestID, "method", c.Request().Method, "path", c.Request().URL.Path, "status", status)
		}

		_ = c.JSON(status, ErrorBody{Code: code, Message: message, RequestID: requestID})
	}
}

// Skeleton 返回模块骨架占位响应。
//
// 参数说明：
// - c：Echo 请求上下文。
// - statusCode：HTTP 状态码。
// - path：模块路径。
// - name：模块中文名称。
// - message：占位说明。
func Skeleton(c *echo.Context, statusCode int, path string, name string, message string) error {
	return c.JSON(statusCode, map[string]any{
		"module":  path,
		"name":    name,
		"status":  "skeleton",
		"message": message,
	})
}

// ResponseStatus 读取 Echo 响应状态码。
//
// 参数说明：
// - c：Echo 请求上下文。
func ResponseStatus(c *echo.Context) int {
	if response, err := echo.UnwrapResponse(c.Response()); err == nil && response.Status != 0 {
		return response.Status
	}
	return http.StatusOK
}

// ResponseCommitted 判断响应是否已经写出。
//
// 参数说明：
// - c：Echo 请求上下文。
func ResponseCommitted(c *echo.Context) bool {
	response, err := echo.UnwrapResponse(c.Response())
	return err == nil && response.Committed
}
