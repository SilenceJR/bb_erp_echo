// Package product 负责产品基础资料接口。
package product

import (
	"net/http"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Handler 处理产品基础资料。
type Handler struct {
	// DB 是产品读写数据库连接。
	DB *gorm.DB
}

// NewHandler 创建产品模块接口处理器。
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
}

// RegisterRoutes 注册产品模块路由。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/products", audit)
	group.GET("", h.ListProducts, require("/api/v1/product", "read"))
	group.POST("", h.CreateProduct, require("/api/v1/product", "write"))

	legacy := v1.Group("/product", audit)
	legacy.GET("", h.ListProducts, require("/api/v1/product", "read"))
	legacy.POST("", h.CreateProduct, require("/api/v1/product", "write"))
}

// ListProducts 查询产品列表。
func (h *Handler) ListProducts(c *echo.Context) error {
	var items []model.Product
	if err := h.DB.Order("id desc").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// CreateProduct 创建产品。
func (h *Handler) CreateProduct(c *echo.Context) error {
	var req struct {
		Name        string `json:"name" validate:"required"`
		Code        string `json:"code" validate:"required"`
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
	item := model.Product{
		Name:        req.Name,
		Code:        req.Code,
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
