// Package customer 负责客户档案接口。
package customer

import (
	"errors"
	"net/http"
	"time"

	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

// CustomerPhoneResponse 是客户列表中联系人电话明细的文档响应结构。
//
// 参数说明：
// - ID：电话明细 ID。
// - ContactID：所属联系人 ID。
// - Phone：电话号码。
// - Label：号码标签。
// - Primary：是否主联系电话。
type CustomerPhoneResponse struct {
	// ID 是电话明细 ID。
	ID uint `json:"id" example:"1"`
	// CreatedAt 是创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 是更新时间。
	UpdatedAt time.Time `json:"updated_at"`
	// ContactID 是所属联系人 ID。
	ContactID uint `json:"contact_id" example:"1"`
	// Phone 是电话号码。
	Phone string `json:"phone" example:"13800000000"`
	// Label 是号码标签。
	Label string `json:"label" example:"手机"`
	// Primary 表示是否主联系电话。
	Primary bool `json:"primary" example:"true"`
}

// CustomerContactResponse 是客户列表中联系人明细的文档响应结构。
//
// 参数说明：
// - ID：联系人 ID。
// - CustomerID：所属客户 ID。
// - Name：联系人姓名。
// - Phones：联系人电话明细。
type CustomerContactResponse struct {
	// ID 是联系人 ID。
	ID uint `json:"id" example:"1"`
	// CreatedAt 是创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 是更新时间。
	UpdatedAt time.Time `json:"updated_at"`
	// CustomerID 是所属客户 ID。
	CustomerID uint `json:"customer_id" example:"1"`
	// Name 是联系人姓名。
	Name string `json:"name" example:"张三"`
	// Phones 是联系人电话明细。
	Phones []CustomerPhoneResponse `json:"phones"`
}

// CustomerResponse 是客户档案的文档响应结构。
//
// 参数说明：
// - ID：客户 ID。
// - Name：客户名称。
// - Code：客户编码。
// - Address：客户地址。
// - Contacts：客户联系人列表。
type CustomerResponse struct {
	// ID 是客户 ID。
	ID uint `json:"id" example:"1"`
	// CreatedAt 是创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 是更新时间。
	UpdatedAt time.Time `json:"updated_at"`
	// Name 是客户名称。
	Name string `json:"name" example:"测试客户"`
	// Code 是客户编码。
	Code string `json:"code" example:"CUST-001"`
	// Phone 是旧版客户电话字段；新接口不再写入，电话请维护到联系人电话明细。
	Phone string `json:"phone" example:""`
	// Address 是客户地址。
	Address string `json:"address" example:"深圳市宝安区"`
	// Contacts 是客户联系人列表。
	Contacts []CustomerContactResponse `json:"contacts"`
}

// CreateCustomerRequest 是创建客户档案的请求体。
//
// 参数说明：
// - Name：客户名称，必填。
// - Code：客户编码，必填且在数据库中唯一。
// - Phone：客户座机或主要联系电话，可选。
type CreateCustomerRequest struct {
	// Name 是客户名称。
	Name string `json:"name" validate:"required" example:"测试客户"`
	// Code 是客户编码，用于内部检索和去重。
	Code string `json:"code" validate:"required" example:"CUST-001"`
	// Phone 是客户座机或主要联系电话。
	Phone string `json:"phone" example:"0755-88888888"`
}

// UpdateCustomerRequest 是更新客户档案的请求体。
//
// 参数说明：
// - Name：客户名称，必填。
// - Code：客户编码，必填且在数据库中唯一。
// - Phone：客户座机或主要联系电话，可选。
// - Address：客户地址，可选。
type UpdateCustomerRequest struct {
	// Name 是客户名称。
	Name string `json:"name" validate:"required" example:"测试客户-更新"`
	// Code 是客户编码，用于内部检索和去重。
	Code string `json:"code" validate:"required" example:"CUST-001"`
	// Phone 是客户座机或主要联系电话。
	Phone string `json:"phone" example:"0755-66666666"`
	// Address 是客户地址。
	Address string `json:"address" example:"深圳市宝安区"`
}

// Handler 处理客户与联系人模块接口。
type Handler struct {
	Service Service
}

// NewHandler 创建客户模块接口处理器。
//
// 参数说明：
// - db：GORM 数据库连接。
func NewHandler(db *gorm.DB) *Handler {
	return NewHandlerWithService(NewService(db))
}

// NewHandlerWithService 支持注入替代实现或测试替身。
func NewHandlerWithService(service Service) *Handler {
	return &Handler{Service: service}
}

// RegisterRoutes 注册客户业务模块路由。
//
// 参数说明：
// - v1：/api/v1 受保护业务路由组。
// - require：权限中间件工厂。
// - audit：操作审计中间件，用于记录客户资料读写操作。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/customers", audit)
	group.GET("", h.ListCustomers, require("/api/v1/customers", "read"))
	group.POST("", h.CreateCustomer, require("/api/v1/customers", "write"))
	group.PATCH("/:id", h.UpdateCustomer, require("/api/v1/customers", "write"))
	group.DELETE("/:id", h.DeleteCustomer, require("/api/v1/customers", "write"))
}

// ListCustomers 查询客户列表。
//
// 参数说明：
// - c：Echo 请求上下文。
//
// 返回说明：
// - 返回按 ID 倒序排列的客户档案列表。
//
// @Summary 查询客户列表
// @Description 返回客户档案列表，并预加载联系人和联系人电话明细。
// @Tags 客户
// @Security BearerAuth
// @Produce json
// @Success 200 {array} CustomerResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/customers [get]
func (h *Handler) ListCustomers(c *echo.Context) error {
	result, err := h.Service.List(pagination.FromEcho(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

// CreateCustomer 创建客户档案。
//
// 请求参数：
// - name：客户名称，必填。
// - code：客户编码，必填且在数据库中唯一。
// - phone：客户座机或主要联系电话，可选。
//
// 返回说明：
// - 创建成功返回 201 和新客户记录。
//
// @Summary 创建客户
// @Description 创建客户名称、编码和客户座机；客户联系人关系在创建联系人时通过 customer_id 建立。
// @Tags 客户
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body CreateCustomerRequest true "创建客户参数"
// @Success 201 {object} CustomerResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/customers [post]
func (h *Handler) CreateCustomer(c *echo.Context) error {
	var req CreateCustomerRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}

	customer, err := h.Service.Create(req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, customer)
}

// UpdateCustomer 通过客户 ID 更新客户档案。
//
// 路径参数：
// - id：客户 ID，必填且必须为正整数。
//
// 请求参数：
// - name：客户名称，必填。
// - code：客户编码，必填且在数据库中唯一。
// - phone：客户座机或主要联系电话，可选。
// - address：客户地址，可选。
//
// 返回说明：
// - 更新成功返回 200 和更新后的客户记录。
// - 客户不存在返回 404。
//
// @Summary 更新客户
// @Description 通过客户 ID 更新客户档案；联系人和电话明细请使用联系人接口维护。
// @Tags 客户
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "客户 ID"
// @Param body body UpdateCustomerRequest true "更新客户参数"
// @Success 200 {object} CustomerResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/customers/{id} [patch]
func (h *Handler) UpdateCustomer(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}

	var req UpdateCustomerRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}

	customer, err := h.Service.Update(id, req)
	if err != nil {
		if errors.Is(err, ErrCustomerNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "客户不存在")
		}
		return err
	}

	return c.JSON(http.StatusOK, customer)
}

// DeleteCustomer 通过客户 ID 删除客户档案。
//
// 路径参数：
// - id：客户 ID，必填且必须为正整数。
//
// 业务说明：
// 当前模型使用 BaseModel 软删除。删除客户只代表客户档案被删除，
// 不代表联系人被删除；联系人和电话明细保留，后续可单独转移、删除或做历史追溯。
//
// 返回说明：
// - 删除成功返回 204。
// - 客户不存在返回 404。
//
// @Summary 删除客户
// @Description 软删除客户本体，不删除联系人；联系人和联系人电话明细保留用于转移或历史追溯。
// @Tags 客户
// @Security BearerAuth
// @Param id path int true "客户 ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/customers/{id} [delete]
func (h *Handler) DeleteCustomer(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}

	if err := h.Service.Delete(id); err != nil {
		if errors.Is(err, ErrCustomerNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "客户不存在")
		}
		return err
	}

	return c.NoContent(http.StatusNoContent)
}
