// Package statistics 实现 ERP 统计报表聚合接口。
package statistics

import (
	"net/http"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/role"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ErrorResponse 是统一错误响应的 Swagger 文档别名。
type ErrorResponse = response.ErrorBody

// Handler 处理统计报表接口。
type Handler struct {
	DB *gorm.DB
}

// DashboardResponse 是统计报表首页聚合响应。
type DashboardResponse struct {
	GeneratedAt      time.Time           `json:"generated_at"`
	CanViewCost      bool                `json:"can_view_cost"`
	Summary          Summary             `json:"summary"`
	Inventory        InventoryStatistics `json:"inventory"`
	WorkOrders       WorkOrderStatistics `json:"workorders"`
	Molds            MoldStatistics      `json:"molds"`
	Business         BusinessStatistics  `json:"business"`
	Audit            AuditStatistics     `json:"audit"`
	RecentWorkOrders []model.WorkOrder   `json:"recent_workorders"`
}

// Summary 是首页顶部关键指标。
type Summary struct {
	Customers          int64 `json:"customers"`
	Suppliers          int64 `json:"suppliers"`
	Contacts           int64 `json:"contacts"`
	WarehouseItems     int64 `json:"warehouse_items"`
	InventoryQuantity  int64 `json:"inventory_quantity"`
	InventoryAmount    int64 `json:"inventory_amount,omitempty"`
	LowStockItems      int64 `json:"low_stock_items"`
	OpenWorkOrders     int64 `json:"open_workorders"`
	UrgentWorkOrders   int64 `json:"urgent_workorders"`
	PendingCloseOrders int64 `json:"pending_close_orders"`
	Molds              int64 `json:"molds"`
	MoldsNeedCare      int64 `json:"molds_need_care"`
}

// InventoryStatistics 是库存维度统计。
type InventoryStatistics struct {
	ByItemType     []NameValue `json:"by_item_type"`
	ByMaterialType []NameValue `json:"by_material_type"`
	LowStock       []StockItem `json:"low_stock"`
	Trend          []TrendItem `json:"trend"`
}

// WorkOrderStatistics 是任务单维度统计。
type WorkOrderStatistics struct {
	ByStatus     []NameValue      `json:"by_status"`
	ByType       []NameValue      `json:"by_type"`
	ByDepartment []DepartmentStat `json:"by_department"`
	Trend        []TrendItem      `json:"trend"`
}

// MoldStatistics 是模具维度统计。
type MoldStatistics struct {
	ByStatus []NameValue `json:"by_status"`
	NeedCare []MoldItem  `json:"need_care"`
}

// BusinessStatistics 是基础资料数量统计。
type BusinessStatistics struct {
	ByMasterData []NameValue `json:"by_master_data"`
}

// AuditStatistics 是操作审计统计。
type AuditStatistics struct {
	ByResult []NameValue `json:"by_result"`
	Trend    []TrendItem `json:"trend"`
}

// NameValue 是通用名称数量统计。
type NameValue struct {
	Name   string `json:"name"`
	Value  int64  `json:"value"`
	Amount int64  `json:"amount,omitempty"`
}

// DepartmentStat 是部门任务处理统计。
type DepartmentStat struct {
	DepartmentID uint   `json:"department_id"`
	Name         string `json:"name"`
	Total        int64  `json:"total"`
	Completed    int64  `json:"completed"`
	Processing   int64  `json:"processing"`
	Partial      int64  `json:"partial"`
	Received     int64  `json:"received"`
}

// StockItem 是低库存明细。
type StockItem struct {
	ItemType    string `json:"item_type"`
	ItemID      uint   `json:"item_id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Category    string `json:"category"`
	Quantity    int64  `json:"quantity"`
	SafetyStock int64  `json:"safety_stock"`
	Amount      int64  `json:"amount,omitempty"`
}

// MoldItem 是需要关注的模具明细。
type MoldItem struct {
	ID                uint       `json:"id"`
	Code              string     `json:"code"`
	Name              string     `json:"name"`
	Status            string     `json:"status"`
	CurrentLocation   string     `json:"current_location"`
	NextMaintenanceAt *time.Time `json:"next_maintenance_at"`
}

// TrendItem 是按日期聚合的趋势项。
type TrendItem struct {
	Date     string `json:"date"`
	Name     string `json:"name,omitempty"`
	Value    int64  `json:"value"`
	Quantity int64  `json:"quantity,omitempty"`
	Amount   int64  `json:"amount,omitempty"`
}

// RegisterRoutes 注册统计报表模块路由。
func RegisterRoutes(v1 *echo.Group, db *gorm.DB, require func(string, string) echo.MiddlewareFunc, audit echo.MiddlewareFunc) {
	handler := &Handler{DB: db}
	group := v1.Group("/statistics", audit)
	group.GET("", handler.Dashboard, require("/api/v1/statistics", "read"))
}

// Dashboard 查询统计报表聚合数据。
// @Summary 查询统计报表
// @Tags statistics
// @Security BearerAuth
// @Success 200 {object} DashboardResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/statistics [get]
func (h *Handler) Dashboard(c echo.Context) error {
	canViewCost := hasCostView(c)
	result := DashboardResponse{GeneratedAt: time.Now(), CanViewCost: canViewCost}
	if err := h.fillSummary(&result); err != nil {
		return err
	}
	if err := h.fillInventory(&result, canViewCost); err != nil {
		return err
	}
	if err := h.fillWorkOrders(&result); err != nil {
		return err
	}
	if err := h.fillMolds(&result); err != nil {
		return err
	}
	if err := h.fillBusiness(&result); err != nil {
		return err
	}
	if err := h.fillAudit(&result); err != nil {
		return err
	}
	if err := h.DB.Preload("DepartmentTasks").Order("id desc").Limit(8).Find(&result.RecentWorkOrders).Error; err != nil {
		return err
	}
	if !canViewCost {
		result.Summary.InventoryAmount = 0
		for index := range result.Inventory.ByItemType {
			result.Inventory.ByItemType[index].Amount = 0
		}
		for index := range result.Inventory.LowStock {
			result.Inventory.LowStock[index].Amount = 0
		}
		for index := range result.Inventory.Trend {
			result.Inventory.Trend[index].Amount = 0
		}
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handler) fillSummary(result *DashboardResponse) error {
	counts := []struct {
		model any
		out   *int64
	}{
		{&model.Customer{}, &result.Summary.Customers},
		{&model.Supplier{}, &result.Summary.Suppliers},
		{&model.Contact{}, &result.Summary.Contacts},
		{&model.Mold{}, &result.Summary.Molds},
	}
	for _, item := range counts {
		if err := h.DB.Model(item.model).Count(item.out).Error; err != nil {
			return err
		}
	}
	var products, materials int64
	if err := h.DB.Model(&model.Product{}).Count(&products).Error; err != nil {
		return err
	}
	if err := h.DB.Model(&model.Material{}).Count(&materials).Error; err != nil {
		return err
	}
	result.Summary.WarehouseItems = products + materials
	var inventoryTotal struct {
		InventoryQuantity int64
		InventoryAmount   int64
	}
	if err := h.DB.Model(&model.InventoryBalance{}).
		Select("COALESCE(SUM(quantity), 0) AS inventory_quantity, COALESCE(SUM(amount), 0) AS inventory_amount").
		Scan(&inventoryTotal).Error; err != nil {
		return err
	}
	result.Summary.InventoryQuantity = inventoryTotal.InventoryQuantity
	result.Summary.InventoryAmount = inventoryTotal.InventoryAmount
	if err := h.DB.Model(&model.WorkOrder{}).
		Where("status IN ?", []string{"draft", "processing", "paused", "pending_close"}).
		Count(&result.Summary.OpenWorkOrders).Error; err != nil {
		return err
	}
	if err := h.DB.Model(&model.WorkOrder{}).Where("priority = ? AND status IN ?", "urgent", []string{"draft", "processing", "paused", "pending_close"}).
		Count(&result.Summary.UrgentWorkOrders).Error; err != nil {
		return err
	}
	if err := h.DB.Model(&model.WorkOrder{}).Where("status = ?", "pending_close").Count(&result.Summary.PendingCloseOrders).Error; err != nil {
		return err
	}
	lowStock, err := h.lowStockItems(true)
	if err != nil {
		return err
	}
	result.Summary.LowStockItems = int64(len(lowStock))
	if err := h.DB.Model(&model.Mold{}).
		Where("status IN ? OR (next_maintenance_at IS NOT NULL AND next_maintenance_at <= ?)", []string{"repairing", "maintenance", "loaned"}, time.Now().AddDate(0, 0, 7)).
		Count(&result.Summary.MoldsNeedCare).Error; err != nil {
		return err
	}
	return nil
}

func (h *Handler) fillInventory(result *DashboardResponse, canViewCost bool) error {
	selectByType := "item_type AS name, COALESCE(SUM(quantity), 0) AS value"
	if canViewCost {
		selectByType += ", COALESCE(SUM(amount), 0) AS amount"
	}
	if err := h.DB.Model(&model.InventoryBalance{}).
		Select(selectByType).Group("item_type").Scan(&result.Inventory.ByItemType).Error; err != nil {
		return err
	}
	if err := h.DB.Model(&model.Material{}).Select("category AS name, COUNT(*) AS value").Group("category").Scan(&result.Inventory.ByMaterialType).Error; err != nil {
		return err
	}
	lowStock, err := h.lowStockItems(false)
	if err != nil {
		return err
	}
	result.Inventory.LowStock = lowStock
	query := h.DB.Model(&model.InventoryLedger{}).
		Select("DATE(created_at) AS date, type AS name, COUNT(*) AS value, COALESCE(SUM(quantity), 0) AS quantity")
	if canViewCost {
		query = query.Select("DATE(created_at) AS date, type AS name, COUNT(*) AS value, COALESCE(SUM(quantity), 0) AS quantity, COALESCE(SUM(amount), 0) AS amount")
	}
	return query.Where("created_at >= ?", time.Now().AddDate(0, 0, -14)).
		Group("DATE(created_at), type").Order("date asc").Scan(&result.Inventory.Trend).Error
}

func (h *Handler) fillWorkOrders(result *DashboardResponse) error {
	if err := h.DB.Model(&model.WorkOrder{}).Select("status AS name, COUNT(*) AS value").Group("status").Scan(&result.WorkOrders.ByStatus).Error; err != nil {
		return err
	}
	if err := h.DB.Model(&model.WorkOrder{}).Select("type AS name, COUNT(*) AS value").Group("type").Scan(&result.WorkOrders.ByType).Error; err != nil {
		return err
	}
	if err := h.DB.Table("department_tasks").
		Select(`department_tasks.department_id, departments.name,
			COUNT(*) AS total,
			SUM(CASE WHEN department_tasks.status = 'completed' THEN 1 ELSE 0 END) AS completed,
			SUM(CASE WHEN department_tasks.status = 'processing' THEN 1 ELSE 0 END) AS processing,
			SUM(CASE WHEN department_tasks.status = 'partial_completed' THEN 1 ELSE 0 END) AS partial,
			SUM(CASE WHEN department_tasks.status = 'received' THEN 1 ELSE 0 END) AS received`).
		Joins("LEFT JOIN departments ON departments.id = department_tasks.department_id").
		Group("department_tasks.department_id, departments.name").Order("total desc").
		Scan(&result.WorkOrders.ByDepartment).Error; err != nil {
		return err
	}
	return h.DB.Model(&model.WorkOrder{}).Select("DATE(created_at) AS date, status AS name, COUNT(*) AS value").
		Where("created_at >= ?", time.Now().AddDate(0, 0, -14)).
		Group("DATE(created_at), status").Order("date asc").Scan(&result.WorkOrders.Trend).Error
}

func (h *Handler) fillMolds(result *DashboardResponse) error {
	if err := h.DB.Model(&model.Mold{}).Select("status AS name, COUNT(*) AS value").Group("status").Scan(&result.Molds.ByStatus).Error; err != nil {
		return err
	}
	var items []model.Mold
	if err := h.DB.Where("status IN ? OR (next_maintenance_at IS NOT NULL AND next_maintenance_at <= ?)", []string{"repairing", "maintenance", "loaned"}, time.Now().AddDate(0, 0, 7)).
		Order("next_maintenance_at asc, id desc").Limit(10).Find(&items).Error; err != nil {
		return err
	}
	for _, item := range items {
		result.Molds.NeedCare = append(result.Molds.NeedCare, MoldItem{
			ID: item.ID, Code: item.Code, Name: item.Name, Status: item.Status,
			CurrentLocation: item.CurrentLocation, NextMaintenanceAt: item.NextMaintenanceAt,
		})
	}
	return nil
}

func (h *Handler) fillBusiness(result *DashboardResponse) error {
	stats := []struct {
		name  string
		model any
	}{
		{"客户", &model.Customer{}},
		{"联系人", &model.Contact{}},
		{"供应商", &model.Supplier{}},
		{"产品", &model.Product{}},
		{"物料", &model.Material{}},
		{"模具", &model.Mold{}},
		{"任务单", &model.WorkOrder{}},
	}
	for _, stat := range stats {
		var count int64
		if err := h.DB.Model(stat.model).Count(&count).Error; err != nil {
			return err
		}
		result.Business.ByMasterData = append(result.Business.ByMasterData, NameValue{Name: stat.name, Value: count})
	}
	return nil
}

func (h *Handler) fillAudit(result *DashboardResponse) error {
	if err := h.DB.Model(&model.AuditLog{}).Select("result AS name, COUNT(*) AS value").Group("result").Scan(&result.Audit.ByResult).Error; err != nil {
		return err
	}
	return h.DB.Model(&model.AuditLog{}).Select("DATE(created_at) AS date, result AS name, COUNT(*) AS value").
		Where("created_at >= ?", time.Now().AddDate(0, 0, -14)).
		Group("DATE(created_at), result").Order("date asc").Scan(&result.Audit.Trend).Error
}

func (h *Handler) lowStockItems(summaryOnly bool) ([]StockItem, error) {
	limit := 10
	if summaryOnly {
		limit = 100000
	}
	var rows []StockItem
	productQuery := h.DB.Table("products").
		Select("'product' AS item_type, products.id AS item_id, products.name, products.code, '产品' AS category, COALESCE(SUM(inventory_balances.quantity), 0) AS quantity, products.safety_stock, COALESCE(SUM(inventory_balances.amount), 0) AS amount").
		Joins("LEFT JOIN inventory_balances ON inventory_balances.item_type = 'product' AND inventory_balances.item_id = products.id").
		Group("products.id").Having("products.safety_stock > 0 AND COALESCE(SUM(inventory_balances.quantity), 0) <= products.safety_stock")
	materialQuery := h.DB.Table("materials").
		Select("'material' AS item_type, materials.id AS item_id, materials.name, materials.code, materials.category, COALESCE(SUM(inventory_balances.quantity), 0) AS quantity, materials.safety_stock, COALESCE(SUM(inventory_balances.amount), 0) AS amount").
		Joins("LEFT JOIN inventory_balances ON inventory_balances.item_type = 'material' AND inventory_balances.item_id = materials.id").
		Group("materials.id").Having("materials.safety_stock > 0 AND COALESCE(SUM(inventory_balances.quantity), 0) <= materials.safety_stock")
	if err := h.DB.Raw("? UNION ALL ? ORDER BY quantity ASC LIMIT ?", productQuery, materialQuery, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
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
