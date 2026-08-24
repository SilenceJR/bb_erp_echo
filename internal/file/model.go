package file

import (
	"time"

	"bb_erp_echo/internal/model"
)

// ImageResponse 是图片 API 的裸 JSON 响应，不包含物理存储路径。
type ImageResponse struct {
	ID           uint      `json:"id"`
	OwnerType    string    `json:"owner_type"`
	OwnerID      uint      `json:"owner_id"`
	UploadedBy   uint      `json:"uploaded_by"`
	Category     string    `json:"category,omitempty"`
	OriginalName string    `json:"original_name"`
	Size         int64     `json:"size"`
	MimeType     string    `json:"mime_type"`
	Extension    string    `json:"extension"`
	ReplacesID   *uint     `json:"replaces_id,omitempty"`
	ContentURL   string    `json:"content_url"`
	CreatedAt    time.Time `json:"created_at"`
}

func toResponse(asset *model.ImageFile) ImageResponse {
	return ImageResponse{ID: asset.ID, OwnerType: asset.OwnerType, OwnerID: asset.OwnerID, UploadedBy: asset.UploadedBy, Category: asset.Category, OriginalName: asset.OriginalName, Size: asset.Size, MimeType: asset.MimeType, Extension: asset.Extension, ReplacesID: asset.ReplacesID, ContentURL: "/api/v1/files/" + idString(asset.ID) + "/content", CreatedAt: asset.CreatedAt}
}
