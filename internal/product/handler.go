// Package product 负责产品基础资料接口。
package product

import (
	"net/http"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/operator"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Handler 处理产品基础资料。
type Handler struct {
	// DB 是产品读写数据库连接。
	DB *gorm.DB
}

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

// createProductRequest 是创建产品的请求体。
type createProductRequest struct {
	Name               string `json:"name" validate:"required" example:"白色外壳"`
	Code               string `json:"code" validate:"required" example:"P-001"`
	Unit               string `json:"unit" example:"个"`
	Spec               string `json:"spec" example:"标准"`
	SafetyStock        int64  `json:"safety_stock" example:"10"`
	DefaultCost        int64  `json:"default_cost" example:"10000"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required" example:"1"`
}

// NewHandler 创建产品模块接口处理器。
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
}

// RegisterRoutes 注册产品模块路由。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/products", audit)
	group.GET("", h.ListProducts, require("/api/v1/products", "read"))
	group.POST("", h.CreateProduct, require("/api/v1/products", "write"))
}

// ListProducts 查询产品列表。
// @Summary 查询产品
// @Tags product
// @Security BearerAuth
// @Produce json
// @Success 200 {array} model.Product
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/products [get]
func (h *Handler) ListProducts(c *echo.Context) error {
	var items []model.Product
	if err := h.DB.Order("id desc").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// CreateProduct 创建产品。
// @Summary 创建产品
// @Description 创建产品主数据；必须选择当前账号部门下的在职员工。
// @Tags product
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createProductRequest true "产品参数"
// @Success 201 {object} model.Product
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/products [post]
func (h *Handler) CreateProduct(c *echo.Context) error {
	var req createProductRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if req.Unit == "" {
		req.Unit = "个"
	}
	var item model.Product
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, req.OperatorEmployeeID); err != nil {
			return err
		}
		item = model.Product{
			Name:             req.Name,
			Code:             req.Code,
			Unit:             req.Unit,
			Spec:             req.Spec,
			SafetyStock:      req.SafetyStock,
			DefaultCost:      req.DefaultCost,
			Status:           model.StatusActive,
			OperatorSnapshot: operator.Snapshot(c),
		}
		return tx.Create(&item).Error
	}); err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}
