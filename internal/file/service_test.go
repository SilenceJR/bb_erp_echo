package file

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bb_erp_echo/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestImageLifecycleAndFormatValidation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ImageFile{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	service := NewService(root, db)
	if err := service.EnsureRoot(); err != nil {
		t.Fatal(err)
	}

	asset, err := service.SaveImage(uploadHeader(t, "one.png", []byte("\x89PNG\r\n\x1a\n")), OwnerProduct, 1, "main", nil, 1)
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

	replacement, err := service.ReplaceImage(asset.ID, uploadHeader(t, "two.gif", []byte("GIF89a")), "detail", 2)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if replacement.ReplacesID == nil || *replacement.ReplacesID != asset.ID {
		t.Fatalf("missing replaces_id: %+v", replacement)
	}
	inherited, err := service.ReplaceImage(replacement.ID, uploadHeader(t, "three.png", []byte("\x89PNG\r\n\x1a\n")), "", 3)
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

func TestDeleteRollsBackWhenPhysicalRemovalFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:delete_rollback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ImageFile{}); err != nil {
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
	if err := service.DeleteImage(asset); err == nil {
		t.Fatal("directory removal unexpectedly succeeded")
	}
	var visible model.ImageFile
	if err := db.First(&visible, asset.ID).Error; err != nil {
		t.Fatalf("soft delete was not rolled back: %v", err)
	}
}

func TestReplaceRollsBackWhenOldPhysicalRemovalFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:replace_rollback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ImageFile{}); err != nil {
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
	if _, err := service.ReplaceImage(old.ID, uploadHeader(t, "new.png", []byte("\x89PNG\r\n\x1a\n")), "", 9); err == nil {
		t.Fatal("replace unexpectedly succeeded")
	}
	var visible model.ImageFile
	if err := db.First(&visible, old.ID).Error; err != nil {
		t.Fatalf("old record was not restored: %v", err)
	}
	var active int64
	db.Model(&model.ImageFile{}).Count(&active)
	if active != 1 {
		t.Fatalf("active records = %d", active)
	}
}

func TestSaveImageRejectsDeclaredAndActualOversize(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:oversize?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ImageFile{}); err != nil {
		t.Fatal(err)
	}
	service := NewService(t.TempDir(), db)
	declared := &multipart.FileHeader{Filename: "large.png", Size: MaxImageSize + 1}
	if _, err := service.SaveImage(declared, OwnerProduct, 1, "", nil, 1); err == nil {
		t.Fatal("declared oversize accepted")
	}
	actualData := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, int(MaxImageSize+1-8))...)
	actual := uploadHeaderWithMax(t, "actual.png", actualData, int(MaxImageSize)+1<<20)
	actual.Size = 1
	if _, err := service.SaveImage(actual, OwnerProduct, 1, "", nil, 1); err == nil {
		t.Fatal("actual oversize accepted")
	} else {
		var validation *ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("actual oversize error = %v, want ValidationError", err)
		}
	}
	var files int
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
		t.Fatalf("oversize upload left %d files", files)
	}
}

func TestSaveImagesValidationFailureLeavesBatchEmpty(t *testing.T) {
	service, db := newBatchTestService(t)
	headers := []*multipart.FileHeader{
		uploadHeader(t, "valid.png", []byte("\x89PNG\r\n\x1a\n")),
		uploadHeader(t, "invalid.png", []byte("not an image")),
	}

	if _, err := service.SaveImages(headers, OwnerProduct, 1, "gallery", 1); err == nil {
		t.Fatal("invalid batch unexpectedly succeeded")
	}
	assertBatchEmpty(t, service, db)
}

func TestSaveImagesWriteFailureCleansEarlierFiles(t *testing.T) {
	service, db := newBatchTestService(t)
	actualData := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, int(MaxImageSize+1-8))...)
	oversized := uploadHeaderWithMax(t, "oversized.png", actualData, int(MaxImageSize)+1<<20)
	// 让声明大小通过首轮校验，实际大小在落盘阶段触发上限，从而覆盖批次中途写入失败。
	oversized.Size = 1
	headers := []*multipart.FileHeader{
		uploadHeader(t, "first.png", []byte("\x89PNG\r\n\x1a\n")),
		oversized,
	}

	if _, err := service.SaveImages(headers, OwnerProduct, 1, "gallery", 1); err == nil {
		t.Fatal("oversized batch unexpectedly succeeded")
	}
	assertBatchEmpty(t, service, db)
}

func TestSaveImagesDatabaseFailureRollsBackRecordsAndFiles(t *testing.T) {
	service, db := newBatchTestService(t)
	if err := db.Exec("CREATE UNIQUE INDEX idx_batch_image_original_name ON image_files(original_name)").Error; err != nil {
		t.Fatal(err)
	}
	headers := []*multipart.FileHeader{
		uploadHeader(t, "duplicate.png", []byte("\x89PNG\r\n\x1a\n")),
		uploadHeader(t, "duplicate.png", []byte("\x89PNG\r\n\x1a\n")),
	}

	if _, err := service.SaveImages(headers, OwnerProduct, 1, "gallery", 1); err == nil {
		t.Fatal("database failure batch unexpectedly succeeded")
	}
	assertBatchEmpty(t, service, db)
}

func newBatchTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ImageFile{}); err != nil {
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
