package mold

import (
	"os"
	"path/filepath"
	"testing"

	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/pagination"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMoldCRUDAndRules(t *testing.T) {
	db := openMoldTestDB(t)
	if err := SeedLocations(db); err != nil {
		t.Fatal(err)
	}
	var location model.MoldLocation
	db.Where("code = ?", "A1-1").First(&location)
	s := NewService(db)
	item, err := s.Create(Input{MoldNumber: "M-001", Model: "产品 A", MoldType: model.MoldTypeCommon, LocationID: location.ID, CommonGroupNo: "G1"})
	if err != nil {
		t.Fatal(err)
	}
	if item.MoldNumber != "M-001" {
		t.Fatalf("unexpected mold: %+v", item)
	}
	if _, err := s.Create(Input{MoldNumber: "M-002", Model: "产品 B", MoldType: model.MoldTypeCommon, LocationID: location.ID}); err != ErrMoldGroupRequired {
		t.Fatalf("group validation = %v", err)
	}
	if _, err := s.Create(Input{MoldNumber: "M-001", Model: "重复", MoldType: model.MoldTypeSingle, LocationID: location.ID}); err != ErrMoldNumberConflict {
		t.Fatalf("duplicate validation = %v", err)
	}
}

func TestSeedLocationsIsCompleteIdempotentAndPreservesDisabledRows(t *testing.T) {
	db := openMoldTestDB(t)
	if err := SeedLocations(db); err != nil {
		t.Fatal(err)
	}
	var initial []model.MoldLocation
	if err := db.Order("code asc").Find(&initial).Error; err != nil {
		t.Fatal(err)
	}
	if len(initial) != 101 {
		t.Fatalf("seeded locations=%d, want 101", len(initial))
	}
	var location model.MoldLocation
	if err := db.Where("code = ?", "A1-1").First(&location).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&location).Update("status", model.MoldLocationDisabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := SeedLocations(db); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&model.MoldLocation{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 101 {
		t.Fatalf("idempotent seed count=%d, want 101", count)
	}
	var preserved model.MoldLocation
	if err := db.Where("code = ?", "A1-1").First(&preserved).Error; err != nil {
		t.Fatal(err)
	}
	if preserved.ID != location.ID || preserved.Status != model.MoldLocationDisabled {
		t.Fatalf("existing location changed: before=%+v after=%+v", location, preserved)
	}
	var pallet model.MoldLocation
	if err := db.Where("code = ?", model.MoldLocationPallet).First(&pallet).Error; err != nil {
		t.Fatal(err)
	}
}

func TestBulkCreateLocationsIsIdempotentAndTransactional(t *testing.T) {
	db := openMoldTestDB(t)
	if err := SeedLocations(db); err != nil {
		t.Fatal(err)
	}
	s := NewService(db)
	created, err := s.BulkCreateLocations(BulkLocationInput{Zone: " e ", Rows: 2, Columns: 2})
	if err != nil || created.Created != 4 {
		t.Fatalf("first bulk create=%+v err=%v", created, err)
	}
	var location model.MoldLocation
	if err := db.Where("code = ?", "E1-1").First(&location).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&location).Update("status", model.MoldLocationDisabled).Error; err != nil {
		t.Fatal(err)
	}
	created, err = s.BulkCreateLocations(BulkLocationInput{Zone: "E", Rows: 2, Columns: 2})
	if err != nil || created.Created != 0 {
		t.Fatalf("repeat bulk create=%+v err=%v", created, err)
	}
	created, err = s.BulkCreateLocations(BulkLocationInput{Zone: "E", Rows: 8, Columns: 8})
	if err != nil || created.Created != 60 {
		t.Fatalf("expanded bulk create=%+v err=%v", created, err)
	}
	var preserved model.MoldLocation
	if err := db.Where("code = ?", "E1-1").First(&preserved).Error; err != nil {
		t.Fatal(err)
	}
	if preserved.ID != location.ID || preserved.Status != model.MoldLocationDisabled {
		t.Fatalf("bulk expansion changed existing location: before=%+v after=%+v", location, preserved)
	}
	for _, input := range []BulkLocationInput{{Zone: "A1", Rows: 1, Columns: 1}, {Zone: "TOOLONGZONE", Rows: 1, Columns: 1}, {Zone: "A", Rows: 0, Columns: 1}, {Zone: "A", Rows: 1, Columns: 101}} {
		if _, err := s.BulkCreateLocations(input); err == nil {
			t.Fatalf("invalid bulk input unexpectedly succeeded: %+v", input)
		}
	}
	if err := db.Exec("CREATE TRIGGER mold_location_bulk_fail BEFORE INSERT ON mold_locations WHEN NEW.code LIKE 'ROLLBACK%' BEGIN SELECT RAISE(ABORT, 'forced failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.BulkCreateLocations(BulkLocationInput{Zone: "ROLLBACK", Rows: 2, Columns: 2}); err == nil {
		t.Fatal("transaction failure unexpectedly succeeded")
	}
	var rollbackCount int64
	if err := db.Model(&model.MoldLocation{}).Where("code LIKE ?", "ROLLBACK%").Count(&rollbackCount).Error; err != nil {
		t.Fatal(err)
	}
	if rollbackCount != 0 {
		t.Fatalf("failed transaction left %d rows", rollbackCount)
	}
	created, err = s.BulkCreateLocations(BulkLocationInput{Zone: "Z", Rows: 100, Columns: 100})
	if err != nil || created.Created != 10000 {
		t.Fatalf("maximum bulk create=%+v err=%v", created, err)
	}
	created, err = s.BulkCreateLocations(BulkLocationInput{Zone: "Z", Rows: 100, Columns: 100})
	if err != nil || created.Created != 0 {
		t.Fatalf("maximum bulk repeat=%+v err=%v", created, err)
	}
}

func TestMoldCountsBulkMoveAndPhysicalDelete(t *testing.T) {
	root := t.TempDir()
	db := openMoldTestDB(t)
	_ = SeedLocations(db)
	var location model.MoldLocation
	db.Where("code = ?", "A1-1").First(&location)
	var target model.MoldLocation
	db.Where("code = ?", "B1-1").First(&target)
	s := NewServiceWithStorage(db, root)
	first, err := s.Create(Input{MoldNumber: "M-010", Model: "A", MoldType: model.MoldTypeSingle, LocationID: location.ID})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(Input{MoldNumber: "M-011", Model: "B", MoldType: model.MoldTypeSingle, LocationID: location.ID})
	if err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(root, "mold", "2026", "01", "image.jpg")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ImageFile{OwnerType: "mold", OwnerID: first.ID, Category: "supplement", OriginalName: "image.jpg", StoragePath: "mold/2026/01/image.jpg", UploadedBy: 1}).Error; err != nil {
		t.Fatal(err)
	}
	result, err := s.List(pagination.Query{Page: 1, PageSize: 20}, ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[1].ImageCount != 1 {
		t.Fatalf("image count = %+v", result.Items)
	}
	if err := s.BulkMove(BulkMoveInput{MoldIDs: []uint{first.ID, second.ID}, LocationID: target.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		t.Fatalf("image still exists: %v", err)
	}
}

func openMoldTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Mold{}, &model.MoldLocation{}, &model.MoldDrawing{}, &model.ImageFile{}, &model.FileCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	return db
}
