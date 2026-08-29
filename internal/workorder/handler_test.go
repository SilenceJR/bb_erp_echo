package workorder

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testValidator struct {
	validate *validator.Validate
}

func (v *testValidator) Validate(i any) error {
	return v.validate.Struct(i)
}

func TestProductionWorkOrderDispatchAndDepartmentFlow(t *testing.T) {
	db := openWorkOrderTestDB(t)
	handler := &Handler{DB: db}
	departments := seedWorkOrderDepartments(t, db)
	product := seedWorkOrderProduct(t, db, "白色外壳", "P-WHITE")

	created := createWorkOrder(t, handler, map[string]any{
		"code":                  "WO-001",
		"type":                  TypeProduction,
		"product_id":            product.ID,
		"planned_quantity":      int64(1000000),
		"target_department_ids": []uint{departments[0].ID, departments[1].ID},
	}, nil)
	if created.Status != StatusDraft || len(created.DepartmentTasks) != 2 {
		t.Fatalf("created = %+v", created)
	}

	dispatched := callWorkOrder(t, handler.Dispatch, http.MethodPost, "/api/v1/workorder/:id/dispatch", nil, map[string]string{"id": idString(created.ID)}, nil, http.StatusOK)
	if dispatched.Status != StatusProcessing {
		t.Fatalf("dispatch status = %s, want %s", dispatched.Status, StatusProcessing)
	}
	for _, task := range dispatched.DepartmentTasks {
		if task.Status != DepartmentTaskReceived {
			t.Fatalf("department task status = %s, want received", task.Status)
		}
	}

	task := dispatched.DepartmentTasks[0]
	deptID := task.DepartmentID
	current := &auth.CurrentUser{ID: 10, Username: "dept", DepartmentID: &deptID, OrganizationID: 1}
	started := callWorkOrder(t, handler.StartDepartmentTask, http.MethodPost, "/api/v1/workorder/department-tasks/:id/start", nil, map[string]string{"id": idString(task.ID)}, current, http.StatusOK)
	if started.DepartmentTasks[0].Status != DepartmentTaskProcessing {
		t.Fatalf("start status = %s", started.DepartmentTasks[0].Status)
	}
	partial := callWorkOrder(t, handler.PartialCompleteDepartmentTask, http.MethodPost, "/api/v1/workorder/department-tasks/:id/partial-complete", map[string]any{
		"completed_quantity": int64(500000),
		"remark":             "完成一半",
	}, map[string]string{"id": idString(task.ID)}, current, http.StatusOK)
	if partial.DepartmentTasks[0].Status != DepartmentTaskPartialCompleted || partial.DepartmentTasks[0].CompletedQuantity != 500000 {
		t.Fatalf("partial task = %+v", partial.DepartmentTasks[0])
	}

	callWorkOrder(t, handler.CompleteDepartmentTask, http.MethodPost, "/api/v1/workorder/department-tasks/:id/complete", nil, map[string]string{"id": idString(task.ID)}, current, http.StatusOK)
	second := dispatched.DepartmentTasks[1]
	secondDeptID := second.DepartmentID
	secondCurrent := &auth.CurrentUser{ID: 11, Username: "dept2", DepartmentID: &secondDeptID, OrganizationID: 1}
	pending := callWorkOrder(t, handler.CompleteDepartmentTask, http.MethodPost, "/api/v1/workorder/department-tasks/:id/complete", nil, map[string]string{"id": idString(second.ID)}, secondCurrent, http.StatusOK)
	if pending.Status != StatusPendingClose {
		t.Fatalf("main status = %s, want pending_close", pending.Status)
	}

	completed := callWorkOrder(t, handler.Complete, http.MethodPost, "/api/v1/workorder/:id/complete", map[string]any{"mode": "normal"}, map[string]string{"id": idString(created.ID)}, nil, http.StatusOK)
	if completed.Status != StatusCompletedNormal {
		t.Fatalf("complete status = %s", completed.Status)
	}
}

func TestWorkOrderValidationAndPauseRules(t *testing.T) {
	db := openWorkOrderTestDB(t)
	handler := &Handler{DB: db}
	departments := seedWorkOrderDepartments(t, db)
	product := seedWorkOrderProduct(t, db, "丝印面板", "P-SILK")
	created := createWorkOrder(t, handler, map[string]any{
		"code":                  "WO-002",
		"type":                  TypeProduction,
		"product_id":            product.ID,
		"planned_quantity":      int64(1000000),
		"target_department_ids": []uint{departments[0].ID},
	}, nil)

	rec := performWorkOrderJSON(t, handler.Complete, http.MethodPost, "/api/v1/workorder/:id/complete", map[string]any{"mode": "normal"}, map[string]string{"id": idString(created.ID)}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("normal complete before pending status = %d body=%s", rec.Code, rec.Body.String())
	}
	callWorkOrder(t, handler.Dispatch, http.MethodPost, "/api/v1/workorder/:id/dispatch", nil, map[string]string{"id": idString(created.ID)}, nil, http.StatusOK)
	rec = performWorkOrderJSON(t, handler.PartialCompleteDepartmentTask, http.MethodPost, "/api/v1/workorder/department-tasks/:id/partial-complete", map[string]any{"completed_quantity": int64(1000000)}, map[string]string{"id": idString(created.DepartmentTasks[0].ID)}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("partial over status = %d body=%s", rec.Code, rec.Body.String())
	}
	callWorkOrder(t, handler.Pause, http.MethodPost, "/api/v1/workorder/:id/pause", map[string]any{"reason": "等料"}, map[string]string{"id": idString(created.ID)}, nil, http.StatusOK)
	rec = performWorkOrderJSON(t, handler.CompleteDepartmentTask, http.MethodPost, "/api/v1/workorder/department-tasks/:id/complete", nil, map[string]string{"id": idString(created.DepartmentTasks[0].ID)}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("complete while paused status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = performWorkOrderJSON(t, handler.Complete, http.MethodPost, "/api/v1/workorder/:id/complete", map[string]any{"mode": "forced"}, map[string]string{"id": idString(created.ID)}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("forced without reason status = %d body=%s", rec.Code, rec.Body.String())
	}
	completed := callWorkOrder(t, handler.Complete, http.MethodPost, "/api/v1/workorder/:id/complete", map[string]any{"mode": "forced", "reason": "客户取消后强制结单"}, map[string]string{"id": idString(created.ID)}, nil, http.StatusOK)
	if completed.Status != StatusCompletedForced {
		t.Fatalf("forced status = %s", completed.Status)
	}
}

func TestDepartmentTaskAccessBoundary(t *testing.T) {
	db := openWorkOrderTestDB(t)
	handler := &Handler{DB: db}
	departments := seedWorkOrderDepartments(t, db)
	product := seedWorkOrderProduct(t, db, "黑色外壳", "P-BLACK")
	created := createWorkOrder(t, handler, map[string]any{
		"code":                  "WO-003",
		"type":                  TypeProduction,
		"product_id":            product.ID,
		"planned_quantity":      int64(1000000),
		"target_department_ids": []uint{departments[0].ID},
	}, nil)
	dispatched := callWorkOrder(t, handler.Dispatch, http.MethodPost, "/api/v1/workorder/:id/dispatch", nil, map[string]string{"id": idString(created.ID)}, nil, http.StatusOK)
	otherDepartmentID := departments[1].ID
	current := &auth.CurrentUser{ID: 12, Username: "other", DepartmentID: &otherDepartmentID, OrganizationID: 1}
	rec := performWorkOrderJSON(t, handler.StartDepartmentTask, http.MethodPost, "/api/v1/workorder/department-tasks/:id/start", nil, map[string]string{"id": idString(dispatched.DepartmentTasks[0].ID)}, current)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross department status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProductionWorkOrderRequiresActiveProductAndUsesSnapshots(t *testing.T) {
	db := openWorkOrderTestDB(t)
	handler := &Handler{DB: db}
	departments := seedWorkOrderDepartments(t, db)
	active := seedWorkOrderProduct(t, db, "主产品", "P-ACTIVE")
	disabled := seedWorkOrderProduct(t, db, "停用产品", "P-DISABLED")
	disabled.Status = model.StatusDisabled
	if err := db.Save(&disabled).Error; err != nil {
		t.Fatalf("disable product: %v", err)
	}

	missing := performWorkOrderJSON(t, handler.Create, http.MethodPost, "/api/v1/workorder", map[string]any{
		"type":                  TypeProduction,
		"planned_quantity":      int64(10000),
		"target_department_ids": []uint{departments[0].ID},
	}, nil, nil)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing product status = %d body=%s", missing.Code, missing.Body.String())
	}

	disabledResponse := performWorkOrderJSON(t, handler.Create, http.MethodPost, "/api/v1/workorder", map[string]any{
		"type":                  TypeProduction,
		"product_id":            disabled.ID,
		"planned_quantity":      int64(10000),
		"target_department_ids": []uint{departments[0].ID},
	}, nil, nil)
	if disabledResponse.Code != http.StatusBadRequest {
		t.Fatalf("disabled product status = %d body=%s", disabledResponse.Code, disabledResponse.Body.String())
	}

	created := createWorkOrder(t, handler, map[string]any{
		"code":                  "WO-SNAPSHOT",
		"type":                  TypeProduction,
		"product_id":            active.ID,
		"product_name":          "客户端伪造名称",
		"planned_quantity":      int64(10000),
		"unit":                  "箱",
		"target_department_ids": []uint{departments[0].ID},
	}, nil)
	if created.ProductID == nil || *created.ProductID != active.ID {
		t.Fatalf("product id snapshot = %+v, want %d", created.ProductID, active.ID)
	}
	if created.ProductName != active.Name || created.Unit != active.Unit {
		t.Fatalf("product snapshot = name:%q unit:%q, want name:%q unit:%q", created.ProductName, created.Unit, active.Name, active.Unit)
	}
}

func TestGeneralWorkOrderClearsProductAssociation(t *testing.T) {
	db := openWorkOrderTestDB(t)
	handler := &Handler{DB: db}
	departments := seedWorkOrderDepartments(t, db)
	product := seedWorkOrderProduct(t, db, "不应关联", "P-GENERAL")
	created := createWorkOrder(t, handler, map[string]any{
		"code":                  "WO-GENERAL",
		"type":                  TypeGeneral,
		"title":                 "通用任务",
		"description":           "通用说明",
		"product_id":            product.ID,
		"target_department_ids": []uint{departments[0].ID},
	}, nil)
	if created.ProductID != nil || created.ProductName != "" || created.Unit != "" {
		t.Fatalf("general task retained product association: %+v", created)
	}
}

func TestCreateTemporaryProductDefaultsAndRejectsDuplicateCode(t *testing.T) {
	db := openWorkOrderTestDB(t)
	handler := &Handler{DB: db}

	created := callProductWorkOrder(t, handler.CreateTemporaryProduct, map[string]any{
		"name": "临时产品",
		"code": "P-TEMP",
		"spec": "试产",
	}, http.StatusCreated)
	if created.Status != model.StatusActive || created.Unit != "个" || created.SafetyStock != 0 || created.DefaultCost != 0 {
		t.Fatalf("temporary product defaults = %+v", created)
	}
	var balances int64
	if err := db.Model(&model.InventoryBalance{}).Where("item_type = ? AND item_id = ?", itemProductForTest, created.ID).Count(&balances).Error; err != nil {
		t.Fatalf("count initial balances: %v", err)
	}
	if balances != 0 {
		t.Fatalf("temporary product created %d inventory balances, want 0", balances)
	}
	var ledgers int64
	if err := db.Model(&model.InventoryLedger{}).Where("item_type = ? AND item_id = ?", itemProductForTest, created.ID).Count(&ledgers).Error; err != nil {
		t.Fatalf("count initial ledgers: %v", err)
	}
	if ledgers != 0 {
		t.Fatalf("temporary product created %d inventory ledgers, want 0", ledgers)
	}

	duplicate := performWorkOrderJSON(t, handler.CreateTemporaryProduct, http.MethodPost, "/api/v1/workorder/products", map[string]any{
		"name": "另一个产品",
		"code": "P-TEMP",
	}, nil, nil)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate code status = %d body=%s", duplicate.Code, duplicate.Body.String())
	}

	missing := performWorkOrderJSON(t, handler.CreateTemporaryProduct, http.MethodPost, "/api/v1/workorder/products", map[string]any{
		"name": "缺少编码",
	}, nil, nil)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("missing code status = %d body=%s", missing.Code, missing.Body.String())
	}
}

func openWorkOrderTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&model.Department{}, &model.Product{}, &model.WorkOrder{}, &model.DepartmentTask{}, &model.WorkOrderFlowLog{}, &model.InventoryBalance{}, &model.InventoryLedger{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

const itemProductForTest = "product"

func seedWorkOrderProduct(t *testing.T, db *gorm.DB, name string, code string) model.Product {
	t.Helper()
	product := model.Product{Name: name, Code: code, Unit: "个", Spec: "标准", Status: model.StatusActive}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}
	return product
}

func seedWorkOrderDepartments(t *testing.T, db *gorm.DB) []model.Department {
	t.Helper()
	departments := []model.Department{
		{Name: "注塑部", Code: "INJECTION", OrganizationID: 1, Status: model.StatusActive},
		{Name: "白壳包装", Code: "WHITE-PACK", OrganizationID: 1, Status: model.StatusActive},
	}
	for index := range departments {
		if err := db.Create(&departments[index]).Error; err != nil {
			t.Fatalf("seed department: %v", err)
		}
	}
	return departments
}

func createWorkOrder(t *testing.T, handler *Handler, body map[string]any, current *auth.CurrentUser) model.WorkOrder {
	t.Helper()
	return callWorkOrder(t, handler.Create, http.MethodPost, "/api/v1/workorder", body, nil, current, http.StatusCreated)
}

func callWorkOrder(t *testing.T, handler echo.HandlerFunc, method string, path string, body any, params map[string]string, current *auth.CurrentUser, wantStatus int) model.WorkOrder {
	t.Helper()
	rec := performWorkOrderJSON(t, handler, method, path, body, params, current)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s status = %d want %d body=%s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	var result model.WorkOrder
	decodeWorkOrderJSON(t, rec, &result)
	return result
}

func callProductWorkOrder(t *testing.T, handler echo.HandlerFunc, body any, wantStatus int) model.Product {
	t.Helper()
	rec := performWorkOrderJSON(t, handler, http.MethodPost, "/api/v1/workorder/products", body, nil, nil)
	if rec.Code != wantStatus {
		t.Fatalf("POST /api/v1/workorder/products status = %d want %d body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	var result model.Product
	decodeWorkOrderJSON(t, rec, &result)
	return result
}

func performWorkOrderJSON(t *testing.T, handler echo.HandlerFunc, method string, path string, body any, params map[string]string, current *auth.CurrentUser) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.Validator = &testValidator{validate: validator.New()}
	var payload *bytes.Reader
	if body == nil {
		payload = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(body)
		payload = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, payload)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if len(params) > 0 {
		names := make([]string, 0, len(params))
		values := make([]string, 0, len(params))
		for key, value := range params {
			names = append(names, key)
			values = append(values, value)
		}
		pathValues := make(echo.PathValues, 0, len(names))
		for i := range names {
			pathValues = append(pathValues, echo.PathValue{Name: names[i], Value: values[i]})
		}
		c.SetPathValues(pathValues)
	}
	if current == nil {
		current = &auth.CurrentUser{ID: 1, Username: "office", OrganizationID: 1}
	}
	c.Set(auth.ContextUserKey, current)
	if err := handler(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}

func decodeWorkOrderJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode json %q: %v", rec.Body.String(), err)
	}
}

func idString(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
