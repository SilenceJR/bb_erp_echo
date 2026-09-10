package mold

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"bb_erp_echo/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMoldServiceLinksProductAndValidatesStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mold-service-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Product{}, &model.Mold{}, &model.MoldLocation{}, &model.MoldDrawing{}, &model.ImageFile{}); err != nil {
		t.Fatal(err)
	}
	product := model.Product{ProductModel: "P-200", Status: model.StatusActive}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	location := model.MoldLocation{Code: "A1-1", Status: model.MoldLocationActive}
	if err := db.Create(&location).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(db)
	created, err := service.Create(Input{ProductID: product.ID, MoldType: model.MoldTypeSingle, CavityCount: "1*2", LocationID: location.ID})
	if err != nil {
		t.Fatal(err)
	}
	if created.ProductID != product.ID {
		t.Fatalf("product link = %d", created.ProductID)
	}
	if err := db.Model(&product).Update("status", model.StatusDisabled).Error; err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(Input{ProductID: product.ID, MoldType: model.MoldTypeSingle, CavityCount: "1*1", LocationID: location.ID})
	if !errors.Is(err, ErrProductDisabled) {
		t.Fatalf("disabled product error = %v", err)
	}
}

func TestDecodeMoldRowsUsesProductModelAndSequence(t *testing.T) {
	rows, errs := decodeMoldRows([][]string{
		{"序号", "产品型号", "模具类型", "模穴数", "模具位置", "共模组号", "备注"},
		{"001", "P-200", "单模", "1*4", "A1-1", "", ""},
	}, map[string]bool{"P-200/001": true})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(rows) != 1 || rows[0].ProductModel != "P-200" || rows[0].Sequence != "001" || rows[0].CavityCount != "1*4" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestStageMoldAssetsSkipsDirectoriesAndCopiesDrawing(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "molds.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	if _, err := zw.Create("P-200/001/"); err != nil {
		t.Fatal(err)
	}
	imageWriter, err := zw.Create("P-200/001/mold.png")
	if err != nil {
		t.Fatal(err)
	}
	imageBytes, err := os.ReadFile(filepath.Join("..", "..", "web", "public", "bobang-logo-hd.png"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bytes.NewReader(imageBytes).WriteTo(imageWriter); err != nil {
		t.Fatal(err)
	}
	drawingWriter, err := zw.Create("P-200/001/drawing.dwg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := drawingWriter.Write([]byte("dwg")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	handler := &Handler{StorageRoot: filepath.Join(dir, "uploads")}
	staged, err := handler.stageMoldAssets([]moldPackageRow{{ProductModel: "P-200", Sequence: "001"}}, archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupStagedMoldAssets(handler.StorageRoot, staged)
	if len(staged) != 2 {
		t.Fatalf("staged count = %d", len(staged))
	}
}
