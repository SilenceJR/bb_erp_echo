package model

const (
	MoldTypeSingle = "single"
	MoldTypeCommon = "common"

	MoldLocationActive   = "active"
	MoldLocationDisabled = "disabled"
	// MoldLocationPallet 是不带货架编号的独立卡板位置。
	MoldLocationPallet = "卡板"
)

// Mold 是产品资料下的模具档案。
type Mold struct {
	BaseModel
	ProductID     uint         `json:"product_id" gorm:"not null;index"`
	Product       Product      `json:"-" gorm:"foreignKey:ProductID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	MoldType      string       `json:"mold_type" gorm:"size:20;not null;index"`
	CavityCount   string       `json:"cavity_count" gorm:"size:60;not null"`
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
