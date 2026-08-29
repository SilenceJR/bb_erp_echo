package inventory

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

const (
	businessPurchaseInbound     = "purchase_inbound"
	businessReturnReworkInbound = "return_rework_inbound"
	businessCustomerOutbound    = "customer_outbound"
	businessDepartmentOutbound  = "department_outbound"
	maxMovementQuantity         = int64(999999999 * 10000)
)

type itemMovementRequest struct {
	BusinessType       string `json:"business_type" validate:"required,oneof=purchase_inbound return_rework_inbound customer_outbound department_outbound"`
	Quantity           int64  `json:"quantity" validate:"required"`
	UnitCost           int64  `json:"unit_cost"`
	SupplierID         *uint  `json:"supplier_id"`
	CustomerID         *uint  `json:"customer_id"`
	DepartmentID       *uint  `json:"department_id"`
	OriginalDocumentID *uint  `json:"original_document_id"`
	Reason             string `json:"reason"`
	Remark             string `json:"remark"`
}

func (h *Handler) GetItemDetail(c *echo.Context) error {
	itemType, itemID, err := itemParams(c)
	if err != nil {
		return err
	}
	item, err := h.loadItem(itemType, itemID)
	if err != nil {
		return err
	}
	warehouse, err := h.defaultWarehouse()
	if err != nil {
		return err
	}
	result := map[string]any{"item": item, "warehouse": warehouse, "quantity": int64(0)}
	var aggregate struct {
		Quantity int64
		Amount   int64
	}
	err = h.DB.Model(&model.InventoryBalance{}).
		Select("COALESCE(SUM(quantity), 0) AS quantity, COALESCE(SUM(amount), 0) AS amount").
		Where("warehouse_id = ? AND item_type = ? AND item_id = ?", warehouse.ID, itemType, itemID).
		Scan(&aggregate).Error
	if err != nil {
		return err
	}
	result["quantity"] = aggregate.Quantity
	if hasCostView(c) {
		result["amount"] = aggregate.Amount
		if aggregate.Quantity > 0 {
			result["avg_cost"] = aggregate.Amount * 10000 / aggregate.Quantity
		} else {
			result["avg_cost"] = int64(0)
		}
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handler) ListItemMovements(c *echo.Context) error {
	itemType, itemID, err := itemParams(c)
	if err != nil {
		return err
	}
	if _, err := h.loadItem(itemType, itemID); err != nil {
		return err
	}
	page := positiveQueryInt(c.QueryParam("page"), 1)
	pageSize := positiveQueryInt(c.QueryParam("page_size"), 20)
	if pageSize > 100 {
		pageSize = 100
	}
	var total int64
	query := h.DB.Model(&model.InventoryDocument{}).
		Joins("JOIN inventory_document_lines ON inventory_document_lines.document_id = inventory_documents.id").
		Where("inventory_document_lines.item_type = ? AND inventory_document_lines.item_id = ?", itemType, itemID)
	if err := query.Count(&total).Error; err != nil {
		return err
	}
	var items []model.InventoryDocument
	if err := query.Preload("Lines", "item_type = ? AND item_id = ?", itemType, itemID).
		Order("inventory_documents.id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"items": trimDocuments(items, hasCostView(c)), "page": page, "page_size": pageSize, "total": total,
	})
}

func (h *Handler) CreateItemMovement(c *echo.Context) error {
	itemType, itemID, err := itemParams(c)
	if err != nil {
		return err
	}
	if _, err := h.loadItem(itemType, itemID); err != nil {
		return err
	}
	var req itemMovementRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if req.Quantity <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "数量必须大于 0")
	}
	if req.Quantity > maxMovementQuantity {
		return echo.NewHTTPError(http.StatusBadRequest, "数量不能超过 999999999")
	}
	strategy, ok := h.movementStrategies[req.BusinessType]
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "不支持的出入库业务类型")
	}
	validationContext := requestMovementValidationContext{handler: h, user: auth.GetCurrentUser(c)}
	if err := strategy.Validate(validationContext, req, itemType, itemID); err != nil {
		return err
	}
	idempotencyKey := c.Request().Header.Get("Idempotency-Key")
	if idempotencyKey != "" {
		var existing model.InventoryDocument
		if err := h.DB.Preload("Lines").Where("idempotency_key = ?", idempotencyKey).First(&existing).Error; err == nil {
			return c.JSON(http.StatusOK, trimDocument(existing, hasCostView(c)))
		}
	}
	warehouse, err := h.defaultWarehouse()
	if err != nil {
		return err
	}
	current := auth.GetCurrentUser(c)
	doc := model.InventoryDocument{
		Code: fmt.Sprintf("INV-%s-%d", time.Now().Format("20060102"), time.Now().UnixNano()),
		Type: strategy.Direction(), BusinessType: req.BusinessType, Status: documentDraft, WarehouseID: warehouse.ID,
		SupplierID: req.SupplierID, CustomerID: req.CustomerID, DepartmentID: req.DepartmentID,
		OriginalDocumentID: req.OriginalDocumentID, Reason: req.Reason, IdempotencyKey: idempotencyKey,
		Lines: []model.InventoryDocumentLine{{ItemType: itemType, ItemID: itemID, Quantity: req.Quantity, UnitCost: req.UnitCost, Remark: req.Remark}},
	}
	if current != nil {
		doc.CreatedBy = current.ID
	}
	if req.UnitCost > 0 {
		doc.Lines[0].Amount = scaledAmount(req.Quantity, req.UnitCost)
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&doc).Error; err != nil {
			return err
		}
		if err := h.postLines(tx, &doc, false); err != nil {
			return err
		}
		now := time.Now()
		doc.Status, doc.PostedAt = documentPosted, &now
		if current != nil {
			doc.PostedBy = &current.ID
		}
		return tx.Save(&doc).Error
	}); err != nil {
		return err
	}
	if err := h.DB.Preload("Lines").First(&doc, doc.ID).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, trimDocument(doc, hasCostView(c)))
}

func (h *Handler) validateOriginalDocument(id uint, itemType string, itemID uint, customerID, departmentID *uint) error {
	var doc model.InventoryDocument
	if err := h.DB.Preload("Lines").First(&doc, id).Error; err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "原出库记录不存在")
	}
	if doc.Type != typeOutbound || doc.Status != documentPosted || !sameOptionalID(doc.CustomerID, customerID) || !sameOptionalID(doc.DepartmentID, departmentID) {
		return echo.NewHTTPError(http.StatusBadRequest, "原出库记录与当前退回来源不一致")
	}
	for _, line := range doc.Lines {
		if line.ItemType == itemType && line.ItemID == itemID {
			return nil
		}
	}
	return echo.NewHTTPError(http.StatusBadRequest, "原出库记录不包含当前物品")
}

func (h *Handler) loadItem(itemType string, id uint) (any, error) {
	if itemType == itemMaterial {
		var item model.Material
		if err := h.DB.First(&item, id).Error; err != nil {
			return nil, itemNotFound(err)
		}
		return item, nil
	}
	var item model.Product
	if err := h.DB.First(&item, id).Error; err != nil {
		return nil, itemNotFound(err)
	}
	return item, nil
}

func (h *Handler) defaultWarehouse() (model.Warehouse, error) {
	item := model.Warehouse{Name: "默认仓库", Code: "MAIN", Status: model.StatusActive}
	err := h.DB.FirstOrCreate(&item, model.Warehouse{Code: item.Code}).Error
	return item, err
}

func (h *Handler) requireRecord(value any, id uint, message string) error {
	if err := h.DB.First(value, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusBadRequest, message)
		}
		return err
	}
	return nil
}

func itemParams(c *echo.Context) (string, uint, error) {
	itemType := c.Param("itemType")
	if itemType != itemMaterial && itemType != itemProduct {
		return "", 0, echo.NewHTTPError(http.StatusBadRequest, "无效物品类型")
	}
	id, err := strconv.ParseUint(c.Param("itemID"), 10, 64)
	if err != nil || id == 0 {
		return "", 0, echo.NewHTTPError(http.StatusBadRequest, "无效物品编号")
	}
	return itemType, uint(id), nil
}

func itemNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "物品不存在")
	}
	return err
}

func sameOptionalID(left, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func positiveQueryInt(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
