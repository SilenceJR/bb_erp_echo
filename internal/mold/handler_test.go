package mold

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

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

// TestCreateMoldWithPlasticFactoryFields 验证模具档案会保存塑胶厂常用属性。
func TestCreateMoldWithPlasticFactoryFields(t *testing.T) {
	db := openMoldTestDB(t)
	handler := NewHandler(db)

	body := map[string]any{
		"code":                   "MOLD-001",
		"name":                   "白壳前模",
		"cavity_count":           8,
		"mold_material":          "ABS",
		"steel":                  "P20",
		"size":                   "450x350x280",
		"weight_gram":            180000,
		"manufacturer":           "深圳模具厂",
		"owner":                  "客户A",
		"storage_location":       "工模架 A1",
		"maintenance_cycle_days": 30,
	}
	rec := performMoldJSON(t, handler.CreateMold, http.MethodPost, "/api/v1/molds", body, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create mold status = %d body=%s", rec.Code, rec.Body.String())
	}

	var created model.Mold
	decodeMoldJSON(t, rec, &created)
	if created.Status != statusInStock || created.CurrentLocation != "工模架 A1" || created.CavityCount != 8 {
		t.Fatalf("created mold mismatch: %+v", created)
	}
	assertMoldEventCount(t, db, created.ID, 1)
}

// TestLoanReturnAndRepairMold 验证模具借出、归还和维修会按生命周期改变状态并写履历。
func TestLoanReturnAndRepairMold(t *testing.T) {
	db := openMoldTestDB(t)
	handler := NewHandler(db)
	item := model.Mold{Code: "MOLD-002", Name: "透明盖后模", Status: statusInStock, CavityCount: 4, CurrentLocation: "工模架 B1"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed mold: %v", err)
	}

	rec := performMoldJSON(t, handler.LoanMold, http.MethodPost, "/api/v1/molds/:id/loan", map[string]any{
		"location":     "注塑车间 1 号机",
		"counterparty": "注塑部",
		"handler_name": "王工",
		"reason":       "生产试模",
	}, map[string]string{"id": strconv.FormatUint(uint64(item.ID), 10)})
	if rec.Code != http.StatusOK {
		t.Fatalf("loan mold status = %d body=%s", rec.Code, rec.Body.String())
	}
	var loaned model.Mold
	decodeMoldJSON(t, rec, &loaned)
	if loaned.Status != statusLoaned || loaned.CurrentLocation != "注塑车间 1 号机" {
		t.Fatalf("loaned mold mismatch: %+v", loaned)
	}

	rec = performMoldJSON(t, handler.ReturnMold, http.MethodPost, "/api/v1/molds/:id/return", map[string]any{
		"location":     "工模架 B1",
		"handler_name": "李工",
	}, map[string]string{"id": strconv.FormatUint(uint64(item.ID), 10)})
	if rec.Code != http.StatusOK {
		t.Fatalf("return mold status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performMoldJSON(t, handler.RepairMold, http.MethodPost, "/api/v1/molds/:id/repair", map[string]any{
		"reason":       "顶针磨损",
		"description":  "更换顶针并清理水路",
		"handler_name": "陈师傅",
		"completed":    false,
	}, map[string]string{"id": strconv.FormatUint(uint64(item.ID), 10)})
	if rec.Code != http.StatusOK {
		t.Fatalf("start repair mold status = %d body=%s", rec.Code, rec.Body.String())
	}
	var repairing model.Mold
	decodeMoldJSON(t, rec, &repairing)
	if repairing.Status != statusRepairing {
		t.Fatalf("repairing mold mismatch: %+v", repairing)
	}

	rec = performMoldJSON(t, handler.RepairMold, http.MethodPost, "/api/v1/molds/:id/repair", map[string]any{
		"reason":       "顶针磨损",
		"description":  "更换顶针并清理水路",
		"handler_name": "陈师傅",
		"completed":    true,
	}, map[string]string{"id": strconv.FormatUint(uint64(item.ID), 10)})
	if rec.Code != http.StatusOK {
		t.Fatalf("complete repair mold status = %d body=%s", rec.Code, rec.Body.String())
	}
	var repaired model.Mold
	decodeMoldJSON(t, rec, &repaired)
	if repaired.Status != statusInStock || repaired.LastRepairAt == nil {
		t.Fatalf("repaired mold mismatch: %+v", repaired)
	}
	assertMoldEventCount(t, db, item.ID, 4)
}

// TestMaintainMoldCalculatesNextDate 验证完成保养后会计算下次保养时间。
func TestMaintainMoldCalculatesNextDate(t *testing.T) {
	db := openMoldTestDB(t)
	handler := NewHandler(db)
	item := model.Mold{Code: "MOLD-003", Name: "丝印定位治具", Status: statusInStock, CavityCount: 1, MaintenanceCycleDays: 15}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed mold: %v", err)
	}

	rec := performMoldJSON(t, handler.MaintainMold, http.MethodPost, "/api/v1/molds/:id/maintenance", map[string]any{
		"description":  "清洁防锈",
		"handler_name": "保养员",
		"completed":    false,
	}, map[string]string{"id": strconv.FormatUint(uint64(item.ID), 10)})
	if rec.Code != http.StatusOK {
		t.Fatalf("start maintenance mold status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = performMoldJSON(t, handler.MaintainMold, http.MethodPost, "/api/v1/molds/:id/maintenance", map[string]any{
		"description":  "清洁防锈",
		"handler_name": "保养员",
		"completed":    true,
	}, map[string]string{"id": strconv.FormatUint(uint64(item.ID), 10)})
	if rec.Code != http.StatusOK {
		t.Fatalf("maintain mold status = %d body=%s", rec.Code, rec.Body.String())
	}
	var maintained model.Mold
	decodeMoldJSON(t, rec, &maintained)
	if maintained.LastMaintenanceAt == nil || maintained.NextMaintenanceAt == nil {
		t.Fatalf("maintenance dates should be set: %+v", maintained)
	}
}

func TestReturnMoldRequiresNonBlankLocation(t *testing.T) {
	db := openMoldTestDB(t)
	handler := NewHandler(db)
	item := model.Mold{Code: "MOLD-RETURN-LOCATION", Name: "归还位置校验模具", Status: statusLoaned, CavityCount: 1, CurrentLocation: "注塑车间"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed mold: %v", err)
	}

	rec := performMoldJSON(t, handler.ReturnMold, http.MethodPost, "/api/v1/molds/:id/return", map[string]any{
		"location": " \t\n ",
	}, map[string]string{"id": strconv.FormatUint(uint64(item.ID), 10)})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var stored model.Mold
	if err := db.First(&stored, item.ID).Error; err != nil {
		t.Fatalf("reload mold: %v", err)
	}
	if stored.Status != statusLoaned || stored.CurrentLocation != "注塑车间" {
		t.Fatalf("return validation changed mold: %+v", stored)
	}
	assertMoldEventCount(t, db, item.ID, 0)
}

func TestCompleteMaintenanceRequiresPositiveCycle(t *testing.T) {
	db := openMoldTestDB(t)
	handler := NewHandler(db)
	item := model.Mold{Code: "MOLD-MAINTENANCE-CYCLE", Name: "保养周期校验模具", Status: statusMaintenance, CavityCount: 1, CurrentLocation: "保养工位"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed mold: %v", err)
	}

	rec := performMoldJSON(t, handler.MaintainMold, http.MethodPost, "/api/v1/molds/:id/maintenance", map[string]any{
		"completed":              true,
		"maintenance_cycle_days": 0,
	}, map[string]string{"id": strconv.FormatUint(uint64(item.ID), 10)})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var stored model.Mold
	if err := db.First(&stored, item.ID).Error; err != nil {
		t.Fatalf("reload mold: %v", err)
	}
	if stored.Status != statusMaintenance || stored.CurrentLocation != "保养工位" || stored.LastMaintenanceAt != nil || stored.NextMaintenanceAt != nil {
		t.Fatalf("maintenance validation changed mold: %+v", stored)
	}
	assertMoldEventCount(t, db, item.ID, 0)
}

func TestStartMaintenanceAllowsMissingCycle(t *testing.T) {
	db := openMoldTestDB(t)
	handler := NewHandler(db)
	item := model.Mold{Code: "MOLD-MAINTENANCE-START", Name: "保养开始模具", Status: statusInStock, CavityCount: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed mold: %v", err)
	}

	rec := performMoldJSON(t, handler.MaintainMold, http.MethodPost, "/api/v1/molds/:id/maintenance", map[string]any{
		"completed":              false,
		"maintenance_cycle_days": 0,
	}, map[string]string{"id": strconv.FormatUint(uint64(item.ID), 10)})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var stored model.Mold
	if err := db.First(&stored, item.ID).Error; err != nil {
		t.Fatalf("reload mold: %v", err)
	}
	if stored.Status != statusMaintenance || stored.MaintenanceCycleDays != 0 {
		t.Fatalf("maintenance start mismatch: %+v", stored)
	}
	assertMoldEventCount(t, db, item.ID, 1)
}

func TestMoldLifecycleRejectsInvalidSourceStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		handle func(*Handler) echo.HandlerFunc
		path   string
		body   map[string]any
	}{
		{
			name: "loan requires in stock", status: statusLoaned, handle: func(h *Handler) echo.HandlerFunc { return h.LoanMold },
			path: "/api/v1/molds/:id/loan", body: map[string]any{"location": "注塑车间", "counterparty": "注塑部"},
		},
		{
			name: "return requires loaned", status: statusInStock, handle: func(h *Handler) echo.HandlerFunc { return h.ReturnMold },
			path: "/api/v1/molds/:id/return", body: map[string]any{"location": "工模架 A1"},
		},
		{
			name: "start repair requires in stock", status: statusLoaned, handle: func(h *Handler) echo.HandlerFunc { return h.RepairMold },
			path: "/api/v1/molds/:id/repair", body: map[string]any{"reason": "顶针磨损", "completed": false},
		},
		{
			name: "complete repair requires repairing", status: statusInStock, handle: func(h *Handler) echo.HandlerFunc { return h.RepairMold },
			path: "/api/v1/molds/:id/repair", body: map[string]any{"reason": "顶针磨损", "completed": true},
		},
		{
			name: "start maintenance requires in stock", status: statusRepairing, handle: func(h *Handler) echo.HandlerFunc { return h.MaintainMold },
			path: "/api/v1/molds/:id/maintenance", body: map[string]any{"completed": false},
		},
		{
			name: "complete maintenance requires maintenance", status: statusInStock, handle: func(h *Handler) echo.HandlerFunc { return h.MaintainMold },
			path: "/api/v1/molds/:id/maintenance", body: map[string]any{"completed": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openMoldTestDB(t)
			handler := NewHandler(db)
			item := model.Mold{Code: "MOLD-CONFLICT-" + tt.name, Name: "状态校验模具", Status: tt.status, CavityCount: 1}
			if err := db.Create(&item).Error; err != nil {
				t.Fatalf("seed mold: %v", err)
			}

			rec := performMoldJSON(t, tt.handle(handler), http.MethodPost, tt.path, tt.body, map[string]string{"id": strconv.FormatUint(uint64(item.ID), 10)})
			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
			}

			var stored model.Mold
			if err := db.First(&stored, item.ID).Error; err != nil {
				t.Fatalf("reload mold: %v", err)
			}
			if stored.Status != tt.status {
				t.Fatalf("status changed to %q, want %q", stored.Status, tt.status)
			}
			assertMoldEventCount(t, db, item.ID, 0)
		})
	}
}

func TestTransitionReturnsConflictWhenStateTurnsStaleBeforeWrite(t *testing.T) {
	db := openMoldTestDB(t)
	service := NewService(db)
	item := model.Mold{Code: "MOLD-STALE-001", Name: "并发校验模具", Status: statusInStock, CavityCount: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed mold: %v", err)
	}

	fired := false
	if err := db.Callback().Update().Before("gorm:update").Register("mold_test_turn_state_stale", func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "molds" {
			return
		}
		fired = true
		if err := tx.Exec("UPDATE molds SET status = ? WHERE id = ?", statusLoaned, item.ID).Error; err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatalf("register stale-state callback: %v", err)
	}

	_, err := service.Transition(item.ID, Transition{
		Status: statusLoaned, EventType: eventLoan, Location: "注塑车间", Counterparty: "注塑部",
	})
	if !errors.Is(err, ErrMoldStatusConflict) {
		t.Fatalf("transition error = %v, want ErrMoldStatusConflict", err)
	}
	if !fired {
		t.Fatal("stale-state callback was not invoked")
	}
	assertMoldEventCount(t, db, item.ID, 0)
}

func TestDeleteMoldRejectsMissingID(t *testing.T) {
	db := openMoldTestDB(t)
	handler := NewHandler(db)

	rec := performMoldJSON(t, handler.DeleteMold, http.MethodDelete, "/api/v1/molds/:id", nil, map[string]string{"id": "999"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing mold status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func openMoldTestDB(t *testing.T) *gorm.DB {
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
	if err := db.AutoMigrate(&model.Mold{}, &model.MoldEvent{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func performMoldJSON(t *testing.T, handler echo.HandlerFunc, method string, path string, body any, params map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	e := echo.New()
	e.Validator = &testValidator{validate: validator.New()}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
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
	if err := handler(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}
	return rec
}

func decodeMoldJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode json %q: %v", rec.Body.String(), err)
	}
}

func assertMoldEventCount(t *testing.T, db *gorm.DB, moldID uint, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.MoldEvent{}).Where("mold_id = ?", moldID).Count(&count).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != want {
		t.Fatalf("event count = %d, want %d", count, want)
	}
}
