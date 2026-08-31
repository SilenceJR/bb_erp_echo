// Package warehouse 负责仓库和库位基础资料接口。
package warehouse

import (
	"errors"
	"net/http"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/operator"
	"bb_erp_echo/internal/role"
	"bb_erp_echo/internal/shared/pagination"
	"bb_erp_echo/internal/shared/request"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Handler 处理仓库和库位基础资料。
type Handler struct {
	// DB 是仓库和库位读写数据库连接。
	DB *gorm.DB
}

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

// createWarehouseRequest 是创建或更新系统默认仓库的请求体。
type createWarehouseRequest struct {
	Name               string `json:"name" validate:"required" example:"主仓库"`
	Code               string `json:"code" example:"MAIN"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required" example:"1"`
}

// createLocationRequest 是创建仓库库位的请求体。
type createLocationRequest struct {
	WarehouseID        uint   `json:"warehouse_id" validate:"required" example:"1"`
	Code               string `json:"code" validate:"required" example:"A-01"`
	Name               string `json:"name" validate:"required" example:"一号库位"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required" example:"1"`
}

// createWarehouseItemRequest 是在仓库标签页下创建物品的请求体。
type createWarehouseItemRequest struct {
	Tab                string `json:"tab" validate:"required" example:"product"`
	Name               string `json:"name" validate:"required" example:"白色外壳"`
	Code               string `json:"code" validate:"required" example:"P-001"`
	Unit               string `json:"unit" example:"个"`
	Spec               string `json:"spec" example:"标准"`
	SafetyStock        int64  `json:"safety_stock" example:"10"`
	DefaultCost        int64  `json:"default_cost" example:"10000"`
	OperatorEmployeeID uint   `json:"operator_employee_id" validate:"required" example:"1"`
}

// CatalogItemsResponse 是仓库物品分页响应。
type CatalogItemsResponse struct {
	Items    []CatalogItem `json:"items"`
	Total    int64         `json:"total" example:"1"`
	Page     int           `json:"page" example:"1"`
	PageSize int           `json:"page_size" example:"20"`
	Keyword  string        `json:"keyword,omitempty" example:"外壳"`
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

	warehouse := v1.Group("/warehouse", audit)
	warehouse.GET("/tabs", h.ListTabs, require("/api/v1/warehouse", "read"))
	warehouse.GET("/items", h.ListItems, require("/api/v1/warehouse", "read"))
	warehouse.POST("/items", h.CreateItem, require("/api/v1/warehouse", "write"))

	locations := v1.Group("/locations", audit)
	locations.GET("", h.ListLocations, require("/api/v1/warehouse", "read"))
	locations.POST("", h.CreateLocation, require("/api/v1/warehouse", "write"))
}

// ListWarehouses 查询单仓库信息。
// @Summary 查询仓库
// @Tags warehouse
// @Security BearerAuth
// @Produce json
// @Success 200 {array} model.Warehouse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/warehouses [get]
func (h *Handler) ListWarehouses(c *echo.Context) error {
	item, err := h.defaultWarehouse(h.DB)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, []model.Warehouse{item})
}

// CreateWarehouse 更新单仓库名称；系统编码固定为 MAIN。
// @Summary 更新系统默认仓库
// @Description 更新系统默认仓库名称；仓库编码固定为 MAIN，必须选择当前账号部门下的在职员工。
// @Tags warehouse
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createWarehouseRequest true "仓库参数"
// @Success 200 {object} model.Warehouse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/warehouses [post]
func (h *Handler) CreateWarehouse(c *echo.Context) error {
	var req createWarehouseRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	if req.Code != "" && req.Code != model.DefaultWarehouseCode {
		return echo.NewHTTPError(http.StatusBadRequest, "系统默认仓库编码固定为 MAIN")
	}
	var item model.Warehouse
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, req.OperatorEmployeeID); err != nil {
			return err
		}
		var err error
		item, err = h.defaultWarehouse(tx)
		if err != nil {
			return err
		}
		item.Name = req.Name
		item.OperatorSnapshot = operator.Snapshot(c)
		return tx.Save(&item).Error
	}); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, item)
}

// ListTabs 查询单仓库内的分类标签。
// @Summary 查询仓库分类标签
// @Tags warehouse
// @Security BearerAuth
// @Produce json
// @Success 200 {array} CatalogTabSpec
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/warehouse/tabs [get]
func (h *Handler) ListTabs(c *echo.Context) error {
	return c.JSON(http.StatusOK, catalogTabs)
}

// ListItems 查询某个仓库标签下的物品。
// @Summary 查询仓库标签物品
// @Tags warehouse
// @Security BearerAuth
// @Produce json
// @Param tab query string false "仓库标签" Enums(product,production_material,regular_product,daily_supply)
// @Param page query int false "页码"
// @Param page_size query int false "每页条数"
// @Param q query string false "模糊关键字"
// @Param keyword query string false "兼容关键词参数"
// @Success 200 {object} CatalogItemsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/warehouse/items [get]
func (h *Handler) ListItems(c *echo.Context) error {
	tab := c.QueryParam("tab")
	if tab == "" {
		tab = tabProduct
	}
	result, err := listCatalogItems(h.DB, tab, pagination.FromEcho(c))
	if err != nil {
		return err
	}
	warehouse, err := h.defaultWarehouse(h.DB)
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
// @Summary 创建仓库物品
// @Description 在指定仓库标签下创建物料或产品；必须选择当前账号部门下的在职员工。
// @Tags warehouse
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createWarehouseItemRequest true "仓库物品参数"
// @Success 201 {object} CatalogItem
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/warehouse/items [post]
func (h *Handler) CreateItem(c *echo.Context) error {
	var req createWarehouseItemRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	input := CatalogItemInput{
		Tab: req.Tab, Name: req.Name, Code: req.Code, Unit: req.Unit, Spec: req.Spec,
		SafetyStock: req.SafetyStock, DefaultCost: req.DefaultCost, OperatorEmployeeID: req.OperatorEmployeeID,
	}
	var item CatalogItem
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, req.OperatorEmployeeID); err != nil {
			return err
		}
		input.operatorSnapshot = operator.Snapshot(c)
		var err error
		item, err = createCatalogItem(tx, input)
		return err
	}); err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

// ListLocations 查询库位列表。
// @Summary 查询仓库库位
// @Tags warehouse
// @Security BearerAuth
// @Produce json
// @Param warehouse_id query int false "仓库 ID"
// @Success 200 {array} model.Location
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/locations [get]
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
// @Summary 创建仓库库位
// @Description 创建系统默认仓库下的库位；必须选择当前账号部门下的在职员工。
// @Tags warehouse
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body createLocationRequest true "库位参数"
// @Success 201 {object} model.Location
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/locations [post]
func (h *Handler) CreateLocation(c *echo.Context) error {
	var req createLocationRequest
	if err := request.BindAndValidate(c, &req); err != nil {
		return err
	}
	var item model.Location
	if err := h.DB.WithContext(c.Request().Context()).Transaction(func(tx *gorm.DB) error {
		if _, err := operator.Resolve(c, tx, req.OperatorEmployeeID); err != nil {
			return err
		}
		warehouse, err := h.defaultWarehouse(tx)
		if err != nil {
			return err
		}
		if req.WarehouseID != warehouse.ID {
			return echo.NewHTTPError(http.StatusNotFound, "仓库不存在")
		}
		item = model.Location{WarehouseID: req.WarehouseID, Code: req.Code, Name: req.Name, Status: model.StatusActive, OperatorSnapshot: operator.Snapshot(c)}
		return tx.Create(&item).Error
	}); err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *Handler) defaultWarehouse(db *gorm.DB) (model.Warehouse, error) {
	var item model.Warehouse
	if err := db.Where("code = ?", model.DefaultWarehouseCode).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Warehouse{}, echo.NewHTTPError(http.StatusInternalServerError, "默认仓库未初始化")
		}
		return model.Warehouse{}, err
	}
	return item, nil
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
