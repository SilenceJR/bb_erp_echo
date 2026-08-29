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
}
