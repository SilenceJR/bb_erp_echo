package middleware

import (
	"log/slog"
	"net/http"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
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
// 记录范围：跳过 GET 和 OPTIONS，只记录会改变状态的请求。
// 身份规则：个人账号记录具体人员；部门终端账号记录部门、终端和“未知”人员。
func Audit(db *gorm.DB, logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			method := c.Request().Method
			if method == http.MethodGet || method == http.MethodOptions {
				return err
			}

			current := auth.GetCurrentUser(c)
			log := model.AuditLog{
				RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
				Object:    c.Path(),
				Action:    method,
				Method:    method,
				Path:      c.Request().URL.Path,
				Status:    response.ResponseStatus(c),
				RemoteIP:  c.RealIP(),
				UserAgent: c.Request().UserAgent(),
				Result:    "success",
			}
			if err != nil || response.ResponseStatus(c) >= http.StatusBadRequest {
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
			if createErr := db.Create(&log).Error; createErr != nil {
				logger.Error("create audit log failed", "error", createErr)
			}
			return err
		}
	}
}
