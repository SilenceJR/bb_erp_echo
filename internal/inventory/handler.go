// Package inventory 负责库存单据、余额和流水接口。
package inventory

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/moduleavailability"
	"bb_erp_echo/internal/operator"
	"bb_erp_echo/internal/role"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
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

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

// NewHandler 创建库存模块接口处理器。
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db, movementStrategies: DefaultMovementStrategies()}
}

// RegisterRoutes 注册库存模块路由。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	deferred := moduleavailability.Middleware(h.DB, "库存", inventoryModuleRequirements()...)
	docs := v1.Group("/inventory-documents", audit)
	docs.GET("", h.ListDocuments, require("/api/v1/inventory-documents", "read"), deferred)
	docs.POST("", h.CreateDocument, require("/api/v1/inventory-documents", "write"), deferred)
	docs.POST("/:id/post", h.PostDocument, require("/api/v1/inventory-documents", "write"), deferred)
	docs.POST("/:id/reverse", h.ReverseDocument, require("/api/v1/inventory-documents", "write"), deferred)

	v1.GET("/inventory-balances", h.ListBalances, require("/api/v1/inventory-balances", "read"), deferred)
	v1.GET("/inventory-ledgers", h.ListLedgers, require("/api/v1/inventory-ledgers", "read"), deferred)

	items := v1.Group("/warehouse/items")
	items.GET("/:itemType/:itemID", h.GetItemDetail, require("/api/v1/warehouse", "read"), deferred)
	items.GET("/:itemType/:itemID/movements", h.ListItemMovements, require("/api/v1/inventory-documents", "read"), deferred)
	items.POST("/:itemType/:itemID/movements", h.CreateItemMovement, audit, require("/api/v1/inventory-documents", "write"), deferred)
}

func inventoryModuleRequirements() []moduleavailability.Requirement {
	return []moduleavailability.Requirement{
		{Model: &model.Supplier{}, Name: "suppliers"},
		{Model: &model.Warehouse{}, Name: "warehouses"},
		{Model: &model.Location{}, Name: "locations"},
		{Model: &model.InventoryDocument{}, Name: "inventory_documents"},
		{Model: &model.InventoryDocumentLine{}, Name: "inventory_document_lines"},
		{Model: &model.InventoryBalance{}, Name: "inventory_balances"},
		{Model: &model.InventoryLedger{}, Name: "inventory_ledgers"},
	}
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

type operatorActionRequest struct {
	OperatorEmployeeID uint `json:"operator_employee_id" validate:"required" example:"1"`
}

type documentRequest struct {
	Code               string        `json:"code" validate:"required"`
	Type               string        `json:"type" validate:"required,oneof=inbound outbound transfer"`
	WarehouseID        uint          `json:"warehouse_id" validate:"required"`
	ToWarehouseID      *uint         `json:"to_warehouse_id"`
	Reason             string        `json:"reason"`
	Lines              []lineRequest `json:"lines" validate:"required,min=1,dive"`
	OperatorEmployeeID uint          `json:"operator_employee_id" validate:"required" example:"1"`
}

type reverseDocumentRequest struct {
	Reason             string `json:"reason" validate:"required"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required" example:"1"`
}

// ListDocuments 查询库存单据。
// @Summary 查询库存单据
// @Description 返回库存单据及其明细；成本字段会按当前账号权限脱敏。
// @Tags inventory
// @Security BearerAuth
// @Produce json
// @Success 200 {array} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/inventory-documents [get]
func (h *Handler) ListDocuments(c *echo.Context) error {
	var items []model.InventoryDocument
	if err := h.DB.Order("id desc").Preload("Lines").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, trimDocuments(items, hasCostView(c)))
}

// CreateDocument 创建库存草稿单据。
// @Summary 创建库存草稿单据
// @Description 创建库存草稿单据；必须选择当前账号部门下的在职员工作为本次操作人。相同 Idempotency-Key 重试会返回首次创建结果。
// @Tags inventory
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param Idempotency-Key header string false "幂等键；相同键重试返回首次创建结果"
// @Param body body documentRequest true "库存单据参数"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/inventory-documents [post]
func (h *Handler) CreateDocument(c *echo.Context) error {
	var req documentRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "请求 JSON 格式错误")
	}
	idempotencyKey := normalizeIdempotencyKey(c.Request().Header.Get("Idempotency-Key"))
	if idempotencyKey != "" && auth.GetCurrentUser(c) == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	metadata := newIdempotencyMetadata(c, idempotencyScopeCreateDocument, documentRequestHash(req))
	var doc model.InventoryDocument
	duplicate := false
	err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		// 操作员工校验必须先于幂等重放判断，并与最终业务写入使用同一
		// 事务；否则已失效的成员关系可借重试路径绕过校验。
		if _, err := operator.Resolve(c, tx, req.OperatorEmployeeID); err != nil {
			return err
		}
		if idempotencyKey != "" {
			existing, found, err := findIdempotentDocument(tx, idempotencyKey, metadata)
			if err != nil {
				return err
			}
			if found {
				doc = existing
				duplicate = true
				return nil
			}
		}
		if err := c.Validate(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "请求参数校验失败")
		}
		if req.Type == typeTransfer && req.ToWarehouseID == nil {
			return echo.NewHTTPError(http.StatusBadRequest, "调拨单必须填写目标仓库")
		}
		identity, err := operator.Get(c)
		if !err {
			return echo.NewHTTPError(http.StatusInternalServerError, "操作员工校验状态丢失")
		}
		current := auth.GetCurrentUser(c)
		doc = model.InventoryDocument{
			Code:           req.Code,
			Type:           req.Type,
			Status:         documentDraft,
			WarehouseID:    req.WarehouseID,
			ToWarehouseID:  req.ToWarehouseID,
			Reason:         req.Reason,
			IdempotencyKey: idempotencyKey,
		}
		if idempotencyKey != "" {
			doc.IdempotencyScope = metadata.scope
			doc.IdempotencyAccountID = metadata.accountID
			doc.IdempotencyOrganizationID = metadata.organizationID
			doc.IdempotencyRequestHash = metadata.requestHash
		}
		if current != nil {
			doc.CreatedBy = current.ID
		}
		if identity.EmployeeID != 0 {
			doc.CreatedByEmployeeID = &identity.EmployeeID
			doc.CreatedByEmployeeName = identity.EmployeeName
			doc.CreatedByDepartmentID = &identity.DepartmentID
			doc.CreatedByDepartmentName = identity.DepartmentName
		}
		if current != nil {
			doc.CreatedByTerminalID = current.TerminalID
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
		if err := tx.Create(&doc).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if idempotencyKey != "" && isUniqueConstraintError(err) {
			existing, found, lookupErr := findIdempotentDocument(h.DB.WithContext(c.Request().Context()), idempotencyKey, metadata)
			if lookupErr != nil {
				return lookupErr
			}
			if found {
				return idempotencyConflictResponse(c, existing)
			}
		}
		return err
	}
	if duplicate {
		return c.JSON(http.StatusOK, trimDocument(doc, hasCostView(c)))
	}
	return c.JSON(http.StatusCreated, trimDocument(doc, hasCostView(c)))
}

// PostDocument 审核过账库存单据。
// @Summary 审核过账库存单据
// @Description 将草稿单据过账并写入库存余额和流水；必须选择当前账号部门下的在职员工。
// @Tags inventory
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "库存单据 ID"
// @Param body body operatorActionRequest true "操作人参数"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/inventory-documents/{id}/post [post]
func (h *Handler) PostDocument(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req operatorActionRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	var doc model.InventoryDocument
	alreadyPosted := false
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		identity, err := operator.Resolve(c, tx, req.OperatorEmployeeID)
		if err != nil {
			return err
		}
		if err := tx.Preload("Lines").First(&doc, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "库存单据不存在")
			}
			return err
		}
		if doc.Status == documentPosted {
			alreadyPosted = true
			return nil
		}
		if doc.Status != documentDraft {
			return echo.NewHTTPError(http.StatusBadRequest, "只有草稿单据可以过账")
		}
		now := time.Now()
		updates := map[string]any{
			"status":                    documentPosted,
			"posted_at":                 now,
			"posted_by_employee_id":     &identity.EmployeeID,
			"posted_by_employee_name":   identity.EmployeeName,
			"posted_by_department_id":   &identity.DepartmentID,
			"posted_by_department_name": identity.DepartmentName,
		}
		if current := auth.GetCurrentUser(c); current != nil {
			updates["posted_by"] = current.ID
			updates["posted_by_terminal_id"] = current.TerminalID
		}
		result := tx.Model(&model.InventoryDocument{}).
			Where("id = ? AND status = ?", doc.ID, documentDraft).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			var currentDoc model.InventoryDocument
			if err := tx.First(&currentDoc, doc.ID).Error; err != nil {
				return err
			}
			if currentDoc.Status == documentPosted {
				doc = currentDoc
				alreadyPosted = true
				return nil
			}
			return echo.NewHTTPError(http.StatusConflict, "库存单据状态已变化，请刷新后重试")
		}
		doc.Status = documentPosted
		doc.PostedAt = &now
		doc.PostedByEmployeeID = &identity.EmployeeID
		doc.PostedByEmployeeName = identity.EmployeeName
		doc.PostedByDepartmentID = &identity.DepartmentID
		doc.PostedByDepartmentName = identity.DepartmentName
		if current := auth.GetCurrentUser(c); current != nil {
			doc.PostedBy = &current.ID
			doc.PostedByTerminalID = current.TerminalID
		}
		if err := h.postLines(tx, &doc, false); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if err := h.DB.Preload("Lines").First(&doc, id).Error; err != nil {
		return err
	}
	_ = alreadyPosted
	return c.JSON(http.StatusOK, trimDocument(doc, hasCostView(c)))
}

// ReverseDocument 冲销已过账库存单据。
// @Summary 冲销已过账库存单据
// @Description 冲销已过账单据并回滚库存余额和流水；必须选择当前账号部门下的在职员工。
// @Tags inventory
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "库存单据 ID"
// @Param body body reverseDocumentRequest true "冲销参数"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/inventory-documents/{id}/reverse [post]
func (h *Handler) ReverseDocument(c *echo.Context) error {
	id, err := request.ParamID(c)
	if err != nil {
		return err
	}
	var req reverseDocumentRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	var doc model.InventoryDocument
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		identity, err := operator.Resolve(c, tx, req.OperatorEmployeeID)
		if err != nil {
			return err
		}
		if err := tx.Preload("Lines").First(&doc, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "库存单据不存在")
			}
			return err
		}
		if doc.Status == documentReversed {
			return nil
		}
		if doc.Status != documentPosted {
			return echo.NewHTTPError(http.StatusBadRequest, "只有已过账单据可以冲销")
		}
		now := time.Now()
		updates := map[string]any{
			"status":                      documentReversed,
			"reason":                      req.Reason,
			"reversed_at":                 now,
			"reversed_by_employee_id":     &identity.EmployeeID,
			"reversed_by_employee_name":   identity.EmployeeName,
			"reversed_by_department_id":   &identity.DepartmentID,
			"reversed_by_department_name": identity.DepartmentName,
		}
		if current := auth.GetCurrentUser(c); current != nil {
			updates["reversed_by"] = current.ID
			updates["reversed_by_terminal_id"] = current.TerminalID
		}
		result := tx.Model(&model.InventoryDocument{}).
			Where("id = ? AND status = ?", doc.ID, documentPosted).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			var currentDoc model.InventoryDocument
			if err := tx.First(&currentDoc, doc.ID).Error; err != nil {
				return err
			}
			if currentDoc.Status == documentReversed {
				doc = currentDoc
				return nil
			}
			return echo.NewHTTPError(http.StatusConflict, "库存单据状态已变化，请刷新后重试")
		}
		doc.Status = documentReversed
		doc.Reason = req.Reason
		doc.ReversedAt = &now
		doc.ReversedByEmployeeID = &identity.EmployeeID
		doc.ReversedByEmployeeName = identity.EmployeeName
		doc.ReversedByDepartmentID = &identity.DepartmentID
		doc.ReversedByDepartmentName = identity.DepartmentName
		if current := auth.GetCurrentUser(c); current != nil {
			doc.ReversedBy = &current.ID
			doc.ReversedByTerminalID = current.TerminalID
		}
		return h.postLines(tx, &doc, true)
	}); err != nil {
		return err
	}
	if err := h.DB.Preload("Lines").First(&doc, id).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, trimDocument(doc, hasCostView(c)))
}

// ListBalances 查询库存余额。
// @Summary 查询库存余额
// @Tags inventory
// @Security BearerAuth
// @Produce json
// @Param warehouse_id query int false "仓库 ID"
// @Success 200 {array} model.InventoryBalance
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/inventory-balances [get]
func (h *Handler) ListBalances(c *echo.Context) error {
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
// @Summary 查询库存流水
// @Tags inventory
// @Security BearerAuth
// @Produce json
// @Success 200 {array} model.InventoryLedger
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/inventory-ledgers [get]
func (h *Handler) ListLedgers(c *echo.Context) error {
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
	if err := tx.Create(&balance).Error; err != nil {
		if !isUniqueConstraintError(err) {
			return balance, err
		}
		// Another writer may have inserted the same balance after our initial
		// read. Re-read the unique row so the caller can continue updating it.
		if findErr := query.First(&balance).Error; findErr != nil {
			return balance, findErr
		}
	}
	return balance, nil
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

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "unique violation") ||
		strings.Contains(message, "duplicate key")
}

func hasCostView(c *echo.Context) bool {
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
	operatorEmployeeID := item.CreatedByEmployeeID
	operatorEmployeeName := item.CreatedByEmployeeName
	operatorDepartmentID := item.CreatedByDepartmentID
	operatorDepartmentName := item.CreatedByDepartmentName
	if operatorEmployeeID == nil {
		operatorEmployeeID = item.PostedByEmployeeID
		operatorEmployeeName = item.PostedByEmployeeName
		operatorDepartmentID = item.PostedByDepartmentID
		operatorDepartmentName = item.PostedByDepartmentName
	}
	if operatorEmployeeID == nil {
		operatorEmployeeID = item.ReversedByEmployeeID
		operatorEmployeeName = item.ReversedByEmployeeName
		operatorDepartmentID = item.ReversedByDepartmentID
		operatorDepartmentName = item.ReversedByDepartmentName
	}
	return map[string]any{
		"id":                          item.ID,
		"created_at":                  item.CreatedAt,
		"updated_at":                  item.UpdatedAt,
		"code":                        item.Code,
		"type":                        item.Type,
		"status":                      item.Status,
		"warehouse_id":                item.WarehouseID,
		"to_warehouse_id":             item.ToWarehouseID,
		"reason":                      item.Reason,
		"business_type":               item.BusinessType,
		"supplier_id":                 item.SupplierID,
		"customer_id":                 item.CustomerID,
		"department_id":               item.DepartmentID,
		"original_document_id":        item.OriginalDocumentID,
		"created_by":                  item.CreatedBy,
		"created_by_employee_id":      item.CreatedByEmployeeID,
		"created_by_employee_name":    item.CreatedByEmployeeName,
		"created_by_department_id":    item.CreatedByDepartmentID,
		"created_by_department_name":  item.CreatedByDepartmentName,
		"created_by_terminal_id":      item.CreatedByTerminalID,
		"posted_by":                   item.PostedBy,
		"posted_by_employee_id":       item.PostedByEmployeeID,
		"posted_by_employee_name":     item.PostedByEmployeeName,
		"posted_by_department_id":     item.PostedByDepartmentID,
		"posted_by_department_name":   item.PostedByDepartmentName,
		"posted_by_terminal_id":       item.PostedByTerminalID,
		"reversed_by":                 item.ReversedBy,
		"reversed_by_employee_id":     item.ReversedByEmployeeID,
		"reversed_by_employee_name":   item.ReversedByEmployeeName,
		"reversed_by_department_id":   item.ReversedByDepartmentID,
		"reversed_by_department_name": item.ReversedByDepartmentName,
		"reversed_by_terminal_id":     item.ReversedByTerminalID,
		"operator_employee_id":        operatorEmployeeID,
		"operator_employee_name":      operatorEmployeeName,
		"operator_department_id":      operatorDepartmentID,
		"operator_department_name":    operatorDepartmentName,
		"posted_at":                   item.PostedAt,
		"reversed_at":                 item.ReversedAt,
		"lines":                       lines,
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
