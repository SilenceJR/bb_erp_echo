// Package warehouse 负责默认仓库、库位和产品数量统计。
package warehouse

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/operator"
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

const itemTypeProduct = "product"

// Handler 处理仓库、库位和产品库存数量。
type Handler struct{ DB *gorm.DB }

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

type warehouseInput struct {
	Name               string `json:"name" validate:"required"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required"`
}

type locationInput struct {
	Code               string `json:"code" validate:"required"`
	Name               string `json:"name" validate:"required"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required"`
}

type ProductStockItem struct {
	ProductID     uint   `json:"product_id"`
	ProductModel  string `json:"product_model"`
	Status        string `json:"status"`
	Quantity      int64  `json:"quantity"`
	LocationCount int64  `json:"location_count"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type ProductStockPage struct {
	Items    []ProductStockItem `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Keyword  string             `json:"keyword,omitempty"`
}

type StockBalance struct {
	LocationID   uint   `json:"location_id"`
	LocationCode string `json:"location_code"`
	Quantity     int64  `json:"quantity"`
	UpdatedAt    string `json:"updated_at"`
}

type StockMovement struct {
	ID              uint   `json:"id"`
	DocumentCode    string `json:"document_code"`
	Action          string `json:"action"`
	Quantity        int64  `json:"quantity"`
	LocationID      uint   `json:"location_id"`
	LocationCode    string `json:"location_code"`
	BalanceQuantity int64  `json:"balance_quantity"`
	Reason          string `json:"reason"`
	OperatorName    string `json:"operator_name"`
	CreatedAt       string `json:"created_at"`
}

type movementInput struct {
	Action             string `json:"action" validate:"required,oneof=inbound outbound transfer adjustment"`
	Quantity           int64  `json:"quantity"`
	LocationID         uint   `json:"location_id"`
	FromLocationID     uint   `json:"from_location_id"`
	ToLocationID       uint   `json:"to_location_id"`
	TargetQuantity     int64  `json:"target_quantity"`
	Reason             string `json:"reason" validate:"required,max=255"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required"`
}

// NewHandler 创建仓库模块处理器。
func NewHandler(db *gorm.DB) *Handler { return &Handler{DB: db} }

// SeedDefaults creates the single default warehouse required by product stock.
func SeedDefaults(db *gorm.DB) error {
	warehouse := model.Warehouse{Name: "主仓库", Code: model.DefaultWarehouseCode, Status: model.StatusActive}
	return db.Where("code = ?", warehouse.Code).FirstOrCreate(&warehouse).Error
}

// RegisterRoutes 注册仓库、库位和产品库存路由。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	warehouses := v1.Group("/warehouses", audit)
	warehouses.GET("", h.ListWarehouses, require("/api/v1/warehouse", "read"))
	warehouses.POST("", h.UpdateWarehouse, require("/api/v1/warehouse", "write"))

	products := v1.Group("/warehouse/products", audit)
	products.GET("", h.ListProductStock, require("/api/v1/warehouse", "read"))
	products.GET("/:id", h.GetProductStock, require("/api/v1/warehouse", "read"))
	products.GET("/:id/movements", h.ListMovements, require("/api/v1/warehouse", "read"))
	products.POST("/:id/movements", h.CreateMovement, require("/api/v1/warehouse", "write"))

	locations := v1.Group("/locations", audit)
	locations.GET("", h.ListLocations, require("/api/v1/warehouse", "read"))
	locations.POST("", h.CreateLocation, require("/api/v1/warehouse", "write"))
}

// ListWarehouses 查询默认仓库。
// @Summary 查询仓库
// @Tags warehouse
// @Security BearerAuth
// @Produce json
// @Success 200 {array} model.Warehouse
// @Router /api/v1/warehouses [get]
func (h *Handler) ListWarehouses(c *echo.Context) error {
	item, err := h.defaultWarehouse()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, []model.Warehouse{item})
}

// UpdateWarehouse 更新默认仓库名称。
// @Summary 更新仓库
// @Tags warehouse
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body warehouseInput true "仓库参数"
// @Success 200 {object} model.Warehouse
// @Router /api/v1/warehouses [post]
func (h *Handler) UpdateWarehouse(c *echo.Context) error {
	var input warehouseInput
	if err := request.BindAndValidate(c, &input); err != nil {
		return err
	}
	var item model.Warehouse
	err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, input.OperatorEmployeeID); err != nil {
			return err
		}
		if err := tx.Where("code = ?", model.DefaultWarehouseCode).First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return echo.NewHTTPError(http.StatusServiceUnavailable, "默认仓库未初始化")
			}
			return err
		}
		item.Name = strings.TrimSpace(input.Name)
		return tx.Save(&item).Error
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, item)
}

// ListProductStock 查询产品数量统计。
// @Summary 查询产品库存
// @Tags warehouse
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页条数"
// @Param q query string false "产品型号"
// @Success 200 {object} ProductStockPage
// @Router /api/v1/warehouse/products [get]
func (h *Handler) ListProductStock(c *echo.Context) error {
	page := pagination.FromEcho(c)
	db := h.DB.Model(&model.Product{})
	if page.Keyword != "" {
		db = pagination.ApplyKeyword(db, page.Keyword, "product_model")
	}
	result, err := pagination.Page[model.Product](db, page, "id desc", nil)
	if err != nil {
		return err
	}
	items := make([]ProductStockItem, 0, len(result.Items))
	for _, product := range result.Items {
		item := ProductStockItem{ProductID: product.ID, ProductModel: product.ProductModel, Status: product.Status, UpdatedAt: product.UpdatedAt.Format(time.RFC3339)}
		var aggregate struct {
			Quantity int64
			Count    int64
		}
		if err := h.DB.Model(&model.InventoryBalance{}).
			Select("COALESCE(SUM(quantity), 0) AS quantity, COUNT(DISTINCT location_id) AS count").
			Where("item_type = ? AND item_id = ?", itemTypeProduct, product.ID).Scan(&aggregate).Error; err != nil {
			return err
		}
		item.Quantity, item.LocationCount = aggregate.Quantity, aggregate.Count
		items = append(items, item)
	}
	return c.JSON(http.StatusOK, ProductStockPage{Items: items, Total: result.Total, Page: result.Page, PageSize: result.PageSize, Keyword: result.Keyword})
}

// GetProductStock 查询产品分库位数量和最近流水。
// @Summary 查询产品库存详情
// @Tags warehouse
// @Security BearerAuth
// @Produce json
// @Param id path int true "产品 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/warehouse/products/{id} [get]
func (h *Handler) GetProductStock(c *echo.Context) error {
	product, err := h.productByID(c.Param("id"))
	if err != nil {
		return err
	}
	var balances []StockBalance
	if err := h.DB.Table("inventory_balances").
		Select("inventory_balances.location_id, COALESCE(locations.code, '') AS location_code, inventory_balances.quantity, inventory_balances.updated_at").
		Joins("LEFT JOIN locations ON locations.id = inventory_balances.location_id").
		Where("inventory_balances.item_type = ? AND inventory_balances.item_id = ?", itemTypeProduct, product.ID).
		Order("inventory_balances.location_id asc").Scan(&balances).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"product": product, "balances": balances})
}

// ListMovements 查询产品库存流水。
// @Summary 查询产品库存流水
// @Tags warehouse
// @Security BearerAuth
// @Produce json
// @Param id path int true "产品 ID"
// @Success 200 {array} StockMovement
// @Router /api/v1/warehouse/products/{id}/movements [get]
func (h *Handler) ListMovements(c *echo.Context) error {
	product, err := h.productByID(c.Param("id"))
	if err != nil {
		return err
	}
	var items []StockMovement
	if err := h.DB.Table("inventory_ledgers").
		Select("inventory_ledgers.id, inventory_documents.code AS document_code, inventory_ledgers.type AS action, inventory_ledgers.quantity, COALESCE(inventory_ledgers.location_id, 0) AS location_id, COALESCE(locations.code, '') AS location_code, inventory_ledgers.balance_qty AS balance_quantity, inventory_documents.reason, inventory_documents.created_by_employee_name AS operator_name, inventory_ledgers.created_at").
		Joins("JOIN inventory_documents ON inventory_documents.id = inventory_ledgers.document_id").
		Joins("LEFT JOIN locations ON locations.id = inventory_ledgers.location_id").
		Where("inventory_ledgers.item_type = ? AND inventory_ledgers.item_id = ?", itemTypeProduct, product.ID).
		Order("inventory_ledgers.id desc").Limit(200).Scan(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// CreateMovement 创建产品数量操作。
// @Summary 创建产品数量操作
// @Tags warehouse
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "产品 ID"
// @Param body body movementInput true "数量操作"
// @Success 201 {object} model.InventoryDocument
// @Router /api/v1/warehouse/products/{id}/movements [post]
func (h *Handler) CreateMovement(c *echo.Context) error {
	product, err := h.productByID(c.Param("id"))
	if err != nil {
		return err
	}
	var input movementInput
	if err := request.BindAndValidate(c, &input); err != nil {
		return err
	}
	input.Reason = strings.TrimSpace(input.Reason)
	var document model.InventoryDocument
	err = h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		identity, err := operator.Resolve(c, tx, input.OperatorEmployeeID)
		if err != nil {
			return err
		}
		warehouse, err := h.defaultWarehouseDB(tx)
		if err != nil {
			return err
		}
		now := time.Now()
		document = model.InventoryDocument{
			Code: fmt.Sprintf("STK-%s-%d", now.Format("20060102"), now.UnixNano()), Type: input.Action,
			Status: "posted", WarehouseID: warehouse.ID, BusinessType: input.Action, Reason: input.Reason,
			CreatedByEmployeeID: &identity.EmployeeID, CreatedByEmployeeName: identity.EmployeeName,
			CreatedByDepartmentID: &identity.DepartmentID, CreatedByDepartmentName: identity.DepartmentName,
			PostedAt: &now,
		}
		if current := auth.GetCurrentUser(c); current != nil {
			document.CreatedBy = current.ID
			document.CreatedByTerminalID = current.TerminalID
		}
		if err := tx.Create(&document).Error; err != nil {
			return err
		}
		switch input.Action {
		case "inbound":
			if input.LocationID == 0 || input.Quantity <= 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "入库必须选择库位并填写大于 0 的数量")
			}
			return h.applyDelta(tx, product, input.LocationID, input.Quantity, &document, input.Reason)
		case "outbound":
			if input.LocationID == 0 || input.Quantity <= 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "出库必须选择库位并填写大于 0 的数量")
			}
			return h.applyDelta(tx, product, input.LocationID, -input.Quantity, &document, input.Reason)
		case "transfer":
			if input.FromLocationID == 0 || input.ToLocationID == 0 || input.FromLocationID == input.ToLocationID || input.Quantity <= 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "库位调整必须选择不同的来源和目标库位，并填写大于 0 的数量")
			}
			if err := h.applyDelta(tx, product, input.FromLocationID, -input.Quantity, &document, input.Reason); err != nil {
				return err
			}
			return h.applyDelta(tx, product, input.ToLocationID, input.Quantity, &document, input.Reason)
		case "adjustment":
			if input.LocationID == 0 || input.TargetQuantity < 0 {
				return echo.NewHTTPError(http.StatusBadRequest, "盘点修正必须选择库位并填写非负目标数量")
			}
			var balance model.InventoryBalance
			if err := tx.Where("item_type = ? AND item_id = ? AND location_id = ?", itemTypeProduct, product.ID, input.LocationID).First(&balance).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			delta := input.TargetQuantity - balance.Quantity
			if delta == 0 {
				return nil
			}
			return h.applyDelta(tx, product, input.LocationID, delta, &document, input.Reason)
		default:
			return echo.NewHTTPError(http.StatusBadRequest, "不支持的数量操作")
		}
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, document)
}

// ListLocations 查询默认仓库库位。
// @Summary 查询仓库库位
// @Tags warehouse
// @Security BearerAuth
// @Produce json
// @Success 200 {array} model.Location
// @Router /api/v1/locations [get]
func (h *Handler) ListLocations(c *echo.Context) error {
	warehouse, err := h.defaultWarehouse()
	if err != nil {
		return err
	}
	var items []model.Location
	if err := h.DB.Where("warehouse_id = ?", warehouse.ID).Order("code asc, id asc").Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// CreateLocation 创建默认仓库库位。
// @Summary 创建仓库库位
// @Tags warehouse
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body locationInput true "库位参数"
// @Success 201 {object} model.Location
// @Router /api/v1/locations [post]
func (h *Handler) CreateLocation(c *echo.Context) error {
	var input locationInput
	if err := request.BindAndValidate(c, &input); err != nil {
		return err
	}
	var item model.Location
	err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, input.OperatorEmployeeID); err != nil {
			return err
		}
		warehouse, err := h.defaultWarehouseDB(tx)
		if err != nil {
			return err
		}
		item = model.Location{WarehouseID: warehouse.ID, Code: strings.TrimSpace(input.Code), Name: strings.TrimSpace(input.Name), Status: model.StatusActive, OperatorSnapshot: operator.Snapshot(c)}
		return tx.Create(&item).Error
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *Handler) applyDelta(tx *gorm.DB, product model.Product, locationID uint, delta int64, document *model.InventoryDocument, reason string) error {
	if product.Status != model.StatusActive {
		return echo.NewHTTPError(http.StatusBadRequest, "产品资料已停用，不能办理库存操作")
	}
	var location model.Location
	if err := tx.Where("id = ? AND status = ?", locationID, model.StatusActive).First(&location).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return echo.NewHTTPError(http.StatusBadRequest, "库存库位不存在或已停用")
		}
		return err
	}
	warehouse, err := h.defaultWarehouseDB(tx)
	if err != nil {
		return err
	}
	var balance model.InventoryBalance
	err = tx.Where("warehouse_id = ? AND location_id = ? AND item_type = ? AND item_id = ?", warehouse.ID, locationID, itemTypeProduct, product.ID).First(&balance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		balance = model.InventoryBalance{WarehouseID: warehouse.ID, LocationID: &locationID, ItemType: itemTypeProduct, ItemID: product.ID}
	} else if err != nil {
		return err
	}
	if balance.Quantity+delta < 0 {
		return echo.NewHTTPError(http.StatusConflict, "库存数量不足")
	}
	balance.Quantity += delta
	if balance.ID == 0 {
		if err := tx.Create(&balance).Error; err != nil {
			return err
		}
	} else if err := tx.Save(&balance).Error; err != nil {
		return err
	}
	line := model.InventoryDocumentLine{DocumentID: document.ID, ItemType: itemTypeProduct, ItemID: product.ID, LocationID: &locationID, Quantity: delta, Remark: reason}
	if err := tx.Create(&line).Error; err != nil {
		return err
	}
	ledger := model.InventoryLedger{DocumentID: document.ID, LineID: line.ID, Type: document.Type, WarehouseID: warehouse.ID, LocationID: &locationID, ItemType: itemTypeProduct, ItemID: product.ID, Quantity: delta, BalanceQty: balance.Quantity}
	return tx.Create(&ledger).Error
}

func (h *Handler) productByID(raw string) (model.Product, error) {
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		return model.Product{}, echo.NewHTTPError(http.StatusBadRequest, "产品 ID 无效")
	}
	var item model.Product
	if err := h.DB.First(&item, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, echo.NewHTTPError(http.StatusNotFound, "产品资料不存在")
		}
		return item, err
	}
	return item, nil
}

func (h *Handler) defaultWarehouse() (model.Warehouse, error) { return h.defaultWarehouseDB(h.DB) }
func (h *Handler) defaultWarehouseDB(db *gorm.DB) (model.Warehouse, error) {
	var item model.Warehouse
	if err := db.Where("code = ?", model.DefaultWarehouseCode).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, echo.NewHTTPError(http.StatusServiceUnavailable, "默认仓库未初始化")
		}
		return item, err
	}
	return item, nil
}
