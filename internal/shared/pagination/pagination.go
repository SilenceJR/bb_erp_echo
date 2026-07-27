// Package pagination 提供分页参数解析约定。
package pagination

import (
	"strconv"

	"github.com/labstack/echo/v5"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 200
)

// Query 是列表接口统一分页参数。
type Query struct {
	// Page 是页码，从 1 开始。
	Page int `json:"page"`
	// PageSize 是每页条数，最大 200。
	PageSize int `json:"page_size"`
	// Offset 是数据库查询偏移量。
	Offset int `json:"offset"`
}

// FromEcho 从请求查询参数中读取分页信息。
//
// 参数说明：
// - c：Echo 请求上下文。
//
// 返回说明：返回已经兜底和裁剪过的分页参数。
func FromEcho(c *echo.Context) Query {
	page := parsePositive(c.QueryParam("page"), defaultPage)
	pageSize := parsePositive(c.QueryParam("page_size"), defaultPageSize)
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return Query{Page: page, PageSize: pageSize, Offset: (page - 1) * pageSize}
}

func parsePositive(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
