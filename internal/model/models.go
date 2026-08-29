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

type BaseModel struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// Organization 是 ERP 的组织边界，当前用于公司级数据隔离。
type Organization struct {
	BaseModel
	Name   string `json:"name" gorm:"size:120;not null;uniqueIndex"`
	Code   string `json:"code" gorm:"size:60;not null;uniqueIndex"`
	Status string `json:"status" gorm:"size:30;not null;default:active"`
}

// Department 是组织下的部门，用于用户归属、终端归属和数据权限判断。
type Department struct {
	BaseModel
	OrganizationID uint   `json:"organization_id" gorm:"not null;default:1;index"`
	Name           string `json:"name" gorm:"size:120;not null"`
	Code           string `json:"code" gorm:"size:60;not null;index"`
	Status         string `json:"status" gorm:"size:30;not null;default:active"`
}

// Terminal 是车间、仓库等公共电脑或设备终端。
//
// 部门终端账号必须绑定 Terminal，审计日志才能落到具体终端。
type Terminal struct {
	BaseModel
	DepartmentID uint   `json:"department_id" gorm:"not null;index"`
	Code         string `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Name         string `json:"name" gorm:"size:120;not null"`
	Location     string `json:"location" gorm:"size:255"`
	Status       string `json:"status" gorm:"size:30;not null;default:active"`
}

// Role 是 RBAC 角色，Casbin 会根据用户角色生成分组策略。
type Role struct {
	BaseModel
	Name        string `json:"name" gorm:"size:120;not null;uniqueIndex"`
	Code        string `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Description string `json:"description" gorm:"size:255"`
	System      bool   `json:"system" gorm:"not null;default:false"`
}

// Permission 是后端接口权限定义。
//
// 参数说明：
// - Code：业务稳定权限码，便于前端和后台展示。
// - Object：Casbin object，当前使用 API 路径。
// - Action：Casbin action，当前使用 read/write。
type Permission struct {
	BaseModel
	Name        string `json:"name" gorm:"size:120;not null"`
	Code        string `json:"code" gorm:"size:120;not null;uniqueIndex"`
	Object      string `json:"object" gorm:"size:120;not null;index"`
	Action      string `json:"action" gorm:"size:60;not null"`
	Description string `json:"description" gorm:"size:255"`
}

// UserRole 是用户与角色的多对多关系。
type UserRole struct {
	BaseModel
	UserID uint `json:"user_id" gorm:"not null;index:idx_user_role,unique"`
	RoleID uint `json:"role_id" gorm:"not null;index:idx_user_role,unique"`
}

// RolePermission 是角色与权限的多对多关系。
type RolePermission struct {
	BaseModel
	RoleID       uint `json:"role_id" gorm:"not null;index:idx_role_permission,unique"`
	PermissionID uint `json:"permission_id" gorm:"not null;index:idx_role_permission,unique"`
}

// AuditLog 是操作审计记录。
//
// 个人账号会记录 PersonName 为具体姓名；部门终端账号会记录部门和终端，
// PersonName 固定为 UnknownPerson，避免虚构具体操作人。
type AuditLog struct {
	BaseModel
	RequestID      string `json:"request_id" gorm:"size:80;index"`
	ActorUserID    *uint  `json:"actor_user_id" gorm:"index"`
	ActorUsername  string `json:"actor_username" gorm:"size:80;index"`
	AccountType    string `json:"account_type" gorm:"size:40;index"`
	OrganizationID *uint  `json:"organization_id" gorm:"index"`
	DepartmentID   *uint  `json:"department_id" gorm:"index"`
	TerminalID     *uint  `json:"terminal_id" gorm:"index"`
	PersonName     string `json:"person_name" gorm:"size:120"`
	Object         string `json:"object" gorm:"size:120;index"`
	Action         string `json:"action" gorm:"size:60;index"`
	Method         string `json:"method" gorm:"size:20"`
	Path           string `json:"path" gorm:"size:255"`
	Status         int    `json:"status"`
	RemoteIP       string `json:"remote_ip" gorm:"size:80"`
	UserAgent      string `json:"user_agent" gorm:"size:255"`
	Result         string `json:"result" gorm:"size:40"`
}

// Customer 是客户模块骨架模型，后续承载客户档案。
type Customer struct {
	BaseModel
	Name     string    `json:"name" gorm:"size:160;not null"`
	Code     string    `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Phone    string    `json:"phone" gorm:"size:60;"`
	Contacts []Contact `json:"contacts" gorm:"foreignKey:CustomerID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Address  string    `json:"address" gorm:"size:255;"`
}

// Supplier 是采购入库使用的供应商档案。
type Supplier struct {
	BaseModel
	Name    string `json:"name" gorm:"size:160;not null"`
	Code    string `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Contact string `json:"contact" gorm:"size:120"`
	Phone   string `json:"phone" gorm:"size:60"`
	Address string `json:"address" gorm:"size:255"`
	Status  string `json:"status" gorm:"size:30;not null;default:active"`
}

// Contact 是联系人模块骨架模型，后续可关联客户。
type Contact struct {
	BaseModel
	CustomerID uint           `json:"customer_id" gorm:"not null;index"`
	Name       string         `json:"name" gorm:"size:120;not null"`
	Phones     []ContactPhone `json:"phones" gorm:"foreignKey:ContactID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

// ContactPhone 是联系人电话明细表。
//
// 业务说明：
// 一个联系人可能有手机、座机、微信同号等多个联系方式。
// 电话拆成明细表后，后续可以单独维护主号码、号码类型和唯一性规则。
type ContactPhone struct {
	BaseModel
	ContactID uint   `json:"contact_id" gorm:"not null;index"`
	Phone     string `json:"phone" gorm:"size:60;not null"`
	Label     string `json:"label" gorm:"size:40"`
	Primary   bool   `json:"primary" gorm:"not null;default:false"`
}

// Warehouse 是仓库模块骨架模型。
type Warehouse struct {
	BaseModel
	Name   string `json:"name" gorm:"size:120;not null"`
	Code   string `json:"code" gorm:"size:60;not null;uniqueIndex"`
	Status string `json:"status" gorm:"size:30;not null;default:active"`
}

// Location 是仓库内的库位。
type Location struct {
	BaseModel
	WarehouseID uint   `json:"warehouse_id" gorm:"not null;index"`
	Code        string `json:"code" gorm:"size:80;not null;index:idx_location_code,unique"`
	Name        string `json:"name" gorm:"size:120;not null"`
	Status      string `json:"status" gorm:"size:30;not null;default:active"`
}

// InventoryBalance 是仓库库位下某个物料或产品的当前结存。
//
// 数量使用 4 位定点整数，金额使用分，避免浮点误差影响库存和成本。
type InventoryBalance struct {
	BaseModel
	WarehouseID uint   `json:"warehouse_id" gorm:"not null;index:idx_inventory_balance,unique"`
	LocationID  *uint  `json:"location_id" gorm:"index:idx_inventory_balance,unique"`
	ItemType    string `json:"item_type" gorm:"size:30;not null;index:idx_inventory_balance,unique"`
	ItemID      uint   `json:"item_id" gorm:"not null;index:idx_inventory_balance,unique"`
	Quantity    int64  `json:"quantity" gorm:"not null;default:0"`
	AvgCost     int64  `json:"avg_cost" gorm:"not null;default:0"`
	Amount      int64  `json:"amount" gorm:"not null;default:0"`
}

// Material 是物料模块骨架模型。
type Material struct {
	BaseModel
	Name         string `json:"name" gorm:"size:160;not null"`
	Code         string `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Category     string `json:"category" gorm:"size:60"`
	Unit         string `json:"unit" gorm:"size:30;not null;default:个"`
	Spec         string `json:"spec" gorm:"size:160"`
	SafetyStock  int64  `json:"safety_stock" gorm:"not null;default:0"`
	CostViewable bool   `json:"-" gorm:"-"`
	DefaultCost  int64  `json:"default_cost,omitempty" gorm:"not null;default:0"`
	Status       string `json:"status" gorm:"size:30;not null;default:active"`
}

// Product 是产品模块骨架模型。
type Product struct {
	BaseModel
	Name        string `json:"name" gorm:"size:160;not null"`
	Code        string `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Unit        string `json:"unit" gorm:"size:30;not null;default:个"`
	Spec        string `json:"spec" gorm:"size:160"`
	SafetyStock int64  `json:"safety_stock" gorm:"not null;default:0"`
	DefaultCost int64  `json:"default_cost,omitempty" gorm:"not null;default:0"`
	Status      string `json:"status" gorm:"size:30;not null;default:active"`
}

// InventoryDocument 是库存业务单据表。
//
// 业务规则：草稿可以修改；审核过账后只能冲销，不能直接编辑。
type InventoryDocument struct {
	BaseModel
	Code               string                  `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Type               string                  `json:"type" gorm:"size:30;not null;index"`
	Status             string                  `json:"status" gorm:"size:30;not null;default:draft;index"`
	WarehouseID        uint                    `json:"warehouse_id" gorm:"not null;index"`
	ToWarehouseID      *uint                   `json:"to_warehouse_id" gorm:"index"`
	Reason             string                  `json:"reason" gorm:"size:255"`
	BusinessType       string                  `json:"business_type" gorm:"size:40;index"`
	SupplierID         *uint                   `json:"supplier_id" gorm:"index"`
	CustomerID         *uint                   `json:"customer_id" gorm:"index"`
	DepartmentID       *uint                   `json:"department_id" gorm:"index"`
	OriginalDocumentID *uint                   `json:"original_document_id" gorm:"index"`
	IdempotencyKey     string                  `json:"idempotency_key" gorm:"size:120;index"`
	CreatedBy          uint                    `json:"created_by" gorm:"index"`
	PostedBy           *uint                   `json:"posted_by" gorm:"index"`
	PostedAt           *time.Time              `json:"posted_at"`
	ReversedBy         *uint                   `json:"reversed_by" gorm:"index"`
	ReversedAt         *time.Time              `json:"reversed_at"`
	Lines              []InventoryDocumentLine `json:"lines" gorm:"foreignKey:DocumentID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

// InventoryDocumentLine 是库存单据明细行。
type InventoryDocumentLine struct {
	BaseModel
	DocumentID uint   `json:"document_id" gorm:"not null;index"`
	ItemType   string `json:"item_type" gorm:"size:30;not null"`
	ItemID     uint   `json:"item_id" gorm:"not null;index"`
	LocationID *uint  `json:"location_id" gorm:"index"`
	Quantity   int64  `json:"quantity" gorm:"not null"`
	UnitCost   int64  `json:"unit_cost,omitempty" gorm:"not null;default:0"`
	Amount     int64  `json:"amount,omitempty" gorm:"not null;default:0"`
	Remark     string `json:"remark" gorm:"size:255"`
}

// InventoryLedger 是库存过账流水。
type InventoryLedger struct {
	BaseModel
	DocumentID  uint   `json:"document_id" gorm:"not null;index"`
	LineID      uint   `json:"line_id" gorm:"not null;index"`
	Type        string `json:"type" gorm:"size:30;not null;index"`
	WarehouseID uint   `json:"warehouse_id" gorm:"not null;index"`
	LocationID  *uint  `json:"location_id" gorm:"index"`
	ItemType    string `json:"item_type" gorm:"size:30;not null;index"`
	ItemID      uint   `json:"item_id" gorm:"not null;index"`
	Quantity    int64  `json:"quantity" gorm:"not null"`
	UnitCost    int64  `json:"unit_cost,omitempty" gorm:"not null;default:0"`
	Amount      int64  `json:"amount,omitempty" gorm:"not null;default:0"`
	BalanceQty  int64  `json:"balance_qty" gorm:"not null"`
	BalanceAmt  int64  `json:"balance_amount,omitempty" gorm:"not null"`
}

// WorkOrder 是可流转任务单主表。
//
// 业务说明：
// 主任务由办公室控制整体状态；部门只提交 DepartmentTask 状态。
// 生产单的数量使用 4 位定点整数，和库存模块保持一致。
type WorkOrder struct {
	BaseModel
	Code            string           `json:"code" gorm:"size:80;not null;uniqueIndex"`
	Title           string           `json:"title" gorm:"size:160;not null"`
	Type            string           `json:"type" gorm:"size:40;not null;default:production;index"`
	Status          string           `json:"status" gorm:"size:40;not null;default:draft;index"`
	Priority        string           `json:"priority" gorm:"size:30;not null;default:normal;index"`
	CustomerID      *uint            `json:"customer_id" gorm:"index"`
	ProductID       *uint            `json:"product_id" gorm:"index"`
	ProductName     string           `json:"product_name" gorm:"size:160"`
	PlannedQuantity int64            `json:"planned_quantity" gorm:"not null;default:0"`
	Unit            string           `json:"unit" gorm:"size:30"`
	DueAt           *time.Time       `json:"due_at"`
	Description     string           `json:"description" gorm:"size:1000"`
	CreatedBy       uint             `json:"created_by" gorm:"index"`
	DispatchedAt    *time.Time       `json:"dispatched_at"`
	CompletedAt     *time.Time       `json:"completed_at"`
	CancelReason    string           `json:"cancel_reason" gorm:"size:500"`
	DepartmentTasks []DepartmentTask `json:"department_tasks" gorm:"foreignKey:WorkOrderID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

// DepartmentTask 是部门子任务，用于把任务单拆到具体部门并行执行。
type DepartmentTask struct {
	BaseModel
	WorkOrderID        uint       `json:"work_order_id" gorm:"not null;index"`
	DepartmentID       uint       `json:"department_id" gorm:"not null;index"`
	Title              string     `json:"title" gorm:"size:160;not null"`
	Status             string     `json:"status" gorm:"size:40;not null;default:received;index"`
	PlannedQuantity    int64      `json:"planned_quantity" gorm:"not null;default:0"`
	CompletedQuantity  int64      `json:"completed_quantity" gorm:"not null;default:0"`
	AssigneeUserID     *uint      `json:"assignee_user_id" gorm:"index"`
	Progress           int        `json:"progress" gorm:"not null;default:0"`
	Remark             string     `json:"remark" gorm:"size:500"`
	AcceptedAt         *time.Time `json:"accepted_at"`
	PartialCompletedAt *time.Time `json:"partial_completed_at"`
	CompletedAt        *time.Time `json:"completed_at"`
}

// WorkOrderFlowLog 记录任务单和部门子任务的流转审计。
type WorkOrderFlowLog struct {
	BaseModel
	WorkOrderID      uint   `json:"work_order_id" gorm:"not null;index"`
	DepartmentTaskID *uint  `json:"department_task_id" gorm:"index"`
	DepartmentID     *uint  `json:"department_id" gorm:"index"`
	ActorUserID      *uint  `json:"actor_user_id" gorm:"index"`
	ActorUsername    string `json:"actor_username" gorm:"size:80;index"`
	Action           string `json:"action" gorm:"size:60;not null;index"`
	StatusBefore     string `json:"status_before" gorm:"size:40"`
	StatusAfter      string `json:"status_after" gorm:"size:40"`
	QuantityBefore   int64  `json:"quantity_before" gorm:"not null;default:0"`
	QuantityAfter    int64  `json:"quantity_after" gorm:"not null;default:0"`
	Reason           string `json:"reason" gorm:"size:500"`
	Remark           string `json:"remark" gorm:"size:500"`
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
		&RefreshSession{},
		&Role{},
		&Permission{},
		&UserRole{},
		&RolePermission{},
		&AuditLog{},
		&Customer{},
		&Supplier{},
		&Contact{},
		&ContactPhone{},
		&Warehouse{},
		&Location{},
		&InventoryBalance{},
		&Material{},
		&Product{},
		&InventoryDocument{},
		&InventoryDocumentLine{},
		&InventoryLedger{},
		&Mold{},
		&MoldEvent{},
		&WorkOrder{},
		&DepartmentTask{},
		&WorkOrderFlowLog{},
		&ImageFile{},
	}
}
