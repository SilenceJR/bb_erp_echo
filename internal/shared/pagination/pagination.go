// Package pagination 提供分页参数解析约定。
package pagination

import (
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
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
	// Keyword 是统一模糊查询关键字，来自 q 或 keyword。
	Keyword string `json:"keyword"`
}

// Result 是列表接口统一分页响应。
type Result[T any] struct {
	Items    []T    `json:"items"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Keyword  string `json:"keyword,omitempty"`
}

// FromEcho 从请求查询参数中读取分页信息。
//
// 参数说明：
// - c：Echo 请求上下文。
//
// 返回说明：返回已经兜底和裁剪过的分页参数。
func FromEcho(c echo.Context) Query {
	page := parsePositive(c.QueryParam("page"), defaultPage)
	pageSize := parsePositive(c.QueryParam("page_size"), defaultPageSize)
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	keyword := strings.TrimSpace(c.QueryParam("q"))
	if keyword == "" {
		keyword = strings.TrimSpace(c.QueryParam("keyword"))
	}
	return Query{Page: page, PageSize: pageSize, Offset: (page - 1) * pageSize, Keyword: keyword}
}

// ApplyKeyword 在指定字段上追加 LIKE 模糊查询。
//
// 参数说明：
// - db：已有 GORM 查询。
// - keyword：用户输入关键字。
// - columns：允许参与模糊查询的列名，必须由调用方硬编码，不能来自用户输入。
func ApplyKeyword(db *gorm.DB, keyword string, columns ...string) *gorm.DB {
	if keyword == "" || len(columns) == 0 {
		return db
	}
	like := "%" + keyword + "%"
	conditions := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	for _, column := range columns {
		conditions = append(conditions, column+" LIKE ?")
		args = append(args, like)
	}
	return db.Where("("+strings.Join(conditions, " OR ")+")", args...)
}

// Page 执行 Count 和分页 Find，返回统一分页结果。
func Page[T any](db *gorm.DB, query Query, order string, preload func(*gorm.DB) *gorm.DB) (Result[T], error) {
	var total int64
	var items []T
	if err := db.Count(&total).Error; err != nil {
		return Result[T]{}, err
	}
	readQuery := db
	if preload != nil {
		readQuery = preload(readQuery)
	}
	if order != "" {
		readQuery = readQuery.Order(order)
	}
	if err := readQuery.Offset(query.Offset).Limit(query.PageSize).Find(&items).Error; err != nil {
		return Result[T]{}, err
	}
	return Result[T]{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize, Keyword: query.Keyword}, nil
}

func parsePositive(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
