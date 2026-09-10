package middleware

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"bb_erp_echo/internal/model"

	"github.com/labstack/echo/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAuditRecordsUnhandledHandlerErrorAsServerFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	handler := Audit(db, slog.New(slog.NewTextHandler(io.Discard, nil)))(func(*echo.Context) error {
		return errors.New("handler failed")
	})
	if err := handler(c); err == nil {
		t.Fatal("handler error = nil, want error")
	}

	var audit model.AuditLog
	if err := db.First(&audit).Error; err != nil {
		t.Fatalf("find audit log: %v", err)
	}
	if audit.Result != "failed" {
		t.Fatalf("audit result = %q, want failed", audit.Result)
	}
	if audit.Status == http.StatusOK {
		t.Fatalf("failed audit status = %d, must not be 200", audit.Status)
	}
	if audit.Status != http.StatusInternalServerError {
		t.Fatalf("failed audit status = %d, want %d", audit.Status, http.StatusInternalServerError)
	}
	if audit.Action != "api:create" {
		t.Fatalf("failed audit action = %q, want api:create", audit.Action)
	}
}

func TestAuditActionUsesStableBusinessCodes(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   string
		record bool
	}{
		{method: http.MethodPost, path: "/api/v1/suppliers", want: "suppliers:create", record: true},
		{method: http.MethodPatch, path: "/api/v1/customers/9", want: "customers:update", record: true},
		{method: http.MethodDelete, path: "/api/v1/molds/9", want: "molds:delete", record: true},
		{method: http.MethodPost, path: "/api/v1/inventory-documents/9/post", want: "warehouses:post", record: true},
		{method: http.MethodPost, path: "/api/v1/inventory-documents/9/reverse", want: "warehouses:reverse", record: true},
		{method: http.MethodPost, path: "/api/v1/workorder/9/dispatch", want: "workorder:dispatch", record: true},
		{method: http.MethodPost, path: "/api/v1/workorder/9/pause", want: "workorder:pause", record: true},
		{method: http.MethodPost, path: "/api/v1/workorder/9/resume", want: "workorder:resume", record: true},
		{method: http.MethodPost, path: "/api/v1/workorder/9/urgent", want: "workorder:urgent", record: true},
		{method: http.MethodPost, path: "/api/v1/workorder/9/complete", want: "workorder:complete", record: true},
		{method: http.MethodPost, path: "/api/v1/workorder/department-tasks/9/start", want: "workorder:start", record: true},
		{method: http.MethodPost, path: "/api/v1/workorder/department-tasks/9/partial-complete", want: "workorder:partial_complete", record: true},
		{method: http.MethodPatch, path: "/api/v1/system/users/9/status", want: "users:status", record: true},
		{method: http.MethodPost, path: "/api/v1/system/users/9/roles", want: "users:assign_roles", record: true},
		{method: http.MethodPost, path: "/api/v1/system/roles/9/permissions", want: "roles:assign_permissions", record: true},
		{method: http.MethodPost, path: "/api/v1/molds/9/drawings", want: "molds:drawing_upload", record: true},
		{method: http.MethodPost, path: "/api/v1/warehouse/products/9/movements", want: "warehouses:movement", record: true},
		{method: http.MethodPost, path: "/api/v1/customers/import/commit", want: "customers:import", record: true},
		{method: http.MethodPost, path: "/api/v1/customers/import/preview", record: false},
		{method: http.MethodGet, path: "/api/v1/customers", record: false},
	}
	for _, tt := range cases {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			got, record := auditAction(tt.method, tt.path)
			if record != tt.record || got != tt.want {
				t.Fatalf("auditAction() = (%q, %v), want (%q, %v)", got, record, tt.want, tt.record)
			}
		})
	}
}

func TestAuditSkipsImportPreview(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.AuditLog{}); err != nil {
		t.Fatalf("migrate audit log: %v", err)
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customers/import/preview", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	handler := Audit(db, slog.New(slog.NewTextHandler(io.Discard, nil)))(func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("preview handler returned error: %v", err)
	}
	var count int64
	if err := db.Model(&model.AuditLog{}).Count(&count).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if count != 0 {
		t.Fatalf("preview audit count = %d, want 0", count)
	}
}
