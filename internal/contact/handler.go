// Package contact 负责客户联系人和联系人电话明细接口。
package contact

import (
	"errors"
	"net/http"
	"time"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

// ContactPhoneResponse 是联系人电话明细响应结构。
//
// 参数说明：
// - ID：电话明细 ID。
// - ContactID：所属联系人 ID。
// - Phone：电话号码。
// - Label：号码标签。
// - Primary：是否主联系电话。
type ContactPhoneResponse struct {
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

// ContactResponse 是联系人响应结构。
//
// 参数说明：
// - ID：联系人 ID。
// - CustomerID：所属客户 ID。
// - Name：联系人姓名。
// - Phones：联系人电话明细。
type ContactResponse struct {
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
	Phones []ContactPhoneResponse `json:"phones"`
}

// ContactPhoneRequest 是联系人电话明细请求项。
//
// 参数说明：
// - Phone：电话号码，必填。
// - Label：号码标签，例如手机、座机、微信同号。
// - Primary：是否为主联系电话。
type ContactPhoneRequest struct {
	// Phone 是电话号码。
	Phone string `json:"phone" validate:"required" example:"13800000000"`
	// Label 是号码标签，例如手机、座机、微信同号。
	Label string `json:"label" example:"手机"`
	// Primary 表示是否为主联系电话。
	Primary bool `json:"primary" example:"true"`
}

// CreateContactRequest 是创建联系人请求体。
//
// 参数说明：
// - CustomerID：所属客户 ID，必填，用于建立客户与联系人的关联关系。
// - Name：联系人姓名，必填。
// - Phones：联系人电话明细，可选。
type CreateContactRequest struct {
	// CustomerID 是联系人所属客户 ID。
	CustomerID uint `json:"customer_id" validate:"required" example:"1"`
	// Name 是联系人姓名。
	Name string `json:"name" validate:"required" example:"张三"`
	// Phones 是联系人电话明细。
	Phones []ContactPhoneRequest `json:"phones" validate:"dive"`
}

// UpdateContactRequest 是更新联系人请求体。
//
// 参数说明：
// - CustomerID：所属客户 ID，必填，可用于把联系人转移到其他客户。
// - Name：联系人姓名，必填。
// - Phones：联系人电话明细，可选；更新时按请求内容整体替换。
type UpdateContactRequest struct {
	// CustomerID 是联系人所属客户 ID。
	CustomerID uint `json:"customer_id" validate:"required" example:"1"`
	// Name 是联系人姓名。
	Name string `json:"name" validate:"required" example:"张三-更新"`
	// Phones 是联系人电话明细；更新时整体替换旧电话。
	Phones []ContactPhoneRequest `json:"phones" validate:"dive"`
}

// Handler 处理客户联系人接口。
type Handler struct {
	// DB 是联系人和联系人电话明细读写数据库连接。
	DB *gorm.DB
}

// NewHandler 创建联系人模块接口处理器。
//
// 参数说明：
// - db：GORM 数据库连接。
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
}

// RegisterRoutes 注册联系人业务模块路由。
//
// 参数说明：
// - v1：/api/v1 受保护业务路由组。
// - require：权限中间件工厂。
// - audit：操作审计中间件，用于记录联系人资料读写操作。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/contacts", audit)
	group.GET("", h.ListContacts, require("/api/v1/contacts", "read"))
	group.GET("/:id", h.GetContact, require("/api/v1/contacts", "read"))
	group.POST("", h.CreateContact, require("/api/v1/contacts", "write"))
	group.PATCH("/:id", h.UpdateContact, require("/api/v1/contacts", "write"))
	group.DELETE("/:id", h.DeleteContact, require("/api/v1/contacts", "write"))
}

// ListContacts 查询联系人列表。
//
// 参数说明：
// - c：Echo 请求上下文。
//
// 返回说明：
// - 返回按 ID 倒序排列的联系人列表，并预加载电话明细。
//
// @Summary 查询联系人列表
// @Description 返回联系人列表，并预加载联系人电话明细。
// @Tags 联系人
// @Security BearerAuth
// @Produce json
// @Success 200 {array} ContactResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/contacts [get]
func (h *Handler) ListContacts(c *echo.Context) error {
	var items []model.Contact
	if err := h.DB.Order("id desc").Preload("Phones").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// GetContact 通过联系人 ID 查询联系人详情。
//
// 路径参数：
// - id：联系人 ID，必填且必须为正整数。
//
// 返回说明：
// - 查询成功返回联系人记录并预加载电话明细。
// - 联系人不存在返回 404。
//
// @Summary 查询联系人详情
// @Description 通过联系人 ID 查询联系人详情，并预加载电话明细。
// @Tags 联系人
// @Security BearerAuth
// @Produce json
// @Param id path int true "联系人 ID"
// @Success 200 {object} ContactResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/contacts/{id} [get]
func (h *Handler) GetContact(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}

	var item model.Contact
	if err := h.DB.Preload("Phones").First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "联系人不存在")
		}
		return err
	}
	return c.JSON(http.StatusOK, item)
}

// CreateContact 创建客户联系人，并通过 customer_id 建立客户关联。
//
// 请求参数：
// - customer_id：所属客户 ID，必填。
// - name：联系人姓名，必填。
// - phones：联系人电话明细，可选；每条电话包含 phone、label、primary。
//
// 返回说明：
// - 创建成功返回 201 和联系人记录，包含电话明细。
//
// @Summary 创建联系人
// @Description 创建联系人时必须携带 customer_id，由联系人接口建立客户与联系人的关联关系。
// @Tags 联系人
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body CreateContactRequest true "创建联系人参数"
// @Success 201 {object} ContactResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/contacts [post]
func (h *Handler) CreateContact(c *echo.Context) error {
	var req CreateContactRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}

	var customer model.Customer
	if err := h.DB.First(&customer, req.CustomerID).Error; err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "客户不存在")
	}

	contact := model.Contact{
		CustomerID: req.CustomerID,
		Name:       req.Name,
	}
	for _, phoneReq := range req.Phones {
		contact.Phones = append(contact.Phones, model.ContactPhone{
			Phone:   phoneReq.Phone,
			Label:   phoneReq.Label,
			Primary: phoneReq.Primary,
		})
	}

	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&contact).Error
	}); err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, contact)
}

// UpdateContact 更新客户联系人，并整体替换联系人电话明细。
//
// 路径参数：
// - id：联系人 ID，必填且必须为正整数。
//
// 请求参数：
// - customer_id：所属客户 ID，必填。
// - name：联系人姓名，必填。
// - phones：联系人电话明细，可选；更新时按请求内容整体替换。
//
// 返回说明：
// - 更新成功返回 200 和更新后的联系人记录。
// - 联系人或目标客户不存在返回 404。
//
// @Summary 更新联系人
// @Description 更新联系人基础信息，并按请求内容整体替换联系人电话明细。
// @Tags 联系人
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "联系人 ID"
// @Param body body UpdateContactRequest true "更新联系人参数"
// @Success 200 {object} ContactResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/contacts/{id} [patch]
func (h *Handler) UpdateContact(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}

	var req UpdateContactRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}

	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		var customer model.Customer
		if err := tx.First(&customer, req.CustomerID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "客户不存在")
			}
			return err
		}

		var contact model.Contact
		if err := tx.First(&contact, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "联系人不存在")
			}
			return err
		}

		contact.CustomerID = req.CustomerID
		contact.Name = req.Name
		if err := tx.Save(&contact).Error; err != nil {
			return err
		}
		if err := tx.Where("contact_id = ?", contact.ID).Delete(&model.ContactPhone{}).Error; err != nil {
			return err
		}
		for _, phoneReq := range req.Phones {
			phone := model.ContactPhone{
				ContactID: contact.ID,
				Phone:     phoneReq.Phone,
				Label:     phoneReq.Label,
				Primary:   phoneReq.Primary,
			}
			if err := tx.Create(&phone).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	var updated model.Contact
	if err := h.DB.Preload("Phones").First(&updated, id).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, updated)
}

// DeleteContact 通过联系人 ID 删除联系人，并同步删除其电话明细。
//
// 路径参数：
// - id：联系人 ID，必填且必须为正整数。
//
// 业务说明：
// 删除联系人代表该联系人资料被删除，联系人电话明细属于联系人资料的一部分，
// 因此会一起软删除。
//
// 返回说明：
// - 删除成功返回 204。
// - 联系人不存在返回 404。
//
// @Summary 删除联系人
// @Description 软删除联系人，并同步软删除其电话明细。
// @Tags 联系人
// @Security BearerAuth
// @Param id path int true "联系人 ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/contacts/{id} [delete]
func (h *Handler) DeleteContact(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}

	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		var contact model.Contact
		if err := tx.First(&contact, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "联系人不存在")
			}
			return err
		}
		if err := tx.Where("contact_id = ?", contact.ID).Delete(&model.ContactPhone{}).Error; err != nil {
			return err
		}
		return tx.Delete(&contact).Error
	}); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
