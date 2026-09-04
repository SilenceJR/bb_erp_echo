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
	if err := db.AutoMigrate(&model.Mold{}, &model.MoldLocation{}, &model.MoldDrawing{}, &model.ImageFile{}); err != nil {
		t.Fatal(err)
	}
	return db
}
