package model

import "time"

// Mold 是塑胶工厂模具台账。
//
// 常用字段覆盖模具编号、客户/产品关联、规格、穴数、当前位置、状态、
// 保养周期和最近维修保养时间，便于办公室、工模和生产部门共享同一份模具状态。
type Mold struct {
	BaseModel
	Code                 string          `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Name                 string          `json:"name" gorm:"size:160;not null"`
	CustomerID           *uint           `json:"customer_id" gorm:"index"`
	Customer             CustomerProfile `json:"-" gorm:"foreignKey:CustomerID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
	ProductID            *uint           `json:"product_id" gorm:"index"`
	CavityCount          int             `json:"cavity_count" gorm:"not null;default:1"`
	MoldMaterial         string          `json:"mold_material" gorm:"size:120"`
	Steel                string          `json:"steel" gorm:"size:120"`
	Size                 string          `json:"size" gorm:"size:120"`
	WeightGram           int64           `json:"weight_gram" gorm:"not null;default:0"`
	Manufacturer         string          `json:"manufacturer" gorm:"size:160"`
	Owner                string          `json:"owner" gorm:"size:120"`
	StorageLocation      string          `json:"storage_location" gorm:"size:160"`
	CurrentLocation      string          `json:"current_location" gorm:"size:160"`
	Status               string          `json:"status" gorm:"size:40;not null;default:in_stock;index"`
	MaintenanceCycleDays int             `json:"maintenance_cycle_days" gorm:"not null;default:0"`
	LastMaintenanceAt    *time.Time      `json:"last_maintenance_at"`
	NextMaintenanceAt    *time.Time      `json:"next_maintenance_at"`
	LastRepairAt         *time.Time      `json:"last_repair_at"`
	Remark               string          `json:"remark" gorm:"size:500"`
	Events               []MoldEvent     `json:"events,omitempty" gorm:"foreignKey:MoldID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

// MoldEvent 是模具借出、归还、维修和保养履历。
//
// 业务规则：模具状态变化必须写履历，后续可作为模具位置追踪和维修保养追溯依据。
type MoldEvent struct {
	BaseModel
	MoldID       uint       `json:"mold_id" gorm:"not null;index"`
	Type         string     `json:"type" gorm:"size:40;not null;index"`
	StatusBefore string     `json:"status_before" gorm:"size:40"`
	StatusAfter  string     `json:"status_after" gorm:"size:40"`
	Location     string     `json:"location" gorm:"size:160"`
	Counterparty string     `json:"counterparty" gorm:"size:160"`
	HandlerName  string     `json:"handler_name" gorm:"size:120"`
	Reason       string     `json:"reason" gorm:"size:255"`
	Description  string     `json:"description" gorm:"size:500"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}
