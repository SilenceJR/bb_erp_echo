// Package operator 提供业务写操作统一使用的员工责任人解析和校验。
package operator

import (
	"errors"
	"net/http"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

// ContextKey 是在 Echo 请求上下文中保存已校验责任人的键。
const ContextKey = "operator_employee"

// Identity 是一次业务写操作的员工和当前账号部门快照。
// 员工姓名、部门名称在请求校验时读取，随后写入业务日志，避免档案变化影响历史。
type Identity struct {
	EmployeeID     uint
	EmployeeName   string
	DepartmentID   uint
	DepartmentName string
}

// Snapshot 返回当前请求已经校验的账号、终端、员工和部门责任快照。
// 调用方应在同一个业务事务内先调用 Resolve，再把返回值写入业务模型。
func Snapshot(c *echo.Context) model.OperatorSnapshot {
	var snapshot model.OperatorSnapshot
	if current := auth.GetCurrentUser(c); current != nil {
		snapshot.OperatorUserID = &current.ID
		snapshot.OperatorUsername = current.Username
		snapshot.OperatorTerminalID = current.TerminalID
	}
	if identity, ok := Get(c); ok {
		snapshot.OperatorEmployeeID = &identity.EmployeeID
		snapshot.OperatorEmployeeName = identity.EmployeeName
		snapshot.OperatorDepartmentID = &identity.DepartmentID
		snapshot.OperatorDepartmentName = identity.DepartmentName
	}
	return snapshot
}

// Resolve 校验操作人属于当前登录账号的启用部门，并将快照写入上下文。
// 新数据库要求 employee_id 必填；认证后的所有业务写入口都必须经过这里。
func Resolve(c *echo.Context, db *gorm.DB, employeeID uint) (Identity, error) {
	if employeeID == 0 {
		return Identity{}, echo.NewHTTPError(http.StatusBadRequest, "必须选择操作员工")
	}
	current := auth.GetCurrentUser(c)
	if current == nil {
		return Identity{}, echo.NewHTTPError(http.StatusUnauthorized, "未登录")
	}
	// 中间件写入的 CurrentUser 可能早于本次业务事务；在事务内重读账号归属，
	// 避免管理员刚换部门后旧请求仍按原部门完成写入。
	var account model.User
	if err := db.WithContext(c.Request().Context()).Select("id", "organization_id", "department_id", "terminal_id", "status").First(&account, current.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Identity{}, echo.NewHTTPError(http.StatusUnauthorized, "账号不存在")
		}
		return Identity{}, err
	}
	if account.Status != model.StatusActive {
		return Identity{}, echo.NewHTTPError(http.StatusForbidden, "账号已停用")
	}
	if account.OrganizationID != current.OrganizationID {
		return Identity{}, echo.NewHTTPError(http.StatusForbidden, "账号组织归属已变化")
	}
	current.DepartmentID = account.DepartmentID
	current.TerminalID = account.TerminalID
	if account.DepartmentID == nil || *account.DepartmentID == 0 {
		return Identity{}, echo.NewHTTPError(http.StatusForbidden, "当前账号未绑定部门，不能执行业务写入")
	}

	var department model.Department
	if err := db.WithContext(c.Request().Context()).
		Where("id = ? AND organization_id = ?", *account.DepartmentID, current.OrganizationID).
		First(&department).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Identity{}, echo.NewHTTPError(http.StatusForbidden, "当前账号部门不存在或不属于当前组织")
		}
		return Identity{}, err
	}
	if department.Status != model.StatusActive {
		return Identity{}, echo.NewHTTPError(http.StatusConflict, "当前账号部门已停用，不能执行业务写入")
	}

	var employee model.Employee
	if err := db.WithContext(c.Request().Context()).First(&employee, employeeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Identity{}, echo.NewHTTPError(http.StatusConflict, "操作员工不存在或关系已失效")
		}
		return Identity{}, err
	}
	if employee.OrganizationID != current.OrganizationID {
		return Identity{}, echo.NewHTTPError(http.StatusForbidden, "操作员工不属于当前组织")
	}
	if employee.Status != model.StatusActive {
		return Identity{}, echo.NewHTTPError(http.StatusConflict, "操作员工已停用")
	}

	var relation model.EmployeeDepartment
	if err := db.WithContext(c.Request().Context()).
		Where("employee_id = ? AND department_id = ?", employee.ID, department.ID).
		First(&relation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Identity{}, echo.NewHTTPError(http.StatusConflict, "操作员工已不属于当前部门")
		}
		return Identity{}, err
	}

	identity := Identity{
		EmployeeID:     employee.ID,
		EmployeeName:   employee.Name,
		DepartmentID:   department.ID,
		DepartmentName: department.Name,
	}
	c.Set(ContextKey, identity)
	return identity, nil
}

// Get 从请求上下文读取已校验的责任人快照。
func Get(c *echo.Context) (Identity, bool) {
	value := c.Get(ContextKey)
	identity, ok := value.(Identity)
	return identity, ok && identity.EmployeeID != 0
}
