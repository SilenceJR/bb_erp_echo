// Package material 负责物料基础资料接口。
package material

import (
	"net/http"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/operator"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Handler 处理物料基础资料。
type Handler struct {
	// DB 是物料读写数据库连接。
	DB *gorm.DB
}

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

// createMaterialRequest 是创建物料的请求体。
type createMaterialRequest struct {
	Name               string `json:"name" validate:"required" example:"铝材"`
	Code               string `json:"code" validate:"required" example:"M-001"`
	Category           string `json:"category" example:"生产物资"`
	Unit               string `json:"unit" example:"个"`
	Spec               string `json:"spec" example:"标准"`
	SafetyStock        int64  `json:"safety_stock" example:"10"`
	DefaultCost        int64  `json:"default_cost" example:"10000"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required" example:"1"`
}

// NewHandler 创建物料模块接口处理器。
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
}

// RegisterRoutes 注册物料模块路由。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/materials", audit)
	group.GET("", h.ListMaterials, require("/api/v1/materials", "read"))
	group.POST("", h.CreateMaterial, require("/api/v1/materials", "write"))
}

// ListMaterials 查询物料列表。
// @Summary 查询物料
// @Tags material
// @Security BearerAuth
// @Produce json
// @Success 200 {array} model.Material
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/materials [get]
func (h *Handler) ListMaterials(c *echo.Context) error {
	var items []model.Material
	if err := h.DB.Order("id desc").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// CreateMaterial 创建物料。
// @Summary 创建物料
// @Description 创建物料主数据；必须选择当前账号部门下的在职员工。
// @Tags material
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createMaterialRequest true "物料参数"
// @Success 201 {object} model.Material
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/materials [post]
func (h *Handler) CreateMaterial(c *echo.Context) error {
	var req createMaterialRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if req.Unit == "" {
		req.Unit = "个"
	}
	var item model.Material
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, req.OperatorEmployeeID); err != nil {
			return err
		}
		item = model.Material{
			Name:             req.Name,
			Code:             req.Code,
			Category:         req.Category,
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
