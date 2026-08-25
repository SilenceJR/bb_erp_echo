// Package material 负责物料基础资料接口。
package material

import (
	"net/http"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Handler 处理物料基础资料。
type Handler struct {
	// DB 是物料读写数据库连接。
	DB *gorm.DB
}

// NewHandler 创建物料模块接口处理器。
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
}

// RegisterRoutes 注册物料模块路由。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/materials", audit)
	group.GET("", h.ListMaterials, require("/api/v1/material", "read"))
	group.POST("", h.CreateMaterial, require("/api/v1/material", "write"))

	legacy := v1.Group("/material", audit)
	legacy.GET("", h.ListMaterials, require("/api/v1/material", "read"))
	legacy.POST("", h.CreateMaterial, require("/api/v1/material", "write"))
}

// ListMaterials 查询物料列表。
func (h *Handler) ListMaterials(c *echo.Context) error {
	var items []model.Material
	if err := h.DB.Order("id desc").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// CreateMaterial 创建物料。
func (h *Handler) CreateMaterial(c *echo.Context) error {
	var req struct {
		Name        string `json:"name" validate:"required"`
		Code        string `json:"code" validate:"required"`
		Category    string `json:"category"`
		Unit        string `json:"unit"`
		Spec        string `json:"spec"`
		SafetyStock int64  `json:"safety_stock"`
		DefaultCost int64  `json:"default_cost"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if req.Unit == "" {
		req.Unit = "个"
	}
	item := model.Material{
		Name:        req.Name,
		Code:        req.Code,
		Category:    req.Category,
		Unit:        req.Unit,
		Spec:        req.Spec,
		SafetyStock: req.SafetyStock,
		DefaultCost: req.DefaultCost,
		Status:      model.StatusActive,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}
