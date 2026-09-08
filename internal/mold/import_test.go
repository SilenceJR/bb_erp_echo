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
	"strings"
	"testing"

	filemodule "bb_erp_echo/internal/file"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/spreadsheet"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
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
	data, err := readTestPackage(t, path, int64(archive.Len()), nil)
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
		"images/MOLD-002+MOLD-003/共用/":      true,
		"images/MOLD-002+MOLD-003/共用/product_material/":       true,
		"images/MOLD-002+MOLD-003/共用/supplement/":             true,
		"images/MOLD-002+MOLD-003/MOLD-002/":                  true,
		"images/MOLD-002+MOLD-003/MOLD-002/product_material/": true,
		"images/MOLD-002+MOLD-003/MOLD-002/supplement/":       true,
		"images/MOLD-002+MOLD-003/MOLD-003/":                  true,
		"images/MOLD-002+MOLD-003/MOLD-003/product_material/": true,
		"images/MOLD-002+MOLD-003/MOLD-003/supplement/":       true,
		"drawings/":                            true,
		"drawings/MOLD-001/":                   true,
		"drawings/MOLD-002+MOLD-003/共用/":       true,
		"drawings/MOLD-002+MOLD-003/MOLD-002/": true,
		"drawings/MOLD-002+MOLD-003/MOLD-003/": true,
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
	data, err := readTestPackage(t, path, int64(len(archiveData)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Errors) != 0 || len(data.Rows) != 3 || data.Rows[0].MoldNumber != "MOLD-001" || len(data.Locations) != 101 || len(data.Unresolved) != 0 {
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
	data, err := readTestPackage(t, archivePath, int64(record.Body.Len()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Errors) != 0 || len(data.Rows) != 1 || len(data.Images) != 1 || len(data.Drawings) != 1 || len(data.Locations) != 101 {
		t.Fatalf("export readback rows=%d images=%d drawings=%d locations=%d errors=%v", len(data.Rows), len(data.Images), len(data.Drawings), len(data.Locations), data.Errors)
	}
}

func TestReadPackageExplicitLocationsAddsOnlyPallet(t *testing.T) {
	xlsx, err := spreadsheet.XLSXWriter{}.Write(t.Context(), spreadsheet.SpreadsheetDocument{
		SheetName: "模具", Columns: moldColumns,
		Rows: [][]string{{"", "LEGACY-001", "旧模具", "单模", "legacy-1", "", "0", ""}}, TotalRows: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for name, content := range map[string][]byte{
		"molds.xlsx":     xlsx,
		"locations.json": []byte(`[{"code":"legacy-1","status":"disabled"}]`),
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
	path := filepath.Join(t.TempDir(), "legacy.zip")
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := readTestPackage(t, path, int64(archive.Len()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Errors) != 0 || len(data.Locations) != 2 {
		t.Fatalf("legacy locations=%+v errors=%v", data.Locations, data.Errors)
	}
	if data.Locations[0].Code != "legacy-1" || data.Locations[0].Status != model.MoldLocationDisabled || data.Locations[1].Code != model.MoldLocationPallet {
		t.Fatalf("legacy locations lost or reordered: %+v", data.Locations)
	}
}

func TestMoldExportAndImportSharedCommonAssets(t *testing.T) {
	root := t.TempDir()
	db := openMoldTestDB(t)
	if err := SeedLocations(db); err != nil {
		t.Fatal(err)
	}
	var location model.MoldLocation
	if err := db.Where("code = ?", "A1-1").First(&location).Error; err != nil {
		t.Fatal(err)
	}
	molds := []model.Mold{
		{MoldNumber: "BB56442", Model: "型号 1", MoldType: model.MoldTypeCommon, CommonGroupNo: "G-001", LocationID: location.ID},
		{MoldNumber: "BB56443", Model: "型号 2", MoldType: model.MoldTypeCommon, CommonGroupNo: "G-001", LocationID: location.ID},
	}
	for i := range molds {
		if err := db.Create(&molds[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i, item := range molds {
		relative := filepath.ToSlash(filepath.Join("mold", "shared", string(rune('a'+i))+".png"))
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, tinyPNG, 0600); err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.ImageFile{OwnerType: "mold", OwnerID: item.ID, Category: "product_material", OriginalName: "shared.png", Size: int64(len(tinyPNG)), MimeType: "image/png", Extension: ".png", StoragePath: relative}).Error; err != nil {
			t.Fatal(err)
		}
	}
	onlyRelative := "mold/shared/only.png"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(onlyRelative)), tinyPNG, 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ImageFile{OwnerType: "mold", OwnerID: molds[1].ID, Category: "supplement", OriginalName: "only.png", Size: int64(len(tinyPNG)), MimeType: "image/png", Extension: ".png", StoragePath: onlyRelative}).Error; err != nil {
		t.Fatal(err)
	}
	for i, item := range molds {
		relative := filepath.ToSlash(filepath.Join("mold", "shared", string(rune('a'+i))+".dwg"))
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(relative)), []byte("same-drawing"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.MoldDrawing{MoldID: item.ID, OriginalName: "shared.dwg", Size: 12, MimeType: "application/octet-stream", Extension: ".dwg", StoragePath: relative}).Error; err != nil {
			t.Fatal(err)
		}
	}
	// A historical number can equal the shared outer directory; its presence
	// must not steal the group's drawings during exported-package readback.
	if err := db.Create(&model.Mold{MoldNumber: "BB56442+BB56443", Model: "历史编号", MoldType: model.MoldTypeSingle, LocationID: location.ID}).Error; err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	record := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/molds/export", nil), record)
	if err := (&Handler{DB: db, StorageRoot: root}).Export(ctx); err != nil {
		t.Fatal(err)
	}
	archiveReader, err := zip.NewReader(bytes.NewReader(record.Body.Bytes()), int64(record.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]bool{}
	for _, item := range archiveReader.File {
		entries[item.Name] = true
	}
	if !entries["images/BB56442+BB56443/共用/product_material/shared.png"] || !entries["images/BB56442+BB56443/BB56443/supplement/only.png"] || !entries["drawings/BB56442+BB56443/共用/shared.dwg"] {
		t.Fatalf("shared archive entries missing: %v", entries)
	}
	archivePath := filepath.Join(t.TempDir(), "shared.zip")
	if err := os.WriteFile(archivePath, record.Body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := readTestPackage(t, archivePath, int64(record.Body.Len()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Errors) != 0 || len(data.Images) != 2 || len(data.Drawings) != 1 {
		t.Fatalf("shared package rows=%d images=%d drawings=%d errors=%v", len(data.Rows), len(data.Images), len(data.Drawings), data.Errors)
	}
	staged, err := (&Handler{StorageRoot: t.TempDir()}).stageAssets(data)
	if err != nil || len(staged) != 5 {
		t.Fatalf("shared package staging failed: assets=%d err=%v", len(staged), err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error { return replaceMoldData(tx, data, staged, 0) }); err != nil {
		t.Fatalf("shared package replacement failed: %v", err)
	}
	var restoredImages, restoredDrawings int64
	db.Model(&model.ImageFile{}).Count(&restoredImages)
	db.Model(&model.MoldDrawing{}).Count(&restoredDrawings)
	if restoredImages != 3 || restoredDrawings != 2 {
		t.Fatalf("restored assets: images=%d drawings=%d", restoredImages, restoredDrawings)
	}

	var sharedImage, specificImage packageAsset
	for _, image := range data.Images {
		if image.Name == "shared.png" {
			sharedImage = image
		} else if image.Name == "only.png" {
			specificImage = image
		}
	}
	if len(sharedImage.Codes) != 2 || len(specificImage.Codes) != 1 || specificImage.Codes[0] != "BB56443" || len(data.Drawings[0].Codes) != 2 {
		t.Fatalf("shared assets were assigned incorrectly: images=%+v drawings=%+v", data.Images, data.Drawings)
	}
}

func TestGroupedPackageMatchingIsExactAndRejectsUnknownMembers(t *testing.T) {
	known := map[string]Input{
		"FL1408": {MoldNumber: "FL1408", MoldType: model.MoldTypeCommon, CommonGroupNo: "G"},
		"FL2814": {MoldNumber: "FL2814", MoldType: model.MoldTypeCommon, CommonGroupNo: "G"},
	}
	entry := &zip.File{Name: "前模.png", UncompressedSize64: 1}
	asset, ok := parseGroupedImageAsset(entry, []string{"images", "FL1408+FL2814", "FL1408-尺寸.png"}, known)
	if !ok || len(asset.Codes) != 1 || asset.Codes[0] != "FL1408" || asset.Category != "supplement" {
		t.Fatalf("exact grouped match=%+v ok=%v", asset, ok)
	}
	asset, ok = parseGroupedImageAsset(entry, []string{"images", "FL1408+FL2814", "FSL2214-尺寸.png"}, known)
	if !ok || len(asset.Codes) != 0 {
		t.Fatalf("unknown alias should remain unresolved: %+v ok=%v", asset, ok)
	}
	if _, ok := parseGroupedImageAsset(entry, []string{"images", "FL1408+UNKNOWN", "FL1408-尺寸.png"}, known); ok {
		t.Fatal("unknown shared member unexpectedly accepted")
	}
	known = map[string]Input{
		"FL1408":  {MoldNumber: "FL1408", MoldType: model.MoldTypeCommon, CommonGroupNo: "SCREENSHOT-GROUP"},
		"FL2814":  {MoldNumber: "FL2814", MoldType: model.MoldTypeCommon, CommonGroupNo: "SCREENSHOT-GROUP"},
		"FL2214":  {MoldNumber: "FL2214", MoldType: model.MoldTypeCommon, CommonGroupNo: "SCREENSHOT-GROUP"},
		"FL15083": {MoldNumber: "FL15083", MoldType: model.MoldTypeCommon, CommonGroupNo: "SCREENSHOT-GROUP"},
	}
	asset, ok = parseGroupedImageAsset(entry, []string{"images", "FL1408+FL2814+FL2214+FL15083", "FL2214-1.png"}, known)
	if !ok || asset.Category != "product_material" || len(asset.Codes) != 1 || asset.Codes[0] != "FL2214" {
		t.Fatalf("unordered shared outer or flat material was rejected: %+v ok=%v", asset, ok)
	}
	if asset, ok = parseGroupedImageAsset(entry, []string{"images", "FL1408+FL2814+FL2214+FL15083", "共用", "原理图.png"}, known); !ok || asset.Category != "supplement" || len(asset.Codes) != 4 {
		t.Fatalf("shared schematic classification failed: %+v ok=%v", asset, ok)
	}

	legacyKnown := map[string]bool{"BB5644": true, "BB56442": true, "BB56443": true}
	legacy, ok := parseImageAsset(entry, []string{"images", "BB56442+BB56443", "product_material", "BB56442-产品图.png"}, legacyKnown)
	if !ok || len(legacy.Codes) != 1 || legacy.Codes[0] != "BB56442" {
		t.Fatalf("legacy shared flat file matched a short mold number: %+v ok=%v", legacy, ok)
	}
}

func TestMoldNumberMatchingUsesNonOverlappingLongestNumbers(t *testing.T) {
	for _, test := range []struct {
		name string
		want string
	}{
		{"AB-CD-1.png", "AB-CD"}, {"AB-CD+AB前模.png", "AB,AB-CD"},
		{"AB-CD+AB-CD-1.png", "AB-CD"}, {"XAB-CD-1.png", ""},
	} {
		if got := strings.Join(moldNumbersInName(test.name, []string{"AB", "AB-CD"}), ","); got != test.want {
			t.Fatalf("%s: got %q, want %q", test.name, got, test.want)
		}
	}
}

func TestReadPackagePreservesHistoricalPlusMoldNumber(t *testing.T) {
	xlsx, err := spreadsheet.XLSXWriter{}.Write(t.Context(), spreadsheet.SpreadsheetDocument{
		SheetName: "模具", Columns: moldColumns,
		Rows: [][]string{{"", "A+B", "历史编号", "单模", "A1-1", "", "0", ""}, {"", "A", "型号A", "共模", "A1-1", "G", "0", ""}, {"", "B", "型号B", "共模", "A1-1", "G", "0", ""}}, TotalRows: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for name, content := range map[string][]byte{
		"molds.xlsx":                        xlsx,
		"images/A+B/product_material/a.png": tinyPNG,
		"drawings/A+B/a.dwg":                []byte("dwg"),
		"drawings/A+B/共用/shared.dwg":        []byte("shared"),
		"drawings/A+B/A/specific.dwg":       []byte("specific"),
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
	path := filepath.Join(t.TempDir(), "historical-plus.zip")
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := readTestPackage(t, path, int64(archive.Len()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Errors) != 0 || len(data.Images) != 1 || len(data.Drawings) != 3 || len(data.Images[0].Codes) != 1 || data.Images[0].Codes[0] != "A+B" {
		t.Fatalf("historical plus mold number was not read back: images=%+v drawings=%+v errors=%v", data.Images, data.Drawings, data.Errors)
	}
	for _, asset := range data.Drawings {
		want := map[string]string{"a.dwg": "A+B", "shared.dwg": "A,B", "specific.dwg": "A"}[asset.Name]
		if got := strings.Join(asset.Codes, ","); got != want {
			t.Fatalf("%s: got %s want %s", asset.Path, got, want)
		}
	}
}

func TestMoldArchiveGroupsAvoidAmbiguousPlusMember(t *testing.T) {
	groups, grouped, err := moldArchiveGroups([]model.Mold{
		{ID: 1, MoldNumber: "A+B", MoldType: model.MoldTypeCommon, CommonGroupNo: "G"},
		{ID: 2, MoldNumber: "C", MoldType: model.MoldTypeCommon, CommonGroupNo: "G"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 || len(grouped) != 0 {
		t.Fatalf("ambiguous plus member unexpectedly grouped: groups=%+v grouped=%v", groups, grouped)
	}
}

var tinyPNG = func() []byte {
	var output bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 32, G: 64, B: 96, A: 255})
	_ = png.Encode(&output, img)
	return output.Bytes()
}()

// Keep ZIP entries usable after parsing, as the production request owner does.
func readTestPackage(t *testing.T, path string, size int64, corrections map[string]ImportCorrection) (packageData, error) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		return packageData{}, err
	}
	t.Cleanup(func() { file.Close() })
	return (&Handler{}).readPackage(file, size, corrections)
}
