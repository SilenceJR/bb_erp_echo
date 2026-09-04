package model

const (
	MoldTypeSingle = "single"
	MoldTypeCommon = "common"

	MoldLocationActive   = "active"
	MoldLocationDisabled = "disabled"
)

// Mold 是按产品型号维护的模具档案。
type Mold struct {
	BaseModel
	MoldNumber    string       `json:"mold_number" gorm:"size:120;not null;uniqueIndex"`
	Model         string       `json:"model" gorm:"size:160;not null"`
	MoldType      string       `json:"mold_type" gorm:"size:20;not null;index"`
	LocationID    uint         `json:"location_id" gorm:"not null;index"`
	Location      MoldLocation `json:"location,omitempty" gorm:"foreignKey:LocationID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	CommonGroupNo string       `json:"common_group_no,omitempty" gorm:"size:120;index"`
	Remark        string       `json:"remark,omitempty" gorm:"size:500"`
}

// MoldLocation 是可维护的固定模具位置字典。
type MoldLocation struct {
	BaseModel
	Code   string `json:"code" gorm:"size:40;not null;uniqueIndex"`
	Status string `json:"status" gorm:"size:20;not null;default:active;index"`
}

// MoldDrawing 是模具原始 DWG/FDWG 文件，不解析其内容。
type MoldDrawing struct {
	BaseModel
	MoldID       uint   `json:"mold_id" gorm:"not null;index"`
	UploadedBy   uint   `json:"uploaded_by" gorm:"not null;index"`
	OriginalName string `json:"original_name" gorm:"size:255;not null"`
	Size         int64  `json:"size" gorm:"not null"`
	MimeType     string `json:"mime_type" gorm:"size:80;not null"`
	Extension    string `json:"extension" gorm:"size:12;not null"`
	StoragePath  string `json:"-" gorm:"size:500;not null;uniqueIndex"`
}
