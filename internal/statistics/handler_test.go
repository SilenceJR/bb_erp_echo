package statistics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/role"

	"github.com/labstack/echo/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDashboardAggregatesAndCostTrim(t *testing.T) {
	db := openStatisticsTestDB(t)
	handler := &Handler{DB: db}
	seedStatisticsData(t, db)

	rec := performStatisticsRequest(t, handler, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d body=%s", rec.Code, rec.Body.String())
	}
	var withoutCost DashboardResponse
	decodeStatisticsJSON(t, rec, &withoutCost)
	if withoutCost.Summary.Customers != 1 || withoutCost.Summary.OpenWorkOrders != 1 || withoutCost.Summary.LowStockItems == 0 {
		t.Fatalf("summary mismatch: %+v", withoutCost.Summary)
	}
	if withoutCost.Summary.InventoryAmount != 0 || withoutCost.Inventory.ByItemType[0].Amount != 0 {
		t.Fatalf("amount should be hidden without cost:view: %+v", withoutCost.Inventory.ByItemType)
	}

	rec = performStatisticsRequest(t, handler, []string{role.CostViewCode})
	var withCost DashboardResponse
	decodeStatisticsJSON(t, rec, &withCost)
	if !withCost.CanViewCost || withCost.Summary.InventoryAmount == 0 {
		t.Fatalf("cost summary missing: %+v", withCost.Summary)
	}
	if len(withCost.WorkOrders.ByDepartment) != 1 || withCost.WorkOrders.ByDepartment[0].Completed != 1 {
		t.Fatalf("department stat mismatch: %+v", withCost.WorkOrders.ByDepartment)
	}
}

func openStatisticsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&model.CustomerCode{}, &model.CustomerProfile{}, &model.Supplier{},
		&model.Department{}, &model.Product{}, &model.Material{},
		&model.InventoryBalance{}, &model.InventoryLedger{},
		&model.WorkOrder{}, &model.DepartmentTask{},
		&model.Mold{}, &model.AuditLog{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func seedStatisticsData(t *testing.T, db *gorm.DB) {
	t.Helper()
	department := model.Department{Name: "注塑部", Code: "INJECTION", OrganizationID: 1, Status: model.StatusActive}
	product := model.Product{Name: "白壳", Code: "P-001", Unit: "个", SafetyStock: 100000, Status: model.StatusActive}
	material := model.Material{Name: "ABS", Code: "M-001", Category: "生产物资", Unit: "kg", SafetyStock: 200000, Status: model.StatusActive}
	customerCode := model.CustomerCode{Code: "BB-001"}
	supplier := model.Supplier{Name: "供应商", Code: "S-001", Status: model.StatusActive}
	for _, item := range []any{&department, &product, &material, &customerCode, &supplier} {
		if err := db.Create(item).Error; err != nil {
			t.Fatalf("seed item: %v", err)
		}
	}
	if err := db.Create(&model.CustomerProfile{CustomerCodeID: customerCode.ID, Name: "客户", ContactName: "联系人", IsDefault: true}).Error; err != nil {
		t.Fatal(err)
	}
	next := time.Now().AddDate(0, 0, 3)
	if err := db.Create(&model.Mold{Code: "MOLD-001", Name: "白壳模具", Status: "loaned", CurrentLocation: "注塑部", NextMaintenanceAt: &next}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.InventoryBalance{WarehouseID: 1, ItemType: "product", ItemID: product.ID, Quantity: 50000, AvgCost: 200, Amount: 1000}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.InventoryLedger{DocumentID: 1, LineID: 1, Type: "inbound", WarehouseID: 1, ItemType: "product", ItemID: product.ID, Quantity: 50000, UnitCost: 200, Amount: 1000, BalanceQty: 50000, BalanceAmt: 1000}).Error; err != nil {
		t.Fatal(err)
	}
	workOrder := model.WorkOrder{Code: "WO-001", Title: "生产单", Type: "production", Status: "processing", Priority: "urgent", ProductName: "白壳", PlannedQuantity: 100000, Unit: "个"}
	if err := db.Create(&workOrder).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.DepartmentTask{WorkOrderID: workOrder.ID, DepartmentID: department.ID, Title: "生产单", Status: "completed", PlannedQuantity: 100000, CompletedQuantity: 100000, Progress: 100}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AuditLog{ActorUsername: "admin", Object: "/api/v1/statistics", Action: "read", Result: "success", Status: 200}).Error; err != nil {
		t.Fatal(err)
	}
}

func performStatisticsRequest(t *testing.T, handler *Handler, permissions []string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/statistics", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(auth.ContextUserKey, &auth.CurrentUser{ID: 1, Username: "tester", OrganizationID: 1, Permissions: permissions})
	if err := handler.Dashboard(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}

func decodeStatisticsJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode json %q: %v", rec.Body.String(), err)
	}
}
