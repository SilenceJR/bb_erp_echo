package mold

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	filemodule "bb_erp_echo/internal/file"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/spreadsheet"

	"github.com/labstack/echo/v5"
)

func TestNaturalAssetSortAndCategoryInference(t *testing.T) {
	names := []string{"CYF-10.jpg", "CYF-2.jpg", "CYF-1.jpg"}
	sort.SliceStable(names, func(i, j int) bool { return naturalAssetLess(names[i], names[j]) })
	if got := names[0] + "," + names[1] + "," + names[2]; got != "CYF-1.jpg,CYF-2.jpg,CYF-10.jpg" {
		t.Fatalf("unexpected natural order: %s", got)
	}
	if got := inferCategory("CYF-1-未知.jpg"); got != "" {
		t.Fatalf("unknown image category should require preview correction, got %q", got)
	}
	if got := inferCategory("CYF-1-前模局部.jpg"); got != "supplement" {
		t.Fatalf("unexpected supplement category: %q", got)
	}
}

func TestMoldImportAcceptsGalleryImageExtensions(t *testing.T) {
	for _, ext := range []string{".jpg", ".JPG", ".jfif", ".png", ".gif", ".webp", ".heic", ".HEIC", ".heif", ".avif", ".bmp", ".tif", ".tiff", ".svg"} {
		if !filemodule.AllowedImageExtension(ext) {
			t.Fatalf("expected mold import to accept %s", ext)
		}
	}
}

func TestReadPackageTemplateAndSharedImage(t *testing.T) {
	xlsx, err := spreadsheet.XLSXWriter{}.Write(t.Context(), spreadsheet.SpreadsheetDocument{
		SheetName: "模具", Columns: moldColumns,
		Rows: [][]string{{"", "A", "产品 A", "单模", "A1-1", "", "99", ""}, {"", "B", "产品 B", "单模", "A1-1", "", "0", ""}}, TotalRows: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for name, content := range map[string][]byte{
		"molds.xlsx":                            xlsx,
		"images/A/product_material/A-1.png":     tinyPNG,
		"images/A+B/product_material/A+B-2.png": tinyPNG,
		"drawings/A/A.dwg":                      []byte("dwg"),
	} {
		w, createErr := zw.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := w.Write(content); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/molds.zip"
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := (&Handler{}).readPackage(path, int64(archive.Len()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Errors) != 0 || len(data.Rows) != 2 || len(data.Images) != 2 || len(data.Drawings) != 1 {
		t.Fatalf("unexpected package parse: rows=%d images=%d drawings=%d errors=%v", len(data.Rows), len(data.Images), len(data.Drawings), data.Errors)
	}
	var shared packageAsset
	for _, image := range data.Images {
		if image.Name == "A+B-2.png" {
			shared = image
		}
	}
	if len(shared.Codes) != 2 || shared.Category != "product_material" {
		t.Fatalf("shared image was not copied to both molds: %+v", shared)
	}
}

func TestMoldImportTemplateIsZipAndCanBeReadBack(t *testing.T) {
	archiveData, err := buildMoldImportTemplate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"molds.xlsx":                        true,
		"locations.json":                    true,
		"images/":                           true,
		"images/MOLD-001/":                  true,
		"images/MOLD-001/product_material/": true,
		"images/MOLD-001/supplement/":       true,
		"drawings/":                         true,
		"drawings/MOLD-001/":                true,
	}
	seen := make(map[string]bool, len(reader.File))
	for _, item := range reader.File {
		seen[item.Name] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("template entries=%v, want=%v", seen, want)
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("template missing entry %q", name)
		}
	}
	path := t.TempDir() + "/mold-template.zip"
	if err := os.WriteFile(path, archiveData, 0600); err != nil {
		t.Fatal(err)
	}
	data, err := (&Handler{}).readPackage(path, int64(len(archiveData)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Errors) != 0 || len(data.Rows) != 1 || data.Rows[0].MoldNumber != "MOLD-001" || len(data.Locations) != 2 || len(data.Unresolved) != 0 {
		t.Fatalf("template readback rows=%+v locations=%+v errors=%v unresolved=%v", data.Rows, data.Locations, data.Errors, data.Unresolved)
	}
}

func TestMoldImportTemplateDownloadContract(t *testing.T) {
	e := echo.New()
	record := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/molds/import-template", nil), record)
	if err := (&Handler{}).ImportTemplate(ctx); err != nil {
		t.Fatal(err)
	}
	if record.Code != http.StatusOK {
		t.Fatalf("status=%d", record.Code)
	}
	if got := record.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("content type=%q", got)
	}
	if got := record.Header().Get("Content-Disposition"); got == "" || !bytes.Contains([]byte(got), []byte(".zip")) {
		t.Fatalf("content disposition=%q", got)
	}
	if _, err := zip.NewReader(bytes.NewReader(record.Body.Bytes()), int64(record.Body.Len())); err != nil {
		t.Fatalf("download is not zip: %v", err)
	}
}

func TestMoldExportCanBeReadBackAsImportPackage(t *testing.T) {
	root := t.TempDir()
	db := openMoldTestDB(t)
	if err := SeedLocations(db); err != nil {
		t.Fatal(err)
	}
	var location model.MoldLocation
	if err := db.Where("code = ?", "A1-1").First(&location).Error; err != nil {
		t.Fatal(err)
	}
	moldItem := model.Mold{MoldNumber: "MOLD-010", Model: "产品 A", MoldType: model.MoldTypeSingle, LocationID: location.ID, Remark: "测试"}
	if err := db.Create(&moldItem).Error; err != nil {
		t.Fatal(err)
	}
	imageRelative := "mold/export/mold-010.png"
	imagePath := filepath.Join(root, filepath.FromSlash(imageRelative))
	if err := os.MkdirAll(filepath.Dir(imagePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, tinyPNG, 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ImageFile{OwnerType: "mold", OwnerID: moldItem.ID, Category: "product_material", OriginalName: "mold-010.png", Size: int64(len(tinyPNG)), MimeType: "image/png", Extension: ".png", StoragePath: imageRelative}).Error; err != nil {
		t.Fatal(err)
	}
	drawingRelative := "mold/export/mold-010.dwg"
	drawingPath := filepath.Join(root, filepath.FromSlash(drawingRelative))
	if err := os.WriteFile(drawingPath, []byte("dwg"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.MoldDrawing{MoldID: moldItem.ID, OriginalName: "mold-010.dwg", Size: 3, MimeType: "application/octet-stream", Extension: ".dwg", StoragePath: drawingRelative}).Error; err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	record := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/molds/export", nil), record)
	if err := (&Handler{DB: db, StorageRoot: root}).Export(ctx); err != nil {
		t.Fatal(err)
	}
	if record.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", record.Code, record.Body.String())
	}
	archiveReader, err := zip.NewReader(bytes.NewReader(record.Body.Bytes()), int64(record.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	archiveEntries := make(map[string]bool, len(archiveReader.File))
	for _, item := range archiveReader.File {
		archiveEntries[item.Name] = true
	}
	for _, name := range []string{"images/", "images/MOLD-010/", "images/MOLD-010/product_material/", "images/MOLD-010/supplement/", "drawings/", "drawings/MOLD-010/"} {
		if !archiveEntries[name] {
			t.Fatalf("formal export missing directory entry %q", name)
		}
	}
	archivePath := filepath.Join(t.TempDir(), "export.zip")
	if err := os.WriteFile(archivePath, record.Body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := (&Handler{}).readPackage(archivePath, int64(record.Body.Len()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Errors) != 0 || len(data.Rows) != 1 || len(data.Images) != 1 || len(data.Drawings) != 1 || len(data.Locations) != 2 {
		t.Fatalf("export readback rows=%d images=%d drawings=%d locations=%d errors=%v", len(data.Rows), len(data.Images), len(data.Drawings), len(data.Locations), data.Errors)
	}
}

var tinyPNG = func() []byte {
	var output bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 32, G: 64, B: 96, A: 255})
	_ = png.Encode(&output, img)
	return output.Bytes()
}()
