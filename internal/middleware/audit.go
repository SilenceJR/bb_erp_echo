package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
	"bb_erp_echo/internal/operator"
	"bb_erp_echo/internal/shared/response"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// Audit 记录写操作审计日志。
//
// 参数说明：
// - db：审计日志写入数据库连接。
// - logger：审计写入失败时使用的结构化日志器。
//
// 记录范围：只记录 POST、PUT、PATCH、DELETE 中实际改变状态的请求；读取、
// OPTIONS 和导入预览不写审计。Action 使用稳定的“模块:动作”编码，Method
// 仍保留原始 HTTP 方法，便于按业务动作检索而不丢失传输层信息。
// 身份规则：个人账号记录具体人员；部门终端账号记录部门、终端和“未知”人员。
func Audit(db *gorm.DB, logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			method := c.Request().Method
			action, record := auditAction(method, c.Request().URL.Path)
			if !record {
				return err
			}

			current := auth.GetCurrentUser(c)
			status := response.ResponseStatus(c)
			// Echo 延迟到上层 HTTPErrorHandler 写错误响应；此时 middleware
			// 看到的响应状态仍可能是 200。审计不能把失败请求记录成成功状态。
			if err != nil && status < http.StatusBadRequest {
				status = response.StatusFromError(err)
			}
			log := model.AuditLog{
				RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
				Object:    c.Path(),
				Action:    action,
				Method:    method,
				Path:      c.Request().URL.Path,
				Status:    status,
				RemoteIP:  c.RealIP(),
				UserAgent: c.Request().UserAgent(),
				Result:    "success",
			}
			if err != nil || status >= http.StatusBadRequest {
				log.Result = "failed"
			}
			if current != nil {
				log.ActorUserID = &current.ID
				log.ActorUsername = current.Username
				log.AccountType = current.AccountType
				log.OrganizationID = &current.OrganizationID
				log.DepartmentID = current.DepartmentID
				log.TerminalID = current.TerminalID
				if current.AccountType == model.AccountTypeDepartmentTerminal {
					log.PersonName = model.UnknownPerson
				} else {
					log.PersonName = current.Name
				}
			}
			if identity, ok := operator.Get(c); ok {
				log.OperatorEmployeeID = &identity.EmployeeID
				log.OperatorEmployeeName = identity.EmployeeName
				log.OperatorDepartmentID = &identity.DepartmentID
				log.OperatorDepartmentName = identity.DepartmentName
			}
			if createErr := db.Create(&log).Error; createErr != nil {
				logger.Error("create audit log failed", "error", createErr)
			}
			return err
		}
	}
}

// auditAction 返回业务稳定动作编码及是否需要记录。它只依赖 URL 路径和
// HTTP 方法，不读取请求体，避免把密码、令牌或业务敏感字段带入审计层。
func auditAction(method, path string) (string, bool) {
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
		return "", false
	}
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	if strings.HasSuffix(path, "/import/preview") {
		return "", false
	}
	module := auditModule(path)
	action := "update"
	switch {
	case strings.HasSuffix(path, "/import/commit"):
		action = "import"
	case strings.HasSuffix(path, "/partial-complete"):
		action = "partial_complete"
	case strings.HasSuffix(path, "/status"):
		action = "status"
	case strings.HasSuffix(path, "/reset-password"):
		action = "reset_password"
	case strings.HasSuffix(path, "/permissions") && strings.Contains(path, "/roles/"):
		action = "assign_permissions"
	case strings.HasSuffix(path, "/roles") && strings.Contains(path, "/users/"):
		action = "assign_roles"
	case strings.HasSuffix(path, "/employees") && strings.Contains(path, "/departments/"):
		action = "replace_employees"
	case strings.HasSuffix(path, "/default"):
		action = "set_default"
	case strings.HasSuffix(path, "/bulk-location"):
		action = "bulk_move"
	case strings.HasSuffix(path, "/bulk"):
		action = "bulk_create"
	case strings.HasSuffix(path, "/movements"):
		action = "movement"
	case strings.HasSuffix(path, "/drawings") && method == http.MethodPost:
		action = "drawing_upload"
	case strings.HasSuffix(path, "/content") && method == http.MethodPut:
		action = "replace_content"
	case strings.HasSuffix(path, "/post"):
		action = "post"
	case strings.HasSuffix(path, "/reverse"):
		action = "reverse"
	case strings.HasSuffix(path, "/dispatch"):
		action = "dispatch"
	case strings.HasSuffix(path, "/pause"):
		action = "pause"
	case strings.HasSuffix(path, "/resume"):
		action = "resume"
	case strings.HasSuffix(path, "/urgent"):
		action = "urgent"
	case strings.HasSuffix(path, "/complete") || strings.HasSuffix(path, "/start"):
		if strings.HasSuffix(path, "/start") {
			action = "start"
		} else {
			action = "complete"
		}
	case method == http.MethodDelete:
		action = "delete"
	case method == http.MethodPost:
		action = "create"
	}
	if module == "" {
		module = "api"
	}
	return module + ":" + action, true
}

func auditModule(path string) string {
	switch {
	case hasAuditPrefix(path, "/api/v1/system/departments"):
		return "departments"
	case hasAuditPrefix(path, "/api/v1/system/employees"):
		return "employees"
	case hasAuditPrefix(path, "/api/v1/system/terminals"):
		return "terminals"
	case hasAuditPrefix(path, "/api/v1/system/users"):
		return "users"
	case hasAuditPrefix(path, "/api/v1/system/roles"):
		return "roles"
	case hasAuditPrefix(path, "/api/v1/customer-codes") || hasAuditPrefix(path, "/api/v1/customers"):
		return "customers"
	case hasAuditPrefix(path, "/api/v1/suppliers"):
		return "suppliers"
	case hasAuditPrefix(path, "/api/v1/inventory-documents") || hasAuditPrefix(path, "/api/v1/inventory-balances") || hasAuditPrefix(path, "/api/v1/inventory-ledgers"):
		return "warehouses"
	case hasAuditPrefix(path, "/api/v1/warehouse") || hasAuditPrefix(path, "/api/v1/warehouses") || hasAuditPrefix(path, "/api/v1/locations") || hasAuditPrefix(path, "/api/v1/materials") || hasAuditPrefix(path, "/api/v1/products"):
		return "warehouses"
	case hasAuditPrefix(path, "/api/v1/mold-locations") || hasAuditPrefix(path, "/api/v1/molds"):
		return "molds"
	case hasAuditPrefix(path, "/api/v1/workorder"):
		return "workorder"
	case hasAuditPrefix(path, "/api/v1/files"):
		return "files"
	default:
		return ""
	}
}

func hasAuditPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}
