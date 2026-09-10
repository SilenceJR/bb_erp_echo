package product

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"bb_erp_echo/internal/model"
	"github.com/labstack/echo/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProductDeleteIsBlockedByMoldReference(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Product{}, &model.Mold{}, &model.MoldLocation{}, &model.MoldDrawing{}, &model.ImageFile{}, &model.FileCleanupTask{}, &model.InventoryBalance{}, &model.InventoryLedger{}, &model.WorkOrder{}); err != nil {
		t.Fatal(err)
	}
	product := model.Product{ProductModel: "P-100", Status: model.StatusActive}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	location := model.MoldLocation{Code: "A1-1", Status: model.MoldLocationActive}
	if err := db.Create(&location).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Mold{ProductID: product.ID, MoldType: model.MoldTypeSingle, CavityCount: "1*1", LocationID: location.ID}).Error; err != nil {
		t.Fatal(err)
	}
	handler := &Handler{DB: db}
	e := echo.New()
	e.DELETE("/api/v1/products/:id", handler.DeleteProduct)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/products/1", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDecodeProductRows(t *testing.T) {
	rows, errs := decodeProductRows([][]string{
		{"序号", "产品型号", "客户型号", "产品材料", "是否刷墨", "状态"},
		{"1", "P-100", "C-1", "ABS 白", "是", "启用"},
	}, map[string]bool{"P-100": true})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(rows) != 1 || rows[0].Product.ProductModel != "P-100" || !rows[0].Product.InkRequired || !rows[0].HasDir {
		t.Fatalf("decoded rows = %+v", rows)
	}
}

func TestStageProductImagesIgnoresDirectoryEntry(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "products.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(file)
	if _, err := zw.Create("P-100/"); err != nil {
		t.Fatal(err)
	}
	writer, err := zw.Create("P-100/size.png")
	if err != nil {
		t.Fatal(err)
	}
	png, err := os.ReadFile(filepath.Join("..", "..", "web", "public", "bobang-logo-hd.png"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bytes.NewReader(png).WriteTo(writer); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	handler := &Handler{StorageRoot: filepath.Join(dir, "uploads")}
	staged, err := handler.stageProductImages([]productPackageRow{{Product: model.Product{ProductModel: "P-100"}}}, archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupStagedProductImages(handler.StorageRoot, staged)
	if len(staged) != 1 || staged[0].OriginalName != "size.png" {
		t.Fatalf("staged = %+v", staged)
	}
}
