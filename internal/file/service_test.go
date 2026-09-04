package file

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bb_erp_echo/internal/model"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestImageLifecycleAndFormatValidation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ImageFile{}, &model.FileCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	service := NewService(root, db)
	if err := service.EnsureRoot(); err != nil {
		t.Fatal(err)
	}

	asset, err := service.SaveImage(uploadHeader(t, "one.png", validPNGBytes()), OwnerProduct, 1, "main", nil, 1)
	if err != nil {
		t.Fatalf("valid upload: %v", err)
	}
	if asset.StoragePath == "" || asset.MimeType != "image/png" {
		t.Fatalf("unexpected asset: %+v", asset)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(asset.StoragePath))); err != nil {
		t.Fatalf("stored file: %v", err)
	}
	if _, err := service.SaveImage(uploadHeader(t, "fake.png", []byte("plain text")), OwnerProduct, 1, "", nil, 1); err == nil {
		t.Fatal("fake image accepted")
	}

	replacement, err := service.ReplaceImage(asset.ID, uploadHeader(t, "two.gif", validGIFBytes()), "detail", 2)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if replacement.ReplacesID == nil || *replacement.ReplacesID != asset.ID {
		t.Fatalf("missing replaces_id: %+v", replacement)
	}
	inherited, err := service.ReplaceImage(replacement.ID, uploadHeader(t, "three.png", validPNGBytes()), "", 3)
	if err != nil {
		t.Fatalf("replace inherited metadata: %v", err)
	}
	if inherited.Category != "detail" || inherited.UploadedBy != 3 {
		t.Fatalf("replacement metadata = %+v", inherited)
	}
	if err := service.DeleteImage(inherited); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var count int64
	db.Model(&model.ImageFile{}).Count(&count)
	if count != 0 {
		t.Fatalf("soft deleted rows visible: %d", count)
	}
}

func TestDeleteKeepsDatabaseStateWhenPhysicalCleanupFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:delete_rollback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ImageFile{}, &model.FileCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	service := NewService(root, db)
	path := filepath.Join(root, "product", "directory")
	if err := os.MkdirAll(filepath.Join(path, "child"), 0755); err != nil {
		t.Fatal(err)
	}
	asset := &model.ImageFile{OwnerType: OwnerProduct, OwnerID: 1, StoragePath: "product/directory", MimeType: "image/png", Size: 1}
	if err := db.Create(asset).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteImage(asset); err != nil {
		t.Fatalf("database delete should succeed despite orphan cleanup: %v", err)
	}
	var visible model.ImageFile
	if err := db.First(&visible, asset.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("deleted record should stay hidden, got: %v", err)
	}
	var pending model.FileCleanupTask
	if err := db.Where("storage_path = ?", "product/directory").First(&pending).Error; err != nil {
		t.Fatalf("cleanup failure was not persisted: %v", err)
	}
	if err := os.Remove(filepath.Join(path, "child")); err != nil {
		t.Fatal(err)
	}
	if err := RetryPendingCleanups(root, db); err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
	if err := db.First(&pending, pending.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("completed cleanup task still exists: %v", err)
	}
}

func TestReplaceKeepsNewRecordWhenOldPhysicalCleanupFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:replace_rollback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ImageFile{}, &model.FileCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	service := NewService(root, db)
	oldDir := filepath.Join(root, "product", "old-directory")
	if err := os.MkdirAll(filepath.Join(oldDir, "child"), 0755); err != nil {
		t.Fatal(err)
	}
	old := &model.ImageFile{OwnerType: OwnerProduct, OwnerID: 1, Category: "main", StoragePath: "product/old-directory", MimeType: "image/png", Size: 1}
	if err := db.Create(old).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReplaceImage(old.ID, uploadHeader(t, "new.png", validPNGBytes()), "", 9); err != nil {
		t.Fatalf("replacement should succeed despite orphan cleanup: %v", err)
	}
	var visible model.ImageFile
	if err := db.First(&visible, old.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("old record should stay hidden, got: %v", err)
	}
	var active int64
	db.Model(&model.ImageFile{}).Count(&active)
	if active != 1 {
		t.Fatalf("active records = %d", active)
	}
}

func TestSaveImageDoesNotTrustDeclaredSizeAsBusinessLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:oversize?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ImageFile{}, &model.FileCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	service := NewService(t.TempDir(), db)
	header := uploadHeader(t, "large.png", validPNGBytes())
	header.Size = 200 << 20
	if _, err := service.SaveImage(header, OwnerProduct, 1, "", nil, 1); err != nil {
		t.Fatalf("large declared size should not be rejected before decoding: %v", err)
	}
}

func TestSaveImagesRejectsBatchOverLimit(t *testing.T) {
	service, _ := newBatchTestService(t)
	headers := make([]*multipart.FileHeader, MaxImageBatch+1)
	for i := range headers {
		headers[i] = uploadHeader(t, fmt.Sprintf("image-%03d.png", i), validPNGBytes())
	}
	_, err := service.SaveImages(headers, OwnerProduct, 1, "gallery", 1)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || !strings.Contains(err.Error(), "一次最多上传 100 张") {
		t.Fatalf("error = %v, want explicit batch limit validation", err)
	}
}

func TestSaveImagesValidationFailureLeavesBatchEmpty(t *testing.T) {
	service, db := newBatchTestService(t)
	headers := []*multipart.FileHeader{
		uploadHeader(t, "valid.png", validPNGBytes()),
		uploadHeader(t, "invalid.png", []byte("not an image")),
	}

	if _, err := service.SaveImages(headers, OwnerProduct, 1, "gallery", 1); err == nil {
		t.Fatal("invalid batch unexpectedly succeeded")
	}
	assertBatchEmpty(t, service, db)
}

func TestSaveImageGeneratesStaticPreviewForExpandedFormats(t *testing.T) {
	service, _ := newBatchTestService(t)
	tests := []struct {
		name string
		data []byte
	}{
		{name: "photo.jpg", data: validJPEGBytes()},
		{name: "photo.jfif", data: validJPEGBytes()},
		{name: "picture.bmp", data: validBMPBytes()},
		{name: "scan.tiff", data: validTIFFBytes()},
		{name: "drawing.svg", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 10"><rect width="20" height="10" fill="#2463eb"/></svg>`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			asset, err := service.SaveImage(uploadHeader(t, test.name, test.data), OwnerProduct, 1, "gallery", nil, 1)
			if err != nil {
				t.Fatalf("save expanded image: %v", err)
			}
			if asset.PreviewPath == "" || asset.PreviewMime != "image/jpeg" || asset.PreviewSize <= 0 {
				t.Fatalf("preview metadata = %+v", asset)
			}
			if _, err := os.Stat(filepath.Join(service.UploadRoot, filepath.FromSlash(asset.PreviewPath))); err != nil {
				t.Fatalf("preview file: %v", err)
			}
		})
	}
}

func TestSaveImagesDatabaseFailureRollsBackRecordsAndFiles(t *testing.T) {
	service, db := newBatchTestService(t)
	if err := db.Exec("CREATE UNIQUE INDEX idx_batch_image_original_name ON image_files(original_name)").Error; err != nil {
		t.Fatal(err)
	}
	headers := []*multipart.FileHeader{
		uploadHeader(t, "duplicate.png", validPNGBytes()),
		uploadHeader(t, "duplicate.png", validPNGBytes()),
	}

	if _, err := service.SaveImages(headers, OwnerProduct, 1, "gallery", 1); err == nil {
		t.Fatal("database failure batch unexpectedly succeeded")
	}
	assertBatchEmpty(t, service, db)
}

func validPNGBytes() []byte {
	var body bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	_ = png.Encode(&body, img)
	return body.Bytes()
}

func validGIFBytes() []byte {
	var body bytes.Buffer
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White})
	_ = gif.Encode(&body, img, nil)
	return body.Bytes()
}

func validJPEGBytes() []byte {
	return encodeTestImage(func(w *bytes.Buffer, img image.Image) error {
		return jpeg.Encode(w, img, &jpeg.Options{Quality: 90})
	})
}

func validBMPBytes() []byte {
	return encodeTestImage(func(w *bytes.Buffer, img image.Image) error {
		return bmp.Encode(w, img)
	})
}

func validTIFFBytes() []byte {
	return encodeTestImage(func(w *bytes.Buffer, img image.Image) error {
		return tiff.Encode(w, img, nil)
	})
}

func encodeTestImage(encode func(*bytes.Buffer, image.Image) error) []byte {
	var body bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	_ = encode(&body, img)
	return body.Bytes()
}

func newBatchTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ImageFile{}, &model.FileCleanupTask{}); err != nil {
		t.Fatal(err)
	}
	return NewService(t.TempDir(), db), db
}

func assertBatchEmpty(t *testing.T, service *Service, db *gorm.DB) {
	t.Helper()
	var count int64
	if err := db.Model(&model.ImageFile{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("image records after failed batch = %d", count)
	}
	files := 0
	if err := filepath.WalkDir(service.UploadRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if files != 0 {
		t.Fatalf("physical files after failed batch = %d", files)
	}
}

func uploadHeader(t *testing.T, name string, data []byte) *multipart.FileHeader {
	return uploadHeaderWithMax(t, name, data, 1<<20)
}

func uploadHeaderWithMax(t *testing.T, name string, data []byte, maxMemory int) *multipart.FileHeader {
	t.Helper()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(int64(maxMemory)); err != nil {
		t.Fatal(err)
	}
	header, err := req.MultipartForm.File["file"][0], error(nil)
	if header == nil || err != nil {
		t.Fatal("missing multipart header")
	}
	return header
}
