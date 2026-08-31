// Package mold 负责模具台账、借出归还、维修保养履历接口。
package mold

import (
	"errors"
	"net/http"
	"strings"

	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Handler 处理模具业务接口。
type Handler struct {
	Service Service
}

// NewHandler 创建模具模块接口处理器。
func NewHandler(db *gorm.DB) *Handler {
	return NewHandlerWithService(NewService(db))
}

func NewHandlerWithService(service Service) *Handler {
	return &Handler{Service: service}
}

// RegisterRoutes 注册模具模块路由。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	group := v1.Group("/molds", audit)
	group.GET("", h.ListMolds, require("/api/v1/mold", "read"))
	group.GET("/:id", h.GetMold, require("/api/v1/mold", "read"))
	group.POST("", h.CreateMold, require("/api/v1/mold", "write"))
	group.PATCH("/:id", h.UpdateMold, require("/api/v1/mold", "write"))
	group.DELETE("/:id", h.DeleteMold, require("/api/v1/mold", "write"))
	group.POST("/:id/loan", h.LoanMold, require("/api/v1/mold", "write"))
	group.POST("/:id/return", h.ReturnMold, require("/api/v1/mold", "write"))
	group.POST("/:id/repair", h.RepairMold, require("/api/v1/mold", "write"))
	group.POST("/:id/maintenance", h.MaintainMold, require("/api/v1/mold", "write"))

	legacy := v1.Group("/mold", audit)
	legacy.GET("", h.ListMolds, require("/api/v1/mold", "read"))
	legacy.POST("", h.CreateMold, require("/api/v1/mold", "write"))
}

type moldRequest struct {
	Code                 string `json:"code" validate:"required"`
	Name                 string `json:"name" validate:"required"`
	CustomerID           *uint  `json:"customer_id"`
	ProductID            *uint  `json:"product_id"`
	CavityCount          int    `json:"cavity_count"`
	MoldMaterial         string `json:"mold_material"`
	Steel                string `json:"steel"`
	Size                 string `json:"size"`
	WeightGram           int64  `json:"weight_gram"`
	Manufacturer         string `json:"manufacturer"`
	Owner                string `json:"owner"`
	StorageLocation      string `json:"storage_location"`
	CurrentLocation      string `json:"current_location"`
	Status               string `json:"status" validate:"omitempty,oneof=in_stock loaned repairing maintenance scrapped"`
	MaintenanceCycleDays int    `json:"maintenance_cycle_days"`
	Remark               string `json:"remark"`
}

// ListMolds 查询模具台账列表。
func (h *Handler) ListMolds(c *echo.Context) error {
	result, err := h.Service.List(pagination.FromEcho(c), c.QueryParam("status"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

// GetMold 查询模具属性和履历。
func (h *Handler) GetMold(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	item, err := h.Service.Get(id)
	if err != nil {
		if errors.Is(err, ErrMoldNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "模具不存在")
		}
		return err
	}
	return c.JSON(http.StatusOK, item)
}

// CreateMold 创建模具档案。
func (h *Handler) CreateMold(c *echo.Context) error {
	var req moldRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	item, err := h.Service.Create(req)
	if err != nil {
		if errors.Is(err, ErrCustomerProfileNotFound) {
			return echo.NewHTTPError(http.StatusBadRequest, "客户资料不存在")
		}
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

// UpdateMold 更新模具基础属性。
func (h *Handler) UpdateMold(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req moldRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	item, err := h.Service.Update(id, req)
	if err != nil {
		if errors.Is(err, ErrMoldNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "模具不存在")
		}
		if errors.Is(err, ErrCustomerProfileNotFound) {
			return echo.NewHTTPError(http.StatusBadRequest, "客户资料不存在")
		}
		return err
	}
	return c.JSON(http.StatusOK, item)
}

// DeleteMold 软删除模具档案。
func (h *Handler) DeleteMold(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	if err := h.Service.Delete(id); err != nil {
		if errors.Is(err, ErrMoldNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "模具不存在")
		}
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// LoanMold 登记模具借出。
func (h *Handler) LoanMold(c *echo.Context) error {
	var req struct {
		Location     string `json:"location" validate:"required"`
		Counterparty string `json:"counterparty" validate:"required"`
		HandlerName  string `json:"handler_name"`
		Reason       string `json:"reason"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	return h.changeStatus(c, statusLoaned, eventLoan, req.Location, req.Counterparty, req.HandlerName, req.Reason, "模具借出")
}

// ReturnMold 登记模具归还入库。
func (h *Handler) ReturnMold(c *echo.Context) error {
	var req struct {
		Location    string `json:"location"`
		HandlerName string `json:"handler_name"`
		Reason      string `json:"reason"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	req.Location = strings.TrimSpace(req.Location)
	if req.Location == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "归还位置不能为空")
	}
	return h.changeStatus(c, statusInStock, eventReturn, req.Location, "", req.HandlerName, req.Reason, "模具归还")
}

// RepairMold 登记模具维修。
func (h *Handler) RepairMold(c *echo.Context) error {
	var req struct {
		Location    string `json:"location"`
		HandlerName string `json:"handler_name"`
		Reason      string `json:"reason" validate:"required"`
		Description string `json:"description"`
		Completed   bool   `json:"completed"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	status := statusRepairing
	if req.Completed {
		status = statusInStock
	}
	return h.changeStatus(c, status, eventRepair, req.Location, "", req.HandlerName, req.Reason, req.Description)
}

// MaintainMold 登记模具保养。
func (h *Handler) MaintainMold(c *echo.Context) error {
	var req struct {
		Location             string `json:"location"`
		HandlerName          string `json:"handler_name"`
		Description          string `json:"description"`
		MaintenanceCycleDays int    `json:"maintenance_cycle_days"`
		Completed            bool   `json:"completed"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	item, err := h.Service.Maintain(id, MaintenanceCommand{
		Location: req.Location, HandlerName: req.HandlerName, Description: req.Description,
		MaintenanceCycleDays: req.MaintenanceCycleDays, Completed: req.Completed,
	})
	if err != nil {
		return moldHTTPError(err)
	}
	return c.JSON(http.StatusOK, item)
}

func (h *Handler) changeStatus(c *echo.Context, nextStatus string, eventType string, location string, counterparty string, handlerName string, reason string, description string) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	item, err := h.Service.Transition(id, Transition{
		Status: nextStatus, EventType: eventType, Location: location, Counterparty: counterparty,
		HandlerName: handlerName, Reason: reason, Description: description,
	})
	if err != nil {
		return moldHTTPError(err)
	}
	return c.JSON(http.StatusOK, item)
}

func moldHTTPError(err error) error {
	if errors.Is(err, ErrMoldNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "模具不存在")
	}
	if errors.Is(err, ErrMoldStatusConflict) {
		return echo.NewHTTPError(http.StatusConflict, "模具当前状态不允许执行该操作，请刷新后重试")
	}
	if errors.Is(err, ErrMoldReturnLocationRequired) {
		return echo.NewHTTPError(http.StatusBadRequest, "归还位置不能为空")
	}
	if errors.Is(err, ErrMoldMaintenanceCycleRequired) {
		return echo.NewHTTPError(http.StatusBadRequest, "完成保养前请填写保养周期")
	}
	return err
}
