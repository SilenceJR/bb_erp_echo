package role

import "errors"

// ErrSuperAdminNotAllowed 表示目标账号类型不能获得超级管理员角色。
var ErrSuperAdminNotAllowed = errors.New("super admin role is not allowed")
