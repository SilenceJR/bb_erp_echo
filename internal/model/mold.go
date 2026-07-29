package model

// Mold 是模具管理模块骨架模型。
type Mold struct {
	BaseModel
	Name string `json:"name" gorm:"size:160;not null"`
	Code string `json:"code" gorm:"size:80;not null;uniqueIndex"`
}
