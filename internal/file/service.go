package file

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bb_erp_echo/internal/model"
	"gorm.io/gorm"
)

var ErrInvalidImage = errors.New("仅支持 JPG、JFIF、PNG、GIF、WebP、HEIC、HEIF、AVIF、BMP、TIFF、SVG 静态图片，且扩展名必须与文件内容匹配")

const (
	MaxImageBatch        = 100
	maxBatchPreviewBytes = 256 << 20
)

// ValidationError 表示可直接返回 400 的上传输入错误。
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string   { return e.Message }
func validationError(message string) error { return &ValidationError{Message: message} }

type Service struct {
	UploadRoot string
	db         *gorm.DB
	sortMu     sync.Mutex
}

func NewService(uploadRoot string, db *gorm.DB) *Service {
	return &Service{UploadRoot: uploadRoot, db: db}
}
func (s *Service) EnsureRoot() error {
	if s.UploadRoot == "" {
		return errors.New("上传根目录不能为空")
	}
	return os.MkdirAll(s.UploadRoot, 0755)
}

type imageUpload struct {
	header      *multipart.FileHeader
	extension   string
	mimeType    string
	preview     []byte
	previewMime string
}

// SaveImage 先写入随机文件名，再创建数据库记录；记录失败时清理新文件。
func (s *Service) SaveImage(header *multipart.FileHeader, ownerType string, ownerID uint, category string, replacesID *uint, uploadedBy uint) (*model.ImageFile, error) {
	if !validOwnerType(ownerType) {
		return nil, validationError(ownerError(ownerType).Error())
	}
	if ownerID == 0 {
		return nil, validationError("owner_id 无效")
	}
	upload, err := prepareImageUpload(header)
	if err != nil {
		return nil, err
	}
	asset, err := s.writeImage(upload, ownerType, ownerID, category, replacesID, uploadedBy)
	if err != nil {
		return nil, fmt.Errorf("%s：%w", upload.header.Filename, err)
	}
	s.sortMu.Lock()
	defer s.sortMu.Unlock()
	asset.SortOrder = s.nextSortOrder(ownerType, ownerID, category)
	if err := s.db.Create(asset).Error; err != nil {
		_ = s.remove(asset.StoragePath)
		if asset.PreviewPath != "" {
			_ = s.remove(asset.PreviewPath)
		}
		return nil, fmt.Errorf("保存图片记录失败: %w", err)
	}
	return asset, nil
}

// SaveImages 批量保存图片。所有图片文件写入成功后才创建数据库记录，
// 数据库事务失败时回滚全部记录并清理本批次已写入的物理文件。
func (s *Service) SaveImages(headers []*multipart.FileHeader, ownerType string, ownerID uint, category string, uploadedBy uint) ([]*model.ImageFile, error) {
	if !validOwnerType(ownerType) {
		return nil, validationError(ownerError(ownerType).Error())
	}
	if ownerID == 0 {
		return nil, validationError("owner_id 无效")
	}
	if len(headers) == 0 {
		return nil, validationError("文件不能为空")
	}
	if len(headers) > MaxImageBatch {
		return nil, validationError(fmt.Sprintf("本次选择了 %d 张图片，一次最多上传 %d 张，请分批上传", len(headers), MaxImageBatch))
	}

	uploads := make([]imageUpload, len(headers))
	var previewBytes int64
	for i, header := range headers {
		upload, err := prepareImageUpload(header)
		if err != nil {
			return nil, err
		}
		previewBytes += int64(len(upload.preview))
		if previewBytes > maxBatchPreviewBytes {
			return nil, validationError("本批图片生成的静态预览总量超过 256 MiB，请减少图片数量后分批上传")
		}
		uploads[i] = upload
	}

	assets := make([]*model.ImageFile, 0, len(uploads))
	paths := make([]string, 0, len(uploads)*2)
	for index, upload := range uploads {
		asset, err := s.writeImage(upload, ownerType, ownerID, category, nil, uploadedBy)
		if err != nil {
			return nil, withFileCleanup(fmt.Errorf("%s：%w", upload.header.Filename, err), s, paths)
		}
		asset.SortOrder = index
		assets = append(assets, asset)
		paths = append(paths, asset.StoragePath)
		if asset.PreviewPath != "" {
			paths = append(paths, asset.PreviewPath)
		}
	}

	s.sortMu.Lock()
	defer s.sortMu.Unlock()
	startOrder := s.nextSortOrder(ownerType, ownerID, category)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for index, asset := range assets {
			asset.SortOrder = startOrder + index
			if err := tx.Create(asset).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, withFileCleanup(fmt.Errorf("批量保存图片记录失败: %w", err), s, paths)
	}
	return assets, nil
}

func (s *Service) nextSortOrder(ownerType string, ownerID uint, category string) int {
	var max int
	s.db.Model(&model.ImageFile{}).Where("owner_type = ? AND owner_id = ? AND category = ?", ownerType, ownerID, category).Select("COALESCE(MAX(sort_order), -1)").Scan(&max)
	return max + 1
}

func prepareImageUpload(header *multipart.FileHeader) (imageUpload, error) {
	if header == nil || header.Size <= 0 {
		return imageUpload{}, validationError("文件不能为空")
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !AllowedImageExtension(ext) {
		return imageUpload{}, validationError(fmt.Sprintf("%s：%s", header.Filename, ErrInvalidImage.Error()))
	}
	preview, previewMime, err := makeStaticPreview(header, ext)
	if err != nil {
		return imageUpload{}, fmt.Errorf("%s：%w", header.Filename, err)
	}
	return imageUpload{header: header, extension: ext, mimeType: ImageMIMEForExtension(ext), preview: preview, previewMime: previewMime}, nil
}

func (s *Service) writeImage(upload imageUpload, ownerType string, ownerID uint, category string, replacesID *uint, uploadedBy uint) (*model.ImageFile, error) {
	src, err := upload.header.Open()
	if err != nil {
		return nil, fmt.Errorf("打开上传文件失败: %w", err)
	}
	defer src.Close()

	now := time.Now()
	relativeDir := filepath.Join(ownerType, now.Format("2006"), now.Format("01"))
	dir := filepath.Join(s.UploadRoot, relativeDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建上传目录失败: %w", err)
	}
	name, err := randomName(upload.extension)
	if err != nil {
		return nil, fmt.Errorf("生成文件名失败: %w", err)
	}
	path := filepath.Join(dir, name)
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, fmt.Errorf("创建目标文件失败: %w", err)
	}
	written, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if copyErr != nil {
			return nil, fmt.Errorf("保存文件失败: %w", copyErr)
		}
		return nil, fmt.Errorf("关闭目标文件失败: %w", closeErr)
	}
	previewName, err := randomName(".jpg")
	if err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("生成预览文件名失败: %w", err)
	}
	previewPath := filepath.Join(dir, "preview-"+previewName)
	previewFile, err := os.OpenFile(previewPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("创建静态预览失败: %w", err)
	}
	_, writeErr := previewFile.Write(upload.preview)
	closeErr = previewFile.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		_ = os.Remove(previewPath)
		if writeErr != nil {
			return nil, fmt.Errorf("保存静态预览失败: %w", writeErr)
		}
		return nil, fmt.Errorf("关闭静态预览失败: %w", closeErr)
	}
	return &model.ImageFile{OwnerType: ownerType, OwnerID: ownerID, UploadedBy: uploadedBy, Category: strings.TrimSpace(category), OriginalName: upload.header.Filename, Size: written, MimeType: upload.mimeType, Extension: upload.extension, StoragePath: filepath.ToSlash(filepath.Join(relativeDir, name)), PreviewPath: filepath.ToSlash(filepath.Join(relativeDir, "preview-"+previewName)), PreviewMime: upload.previewMime, PreviewSize: int64(len(upload.preview)), ReplacesID: replacesID}, nil
}

func withFileCleanup(err error, service *Service, paths []string) error {
	if cleanupErr := service.cleanupFiles(paths); cleanupErr != nil {
		return errors.Join(err, fmt.Errorf("清理批量图片文件失败: %w", cleanupErr))
	}
	return err
}

func (s *Service) cleanupFiles(paths []string) error {
	var cleanupErr error
	for _, path := range paths {
		if err := s.remove(path); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("%s: %w", path, err))
		}
	}
	return cleanupErr
}

func (s *Service) ReplaceImage(id uint, header *multipart.FileHeader, category string, uploadedBy uint) (*model.ImageFile, error) {
	var old model.ImageFile
	if err := s.db.First(&old, id).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(category) == "" {
		category = old.Category
	}
	upload, err := prepareImageUpload(header)
	if err != nil {
		return nil, err
	}
	asset, err := s.writeImage(upload, old.OwnerType, old.OwnerID, category, &old.ID, uploadedBy)
	if err != nil {
		return nil, fmt.Errorf("%s：%w", upload.header.Filename, err)
	}
	asset.SortOrder = old.SortOrder
	cleanupPaths := []string{old.StoragePath}
	if old.PreviewPath != "" {
		cleanupPaths = append(cleanupPaths, old.PreviewPath)
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		deleted := tx.Where("id = ? AND deleted_at IS NULL", old.ID).Delete(&model.ImageFile{})
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected != 1 {
			return validationError("原图片已被其他操作替换或删除，请刷新后重试")
		}
		if err := tx.Create(asset).Error; err != nil {
			return err
		}
		return QueueCleanupTasks(tx, cleanupPaths)
	}); err != nil {
		_ = s.remove(asset.StoragePath)
		if asset.PreviewPath != "" {
			_ = s.remove(asset.PreviewPath)
		}
		return nil, fmt.Errorf("替换图片记录失败: %w", err)
	}
	// 数据库状态提交后再清理旧文件；清理失败只留下可重试的孤儿文件，
	// 不会让仍可见的记录指向已删除文件。
	CleanupStoredPaths(s.UploadRoot, s.db, cleanupPaths)
	return asset, nil
}

func (s *Service) DeleteImage(asset *model.ImageFile) error {
	cleanupPaths := []string{asset.StoragePath}
	if asset.PreviewPath != "" {
		cleanupPaths = append(cleanupPaths, asset.PreviewPath)
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(asset).Error; err != nil {
			return err
		}
		return QueueCleanupTasks(tx, cleanupPaths)
	}); err != nil {
		return err
	}
	CleanupStoredPaths(s.UploadRoot, s.db, cleanupPaths)
	return nil
}
func (s *Service) Open(asset *model.ImageFile) (*os.File, error) {
	path, err := s.safePath(asset.StoragePath)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}
func (s *Service) OpenPreview(asset *model.ImageFile) (*os.File, error) {
	if asset.PreviewPath == "" {
		return nil, os.ErrNotExist
	}
	path, err := s.safePath(asset.PreviewPath)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}
func (s *Service) safePath(relative string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("非法文件路径")
	}
	owner := filepath.VolumeName(clean)
	parts := strings.Split(filepath.ToSlash(clean), "/")
	if owner != "" || len(parts) < 2 || !validOwnerType(parts[0]) {
		return "", errors.New("非法文件路径")
	}
	return filepath.Join(s.UploadRoot, clean), nil
}
func (s *Service) remove(relative string) error {
	path, err := s.safePath(relative)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func randomName(ext string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b) + ext, nil
}
func AllowedImageExtension(ext string) bool {
	ext = strings.ToLower(ext)
	switch ext {
	case ".jpg", ".jpeg", ".jfif", ".png", ".webp", ".gif", ".heic", ".heif", ".avif", ".bmp", ".tif", ".tiff", ".svg":
		return true
	}
	return false
}
func ImageMIMEForExtension(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".jpg", ".jpeg", ".jfif":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	case ".avif":
		return "image/avif"
	case ".bmp":
		return "image/bmp"
	case ".tif", ".tiff":
		return "image/tiff"
	case ".svg":
		return "image/svg+xml"
	}
	return "application/octet-stream"
}
