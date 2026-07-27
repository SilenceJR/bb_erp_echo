package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	// AccountTypePersonal 表示个人账号，审计日志会记录具体责任人。
	AccountTypePersonal = "personal"
	// AccountTypeDepartmentTerminal 表示部门公共终端账号，审计日志只记录部门和终端。
	AccountTypeDepartmentTerminal = "department_terminal"

	// StatusActive 表示记录启用。
	StatusActive = "active"
	// StatusDisabled 表示记录停用。
	StatusDisabled = "disabled"

	// UnknownPerson 用于部门终端账号审计，表示无法确认具体操作人员。
	UnknownPerson = "未知"
)

// Organization 是 ERP 的组织边界，当前用于公司级数据隔离。
type Organization struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" gorm:"size:120;not null;uniqueIndex"`
	Code      string         `json:"code" gorm:"size:60;not null;uniqueIndex"`
	Status    string         `json:"status" gorm:"size:30;not null;default:active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// Department 是组织下的部门，用于用户归属、终端归属和数据权限判断。
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

// Terminal 是车间、仓库等公共电脑或设备终端。
//
// 部门终端账号必须绑定 Terminal，审计日志才能落到具体终端。
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

// Role 是 RBAC 角色，Casbin 会根据用户角色生成分组策略。
type Role struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:120;not null;uniqueIndex"`
	Code        string    `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Description string    `json:"description" gorm:"size:255"`
	System      bool      `json:"system" gorm:"not null;default:false"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Permission 是后端接口权限定义。
//
// 参数说明：
// - Code：业务稳定权限码，便于前端和后台展示。
// - Object：Casbin object，当前使用 API 路径。
// - Action：Casbin action，当前使用 read/write。
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

// UserRole 是用户与角色的多对多关系。
type UserRole struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"user_id" gorm:"not null;index:idx_user_role,unique"`
	RoleID    uint      `json:"role_id" gorm:"not null;index:idx_user_role,unique"`
	CreatedAt time.Time `json:"created_at"`
}

// RolePermission 是角色与权限的多对多关系。
type RolePermission struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	RoleID       uint      `json:"role_id" gorm:"not null;index:idx_role_permission,unique"`
	PermissionID uint      `json:"permission_id" gorm:"not null;index:idx_role_permission,unique"`
	CreatedAt    time.Time `json:"created_at"`
}

// AuditLog 是操作审计记录。
//
// 个人账号会记录 PersonName 为具体姓名；部门终端账号会记录部门和终端，
// PersonName 固定为 UnknownPerson，避免虚构具体操作人。
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

// Customer 是客户模块骨架模型，后续承载客户档案。
type Customer struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:160;not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Contact 是联系人模块骨架模型，后续可关联客户。
type Contact struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	CustomerID *uint     `json:"customer_id" gorm:"index"`
	Name       string    `json:"name" gorm:"size:120;not null"`
	Phone      string    `json:"phone" gorm:"size:60"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Warehouse 是仓库模块骨架模型。
type Warehouse struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:120;not null"`
	Code      string    `json:"code" gorm:"size:60;not null;uniqueIndex"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// InventoryItem 是库存模块骨架模型，用于表达仓库内 SKU 数量。
type InventoryItem struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	WarehouseID uint      `json:"warehouse_id" gorm:"index"`
	SKU         string    `json:"sku" gorm:"size:80;index"`
	Quantity    float64   `json:"quantity"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Material 是物料模块骨架模型。
type Material struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:160;not null"`
	Code      string    `json:"code" gorm:"size:80;not null;uniqueIndex"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Product 是产品模块骨架模型。
type Product struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:160;not null"`
	Code      string    `json:"code" gorm:"size:80;not null;uniqueIndex"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Mold 是模具管理模块骨架模型。
type Mold struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:160;not null"`
	Code      string    `json:"code" gorm:"size:80;not null;uniqueIndex"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WorkOrder 是任务单模块骨架模型。
type WorkOrder struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Code      string    `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Title     string    `json:"title" gorm:"size:160;not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DepartmentTask 是部门子任务骨架模型，用于把任务单拆到具体部门执行。
type DepartmentTask struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	WorkOrderID  uint      `json:"work_order_id" gorm:"index"`
	DepartmentID uint      `json:"department_id" gorm:"index"`
	Title        string    `json:"title" gorm:"size:160;not null"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AllModels 返回需要由 GORM AutoMigrate 管理的全部模型。
//
// 参数说明：无。
// 返回说明：返回模型指针切片，调用方直接传给 db.AutoMigrate。
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
