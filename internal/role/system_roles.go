package role

import "bb_erp_echo/internal/model"

// fixedRoleDefinition describes an application-owned role whose permissions
// are maintained by the server rather than by the role management UI.
//
// Keep this registry internal. The Role model and role-management API retain
// their existing wire shape; adding a built-in role in the future only needs
// a new definition and its seed rule here.
type fixedRoleDefinition struct {
	Code        string
	Name        string
	Description string
}

// fixedRoleDefinitions is the single source of truth for built-in roles. The
// first release has exactly one fixed role: super_admin.
var fixedRoleDefinitions = []fixedRoleDefinition{
	{
		Code:        SuperAdminCode,
		Name:        "超级管理员",
		Description: "系统内置管理员角色",
	},
}

func fixedRoleDefinitionByCode(code string) (fixedRoleDefinition, bool) {
	for _, definition := range fixedRoleDefinitions {
		if definition.Code == code {
			return definition, true
		}
	}
	return fixedRoleDefinition{}, false
}

// isFixedRole is the shared guard for role mutation. The code registry is the
// canonical definition for built-in roles. The persisted System marker remains
// part of the check for upgrade compatibility with historical system roles;
// SeedSystemData normalizes the known legacy roles before management writes.
func isFixedRole(item model.Role) bool {
	if _, ok := fixedRoleDefinitionByCode(item.Code); ok {
		return true
	}
	return item.System
}
