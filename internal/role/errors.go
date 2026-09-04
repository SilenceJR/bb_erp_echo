package role

import "errors"

// ErrSuperAdminNotAllowed 表示目标账号类型不能获得超级管理员角色。
var ErrSuperAdminNotAllowed = errors.New("super admin role is not allowed")

var (
	// ErrInvalidRoleID 表示请求中包含不存在或无效的角色 ID。
	ErrInvalidRoleID = errors.New("invalid role id")
	// ErrInvalidPermissionID 表示请求中包含不存在或无效的权限 ID。
	ErrInvalidPermissionID = errors.New("invalid permission id")
	// ErrInvalidUserID 表示请求目标账号不存在。
	ErrInvalidUserID = errors.New("invalid user id")
	// ErrAssignmentActorRequired 表示管理写操作缺少操作者身份。
	ErrAssignmentActorRequired = errors.New("assignment actor is required")
	// ErrAssignmentPermissionDenied 表示操作者无对应的管理写权限。
	ErrAssignmentPermissionDenied = errors.New("assignment permission denied")
	// ErrAssignmentOrganizationDenied 表示目标账号不属于操作者组织。
	ErrAssignmentOrganizationDenied = errors.New("assignment organization denied")
	// ErrAssignmentSelfDenied 表示不允许修改操作者自己的角色。
	ErrAssignmentSelfDenied = errors.New("assignment self modification denied")
	// ErrManagerCannotGrant 表示普通管理员不能授予自身没有的权限。
	ErrManagerCannotGrant = errors.New("manager cannot grant permissions outside effective set")
	// ErrSystemRoleLocked 表示唯一锁定的超级管理员角色不可被普通接口改写。
	ErrSystemRoleLocked = errors.New("system role is locked")
	// ErrLastSuperAdmin 表示操作会导致没有任何有效超级管理员。
	ErrLastSuperAdmin = errors.New("at least one active super admin must remain")
	// ErrDuplicateAssignmentID 表示请求中出现重复的关联 ID。
	ErrDuplicateAssignmentID = errors.New("duplicate assignment id")
	// ErrAssignmentConflict 表示目标账号在事务提交前已被其他请求修改。
	ErrAssignmentConflict = errors.New("assignment target changed")
)
