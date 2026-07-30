// Package mold 负责模具台账、借出归还、维修保养履历接口。
package mold

import (
	"errors"
	"net/http"
	"time"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

const (
	statusInStock     = "in_stock"
	statusLoaned      = "loaned"
	statusRepairing   = "repairing"
	statusMaintenance = "maintenance"
	statusScrapped    = "scrapped"

	eventCreate      = "create"
	eventLoan        = "loan"
	eventReturn      = "return"
	eventRepair      = "repair"
	eventMaintenance = "maintenance"
)

// Handler 处理模具业务接口。
type Handler struct {
	// DB 是模具台账和履历读写数据库连接。
	DB *gorm.DB
}

// NewHandler 创建模具模块接口处理器。
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
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
	var items []model.Mold
	query := h.DB.Order("id desc")
	if status := c.QueryParam("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// GetMold 查询模具属性和履历。
func (h *Handler) GetMold(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var item model.Mold
	if err := h.DB.Preload("Events", func(db *gorm.DB) *gorm.DB {
		return db.Order("id desc")
	}).First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
	item := moldFromRequest(req)
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		return createEvent(tx, item, eventCreate, "", item.Status, item.CurrentLocation, "", "", "新建模具档案", "")
	}); err != nil {
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
	var item model.Mold
	if err := h.DB.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "模具不存在")
		}
		return err
	}
	beforeStatus := item.Status
	applyMoldRequest(&item, req)
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&item).Error; err != nil {
			return err
		}
		if beforeStatus != item.Status {
			return createEvent(tx, item, "status_change", beforeStatus, item.Status, item.CurrentLocation, "", "", "更新模具状态", "")
		}
		return nil
	}); err != nil {
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
	if err := h.DB.Delete(&model.Mold{}, id).Error; err != nil {
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
	status := statusMaintenance
	if req.Completed {
		status = statusInStock
	}
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var item model.Mold
	if err := h.DB.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "模具不存在")
		}
		return err
	}
	beforeStatus := item.Status
	now := time.Now()
	item.Status = status
	if req.Location != "" {
		item.CurrentLocation = req.Location
	}
	if req.MaintenanceCycleDays > 0 {
		item.MaintenanceCycleDays = req.MaintenanceCycleDays
	}
	if req.Completed {
		item.LastMaintenanceAt = &now
		if item.MaintenanceCycleDays > 0 {
			next := now.AddDate(0, 0, item.MaintenanceCycleDays)
			item.NextMaintenanceAt = &next
		}
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&item).Error; err != nil {
			return err
		}
		return createEvent(tx, item, eventMaintenance, beforeStatus, item.Status, item.CurrentLocation, "", req.HandlerName, "模具保养", req.Description)
	}); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, item)
}

func (h *Handler) changeStatus(c *echo.Context, nextStatus string, eventType string, location string, counterparty string, handlerName string, reason string, description string) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var item model.Mold
	if err := h.DB.First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "模具不存在")
		}
		return err
	}
	beforeStatus := item.Status
	item.Status = nextStatus
	if location != "" {
		item.CurrentLocation = location
	}
	now := time.Now()
	if eventType == eventRepair && nextStatus == statusInStock {
		item.LastRepairAt = &now
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&item).Error; err != nil {
			return err
		}
		return createEvent(tx, item, eventType, beforeStatus, item.Status, item.CurrentLocation, counterparty, handlerName, reason, description)
	}); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, item)
}

func moldFromRequest(req moldRequest) model.Mold {
	item := model.Mold{}
	applyMoldRequest(&item, req)
	if item.Status == "" {
		item.Status = statusInStock
	}
	if item.CavityCount == 0 {
		item.CavityCount = 1
	}
	if item.CurrentLocation == "" {
		item.CurrentLocation = item.StorageLocation
	}
	return item
}

func applyMoldRequest(item *model.Mold, req moldRequest) {
	item.Code = req.Code
	item.Name = req.Name
	item.CustomerID = req.CustomerID
	item.ProductID = req.ProductID
	item.CavityCount = req.CavityCount
	item.MoldMaterial = req.MoldMaterial
	item.Steel = req.Steel
	item.Size = req.Size
	item.WeightGram = req.WeightGram
	item.Manufacturer = req.Manufacturer
	item.Owner = req.Owner
	item.StorageLocation = req.StorageLocation
	item.CurrentLocation = req.CurrentLocation
	item.MaintenanceCycleDays = req.MaintenanceCycleDays
	item.Remark = req.Remark
	if req.Status != "" {
		item.Status = req.Status
	}
	if item.Status == "" {
		item.Status = statusInStock
	}
	if item.CavityCount <= 0 {
		item.CavityCount = 1
	}
	if item.CurrentLocation == "" {
		item.CurrentLocation = item.StorageLocation
	}
}

func createEvent(tx *gorm.DB, item model.Mold, eventType string, before string, after string, location string, counterparty string, handlerName string, reason string, description string) error {
	now := time.Now()
	event := model.MoldEvent{
		MoldID:       item.ID,
		Type:         eventType,
		StatusBefore: before,
		StatusAfter:  after,
		Location:     location,
		Counterparty: counterparty,
		HandlerName:  handlerName,
		Reason:       reason,
		Description:  description,
		StartedAt:    &now,
	}
	if eventType == eventReturn || eventType == eventMaintenance || (eventType == eventRepair && after == statusInStock) {
		event.FinishedAt = &now
	}
	return tx.Create(&event).Error
}
