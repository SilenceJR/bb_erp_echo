// Package audit 负责操作审计查询接口。
package audit

import (
	"net/http"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Handler 处理审计查询接口。
type Handler struct {
	// DB 是审计日志查询数据库连接。
	DB *gorm.DB
}

// NewHandler 创建审计接口处理器。
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
}

// RegisterRoutes 注册审计查询路由。
func (h *Handler) RegisterRoutes(system *echo.Group, require func(string, string) echo.MiddlewareFunc) {
	system.GET("/audits", h.ListAudits, require("/api/v1/system/audits", "read"))
}

// ListAudits 查询最近 200 条操作审计。
//
// 数据范围：当前实现按当前用户所属组织过滤。
func (h *Handler) ListAudits(c *echo.Context) error {
	var items []model.AuditLog
	query := h.DB.Order("id desc").Limit(200)
	if current := auth.GetCurrentUser(c); current != nil {
		query = query.Where("organization_id = ?", current.OrganizationID)
	}
	if err := query.Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}
