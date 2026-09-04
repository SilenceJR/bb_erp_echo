package model

import "time"

// ImageFile 是受保护的业务图片资产记录。
// StoragePath 仅供服务端访问文件系统，不向 API 响应暴露。
type ImageFile struct {
	BaseModel
	OwnerType    string `json:"owner_type" gorm:"size:40;not null;index:idx_image_owner"`
	OwnerID      uint   `json:"owner_id" gorm:"not null;index:idx_image_owner"`
	UploadedBy   uint   `json:"uploaded_by" gorm:"not null;index"`
	Category     string `json:"category,omitempty" gorm:"size:60;index"`
	SortOrder    int    `json:"sort_order" gorm:"not null;default:0;index"`
	OriginalName string `json:"original_name" gorm:"size:255;not null"`
	Size         int64  `json:"size" gorm:"not null"`
	MimeType     string `json:"mime_type" gorm:"size:80;not null"`
	Extension    string `json:"extension" gorm:"size:12;not null"`
	StoragePath  string `json:"-" gorm:"size:500;not null;uniqueIndex"`
	PreviewPath  string `json:"-" gorm:"size:500"`
	PreviewMime  string `json:"-" gorm:"size:80"`
	PreviewSize  int64  `json:"-"`
	ReplacesID   *uint  `json:"replaces_id,omitempty" gorm:"index"`
}

// FileCleanupTask 持久化数据库状态提交后未能立即清理的孤儿文件，供服务启动时重试。
type FileCleanupTask struct {
	ID          uint      `gorm:"primaryKey"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
	StoragePath string    `gorm:"size:500;not null;uniqueIndex"`
	Attempts    int       `gorm:"not null;default:0"`
	LastError   string    `gorm:"size:1000"`
}
