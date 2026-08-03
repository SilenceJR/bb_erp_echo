// Package warehouse 负责仓库和库位基础资料接口。
package warehouse

import (
	"net/http"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/role"
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Handler 处理仓库和库位基础资料。
type Handler struct {
	// DB 是仓库和库位读写数据库连接。
	DB *gorm.DB
}

// NewHandler 创建仓库模块接口处理器。
func NewHandler(db *gorm.DB) *Handler {
	return &Handler{DB: db}
}

// RegisterRoutes 注册仓库模块路由。
func (h *Handler) RegisterRoutes(v1 *echo.Group, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	warehouses := v1.Group("/warehouses", audit)
	warehouses.GET("", h.ListWarehouses, require("/api/v1/warehouse", "read"))
	warehouses.POST("", h.CreateWarehouse, require("/api/v1/warehouse", "write"))

	legacy := v1.Group("/warehouse", audit)
	legacy.GET("", h.ListWarehouses, require("/api/v1/warehouse", "read"))
	legacy.POST("", h.CreateWarehouse, require("/api/v1/warehouse", "write"))
	legacy.GET("/tabs", h.ListTabs, require("/api/v1/warehouse", "read"))
	legacy.GET("/items", h.ListItems, require("/api/v1/warehouse", "read"))
	legacy.POST("/items", h.CreateItem, require("/api/v1/warehouse", "write"))

	locations := v1.Group("/locations", audit)
	locations.GET("", h.ListLocations, require("/api/v1/warehouse", "read"))
	locations.POST("", h.CreateLocation, require("/api/v1/warehouse", "write"))
}

// ListWarehouses 查询单仓库信息。
func (h *Handler) ListWarehouses(c *echo.Context) error {
	item, err := h.ensureDefaultWarehouse()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, []model.Warehouse{item})
}

// CreateWarehouse 更新单仓库名称和编码。
func (h *Handler) CreateWarehouse(c *echo.Context) error {
	var req struct {
		Name string `json:"name" validate:"required"`
		Code string `json:"code" validate:"required"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	item, err := h.ensureDefaultWarehouse()
	if err != nil {
		return err
	}
	item.Name = req.Name
	item.Code = req.Code
	if err := h.DB.Save(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, item)
}

// ListTabs 查询单仓库内的分类标签。
func (h *Handler) ListTabs(c *echo.Context) error {
	return c.JSON(http.StatusOK, catalogTabs)
}

// ListItems 查询某个仓库标签下的物品。
func (h *Handler) ListItems(c *echo.Context) error {
	tab := c.QueryParam("tab")
	if tab == "" {
		tab = tabProduct
	}
	result, err := listCatalogItems(h.DB, tab, pagination.FromEcho(c))
	if err != nil {
		return err
	}
	warehouse, err := h.ensureDefaultWarehouse()
	if err != nil {
		return err
	}
	showCost := warehouseCostView(c)
	for index := range result.Items {
		var aggregate struct {
			Quantity int64
			Amount   int64
		}
		if err := h.DB.Model(&model.InventoryBalance{}).
			Select("COALESCE(SUM(quantity), 0) AS quantity, COALESCE(SUM(amount), 0) AS amount").
			Where("warehouse_id = ? AND item_type = ? AND item_id = ?", warehouse.ID, result.Items[index].ItemType, result.Items[index].ID).
			Scan(&aggregate).Error; err != nil {
			return err
		}
		result.Items[index].Quantity = aggregate.Quantity
		if showCost {
			result.Items[index].Amount = aggregate.Amount
			if aggregate.Quantity > 0 {
				result.Items[index].AvgCost = aggregate.Amount * 10000 / aggregate.Quantity
			}
		} else {
			result.Items[index].DefaultCost = 0
		}
	}
	return c.JSON(http.StatusOK, result)
}

// CreateItem 在仓库标签下创建物品。
func (h *Handler) CreateItem(c *echo.Context) error {
	var req CatalogItemInput
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	item, err := createCatalogItem(h.DB, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

// ListLocations 查询库位列表。
func (h *Handler) ListLocations(c *echo.Context) error {
	var items []model.Location
	query := h.DB.Order("id desc")
	if warehouseID := c.QueryParam("warehouse_id"); warehouseID != "" {
		query = query.Where("warehouse_id = ?", warehouseID)
	}
	if err := query.Find(&items).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusOK, items)
}

// CreateLocation 创建库位。
func (h *Handler) CreateLocation(c *echo.Context) error {
	var req struct {
		WarehouseID uint   `json:"warehouse_id" validate:"required"`
		Code        string `json:"code" validate:"required"`
		Name        string `json:"name" validate:"required"`
	}
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	warehouse, err := h.ensureDefaultWarehouse()
	if err != nil {
		return err
	}
	if req.WarehouseID != warehouse.ID {
		return echo.NewHTTPError(http.StatusNotFound, "仓库不存在")
	}
	item := model.Location{WarehouseID: req.WarehouseID, Code: req.Code, Name: req.Name, Status: model.StatusActive}
	if err := h.DB.Create(&item).Error; err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *Handler) ensureDefaultWarehouse() (model.Warehouse, error) {
	item := model.Warehouse{Name: "默认仓库", Code: "MAIN", Status: model.StatusActive}
	err := h.DB.FirstOrCreate(&item, model.Warehouse{Code: item.Code}).Error
	return item, err
}

func warehouseCostView(c *echo.Context) bool {
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
