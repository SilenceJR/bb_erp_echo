package domain

import (
	"time"

	"gorm.io/gorm"
)

const (
	AccountTypePersonal           = "personal"
	AccountTypeDepartmentTerminal = "department_terminal"

	StatusActive   = "active"
	StatusDisabled = "disabled"

	UnknownPerson = "未知"
)

type Organization struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" gorm:"size:120;not null;uniqueIndex"`
	Code      string         `json:"code" gorm:"size:60;not null;uniqueIndex"`
	Status    string         `json:"status" gorm:"size:30;not null;default:active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

type Department struct {
	ID             uint           `json:"id" gorm:"primaryKey"`
	OrganizationID uint           `json:"organization_id" gorm:"not null;index"`
	Name           string         `json:"name" gorm:"size:120;not null"`
	Code           string         `json:"code" gorm:"size:60;not null;index"`
	Status         string         `json:"status" gorm:"size:30;not null;default:active"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

type Terminal struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	DepartmentID uint           `json:"department_id" gorm:"not null;index"`
	Code         string         `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Name         string         `json:"name" gorm:"size:120;not null"`
	Location     string         `json:"location" gorm:"size:255"`
	Status       string         `json:"status" gorm:"size:30;not null;default:active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

type User struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	Username       string     `json:"username" gorm:"size:80;not null;uniqueIndex"`
	AccountType    string     `json:"account_type" gorm:"size:40;not null;index"`
	Name           string     `json:"name" gorm:"size:120;not null"`
	OrganizationID uint       `json:"organization_id" gorm:"not null;index"`
	DepartmentID   *uint      `json:"department_id" gorm:"index"`
	TerminalID     *uint      `json:"terminal_id" gorm:"index"`
	Status         string     `json:"status" gorm:"size:30;not null;default:active"`
	PasswordHash   string     `json:"-" gorm:"size:255;not null"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Role struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:120;not null;uniqueIndex"`
	Code        string    `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Description string    `json:"description" gorm:"size:255"`
	System      bool      `json:"system" gorm:"not null;default:false"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Permission struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:120;not null"`
	Code        string    `json:"code" gorm:"size:120;not null;uniqueIndex"`
	Object      string    `json:"object" gorm:"size:120;not null;index"`
	Action      string    `json:"action" gorm:"size:60;not null"`
	Description string    `json:"description" gorm:"size:255"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserRole struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null;index:idx_user_role,unique"`
	RoleID    uint      `json:"role_id" gorm:"not null;index:idx_user_role,unique"`
	CreatedAt time.Time `json:"created_at"`
}

type RolePermission struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	RoleID       uint      `json:"role_id" gorm:"not null;index:idx_role_permission,unique"`
	PermissionID uint      `json:"permission_id" gorm:"not null;index:idx_role_permission,unique"`
	CreatedAt    time.Time `json:"created_at"`
}

type AuditLog struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	RequestID      string    `json:"request_id" gorm:"size:80;index"`
	ActorUserID    *uint     `json:"actor_user_id" gorm:"index"`
	ActorUsername  string    `json:"actor_username" gorm:"size:80;index"`
	AccountType    string    `json:"account_type" gorm:"size:40;index"`
	OrganizationID *uint     `json:"organization_id" gorm:"index"`
	DepartmentID   *uint     `json:"department_id" gorm:"index"`
	TerminalID     *uint     `json:"terminal_id" gorm:"index"`
	PersonName     string    `json:"person_name" gorm:"size:120"`
	Object         string    `json:"object" gorm:"size:120;index"`
	Action         string    `json:"action" gorm:"size:60;index"`
	Method         string    `json:"method" gorm:"size:20"`
	Path           string    `json:"path" gorm:"size:255"`
	Status         int       `json:"status"`
	RemoteIP       string    `json:"remote_ip" gorm:"size:80"`
	UserAgent      string    `json:"user_agent" gorm:"size:255"`
	Result         string    `json:"result" gorm:"size:40"`
	CreatedAt      time.Time `json:"created_at"`
}

type Customer struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:160;not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Contact struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	CustomerID *uint     `json:"customer_id" gorm:"index"`
	Name       string    `json:"name" gorm:"size:120;not null"`
	Phone      string    `json:"phone" gorm:"size:60"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Warehouse struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:120;not null"`
	Code      string    `json:"code" gorm:"size:60;not null;uniqueIndex"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type InventoryItem struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	WarehouseID uint      `json:"warehouse_id" gorm:"index"`
	SKU         string    `json:"sku" gorm:"size:80;index"`
	Quantity    float64   `json:"quantity"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Material struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:160;not null"`
	Code      string    `json:"code" gorm:"size:80;not null;uniqueIndex"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Product struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:160;not null"`
	Code      string    `json:"code" gorm:"size:80;not null;uniqueIndex"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Mold struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:160;not null"`
	Code      string    `json:"code" gorm:"size:80;not null;uniqueIndex"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type WorkOrder struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Code      string    `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Title     string    `json:"title" gorm:"size:160;not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DepartmentTask struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	WorkOrderID  uint      `json:"work_order_id" gorm:"index"`
	DepartmentID uint      `json:"department_id" gorm:"index"`
	Title        string    `json:"title" gorm:"size:160;not null"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func AllModels() []any {
	return []any{
		&Organization{},
		&Department{},
		&Terminal{},
		&User{},
		&Role{},
		&Permission{},
		&UserRole{},
		&RolePermission{},
		&AuditLog{},
		&Customer{},
		&Contact{},
		&Warehouse{},
		&InventoryItem{},
		&Material{},
		&Product{},
		&Mold{},
		&WorkOrder{},
		&DepartmentTask{},
	}
}
