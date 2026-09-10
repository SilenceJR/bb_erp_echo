package notification

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"bb_erp_echo/internal/model"

	"gorm.io/gorm"
)

// Service 负责当前在线账号的通知收件人解析和 Hub 投递。
type Service struct {
	db     *gorm.DB
	hub    *Hub
	config Config
	logger *slog.Logger
}

// NewService 创建无持久化通知服务。
//
// db 仅用于每次投递时重查账号、组织、部门、终端和权限；通知内容本身不会
// 写入该数据库。
func NewService(db *gorm.DB, config Config, logger *slog.Logger) *Service {
	config = config.withDefaults()
	return &Service{db: db, hub: NewHub(config), config: config, logger: logger}
}

// Hub 返回底层在线连接 Hub，主要用于测试和服务生命周期管理。
func (s *Service) Hub() *Hub {
	if s == nil {
		return nil
	}
	return s.hub
}

// Subscribe 注册账号的 SSE 连接。
func (s *Service) Subscribe(userID uint) (<-chan Notification, func(), error) {
	if s == nil || s.hub == nil {
		return nil, func() {}, ErrClosed
	}
	return s.hub.Subscribe(userID)
}

// Metrics 返回通知 Hub 的结构化运行指标快照。
func (s *Service) Metrics() HubMetrics {
	if s == nil || s.hub == nil {
		return HubMetrics{}
	}
	return s.hub.Metrics()
}

// Close 关闭所有连接。服务关闭后不会保留、重放或写入未发送消息。
func (s *Service) Close() {
	if s == nil || s.hub == nil {
		return
	}
	s.hub.Close()
}

// Publish 根据 Change 的当前数据库权限把通知投递给在线账号。
//
// Publish 失败只记录日志，不把错误返回给已经成功完成的业务写请求；通知是
// best-effort 的辅助能力。收件人解析始终读取当前数据库状态，不使用连接建立
// 时缓存的权限快照。
func (s *Service) Publish(change Change) error {
	if s == nil || s.hub == nil {
		return ErrClosed
	}
	if s.hub.isClosed() {
		return ErrClosed
	}
	n, err := notificationFromChange(change)
	if err != nil {
		return err
	}
	if s.db == nil {
		return errors.New("notification database is nil")
	}
	recipients, err := s.resolveRecipients(change)
	if err != nil {
		s.logResolveError(err, change)
		return err
	}
	s.hub.PublishTo(recipients, n)
	return nil
}

func (s *Service) logResolveError(err error, change Change) {
	if s.logger == nil {
		return
	}
	s.logger.Warn("resolve notification recipients failed", "error", err, "module", change.Module)
}

type recipientRow struct {
	ID uint
}

// permissionRecipientRow 是当前数据库联表解析出的可收件账号。权限解析必须
// 在一次查询中完成，避免通知广播按在线账号数量产生 N+1 查询。
type permissionRecipientRow struct {
	ID uint `gorm:"column:id"`
}

func (s *Service) resolveRecipients(change Change) ([]uint, error) {
	if change.OrganizationID == 0 {
		return nil, nil
	}
	requirements := normalizedPermissions(change.ReadPermissions)
	if len(requirements) == 0 {
		return nil, nil
	}
	var users []recipientRow
	query := s.db.Model(&model.User{}).
		Select("users.id").
		Joins("JOIN organizations ON organizations.id = users.organization_id AND organizations.deleted_at IS NULL").
		Joins("LEFT JOIN departments ON departments.id = users.department_id AND departments.deleted_at IS NULL").
		Joins("LEFT JOIN terminals ON terminals.id = users.terminal_id AND terminals.deleted_at IS NULL").
		Where("users.organization_id = ? AND users.status = ? AND organizations.status = ?", change.OrganizationID, model.StatusActive, model.StatusActive).
		Where("(users.department_id IS NULL OR (departments.id IS NOT NULL AND departments.organization_id = users.organization_id AND departments.status = ?))", model.StatusActive).
		Where("(users.terminal_id IS NULL OR (terminals.id IS NOT NULL AND terminals.department_id = users.department_id AND terminals.status = ?))", model.StatusActive)
	if change.ActorUserID != 0 {
		query = query.Where("users.id <> ?", change.ActorUserID)
	}
	if err := query.Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	ids := make([]uint, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	return s.filterRecipientsByPermission(ids, requirements)
}

func (s *Service) filterRecipientsByPermission(userIDs []uint, requirements []PermissionRequirement) ([]uint, error) {
	if len(userIDs) == 0 || len(requirements) == 0 {
		return nil, nil
	}
	predicates := make([]string, 0, len(requirements))
	args := make([]any, 0, len(requirements)*2)
	for _, requirement := range requirements {
		predicates = append(predicates, "permissions.object = ? AND permissions.action = ?")
		args = append(args, requirement.Object, requirement.Action)
	}
	var rows []permissionRecipientRow
	query := s.db.Model(&model.User{}).
		Select("DISTINCT users.id").
		Joins("JOIN user_roles ON user_roles.user_id = users.id AND user_roles.deleted_at IS NULL").
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.deleted_at IS NULL").
		Joins("JOIN role_permissions ON role_permissions.role_id = roles.id AND role_permissions.deleted_at IS NULL").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id AND permissions.deleted_at IS NULL").
		Where("users.id IN ?", userIDs).
		Where("("+strings.Join(predicates, " OR ")+")", args...)
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	allowed := make([]uint, 0, len(rows))
	for _, row := range rows {
		if row.ID != 0 {
			allowed = append(allowed, row.ID)
		}
	}
	return allowed, nil
}

func normalizedPermissions(requirements []PermissionRequirement) []PermissionRequirement {
	seen := make(map[string]struct{}, len(requirements))
	result := make([]PermissionRequirement, 0, len(requirements))
	for _, requirement := range requirements {
		requirement.Object = strings.TrimSpace(requirement.Object)
		requirement.Action = strings.TrimSpace(requirement.Action)
		if requirement.Object == "" || requirement.Action == "" {
			continue
		}
		key := requirement.Object + "\x00" + requirement.Action
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, requirement)
	}
	return result
}

func notificationFromChange(change Change) (Notification, error) {
	change.Module = strings.TrimSpace(change.Module)
	if change.Module == "" {
		return Notification{}, errors.New("notification module is required")
	}
	if change.Operation == "" {
		change.Operation = OperationUpdate
	}
	if change.Count <= 0 {
		change.Count = 1
	}
	if change.OccurredAt.IsZero() {
		change.OccurredAt = time.Now()
	}
	if change.ModuleTitle == "" {
		change.ModuleTitle = moduleTitle(change.Module)
	}
	items := sanitizeItems(change.Items, change.EntityType, change.Operation)
	truncated := len(change.Items) > len(items)
	if change.Count > len(items) && len(items) > 0 {
		truncated = true
	}
	n := Notification{
		V:            SchemaVersion,
		ID:           newNotificationID(),
		Kind:         KindDataChanged,
		Priority:     PriorityNormal,
		OccurredAt:   change.OccurredAt,
		DisplayForMS: 8000,
		Module:       change.Module,
		Title:        singleTitle(change),
		Count:        change.Count,
		Items:        items,
		Action:       NotificationAction{Type: ActionOpenModule, Module: change.Module},
		Refresh:      RefreshHint{Module: change.Module, InvalidateAll: change.InvalidateAll},
		Truncated:    truncated,
	}
	if len(items) == 1 && change.Count == 1 && items[0].EntityID != 0 && entityCanOpen(items[0].EntityType) {
		n.Action = NotificationAction{Type: ActionOpenEntity, Module: change.Module, EntityType: items[0].EntityType, EntityID: items[0].EntityID}
		n.Refresh.EntityType = items[0].EntityType
		n.Refresh.EntityID = items[0].EntityID
	}
	n.Summary = summaryFor(n)
	return n, nil
}

// entityCanOpen 只允许客户端已有明确详情入口的实体使用 open_entity。
// 例如客户编码虽然有 ID，但客户页面的详情入口对应的是客户资料，不能
// 把编码 ID 错当成资料 ID；此类消息退回模块列表。
func entityCanOpen(entityType string) bool {
	switch strings.TrimSpace(entityType) {
	case "customer_code", "inventory_document":
		return false
	default:
		return true
	}
}

func sanitizeItems(items []ChangeItem, entityType, operation string) []NotificationItem {
	result := make([]NotificationItem, 0, minInt(3, len(items)))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.EntityType == "" {
			item.EntityType = entityType
		}
		if item.Operation == "" {
			item.Operation = operation
		}
		if item.EntityID == 0 || strings.TrimSpace(item.EntityType) == "" {
			continue
		}
		item.EntityType = strings.TrimSpace(item.EntityType)
		key := item.EntityType + ":" + uintString(item.EntityID)

		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if len(result) >= 3 {
			continue
		}
		result = append(result, NotificationItem{
			EntityType: item.EntityType,
			EntityID:   item.EntityID,
			Operation:  safeOperation(item.Operation),
			Label:      safeLabel(item.Label),
			Fields:     sanitizeFields(item.Fields),
		})
	}
	return result
}

func sanitizeFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	// Change callers should already use a registered whitelist. This final
	// boundary avoids accidentally serializing arbitrary model fields if a new
	// caller passes a large map in the future.
	allowed := map[string]struct{}{
		"product_model": {}, "customer_model": {}, "material": {}, "ink_required": {}, "status": {}, "mold_type": {}, "cavity_count": {}, "common_group_no": {},
	}
	result := make(map[string]string)
	for key, value := range fields {
		if _, ok := allowed[key]; !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > 160 {
			value = value[:160]
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func safeLabel(label string) string {
	label = strings.TrimSpace(label)
	if len(label) > 160 {
		return label[:160]
	}
	return label
}

func safeOperation(operation string) string {
	switch operation {
	case OperationCreate, OperationUpdate, OperationDelete, OperationImport:
		return operation
	default:
		return OperationUpdate
	}
}

func moduleTitle(module string) string {
	switch module {
	case "departments":
		return "部门"
	case "employees":
		return "员工档案"
	case "users":
		return "用户账号"
	case "terminals":
		return "终端"
	case "roles":
		return "角色"
	case "customers":
		return "客户资料"
	case "suppliers":
		return "供应商"
	case "warehouses":
		return "仓库"
	case "molds":
		return "模具"
	case "workorder":
		return "任务单"
	case "files":
		return "业务资料"
	default:
		return module
	}
}

func operationTitle(operation string) string {
	switch operation {
	case OperationCreate:
		return "新增"
	case OperationDelete:
		return "删除"
	case OperationImport:
		return "导入"
	default:
		return "更新"
	}
}

func singleTitle(change Change) string {
	title := change.ModuleTitle
	if title == "" {
		title = moduleTitle(change.Module)
	}
	return title + "有新变更"
}

func mergedTitle(module string, count int) string {
	return moduleTitle(module) + "有新变更"
}

func summaryFor(n Notification) string {
	title := moduleTitle(n.Module)
	if n.Count == 1 && len(n.Items) == 1 {
		item := n.Items[0]
		label := item.Label
		if label == "" {
			label = "#" + uintString(item.EntityID)
		}
		return fmt.Sprintf("%s %s%s", title, label, operationTitle(item.Operation))
	}
	if n.Count <= 0 {
		return title + "有新变更"
	}
	return fmt.Sprintf("%s有 %d 条数据发生变更", title, n.Count)
}

func newNotificationID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return hex.EncodeToString(bytes[:])
	}
	// crypto/rand failure is exceptionally unlikely. The timestamp remains
	// process-local unique enough for a best-effort ephemeral message ID.
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func uintString(value uint) string {
	return fmt.Sprintf("%d", value)
}

func (h *Hub) isClosed() bool {
	if h == nil {
		return true
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.closed
}
