// Package notification 提供进程内的实时数据变更通知。
//
// 通知是 best-effort、at-most-once 的会话内消息，不写入数据库。业务写入
// 成功返回后由 MutationMiddleware 生成 Change，再由 Service 根据当前数据库
// 中的账号、组织状态和权限解析收件人。
package notification

import "time"

const (
	// SchemaVersion 是通知线上结构版本。
	SchemaVersion = 1

	// KindDataChanged 表示业务数据新增、修改、删除或导入。
	KindDataChanged = "data_changed"
	// KindTaskTransition 为后续任务流转消息保留的类型名。
	KindTaskTransition = "task_transition"

	// PriorityNormal 是普通资料变更的优先级。
	PriorityNormal = "normal"

	// ActionOpenModule 要求客户端打开对应模块。
	ActionOpenModule = "open_module"
	// ActionOpenEntity 要求客户端打开对应实体详情。
	ActionOpenEntity = "open_entity"

	// OperationCreate 表示新增数据。
	OperationCreate = "create"
	// OperationUpdate 表示修改或业务状态变化。
	OperationUpdate = "update"
	// OperationDelete 表示删除数据。
	OperationDelete = "delete"
	// OperationImport 表示批量导入。
	OperationImport = "import"
)

// PermissionRequirement 描述一个可接收变更通知所需的权限。
//
// Action 通常为 read；系统管理列表沿用 HTTP 权限中间件的兼容规则，必要
// 时也会允许对应的 write 权限接收列表变更。
type PermissionRequirement struct {
	Object string
	Action string
}

// Change 是业务事务成功后的内部变更事件。
//
// Change 不会直接序列化给客户端；ReadPermissions、OrganizationID 和
// ActorUserID 只用于服务端收件人解析。调用方不得把密码、令牌、成本、文件
// 路径或完整模型放进 Label/Fields。
type Change struct {
	Module          string
	ModuleTitle     string
	EntityType      string
	OrganizationID  uint
	DepartmentID    *uint
	ActorUserID     uint
	Operation       string
	Count           int
	Items           []ChangeItem
	ReadPermissions []PermissionRequirement
	InvalidateAll   bool
	OccurredAt      time.Time
}

// ChangeItem 是允许显示给在线用户的最小实体摘要。
type ChangeItem struct {
	EntityType string
	EntityID   uint
	Operation  string
	Label      string
	// Fields 仅允许注册表中定义的少量非敏感展示字段。
	Fields map[string]string
}

// Notification 是 SSE data 字段中的稳定版本化结构。
type Notification struct {
	V            int                `json:"v"`
	ID           string             `json:"id"`
	Kind         string             `json:"kind"`
	Priority     string             `json:"priority"`
	OccurredAt   time.Time          `json:"occurred_at"`
	DisplayForMS int                `json:"display_for_ms"`
	Module       string             `json:"module"`
	Title        string             `json:"title"`
	Summary      string             `json:"summary"`
	Count        int                `json:"count"`
	Items        []NotificationItem `json:"items,omitempty"`
	Action       NotificationAction `json:"action"`
	Refresh      RefreshHint        `json:"refresh"`
	Truncated    bool               `json:"truncated"`
}

// NotificationItem 是客户端显示用的安全实体摘要。
type NotificationItem struct {
	EntityType string            `json:"entity_type"`
	EntityID   uint              `json:"entity_id"`
	Operation  string            `json:"operation"`
	Label      string            `json:"label,omitempty"`
	Fields     map[string]string `json:"fields,omitempty"`
}

// NotificationAction 描述客户端点击通知后的打开动作。
//
// Type 只有 open_module/open_entity 两个 V1 值；模块和实体字段始终使用
// 安全白名单值，客户端点击后仍需通过目标接口重新鉴权。
type NotificationAction struct {
	Type       string `json:"type"`
	Module     string `json:"module"`
	EntityType string `json:"entity_type,omitempty"`
	EntityID   uint   `json:"entity_id,omitempty"`
}

// RefreshHint 告知客户端需要刷新哪一个模块或实体。
type RefreshHint struct {
	Module        string `json:"module"`
	EntityType    string `json:"entity_type,omitempty"`
	EntityID      uint   `json:"entity_id,omitempty"`
	InvalidateAll bool   `json:"invalidate_all"`
}

// Config 是通知服务的运行参数。
type Config struct {
	// MergeWindow 是同一收件账号、同一模块的服务端合并时间窗口。
	MergeWindow time.Duration
	// HeartbeatInterval 是 SSE 保活注释的发送间隔。
	HeartbeatInterval time.Duration
	// QueueSize 是每条在线连接的有界发送队列大小。
	QueueSize int
	// MaxConnectionsPerUser 限制同一账号的在线连接数。
	MaxConnectionsPerUser int
	// MaxConnections 限制本进程全部在线连接数。
	MaxConnections int
}

func (c Config) withDefaults() Config {
	if c.MergeWindow <= 0 {
		c.MergeWindow = 500 * time.Millisecond
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = 20 * time.Second
	}
	if c.QueueSize <= 0 {
		c.QueueSize = 32
	}
	if c.MaxConnectionsPerUser <= 0 {
		c.MaxConnectionsPerUser = 8
	}
	if c.MaxConnections <= 0 {
		c.MaxConnections = 256
	}
	return c
}
