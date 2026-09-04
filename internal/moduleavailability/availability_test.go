package moduleavailability

import (
	"errors"
	"net/http"
	"testing"

	"bb_erp_echo/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCheckReturnsStableUnavailableErrorWithoutSQLDetails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	err = Check(db, "仓库", Requirement{Model: &model.Warehouse{}, Name: "warehouses"})
	var unavailable *Error
	if !errors.As(err, &unavailable) {
		t.Fatalf("check error = %v, want module availability error", err)
	}
	if unavailable.Code() != Code || unavailable.StatusCode() != http.StatusServiceUnavailable {
		t.Fatalf("error contract = %s/%d", unavailable.Code(), unavailable.StatusCode())
	}
	if unavailable.PublicMessage() == "" || len(unavailable.MissingTable) != 1 || unavailable.MissingTable[0] != "warehouses" {
		t.Fatalf("error body = %+v", unavailable)
	}
}

func TestCheckSucceedsAfterRequiredTableExists(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Supplier{}); err != nil {
		t.Fatalf("migrate supplier: %v", err)
	}
	if err := Check(db, "供应商", Requirement{Model: &model.Supplier{}, Name: "suppliers"}); err != nil {
		t.Fatalf("available module rejected: %v", err)
	}
}
