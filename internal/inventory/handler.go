// Package inventory 负责库存单据、余额和流水接口。
package inventory

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/role"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	documentDraft    = "draft"
	documentPosted   = "posted"
	documentReversed = "reversed"
	typeInbound      = "inbound"
	typeOutbound     = "outbound"
	typeTransfer     = "transfer"
	itemMaterial     = "material"
	itemProduct      = "product"
)

// Handler 处理库存业务接口。
type Handler struct {
	// DB 是库存单据、余额和流水读写数据库连接。
	DB                 *gorm.DB
	movementStrategies map[string]MovementStrategy
}

// NewHandler 创建库存模块接口处理器。
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db, movementStrategies: DefaultMovementStrategies()}
}

// RegisterRoutes 注册库存模块路由。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	docs := v1.Group("/inventory-documents", audit)
	docs.GET("", h.ListDocuments, require("/api/v1/inventory-documents", "read"))
	docs.POST("", h.CreateDocument, require("/api/v1/inventory-documents", "write"))
	docs.POST("/:id/post", h.PostDocument, require("/api/v1/inventory-documents", "write"))
	docs.POST("/:id/reverse", h.ReverseDocument, require("/api/v1/inventory-documents", "write"))

	v1.GET("/inventory-balances", h.ListBalances, require("/api/v1/inventory-balances", "read"))
	v1.GET("/inventory-ledgers", h.ListLedgers, require("/api/v1/inventory-ledgers", "read"))

	items := v1.Group("/warehouse/items")
	items.GET("/:itemType/:itemID", h.GetItemDetail, require("/api/v1/warehouse", "read"))
	items.GET("/:itemType/:itemID/movements", h.ListItemMovements, require("/api/v1/inventory-documents", "read"))
	items.POST("/:itemType/:itemID/movements", h.CreateItemMovement, audit, require("/api/v1/inventory-documents", "write"))

	legacy := v1.Group("/inventory", audit)
	legacy.GET("", h.ListBalances, require("/api/v1/inventory", "read"))
	legacy.POST("", h.CreateDocument, require("/api/v1/inventory", "write"))
}

type lineRequest struct {
	ItemType   string `json:"item_type" validate:"required,oneof=material product"`
	ItemID     uint   `json:"item_id" validate:"required"`
	LocationID *uint  `json:"location_id"`
	Quantity   int64  `json:"quantity" validate:"required"`
	UnitCost   int64  `json:"unit_cost"`
	Amount     int64  `json:"amount"`
	Remark     string `json:"remark"`
}

// ListDocuments 查询库存单据。
func (h *Handler) ListDocuments(c echo.Context) error {
	var items []model.InventoryDocument
	if err := h.DB.Order("id desc").Preload("Lines").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, trimDocuments(items, hasCostView(c)))
}

// CreateDocument 创建库存草稿单据。
func (h *Handler) CreateDocument(c echo.Context) error {
	var req struct {
		Code          string        `json:"code" validate:"required"`
		Type          string        `json:"type" validate:"required,oneof=inbound outbound transfer"`
		WarehouseID   uint          `json:"warehouse_id" validate:"required"`
		ToWarehouseID *uint         `json:"to_warehouse_id"`
		Reason        string        `json:"reason"`
		Lines         []lineRequest `json:"lines" validate:"required,min=1,dive"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if req.Type == typeTransfer && req.ToWarehouseID == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "调拨单必须填写目标仓库")
	}
	idempotencyKey := c.Request().Header.Get("Idempotency-Key")
	if idempotencyKey != "" {
		var existing model.InventoryDocument
		if err := h.DB.Preload("Lines").Where("idempotency_key = ?", idempotencyKey).First(&existing).Error; err == nil {
			return c.JSON(http.StatusOK, trimDocument(existing, hasCostView(c)))
		}
	}
	current := auth.GetCurrentUser(c)
	doc := model.InventoryDocument{
		Code:           req.Code,
		Type:           req.Type,
		Status:         documentDraft,
		WarehouseID:    req.WarehouseID,
		ToWarehouseID:  req.ToWarehouseID,
		Reason:         req.Reason,
		IdempotencyKey: idempotencyKey,
	}
	if current != nil {
		doc.CreatedBy = current.ID
	}
	for _, lineReq := range req.Lines {
		if lineReq.Quantity <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "单据数量必须大于 0")
		}
		amount := lineReq.Amount
		if amount == 0 && lineReq.UnitCost > 0 {
			amount = scaledAmount(lineReq.Quantity, lineReq.UnitCost)
		}
		doc.Lines = append(doc.Lines, model.InventoryDocumentLine{
			ItemType:   lineReq.ItemType,
			ItemID:     lineReq.ItemID,
			LocationID: lineReq.LocationID,
			Quantity:   lineReq.Quantity,
			UnitCost:   lineReq.UnitCost,
			Amount:     amount,
			Remark:     lineReq.Remark,
		})
	}
	if err := h.DB.Create(&doc).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, trimDocument(doc, hasCostView(c)))
}

// PostDocument 审核过账库存单据。
func (h *Handler) PostDocument(c echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var doc model.InventoryDocument
	if err := h.DB.Preload("Lines").First(&doc, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "库存单据不存在")
		}
		return err
	}
	if doc.Status == documentPosted {
		return c.JSON(http.StatusOK, trimDocument(doc, hasCostView(c)))
	}
	if doc.Status != documentDraft {
		return echo.NewHTTPError(http.StatusBadRequest, "只有草稿单据可以过账")
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := h.postLines(tx, &doc, false); err != nil {
			return err
		}
		now := time.Now()
		doc.Status = documentPosted
		doc.PostedAt = &now
		if current := auth.GetCurrentUser(c); current != nil {
			doc.PostedBy = &current.ID
		}
		return tx.Save(&doc).Error
	}); err != nil {
		return err
	}
	if err := h.DB.Preload("Lines").First(&doc, id).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, trimDocument(doc, hasCostView(c)))
}

// ReverseDocument 冲销已过账库存单据。
func (h *Handler) ReverseDocument(c echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req struct {
		Reason string `json:"reason" validate:"required"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	var doc model.InventoryDocument
	if err := h.DB.Preload("Lines").First(&doc, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "库存单据不存在")
		}
		return err
	}
	if doc.Status != documentPosted {
		return echo.NewHTTPError(http.StatusBadRequest, "只有已过账单据可以冲销")
	}
	if err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := h.postLines(tx, &doc, true); err != nil {
			return err
		}
		now := time.Now()
		doc.Status = documentReversed
		doc.Reason = req.Reason
		doc.ReversedAt = &now
		if current := auth.GetCurrentUser(c); current != nil {
			doc.ReversedBy = &current.ID
		}
		return tx.Save(&doc).Error
	}); err != nil {
		return err
	}
	if err := h.DB.Preload("Lines").First(&doc, id).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, trimDocument(doc, hasCostView(c)))
}

// ListBalances 查询库存余额。
func (h *Handler) ListBalances(c echo.Context) error {
	var items []model.InventoryBalance
	query := h.DB.Order("id desc")
	if warehouseID := c.QueryParam("warehouse_id"); warehouseID != "" {
		query = query.Where("warehouse_id = ?", warehouseID)
	}
	if err := query.Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, trimBalances(items, hasCostView(c)))
}

// ListLedgers 查询库存流水。
func (h *Handler) ListLedgers(c echo.Context) error {
	var items []model.InventoryLedger
	if err := h.DB.Order("id desc").Limit(500).Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, trimLedgers(items, hasCostView(c)))
}

func (h *Handler) postLines(tx *gorm.DB, doc *model.InventoryDocument, reverse bool) error {
	for _, line := range doc.Lines {
		if err := h.ensureItemExists(tx, line.ItemType, line.ItemID); err != nil {
			return err
		}
		switch doc.Type {
		case typeInbound:
			if err := h.applyIn(tx, doc, line, reverse); err != nil {
				return err
			}
		case typeOutbound:
			if err := h.applyOut(tx, doc, line, reverse); err != nil {
				return err
			}
		case typeTransfer:
			if err := h.applyTransfer(tx, doc, line, reverse); err != nil {
				return err
			}
		default:
			return echo.NewHTTPError(http.StatusBadRequest, "不支持的库存单据类型")
		}
	}
	return nil
}

func (h *Handler) applyIn(tx *gorm.DB, doc *model.InventoryDocument, line model.InventoryDocumentLine, reverse bool) error {
	if reverse {
		return h.moveOut(tx, doc, line, doc.WarehouseID, line.LocationID, typeInbound)
	}
	return h.moveIn(tx, doc, line, doc.WarehouseID, line.LocationID, line.Amount, typeInbound)
}

func (h *Handler) applyOut(tx *gorm.DB, doc *model.InventoryDocument, line model.InventoryDocumentLine, reverse bool) error {
	if reverse {
		return h.moveIn(tx, doc, line, doc.WarehouseID, line.LocationID, line.Amount, typeOutbound)
	}
	return h.moveOut(tx, doc, line, doc.WarehouseID, line.LocationID, typeOutbound)
}

func (h *Handler) applyTransfer(tx *gorm.DB, doc *model.InventoryDocument, line model.InventoryDocumentLine, reverse bool) error {
	if doc.ToWarehouseID == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "调拨单缺少目标仓库")
	}
	fromID, toID := doc.WarehouseID, *doc.ToWarehouseID
	if reverse {
		fromID, toID = toID, fromID
	}
	amount, err := h.moveOutReturningAmount(tx, doc, line, fromID, line.LocationID, typeTransfer)
	if err != nil {
		return err
	}
	return h.moveIn(tx, doc, line, toID, line.LocationID, amount, typeTransfer)
}

func (h *Handler) moveIn(tx *gorm.DB, doc *model.InventoryDocument, line model.InventoryDocumentLine, warehouseID uint, locationID *uint, amount int64, typ string) error {
	balance, err := h.findOrCreateBalance(tx, warehouseID, locationID, line.ItemType, line.ItemID)
	if err != nil {
		return err
	}
	if amount == 0 && line.UnitCost > 0 {
		amount = scaledAmount(line.Quantity, line.UnitCost)
	}
	newQty := balance.Quantity + line.Quantity
	newAmount := balance.Amount + amount
	balance.Quantity = newQty
	balance.Amount = newAmount
	if newQty > 0 {
		balance.AvgCost = newAmount * 10000 / newQty
	}
	if err := tx.Save(&balance).Error; err != nil {
		return err
	}
	return h.createLedger(tx, doc, line, warehouseID, locationID, typ, line.Quantity, balance.AvgCost, amount, balance)
}

func (h *Handler) moveOut(tx *gorm.DB, doc *model.InventoryDocument, line model.InventoryDocumentLine, warehouseID uint, locationID *uint, typ string) error {
	_, err := h.moveOutReturningAmount(tx, doc, line, warehouseID, locationID, typ)
	return err
}

func (h *Handler) moveOutReturningAmount(tx *gorm.DB, doc *model.InventoryDocument, line model.InventoryDocumentLine, warehouseID uint, locationID *uint, typ string) (int64, error) {
	balance, err := h.findOrCreateBalance(tx, warehouseID, locationID, line.ItemType, line.ItemID)
	if err != nil {
		return 0, err
	}
	if balance.Quantity < line.Quantity {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "库存不足，默认禁止负库存")
	}
	amount := scaledAmount(line.Quantity, balance.AvgCost)
	balance.Quantity -= line.Quantity
	balance.Amount -= amount
	if balance.Quantity == 0 {
		balance.AvgCost = 0
		balance.Amount = 0
	}
	if err := tx.Save(&balance).Error; err != nil {
		return 0, err
	}
	return amount, h.createLedger(tx, doc, line, warehouseID, locationID, typ, -line.Quantity, balance.AvgCost, -amount, balance)
}

func (h *Handler) findOrCreateBalance(tx *gorm.DB, warehouseID uint, locationID *uint, itemType string, itemID uint) (model.InventoryBalance, error) {
	var balance model.InventoryBalance
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("warehouse_id = ? AND item_type = ? AND item_id = ?", warehouseID, itemType, itemID)
	if locationID == nil {
		query = query.Where("location_id IS NULL")
	} else {
		query = query.Where("location_id = ?", *locationID)
	}
	err := query.First(&balance).Error
	if err == nil {
		return balance, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return balance, err
	}
	balance = model.InventoryBalance{WarehouseID: warehouseID, LocationID: locationID, ItemType: itemType, ItemID: itemID}
	return balance, tx.Create(&balance).Error
}

func (h *Handler) createLedger(tx *gorm.DB, doc *model.InventoryDocument, line model.InventoryDocumentLine, warehouseID uint, locationID *uint, typ string, qty int64, unitCost int64, amount int64, balance model.InventoryBalance) error {
	ledger := model.InventoryLedger{
		DocumentID:  doc.ID,
		LineID:      line.ID,
		Type:        typ,
		WarehouseID: warehouseID,
		LocationID:  locationID,
		ItemType:    line.ItemType,
		ItemID:      line.ItemID,
		Quantity:    qty,
		UnitCost:    unitCost,
		Amount:      amount,
		BalanceQty:  balance.Quantity,
		BalanceAmt:  balance.Amount,
	}
	return tx.Create(&ledger).Error
}

func (h *Handler) ensureItemExists(tx *gorm.DB, itemType string, itemID uint) error {
	switch itemType {
	case itemMaterial:
		var item model.Material
		if err := tx.First(&item, itemID).Error; err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "物料不存在")
		}
	case itemProduct:
		var item model.Product
		if err := tx.First(&item, itemID).Error; err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "产品不存在")
		}
	default:
		return fmt.Errorf("unsupported item type %s", itemType)
	}
	return nil
}

func scaledAmount(quantity int64, unitCost int64) int64 {
	return quantity * unitCost / 10000
}

func hasCostView(c echo.Context) bool {
	current := auth.GetCurrentUser(c)
	if current == nil {
		return false
	}
	for _, permission := range current.Permissions {
		if permission == role.CostViewCode {
			return true
		}
	}
	return false
}

func trimDocuments(items []model.InventoryDocument, showCost bool) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, trimDocument(item, showCost))
	}
	return result
}

func trimDocument(item model.InventoryDocument, showCost bool) map[string]any {
	lines := make([]map[string]any, 0, len(item.Lines))
	for _, line := range item.Lines {
		row := map[string]any{
			"id":          line.ID,
			"document_id": line.DocumentID,
			"item_type":   line.ItemType,
			"item_id":     line.ItemID,
			"location_id": line.LocationID,
			"quantity":    line.Quantity,
			"remark":      line.Remark,
		}
		if showCost {
			row["unit_cost"] = line.UnitCost
			row["amount"] = line.Amount
		}
		lines = append(lines, row)
	}
	return map[string]any{
		"id":                   item.ID,
		"created_at":           item.CreatedAt,
		"updated_at":           item.UpdatedAt,
		"code":                 item.Code,
		"type":                 item.Type,
		"status":               item.Status,
		"warehouse_id":         item.WarehouseID,
		"to_warehouse_id":      item.ToWarehouseID,
		"reason":               item.Reason,
		"business_type":        item.BusinessType,
		"supplier_id":          item.SupplierID,
		"customer_id":          item.CustomerID,
		"department_id":        item.DepartmentID,
		"original_document_id": item.OriginalDocumentID,
		"posted_at":            item.PostedAt,
		"reversed_at":          item.ReversedAt,
		"lines":                lines,
	}
}

func trimBalances(items []model.InventoryBalance, showCost bool) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row := map[string]any{
			"id":           item.ID,
			"warehouse_id": item.WarehouseID,
			"location_id":  item.LocationID,
			"item_type":    item.ItemType,
			"item_id":      item.ItemID,
			"quantity":     item.Quantity,
		}
		if showCost {
			row["avg_cost"] = item.AvgCost
			row["amount"] = item.Amount
		}
		result = append(result, row)
	}
	return result
}

func trimLedgers(items []model.InventoryLedger, showCost bool) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row := map[string]any{
			"id":           item.ID,
			"document_id":  item.DocumentID,
			"line_id":      item.LineID,
			"type":         item.Type,
			"warehouse_id": item.WarehouseID,
			"location_id":  item.LocationID,
			"item_type":    item.ItemType,
			"item_id":      item.ItemID,
			"quantity":     item.Quantity,
			"balance_qty":  item.BalanceQty,
		}
		if showCost {
			row["unit_cost"] = item.UnitCost
			row["amount"] = item.Amount
			row["balance_amount"] = item.BalanceAmt
		}
		result = append(result, row)
	}
	return result
}
