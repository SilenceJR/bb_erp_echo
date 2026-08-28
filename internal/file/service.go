package file

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bb_erp_echo/internal/model"
	"gorm.io/gorm"
)

const MaxImageSize int64 = 20 << 20

var ErrInvalidImage = errors.New("仅支持 JPEG、PNG、WebP、GIF 图片，且扩展名和 MIME 必须匹配")

// ValidationError 表示可直接返回 400 的上传输入错误。
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string   { return e.Message }
func validationError(message string) error { return &ValidationError{Message: message} }

type Service struct {
	UploadRoot string
	db         *gorm.DB
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
	header    *multipart.FileHeader
	extension string
	mimeType  string
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
		return nil, err
	}
	if err := s.db.Create(asset).Error; err != nil {
		_ = s.remove(asset.StoragePath)
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

	uploads := make([]imageUpload, len(headers))
	for i, header := range headers {
		upload, err := prepareImageUpload(header)
		if err != nil {
			return nil, err
		}
		uploads[i] = upload
	}

	assets := make([]*model.ImageFile, 0, len(uploads))
	paths := make([]string, 0, len(uploads))
	for _, upload := range uploads {
		asset, err := s.writeImage(upload, ownerType, ownerID, category, nil, uploadedBy)
		if err != nil {
			return nil, withFileCleanup(err, s, paths)
		}
		assets = append(assets, asset)
		paths = append(paths, asset.StoragePath)
	}

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, asset := range assets {
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

func prepareImageUpload(header *multipart.FileHeader) (imageUpload, error) {
	if header == nil || header.Size <= 0 {
		return imageUpload{}, validationError("文件不能为空")
	}
	if header.Size > MaxImageSize {
		return imageUpload{}, validationError(fmt.Sprintf("图片大小不能超过 %dMiB", MaxImageSize>>20))
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExtension(ext) {
		return imageUpload{}, validationError(ErrInvalidImage.Error())
	}
	src, err := header.Open()
	if err != nil {
		return imageUpload{}, fmt.Errorf("打开上传文件失败: %w", err)
	}
	defer src.Close()
	buf := make([]byte, 512)
	n, readErr := src.Read(buf)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return imageUpload{}, fmt.Errorf("读取上传文件失败: %w", readErr)
	}
	mimeType := http.DetectContentType(buf[:n])
	if !mimeMatchesExtension(mimeType, ext) {
		return imageUpload{}, validationError(ErrInvalidImage.Error())
	}
	return imageUpload{header: header, extension: ext, mimeType: mimeType}, nil
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
	written, copyErr := io.Copy(dst, io.LimitReader(src, MaxImageSize+1))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if copyErr != nil {
			return nil, fmt.Errorf("保存文件失败: %w", copyErr)
		}
		return nil, fmt.Errorf("关闭目标文件失败: %w", closeErr)
	}
	if written > MaxImageSize {
		_ = os.Remove(path)
		return nil, validationError(fmt.Sprintf("图片大小不能超过 %dMiB", MaxImageSize>>20))
	}
	return &model.ImageFile{OwnerType: ownerType, OwnerID: ownerID, UploadedBy: uploadedBy, Category: strings.TrimSpace(category), OriginalName: upload.header.Filename, Size: written, MimeType: upload.mimeType, Extension: upload.extension, StoragePath: filepath.ToSlash(filepath.Join(relativeDir, name)), ReplacesID: replacesID}, nil
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
	asset, err := s.SaveImage(header, old.OwnerType, old.OwnerID, category, &old.ID, uploadedBy)
	if err != nil {
		return nil, err
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&old).Error; err != nil {
			return err
		}
		if err := s.remove(old.StoragePath); err != nil {
			return err
		}
		// 旧文件删除发生在事务提交前；提交失败时存在极小的物理文件已删、记录仍可见窗口。
		return nil
	}); err != nil {
		_ = s.remove(asset.StoragePath)
		_ = s.db.Delete(asset).Error
		return nil, fmt.Errorf("替换图片记录失败: %w", err)
	}
	return asset, nil
}

func (s *Service) DeleteImage(asset *model.ImageFile) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(asset).Error; err != nil {
			return err
		}
		if err := s.remove(asset.StoragePath); err != nil {
			// 物理删除失败时回滚软删除，避免客户端看到“失败”而记录已不可见。
			return err
		}
		// 文件删除发生在事务提交前；若提交本身失败会留下极小的物理文件已删、记录仍可见窗口。
		return nil
	})
}
func (s *Service) Open(asset *model.ImageFile) (*os.File, error) {
	path, err := s.safePath(asset.StoragePath)
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
func allowedExtension(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	}
	return false
}
func mimeMatchesExtension(mimeType, ext string) bool {
	switch ext {
	case ".jpg", ".jpeg":
		return mimeType == "image/jpeg"
	case ".png":
		return mimeType == "image/png"
	case ".webp":
		return mimeType == "image/webp"
	case ".gif":
		return mimeType == "image/gif"
	}
	return false
}
