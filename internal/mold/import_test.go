package mold

import (
	"archive/zip"
	"bytes"
	"os"
	"sort"
	"testing"

	"bb_erp_echo/internal/spreadsheet"
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

func TestImageMimeMatchesExtension(t *testing.T) {
	if !imageMimeMatchesExtension("image/jpeg", ".jpg") || !imageMimeMatchesExtension("image/png", ".png") {
		t.Fatal("expected supported image MIME types to match")
	}
	if imageMimeMatchesExtension("image/png", ".jpg") || imageMimeMatchesExtension("application/pdf", ".png") {
		t.Fatal("unexpected image MIME match")
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

var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}
