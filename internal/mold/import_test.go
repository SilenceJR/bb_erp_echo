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
	"golang.org/x/text/encoding/simplifiedchinese"
	"gorm.io/gorm"
)

func TestNaturalAssetSortAndCategoryInference(t *testing.T) {
	names := []string{"CYF-10.jpg", "CYF-2.jpg", "CYF-1.jpg"}
	sort.SliceStable(names, func(i, j int) bool { return naturalAssetLess(names[i], names[j]) })
	if got := names[0] + "," + names[1] + "," + names[2]; got != "CYF-1.jpg,CYF-2.jpg,CYF-10.jpg" {
		t.Fatalf("unexpected natural order: %s", got)
	}
	if got := inferCategory("CYF-1-未知.jpg"); got != "supplement" {
		t.Fatalf("non-numbered image should be mold image, got %q", got)
	}
	if got := inferCategory("CYF-1-前模局部.jpg"); got != "supplement" {
		t.Fatalf("unexpected supplement category: %q", got)
	}
	if got := inferCategory("CYF-1-关系图.jpg"); got != "supplement" {
		t.Fatalf("other images should be mold images, got %q", got)
	}
	if got := inferCategory("CYF-1.jpg"); got != "product_material" {
		t.Fatalf("numbered image should be product image, got %q", got)
	}
	if got := inferCategory("CYF产品刷墨图.jpg"); got != "product_material" {
		t.Fatalf("inked product image should be product image, got %q", got)
	}
}

func TestMoldImportAcceptsGalleryImageExtensions(t *testing.T) {
	for _, ext := range []string{".jpg", ".JPG", ".jfif", ".png", ".gif", ".webp", ".heic", ".HEIC", ".heif", ".avif", ".bmp", ".tif", ".tiff", ".svg"} {
		if !filemodule.AllowedImageExtension(ext) {
			t.Fatalf("expected mold import to accept %s", ext)
		}
	}
}

func TestLegacySingleMoldFlatImageUsesDirectoryOwnership(t *testing.T) {
	entry := &zip.File{Name: "关系图.png", UncompressedSize64: 1}
	asset, ok := parseImageAsset(entry, []string{"images", "A", "关系图.png"}, map[string]bool{"A": true})
	if !ok || strings.Join(asset.Codes, ",") != "A" || strings.Join(asset.AllowedCodes, ",") != "A" || asset.Category != "supplement" {
		t.Fatalf("legacy single-mold image ownership failed: ok=%v asset=%+v", ok, asset)
	}
}

func TestNormalizeMoldZipEntryNameAcceptsLegacyGBK(t *testing.T) {
	encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("001/BB3611/BB3611产品刷墨图.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	entry := &zip.File{FileHeader: zip.FileHeader{Name: string(encoded), NonUTF8: true}}
	if err := normalizeMoldZipEntryName(entry); err != nil {
		t.Fatal(err)
	}
	if entry.Name != "001/BB3611/BB3611产品刷墨图.jpg" || entry.NonUTF8 {
		t.Fatalf("legacy name was not decoded: name=%q non_utf8=%v", entry.Name, entry.NonUTF8)
	}
}

func TestNormalizeMoldZipEntryNamePreservesValidUTF8WithoutFlag(t *testing.T) {
	entry := &zip.File{FileHeader: zip.FileHeader{Name: "模具/前模局部图.jpg", NonUTF8: true}}
	if err := normalizeMoldZipEntryName(entry); err != nil {
		t.Fatal(err)
	}
	if entry.Name != "模具/前模局部图.jpg" || entry.NonUTF8 {
		t.Fatalf("valid UTF-8 name was changed: name=%q non_utf8=%v", entry.Name, entry.NonUTF8)
	}
}

func TestParseMoldRowsRestoresOmittedBlankSerialCell(t *testing.T) {
	raw := [][]string{
		{"序号", "模具编号", "模具型号", "模具类型", "模具位置", "共模组号", "图片总数", "备注"},
		{"BB3611", "010", "单模", "A1-1", "", "", ""},
		{"XR5129", "011", "共模", "A1-1", "001", "", ""},
	}
	rows, errs := parseMoldRows(raw)
	if len(errs) != 0 || len(rows) != 2 {
		t.Fatalf("rows=%+v errors=%+v", rows, errs)
	}
	if rows[0].MoldNumber != "BB3611" || rows[0].Model != "010" || rows[1].MoldNumber != "XR5129" || rows[1].CommonGroupNo != "001" {
		t.Fatalf("shifted workbook row was not restored: %+v", rows)
	}
}

func TestParseMoldRowsRejectsDuplicateModelsForFlatDirectoryMatching(t *testing.T) {
	raw := [][]string{
		{"序号", "模具编号", "模具型号", "模具类型", "模具位置", "共模组号", "图片总数", "备注"},
		{"", "010", "BB3611", "单模", "A1-1", "", "", ""},
		{"", "011", "BB3611", "单模", "A1-1", "", "", ""},
	}
	rows, errs := parseMoldRows(raw)
	if len(rows) != 1 || len(errs) != 1 || errs[0].Column != "模具型号" || !strings.Contains(errs[0].Reason, "无法唯一匹配") {
		t.Fatalf("rows=%+v errors=%+v", rows, errs)
	}
}

func TestReadPackageFlatRelationshipArchiveWithWrapper(t *testing.T) {
	xlsx, err := spreadsheet.XLSXWriter{}.Write(t.Context(), spreadsheet.SpreadsheetDocument{
		SheetName: "模具", Columns: moldColumns,
		Rows: [][]string{
			{"", "SINGLE", "单模产品", "单模", "A1-1", "", "0", ""},
			{"", "C1", "共模产品1", "共模", "A1-1", "GROUP-1", "0", ""},
			{"", "C2", "共模产品2", "共模", "A1-1", "GROUP-1", "0", ""},
		}, TotalRows: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	entries := map[string][]byte{
		"001/molds.xlsx":                     xlsx,
		"001/单模产品/单模产品-1.png":                tinyPNG,
		"001/单模产品/单模产品-2.png":                tinyPNG,
		"001/单模产品/单模产品产品刷墨图.png":             tinyPNG,
		"001/单模产品/关系图.png":                   tinyPNG,
		"001/共模产品1+共模产品2/共模产品1-1.png":        tinyPNG,
		"001/共模产品1+共模产品2/共模产品2前模局部图.png":     tinyPNG,
		"001/共模产品1+共模产品2/共模产品1+共模产品2后模图.png": tinyPNG,
		"001/共模产品1+共模产品2/无法识别成员.png":         tinyPNG,
		"001/共模产品1+共模产品2/共模产品1+共模产品2-结构.dwg": []byte("dwg"),
		"001/共模产品1+共模产品2/别名共模结构图.dwg":        []byte("shared dwg"),
		"001/单模产品/忽略.txt":                    []byte("unknown"),
		"001/共模产品1+共模产品2/desktop.ini":        []byte("system metadata"),
		"001/__MACOSX/._molds.xlsx":          []byte("system metadata"),
		"001/.DS_Store":                      []byte("system metadata"),
		"001/locations.json":                 []byte(`[{"code":"A1-1","status":"active"}]`),
	}
	for name, content := range entries {
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
	path := filepath.Join(t.TempDir(), "flat.zip")
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := readTestPackage(t, path, int64(archive.Len()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Errors) != 1 || !strings.Contains(data.Errors[0].Value, "忽略.txt") {
		t.Fatalf("unexpected flat archive errors: %+v", data.Errors)
	}
	if len(data.Rows) != 3 || len(data.Images) != 7 || len(data.Drawings) != 1 || len(data.Unresolved) != 2 {
		t.Fatalf("flat archive parse rows=%d images=%d drawings=%d unresolved=%d errors=%v", len(data.Rows), len(data.Images), len(data.Drawings), len(data.Unresolved), data.Errors)
	}
	if len(data.AssetMolds) != 3 || !data.AssetMolds["SINGLE"] || !data.AssetMolds["C1"] || !data.AssetMolds["C2"] {
		t.Fatalf("flat archive scopes=%v", data.AssetMolds)
	}
	for _, image := range data.Images {
		switch image.Name {
		case "单模产品-1.png", "单模产品-2.png", "单模产品产品刷墨图.png":
			if image.Category != "product_material" || len(image.Codes) != 1 || image.Codes[0] != "SINGLE" {
				t.Fatalf("unexpected product image: %+v", image)
			}
		case "关系图.png":
			if image.Category != "supplement" || len(image.Codes) != 1 || image.Codes[0] != "SINGLE" {
				t.Fatalf("unexpected mold image: %+v", image)
			}
		}
	}
	corrections := map[string]ImportCorrection{
		"共模产品1+共模产品2/无法识别成员.png":  {Codes: []string{"C1", "C2"}, Category: "模具图"},
		"共模产品1+共模产品2/别名共模结构图.dwg": {Codes: []string{"C1", "C2"}},
	}
	corrected, err := readTestPackage(t, path, int64(archive.Len()), corrections)
	if err != nil {
		t.Fatal(err)
	}
	if len(corrected.Errors) != 1 || len(corrected.Unresolved) != 0 || len(corrected.Images) != 8 || len(corrected.Drawings) != 2 {
		t.Fatalf("corrected flat archive images=%d drawings=%d unresolved=%d errors=%v", len(corrected.Images), len(corrected.Drawings), len(corrected.Unresolved), corrected.Errors)
	}
}

func TestFlatRelationshipMatchesModelsAndReturnsMoldNumbers(t *testing.T) {
	known := map[string]Input{
		"P-010": {MoldNumber: "BB3611", Model: "P-010", MoldType: model.MoldTypeSingle},
		"P-011": {MoldNumber: "XR5129", Model: "P-011", MoldType: model.MoldTypeCommon, CommonGroupNo: "G"},
		"P-012": {MoldNumber: "XR4245", Model: "P-012", MoldType: model.MoldTypeCommon, CommonGroupNo: "G"},
	}
	imageEntry := &zip.File{Name: "P-011+P-012后模图.PNG", UncompressedSize64: 1}
	asset, ok, ambiguous := parseFlatImageAsset(imageEntry, []string{"P-011+P-012", "P-011+P-012后模图.PNG"}, known)
	if !ok || ambiguous || asset.Category != "supplement" || strings.Join(asset.Codes, ",") != "XR4245,XR5129" || strings.Join(asset.AllowedCodes, ",") != "XR4245,XR5129" {
		t.Fatalf("model group matching did not return mold numbers: asset=%+v ok=%v ambiguous=%v", asset, ok, ambiguous)
	}
	asset, ok, ambiguous = parseFlatImageAsset(imageEntry, []string{"P-010", "P-010-1.PNG"}, known)
	if !ok || ambiguous || asset.Category != "product_material" || strings.Join(asset.Codes, ",") != "BB3611" || strings.Join(asset.AllowedCodes, ",") != "BB3611" {
		t.Fatalf("model single matching did not return mold number: asset=%+v ok=%v ambiguous=%v", asset, ok, ambiguous)
	}
	if _, ok, _ = parseFlatImageAsset(imageEntry, []string{"BB3611", "BB3611-1.PNG"}, known); ok {
		t.Fatal("flat relationship directory must not be resolved by mold number")
	}
	drawingEntry := &zip.File{Name: "P-011+P-012-结构.FDWG", UncompressedSize64: 1}
	drawing, ok, ambiguous := parseFlatDrawingAsset(drawingEntry, []string{"P-011+P-012", "P-011+P-012-结构.FDWG"}, known)
	if !ok || ambiguous || strings.Join(drawing.Codes, ",") != "XR4245,XR5129" || strings.Join(drawing.AllowedCodes, ",") != "XR4245,XR5129" {
		t.Fatalf("model drawing matching did not return mold numbers: asset=%+v ok=%v ambiguous=%v", drawing, ok, ambiguous)
	}
}

func TestFlatRelationshipArchiveRequiresCorrectionForAmbiguousPlusDirectory(t *testing.T) {
	xlsx, err := spreadsheet.XLSXWriter{}.Write(t.Context(), spreadsheet.SpreadsheetDocument{
		SheetName: "模具", Columns: moldColumns,
		Rows: [][]string{
			{"", "A+B", "共模 A+共模 B", "单模", "A1-1", "", "0", ""},
			{"", "A", "共模 A", "共模", "A1-1", "GROUP", "0", ""},
			{"", "B", "共模 B", "共模", "A1-1", "GROUP", "0", ""},
		}, TotalRows: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for name, content := range map[string][]byte{
		"molds.xlsx":              xlsx,
		"共模 A+共模 B/ambiguous.png": tinyPNG,
		"共模 A+共模 B/ambiguous.dwg": []byte("dwg"),
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
	path := filepath.Join(t.TempDir(), "ambiguous.zip")
	if err := os.WriteFile(path, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := readTestPackage(t, path, int64(archive.Len()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Errors) != 0 || len(data.Unresolved) != 2 {
		t.Fatalf("ambiguous folder must require correction: errors=%+v unresolved=%+v", data.Errors, data.Unresolved)
	}
	corrected, err := readTestPackage(t, path, int64(archive.Len()), map[string]ImportCorrection{
		"共模 A+共模 B/ambiguous.png": {Codes: []string{"A", "B"}, Category: "模具图"},
		"共模 A+共模 B/ambiguous.dwg": {Codes: []string{"A", "B"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(corrected.Errors) != 0 || len(corrected.Unresolved) != 0 || len(corrected.Images) != 1 || len(corrected.Drawings) != 1 {
		t.Fatalf("ambiguous correction failed: errors=%+v unresolved=%+v images=%+v drawings=%+v", corrected.Errors, corrected.Unresolved, corrected.Images, corrected.Drawings)
	}
	if corrected.AssetMolds["A+B"] || !corrected.AssetMolds["A"] || !corrected.AssetMolds["B"] {
		t.Fatalf("group interpretation must replace only group members: %v", corrected.AssetMolds)
	}
	single, err := readTestPackage(t, path, int64(archive.Len()), map[string]ImportCorrection{
		"共模 A+共模 B/ambiguous.png": {Codes: []string{"A+B"}, Category: "模具图"},
		"共模 A+共模 B/ambiguous.dwg": {Codes: []string{"A+B"}},
	})
	if err != nil || len(single.Errors) != 0 || !single.AssetMolds["A+B"] || single.AssetMolds["A"] || single.AssetMolds["B"] {
		t.Fatalf("single interpretation scope failed: data=%+v err=%v", single, err)
	}
	mixed, err := readTestPackage(t, path, int64(archive.Len()), map[string]ImportCorrection{
		"共模 A+共模 B/ambiguous.png": {Codes: []string{"A+B"}, Category: "模具图"},
		"共模 A+共模 B/ambiguous.dwg": {Codes: []string{"A", "B"}},
	})
	if err != nil || len(mixed.Errors) == 0 || !strings.Contains(mixed.Errors[0].Reason, "不能混合") {
		t.Fatalf("mixed interpretation must be rejected: data=%+v err=%v", mixed, err)
	}
	withinAssetMixed, err := readTestPackage(t, path, int64(archive.Len()), map[string]ImportCorrection{
		"共模 A+共模 B/ambiguous.png": {Codes: []string{"A+B", "A"}, Category: "模具图"},
		"共模 A+共模 B/ambiguous.dwg": {Codes: []string{"A+B"}},
	})
	if err != nil || len(withinAssetMixed.Errors) == 0 || !strings.Contains(withinAssetMixed.Errors[0].Reason, "人工修正无效") {
		t.Fatalf("one asset cannot mix single and group choices: data=%+v err=%v", withinAssetMixed, err)
	}
}

func TestAmbiguousEmptyDirectoryCannotSilentlyPreserveAssets(t *testing.T) {
	data := packageData{AssetMolds: map[string]bool{}}
	inputs := map[string]Input{
		"A+B": {MoldNumber: "A+B", MoldType: model.MoldTypeSingle},
		"A":   {MoldNumber: "A", MoldType: model.MoldTypeCommon, CommonGroupNo: "G"},
		"B":   {MoldNumber: "B", MoldType: model.MoldTypeCommon, CommonGroupNo: "G"},
	}
	resolveAmbiguousAssetScopes(&data, inputs, map[string]*zip.File{"A+B/": {}})
	if len(data.Errors) != 1 || !strings.Contains(data.Errors[0].Reason, "空资料目录") || len(data.AssetMolds) != 0 {
		t.Fatalf("ambiguous empty directory must be rejected: errors=%+v scopes=%v", data.Errors, data.AssetMolds)
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
		"molds.xlsx":     true,
		"locations.json": true,
		"示例产品/":          true,
		"示例共模 A+示例共模 B/": true,
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
	for _, name := range []string{"产品 A/"} {
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
	// Flat group exports use the exact group path.  A historical single mold
	// with the same plus-joined number is covered by the ambiguity test below.

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
	if !entries["型号 1+型号 2/型号 1+型号 2-1.png"] || !entries["型号 1+型号 2/型号 2-only.png"] || !entries["型号 1+型号 2/型号 1+型号 2-shared.dwg"] {
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
		if len(image.Codes) == 2 {
			sharedImage = image
		} else if len(image.Codes) == 1 {
			specificImage = image
		}
	}
	if len(sharedImage.Codes) != 2 || len(specificImage.Codes) != 1 || specificImage.Codes[0] != "BB56443" || len(data.Drawings[0].Codes) != 2 {
		t.Fatalf("shared assets were assigned incorrectly: images=%+v drawings=%+v", data.Images, data.Drawings)
	}
}

func TestReplaceMoldDataPreservesAssetsOnlyWhenDirectoryIsAbsent(t *testing.T) {
	db := openMoldTestDB(t)
	if err := SeedLocations(db); err != nil {
		t.Fatal(err)
	}
	var location model.MoldLocation
	if err := db.Where("code = ?", "A1-1").First(&location).Error; err != nil {
		t.Fatal(err)
	}
	keep := model.Mold{MoldNumber: "KEEP", Model: "old", MoldType: model.MoldTypeSingle, LocationID: location.ID}
	clear := model.Mold{MoldNumber: "CLEAR", Model: "old", MoldType: model.MoldTypeSingle, LocationID: location.ID}
	removed := model.Mold{MoldNumber: "REMOVED", Model: "old", MoldType: model.MoldTypeSingle, LocationID: location.ID}
	for _, item := range []*model.Mold{&keep, &clear, &removed} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []model.ImageFile{
		{OwnerType: "mold", OwnerID: keep.ID, Category: "supplement", OriginalName: "keep.png", StoragePath: "keep.png"},
		{OwnerType: "mold", OwnerID: clear.ID, Category: "supplement", OriginalName: "clear.png", StoragePath: "clear.png"},
		{OwnerType: "mold", OwnerID: removed.ID, Category: "supplement", OriginalName: "removed.png", StoragePath: "removed.png"},
	} {
		if err := db.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}
	data := packageData{
		Rows: []Input{
			{MoldNumber: "KEEP", Model: "new", MoldType: model.MoldTypeSingle, LocationCode: "A1-1"},
			{MoldNumber: "CLEAR", Model: "new", MoldType: model.MoldTypeSingle, LocationCode: "A1-1"},
		},
		Locations:  defaultMoldLocations(),
		AssetMolds: map[string]bool{"CLEAR": true},
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return replaceMoldData(tx, data, nil, 1) }); err != nil {
		t.Fatal(err)
	}
	var images []model.ImageFile
	if err := db.Where("owner_type = ?", "mold").Find(&images).Error; err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0].OriginalName != "keep.png" {
		t.Fatalf("preserved images=%+v", images)
	}
	var kept model.Mold
	if err := db.Where("mold_number = ?", "KEEP").First(&kept).Error; err != nil {
		t.Fatal(err)
	}
	if images[0].OwnerID != kept.ID || kept.Model != "new" {
		t.Fatalf("asset was not migrated to replacement mold: image=%+v mold=%+v", images[0], kept)
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

func TestMoldArchiveGroupsDoNotDropMembersWhenOuterCollidesWithSingleMold(t *testing.T) {
	molds := []model.Mold{
		{ID: 1, MoldNumber: "A", MoldType: model.MoldTypeCommon, CommonGroupNo: "G"},
		{ID: 2, MoldNumber: "B", MoldType: model.MoldTypeCommon, CommonGroupNo: "G"},
		{ID: 3, MoldNumber: "A+B", MoldType: model.MoldTypeSingle},
	}
	groups, grouped, err := moldArchiveGroups(molds)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 || len(grouped) != 0 {
		t.Fatalf("colliding group must fall back to per-mold folders: groups=%+v grouped=%v", groups, grouped)
	}
}

func TestProductInkImageDoesNotConsumeNumberedExportSequence(t *testing.T) {
	images := []model.ImageFile{
		{Category: "product_material", OriginalName: "M产品刷墨图.jpg"},
		{Category: "product_material", OriginalName: "first.jpg"},
	}
	sequence := 1
	names := make([]string, 0, len(images))
	for _, image := range images {
		names = append(names, archiveImageOutputName(image, "M", sequence, false))
		if image.Category == "product_material" && !isProductInkImage(image.OriginalName) {
			sequence++
		}
	}
	if got := strings.Join(names, ","); got != "M产品刷墨图.jpg,M-1.jpg" {
		t.Fatalf("unexpected export names: %s", got)
	}
}

func TestMoldImageExportNameCannotBecomeProductImage(t *testing.T) {
	for _, original := range []string{"尺寸-1.jpg", "尺寸.jpg", "误名产品刷墨图.jpg"} {
		name := archiveImageOutputName(model.ImageFile{Category: "supplement", OriginalName: original}, "M", 1, false)
		name = uniqueArchiveFileName(name, 12, "M", map[string]struct{}{filepath.ToSlash(filepath.Join("M", name)): {}})
		if got := inferCategory(name); got != "supplement" {
			t.Fatalf("mold image %q exported as %q and became %s", original, name, got)
		}
	}
}

func TestMemberSpecificArchiveNameDoesNotExpandRelationship(t *testing.T) {
	group := []model.Mold{{MoldNumber: "A"}, {MoldNumber: "B"}}
	for _, test := range []struct{ name, label string }{{"A+B前模图.jpg", "模具图"}, {"A+B结构.dwg", "图纸"}} {
		name := memberSpecificArchiveName("A", test.name, group, test.label)
		if got := strings.Join(moldNumbersInName(name, []string{"A", "B"}), ","); got != "A" {
			t.Fatalf("specific asset %q exported as %q with owners %s", test.name, name, got)
		}
	}
}

func TestMemberSpecificProductInkFallbackKeepsProductCategory(t *testing.T) {
	group := []model.Mold{{MoldNumber: "A"}, {MoldNumber: "B"}}
	name := memberSpecificArchiveName("A", "A+B产品刷墨图.jpg", group, "产品刷墨图")
	if got := strings.Join(moldNumbersInName(name, []string{"A", "B"}), ","); got != "A" || inferCategory(name) != "product_material" {
		t.Fatalf("product ink fallback lost owner or category: name=%q owners=%q category=%q", name, got, inferCategory(name))
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
