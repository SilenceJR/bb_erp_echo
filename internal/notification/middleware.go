package notification

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
)

// MutationMiddleware 返回一个在成功业务写请求完成后发布通知的 Echo 中间件。
//
// 中间件位于受 JWT 保护的路由组上，因此只有已经通过后端鉴权的写请求会被
// 观察。通知在 next 返回且 HTTP 状态为 2xx 后发布；业务事务回滚或错误响应
// 不会广播。Publish 的错误被有意忽略，避免 best-effort 通知影响已成功的业务
// 请求。
func MutationMiddleware(service *Service) echo.MiddlewareFunc {
	if service == nil {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return next
		}
	}
	return service.MutationMiddleware()
}

// MutationMiddleware 返回 Service 绑定的写请求通知中间件。
func (s *Service) MutationMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !isMutationMethod(c.Request().Method) {
				return next(c)
			}

			pre := s.preMutationDescriptor(c)
			original := c.Response()
			capture := newCaptureResponseWriter(original)
			c.SetResponse(capture)
			err := next(c)
			// 必须在返回给 Echo 前恢复原 Echo Response，避免统一错误处理器
			// 看不到 Committed/Status，也避免 wrapper 泄露到 keep-alive 请求。
			c.SetResponse(original)

			status := response.ResponseStatus(c)
			if err != nil && status < http.StatusBadRequest {
				status = response.StatusFromError(err)
			}
			if err != nil || status < http.StatusOK || status >= http.StatusMultipleChoices {
				return err
			}
			// 库存单据创建接口用 Idempotency-Key 返回 200 表示命中已有
			// 结果；该响应不是新的业务提交，不能重复广播。首次创建返回
			// 201，仍会按正常路径发布变更。
			if isIdempotentReplay(c, status) {
				return err
			}
			change, ok := s.changeForMutation(c, pre, capture)
			if ok {
				_ = s.Publish(change)
			}
			return err
		}
	}
}

func isMutationMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isIdempotentReplay(c *echo.Context, status int) bool {
	if c == nil || status != http.StatusOK || c.Request().Method != http.MethodPost {
		return false
	}
	return c.Request().Header.Get("Idempotency-Key") != "" && c.Request().URL.Path == "/api/v1/inventory-documents"
}

type mutationDescriptor struct {
	module          string
	moduleTitle     string
	entityType      string
	readPermissions []PermissionRequirement
	operation       string
	invalidateAll   bool
	entityID        uint
}

func (s *Service) preMutationDescriptor(c *echo.Context) *mutationDescriptor {
	path := c.Request().URL.Path
	if !hasPathPrefix(path, "/api/v1/files") || hasPathPrefix(path, "/api/v1/files/images") {
		return nil
	}
	id := parseUint(c.Param("id"))
	if id == 0 || s.db == nil {
		return nil
	}
	var asset model.ImageFile
	if err := s.db.First(&asset, id).Error; err != nil {
		return nil
	}
	d, ok := descriptorForOwner(asset.OwnerType)
	if !ok {
		return nil
	}
	d.entityID = asset.OwnerID
	return &d
}

func (s *Service) changeForMutation(c *echo.Context, pre *mutationDescriptor, capture *captureResponseWriter) (Change, bool) {
	path := c.Request().URL.Path
	d, ok := descriptorForPath(path)
	if !ok && pre != nil {
		d, ok = *pre, true
	}
	if !ok && hasPathPrefix(path, "/api/v1/files/images") {
		d, ok = descriptorForOwner(strings.TrimSpace(c.FormValue("owner_type")))
		if ok {
			d.entityID = parseUint(c.FormValue("owner_id"))
		}
	}
	if !ok {
		return Change{}, false
	}
	if d.operation == "" {
		d.operation = operationFor(c.Request().Method, path)
	}
	if d.entityID == 0 {
		d.entityID = firstID(c, capture)
	}
	pathEntityID := d.entityID
	items, count, hasCount := responseItems(capture, d.entityType, d.operation)
	ownerTarget := pre != nil
	if !ownerTarget && hasPathPrefix(path, "/api/v1/files/images") {
		ownerTarget = d.entityID != 0
	}
	if ownerTarget && d.entityID != 0 {
		// 文件接口返回的是附件实体，但通知目标应是附件所属的业务
		// 实体，避免点击后打开一个没有详情路由的 ImageFile。
		items = []ChangeItem{{EntityType: d.entityType, EntityID: d.entityID, Operation: d.operation}}
		if !hasCount || count <= 0 {
			count = 1
		}
		hasCount = true
	}
	if !ownerTarget && pathEntityID != 0 {
		// 嵌套路由返回的可能是附属记录（例如模具图纸或部门成员），
		// 但刷新/定位目标仍然是 URL 中已经鉴权过的业务实体。
		if len(items) == 0 {
			items = []ChangeItem{{EntityType: d.entityType, EntityID: pathEntityID, Operation: d.operation, Label: labelFromResponse(capture)}}
		} else {
			items[0].EntityID = pathEntityID
			items[0].EntityType = d.entityType
		}
		d.entityID = pathEntityID
	}
	if len(items) == 0 && d.entityID != 0 {
		items = []ChangeItem{{EntityType: d.entityType, EntityID: d.entityID, Operation: d.operation, Label: labelFromResponse(capture)}}
	}
	if count == 0 && hasCount {
		// 幂等批量接口可能成功但实际没有新增/修改，不能制造虚假变更。
		return Change{}, false
	}
	if count <= 0 {
		count = 1
	}
	if len(items) > count && count > 0 {
		count = len(items)
	}
	if len(items) > 3 {
		items = items[:3]
	}
	if count > 1 || d.entityID == 0 {
		d.invalidateAll = true
	}
	current := auth.GetCurrentUser(c)
	if current == nil || current.ID == 0 || current.OrganizationID == 0 {
		return Change{}, false
	}
	return Change{
		Module:          d.module,
		ModuleTitle:     d.moduleTitle,
		EntityType:      d.entityType,
		OrganizationID:  current.OrganizationID,
		ActorUserID:     current.ID,
		Operation:       d.operation,
		Count:           count,
		Items:           items,
		ReadPermissions: d.readPermissions,
		InvalidateAll:   d.invalidateAll,
	}, true
}

func operationFor(method, path string) string {
	if strings.Contains(path, "/import/commit") {
		return OperationImport
	}
	if method == http.MethodDelete {
		return OperationDelete
	}
	if method == http.MethodPost {
		if pathHasAction(path) {
			return OperationUpdate
		}
		return OperationCreate
	}
	return OperationUpdate
}

func pathHasAction(path string) bool {
	for _, action := range []string{"/post", "/reverse", "/dispatch", "/pause", "/resume", "/urgent", "/complete", "/start", "/partial-complete", "/bulk-location"} {
		if strings.HasSuffix(path, action) || strings.Contains(path, action+"/") {
			return true
		}
	}
	return hasPathPrefix(path, "/api/v1/system/users/") || hasPathPrefix(path, "/api/v1/system/roles/") || hasPathPrefix(path, "/api/v1/system/departments/")
}

func descriptorForPath(path string) (mutationDescriptor, bool) {
	// 预览只创建一次性校验会话，不改变业务资料，不应通知在线用户。
	if hasPathPrefix(path, "/api/v1/customers/import/preview") || hasPathPrefix(path, "/api/v1/molds/import/preview") {
		return mutationDescriptor{}, false
	}
	if hasPathPrefix(path, "/api/v1/customers/import/commit") {
		return descriptor("customers", "客户资料", "customer_profile", []PermissionRequirement{{Object: "/api/v1/customers", Action: "read"}}, OperationImport, true), true
	}
	if hasPathPrefix(path, "/api/v1/molds/import/commit") {
		return descriptor("molds", "模具", "mold", []PermissionRequirement{{Object: "/api/v1/molds", Action: "read"}}, OperationImport, true), true
	}

	switch {
	case hasPathPrefix(path, "/api/v1/system/departments"):
		return descriptor("departments", "部门", "department", []PermissionRequirement{{Object: "/api/v1/system/departments", Action: "read"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/system/employees"):
		return descriptor("employees", "员工档案", "employee", []PermissionRequirement{{Object: "/api/v1/system/employees", Action: "read"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/system/terminals"):
		return descriptor("terminals", "终端", "terminal", []PermissionRequirement{{Object: "/api/v1/system/terminals", Action: "read"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/system/users"):
		return descriptor("users", "用户账号", "user", []PermissionRequirement{{Object: "/api/v1/system/users", Action: "read"}, {Object: "/api/v1/system/users", Action: "write"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/system/roles"):
		return descriptor("roles", "角色", "role", []PermissionRequirement{{Object: "/api/v1/system/roles", Action: "read"}, {Object: "/api/v1/system/roles", Action: "write"}, {Object: "/api/v1/system/users", Action: "write"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/customer-codes"):
		return descriptor("customers", "客户资料", "customer_code", []PermissionRequirement{{Object: "/api/v1/customers", Action: "read"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/customers"):
		return descriptor("customers", "客户资料", "customer_profile", []PermissionRequirement{{Object: "/api/v1/customers", Action: "read"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/suppliers"):
		return descriptor("suppliers", "供应商", "supplier", []PermissionRequirement{{Object: "/api/v1/suppliers", Action: "read"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/inventory-documents") || hasPathPrefix(path, "/api/v1/inventory-balances") || hasPathPrefix(path, "/api/v1/inventory-ledgers"):
		return descriptor("warehouses", "仓库", "inventory_document", []PermissionRequirement{{Object: "/api/v1/inventory-documents", Action: "read"}, {Object: "/api/v1/warehouse", Action: "read"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/warehouse") || hasPathPrefix(path, "/api/v1/warehouses") || hasPathPrefix(path, "/api/v1/locations"):
		return descriptor("warehouses", "仓库", "warehouse_item", []PermissionRequirement{{Object: "/api/v1/warehouse", Action: "read"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/materials"):
		return descriptor("warehouses", "仓库", "material", []PermissionRequirement{{Object: "/api/v1/materials", Action: "read"}, {Object: "/api/v1/warehouse", Action: "read"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/products"):
		return descriptor("warehouses", "仓库", "product", []PermissionRequirement{{Object: "/api/v1/products", Action: "read"}, {Object: "/api/v1/warehouse", Action: "read"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/mold-locations") || hasPathPrefix(path, "/api/v1/molds"):
		return descriptor("molds", "模具", "mold", []PermissionRequirement{{Object: "/api/v1/molds", Action: "read"}}, "", false), true
	case hasPathPrefix(path, "/api/v1/workorder"):
		return descriptor("workorder", "任务单", "workorder", []PermissionRequirement{{Object: "/api/v1/workorder", Action: "read"}}, "", false), true
	default:
		return mutationDescriptor{}, false
	}
}

func descriptorForOwner(ownerType string) (mutationDescriptor, bool) {
	switch strings.TrimSpace(ownerType) {
	case "product":
		return descriptor("warehouses", "仓库", "product", []PermissionRequirement{{Object: "/api/v1/warehouse", Action: "read"}}, "", false), true
	case "mold":
		return descriptor("molds", "模具", "mold", []PermissionRequirement{{Object: "/api/v1/molds", Action: "read"}}, "", false), true
	case "workorder", "department_task":
		return descriptor("workorder", "任务单", "workorder", []PermissionRequirement{{Object: "/api/v1/workorder", Action: "read"}}, "", false), true
	default:
		return mutationDescriptor{}, false
	}
}

func descriptor(module, title, entityType string, permissions []PermissionRequirement, operation string, invalidateAll bool) mutationDescriptor {
	return mutationDescriptor{module: module, moduleTitle: title, entityType: entityType, readPermissions: permissions, operation: operation, invalidateAll: invalidateAll}
}

func hasPathPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func parseUint(raw string) uint {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil || value == 0 {
		return 0
	}
	return uint(value)
}

type captureResponseWriter struct {
	http.ResponseWriter
	buf       bytes.Buffer
	maxBytes  int
	truncated bool
}

func newCaptureResponseWriter(writer http.ResponseWriter) *captureResponseWriter {
	return &captureResponseWriter{ResponseWriter: writer, maxBytes: 256 << 10}
}

func (w *captureResponseWriter) Write(p []byte) (int, error) {
	if w.buf.Len() < w.maxBytes {
		remaining := w.maxBytes - w.buf.Len()
		if len(p) <= remaining {
			_, _ = w.buf.Write(p)
		} else {
			_, _ = w.buf.Write(p[:remaining])
			w.truncated = true
		}
	} else {
		w.truncated = true
	}
	return w.ResponseWriter.Write(p)
}

// Unwrap keeps Echo response status inspection and ResponseController support.
func (w *captureResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *captureResponseWriter) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *captureResponseWriter) body() []byte { return bytes.TrimSpace(w.buf.Bytes()) }

func firstID(c *echo.Context, capture *captureResponseWriter) uint {
	for _, name := range []string{"id", "itemID", "item_id"} {
		if id := parseUint(c.Param(name)); id != 0 {
			return id
		}
	}
	if capture == nil || capture.truncated {
		return 0
	}
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(capture.body()))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil {
		return 0
	}
	if object, ok := payload.(map[string]any); ok {
		return numberID(object["id"])
	}
	return 0
}

func responseItems(capture *captureResponseWriter, entityType, operation string) ([]ChangeItem, int, bool) {
	if capture == nil || capture.truncated || len(capture.body()) == 0 {
		return nil, 0, false
	}
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(capture.body()))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil {
		return nil, 0, false
	}
	if list, ok := payload.([]any); ok {
		items := make([]ChangeItem, 0, minInt(3, len(list)))
		for _, raw := range list {
			object, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if id := numberID(object["id"]); id != 0 {
				items = append(items, ChangeItem{EntityType: entityType, EntityID: id, Operation: operation, Label: labelFromObject(object), Fields: fieldsFromObject(object)})
			}
		}
		return items, len(list), true
	}
	object, ok := payload.(map[string]any)
	if !ok {
		return nil, 0, false
	}
	count, hasCount := countFromObject(object)
	if id := numberID(object["id"]); id != 0 {
		return []ChangeItem{{EntityType: entityType, EntityID: id, Operation: operation, Label: labelFromObject(object), Fields: fieldsFromObject(object)}}, maxInt(count, 1), true
	}
	return nil, count, hasCount
}

func labelFromResponse(capture *captureResponseWriter) string {
	if capture == nil || capture.truncated || len(capture.body()) == 0 {
		return ""
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(capture.body()))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil {
		return ""
	}
	return labelFromObject(payload)
}

func countFromObject(object map[string]any) (int, bool) {
	// 导入接口会同时返回明细、编码或附件统计；优先使用代表业务主
	// 实体的字段，避免把一条主记录和它的附属项错误相加。
	for _, key := range []string{"imported_profiles", "molds", "imported_codes"} {
		if number, ok := object[key].(json.Number); ok {
			n, err := strconv.Atoi(string(number))
			if err == nil {
				return n, true
			}
		}
	}
	// 批量业务动作通常返回各操作结果的计数，三者互斥分组后求和。
	total := 0
	found := false
	for _, key := range []string{"created", "updated", "deleted"} {
		if number, ok := object[key].(json.Number); ok {
			n, err := strconv.Atoi(string(number))
			if err == nil {
				total += n
				found = true
			}
		}
	}
	return total, found
}

func labelFromObject(object map[string]any) string {
	for _, key := range []string{"code", "name", "short_name", "mold_number", "model", "username"} {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			value = strings.TrimSpace(value)
			if len(value) > 160 {
				return value[:160]
			}
			return value
		}
	}
	return ""
}

func fieldsFromObject(object map[string]any) map[string]string {
	fields := make(map[string]string)
	for _, key := range []string{"code", "name", "short_name", "status", "type", "mold_number", "model"} {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			fields[key] = strings.TrimSpace(value)
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func numberID(value any) uint {
	switch value := value.(type) {
	case json.Number:
		return parseUint(string(value))
	case float64:
		if value > 0 {
			return uint(value)
		}
	case string:
		return parseUint(value)
	}
	return 0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
