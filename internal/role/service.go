// Package role 负责角色、权限和 Casbin 策略装载。
package role

import (
	"fmt"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/config"
	"bb_erp_echo/internal/model"

	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	"gorm.io/gorm"
)

const (
	// SuperAdminCode 是系统内置超级管理员角色编码。
	SuperAdminCode = "super_admin"
	// BossCode 是老板角色编码，默认拥有成本查看权限。
	BossCode = "boss"
	// TerminalOperatorCode 是部门公共终端账号默认角色编码。
	TerminalOperatorCode = "department_terminal_operator"
	// CostViewCode 是成本字段查看权限编码。
	CostViewCode = "cost:view"
	// TemporaryProductWriteCode 是生产单内临时建立仓库产品档案的权限编码。
	TemporaryProductWriteCode = "workorder:temporary-product:write"
)

// Service 封装角色、权限和策略操作。
type Service struct {
	// DB 是角色、权限和关联关系持久化连接。
	DB *gorm.DB
	// Enforcer 是 Casbin 内存权限引擎。
	Enforcer *casbin.Enforcer
}

// AssignmentService 描述角色与权限、用户与角色的分配能力。
// Handler 依赖此接口而不是具体实现，便于替换持久层或策略引擎。
type AssignmentService interface {
	RolePermissionIDs(roleIDs []uint) (map[uint][]uint, error)
	ReplaceRolePermissions(roleID uint, permissionIDs []uint) error
}

// UserRoleService 是用户模块所需的最小角色服务接口。
type UserRoleService interface {
	UserRoleIDs(userIDs []uint) (map[uint][]uint, error)
	ReplaceUserRoles(userID uint, roleIDs []uint, allowSuperAdmin bool) error
	AssignRoleCodes(userID uint, codes []string) error
}

// NewService 创建角色权限服务。
//
// 参数说明：
// - db：GORM 数据库连接。
// - enforcer：Casbin 权限引擎。
func NewService(db *gorm.DB, enforcer *casbin.Enforcer) *Service {
	return &Service{DB: db, Enforcer: enforcer}
}

// NewEnforcer 创建 Casbin 权限引擎。
//
// 权限模型说明：
// - sub：用户或角色。
// - obj：接口资源路径。
// - act：动作，当前约定为 read/write。
// - org/dept：组织和部门数据范围，* 表示不限制。
func NewEnforcer() (*casbin.Enforcer, error) {
	m, err := casbinmodel.NewModelFromString(`
[request_definition]
r = sub, obj, act, org, dept

[policy_definition]
p = sub, obj, act, org, dept

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && regexMatch(r.act, p.act) && (p.org == "*" || p.org == r.org) && (p.dept == "*" || p.dept == r.dept)
`)
	if err != nil {
		return nil, fmt.Errorf("create casbin model: %w", err)
	}
	return casbin.NewEnforcer(m)
}

// SeedSystemData 初始化系统运行必需的基础数据。
//
// 参数说明：
// - cfg：系统配置，读取默认管理员账号。
func (s *Service) SeedSystemData(cfg *config.Config) error {
	org := model.Organization{Name: "博邦", Code: "BOBANG", Status: model.StatusActive}
	if err := s.DB.FirstOrCreate(&org, model.Organization{Code: org.Code}).Error; err != nil {
		return err
	}

	dept := model.Department{OrganizationID: org.ID, Name: "办公室", Code: "HQ", Status: model.StatusActive}
	if err := s.DB.Where(" code = ?", dept.Code).FirstOrCreate(&dept).Error; err != nil {
		return err
	}

	terminal := model.Terminal{DepartmentID: dept.ID, Code: "injection-terminal-01", Name: "注塑车间电脑01", Status: model.StatusActive}
	if err := s.DB.FirstOrCreate(&terminal, model.Terminal{Code: terminal.Code}).Error; err != nil {
		return err
	}

	warehouse := model.Warehouse{Name: "默认仓库", Code: "MAIN", Status: model.StatusActive}
	if err := s.DB.FirstOrCreate(&warehouse, model.Warehouse{Code: warehouse.Code}).Error; err != nil {
		return err
	}

	for _, permission := range DefaultPermissions() {
		if err := s.DB.FirstOrCreate(&permission, model.Permission{Code: permission.Code}).Error; err != nil {
			return err
		}
	}
	if err := s.backfillUpdateReadPermission(); err != nil {
		return err
	}

	super := model.Role{Name: "超级管理员", Code: SuperAdminCode, Description: "系统内置管理员角色", System: true}
	if err := s.DB.FirstOrCreate(&super, model.Role{Code: super.Code}).Error; err != nil {
		return err
	}
	boss := model.Role{Name: "老板", Code: BossCode, Description: "默认拥有成本查看权限", System: true}
	if err := s.DB.FirstOrCreate(&boss, model.Role{Code: boss.Code}).Error; err != nil {
		return err
	}
	terminalRole := model.Role{Name: "部门终端操作员", Code: TerminalOperatorCode, Description: "公共部门终端账号使用", System: true}
	if err := s.DB.FirstOrCreate(&terminalRole, model.Role{Code: terminalRole.Code}).Error; err != nil {
		return err
	}

	if err := s.AttachAllExcept(super.ID, []string{CostViewCode}); err != nil {
		return err
	}
	if err := s.AttachPermissionCodes(boss.ID, []string{CostViewCode}); err != nil {
		return err
	}
	if err := s.AttachPermissionCodes(terminalRole.ID, []string{"workorder:read", "workorder:write", "tasks:read", "tasks:write", "warehouse:read", "inventory:read"}); err != nil {
		return err
	}

	hash, err := auth.HashPassword(cfg.Admin.Password)
	if err != nil {
		return err
	}
	admin := model.User{
		Username:       cfg.Admin.Username,
		AccountType:    model.AccountTypePersonal,
		Name:           cfg.Admin.Name,
		OrganizationID: org.ID,
		DepartmentID:   &dept.ID,
		Status:         model.StatusActive,
		PasswordHash:   hash,
	}
	if err := s.DB.FirstOrCreate(&admin, model.User{Username: admin.Username}).Error; err != nil {
		return err
	}
	return s.AssignRoleCodes(admin.ID, []string{SuperAdminCode})
}

// backfillUpdateReadPermission 为升级前已拥有更新维护权限的角色补充只读权限。
func (s *Service) backfillUpdateReadPermission() error {
	var writePermission model.Permission
	if err := s.DB.Where("code = ?", "system:updates:write").First(&writePermission).Error; err != nil {
		return err
	}
	var readPermission model.Permission
	if err := s.DB.Where("code = ?", "system:updates:read").First(&readPermission).Error; err != nil {
		return err
	}
	var roleIDs []uint
	if err := s.DB.Model(&model.RolePermission{}).
		Where("permission_id = ?", writePermission.ID).
		Pluck("role_id", &roleIDs).Error; err != nil {
		return err
	}
	for _, roleID := range roleIDs {
		binding := model.RolePermission{RoleID: roleID, PermissionID: readPermission.ID}
		if err := s.DB.FirstOrCreate(&binding, model.RolePermission{
			RoleID: roleID, PermissionID: readPermission.ID,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// AttachAllExcept 为角色追加除指定权限码以外的全部权限。
//
// 参数说明：
// - roleID：角色 ID。
// - excludedCodes：需要排除的权限编码。
func (s *Service) AttachAllExcept(roleID uint, excludedCodes []string) error {
	var permissions []model.Permission
	query := s.DB
	if len(excludedCodes) > 0 {
		query = query.Where("code NOT IN ?", excludedCodes)
		if err := s.DB.Where("role_id = ? AND permission_id IN (SELECT id FROM permissions WHERE code IN ?)", roleID, excludedCodes).
			Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
	}
	if err := query.Find(&permissions).Error; err != nil {
		return err
	}
	ids := make([]uint, 0, len(permissions))
	for _, permission := range permissions {
		ids = append(ids, permission.ID)
	}
	return s.AttachPermissions(roleID, ids)
}

// AttachPermissions 为角色追加权限。
//
// 参数说明：
// - roleID：角色 ID。
// - permissionIDs：权限 ID 列表；为空时表示绑定全部权限。
func (s *Service) AttachPermissions(roleID uint, permissionIDs []uint) error {
	if len(permissionIDs) == 0 {
		var permissions []model.Permission
		if err := s.DB.Find(&permissions).Error; err != nil {
			return err
		}
		for _, permission := range permissions {
			permissionIDs = append(permissionIDs, permission.ID)
		}
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		for _, permissionID := range permissionIDs {
			row := model.RolePermission{RoleID: roleID, PermissionID: permissionID}
			if err := tx.FirstOrCreate(&row, model.RolePermission{RoleID: roleID, PermissionID: permissionID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// AttachPermissionCodes 根据权限码为角色追加权限。
//
// 参数说明：
// - roleID：角色 ID。
// - codes：权限编码列表，例如 tasks:read。
func (s *Service) AttachPermissionCodes(roleID uint, codes []string) error {
	var permissions []model.Permission
	if err := s.DB.Where("code IN ?", codes).Find(&permissions).Error; err != nil {
		return err
	}
	var ids []uint
	for _, permission := range permissions {
		ids = append(ids, permission.ID)
	}
	return s.AttachPermissions(roleID, ids)
}

// AssignRoleCodes 根据角色编码为用户追加角色。
//
// 参数说明：
// - userID：用户 ID。
// - codes：角色编码列表，例如 super_admin。
func (s *Service) AssignRoleCodes(userID uint, codes []string) error {
	var roles []model.Role
	if err := s.DB.Where("code IN ?", codes).Find(&roles).Error; err != nil {
		return err
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		for _, role := range roles {
			row := model.UserRole{UserID: userID, RoleID: role.ID}
			if err := tx.FirstOrCreate(&row, model.UserRole{UserID: userID, RoleID: role.ID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RolePermissionIDs 批量查询角色当前绑定的权限，避免列表接口逐角色查询。
func (s *Service) RolePermissionIDs(roleIDs []uint) (map[uint][]uint, error) {
	result := make(map[uint][]uint, len(roleIDs))
	for _, roleID := range roleIDs {
		result[roleID] = []uint{}
	}
	if len(roleIDs) == 0 {
		return result, nil
	}
	var rows []model.RolePermission
	if err := s.DB.Where("role_id IN ?", roleIDs).Order("role_id, permission_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.RoleID] = append(result[row.RoleID], row.PermissionID)
	}
	return result, nil
}

// UserRoleIDs 批量查询用户当前绑定的角色，避免列表接口逐用户查询。
func (s *Service) UserRoleIDs(userIDs []uint) (map[uint][]uint, error) {
	result := make(map[uint][]uint, len(userIDs))
	for _, userID := range userIDs {
		result[userID] = []uint{}
	}
	if len(userIDs) == 0 {
		return result, nil
	}
	var rows []model.UserRole
	if err := s.DB.Where("user_id IN ?", userIDs).Order("user_id, role_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.UserID] = append(result[row.UserID], row.RoleID)
	}
	return result, nil
}

// ReplaceRolePermissions 原子替换角色权限并刷新运行时策略。
func (s *Service) ReplaceRolePermissions(roleID uint, permissionIDs []uint) error {
	if err := s.replaceAssociations(
		&model.RolePermission{},
		"role_id",
		roleID,
		len(permissionIDs),
		func(index int) any {
			return &model.RolePermission{RoleID: roleID, PermissionID: permissionIDs[index]}
		},
	); err != nil {
		return err
	}
	return s.ReloadPolicies()
}

// ReplaceUserRoles 原子替换用户角色并刷新运行时策略。
func (s *Service) ReplaceUserRoles(userID uint, roleIDs []uint, allowSuperAdmin bool) error {
	if !allowSuperAdmin && s.IncludesSystemRole(roleIDs) {
		return ErrSuperAdminNotAllowed
	}
	if err := s.replaceAssociations(
		&model.UserRole{},
		"user_id",
		userID,
		len(roleIDs),
		func(index int) any {
			return &model.UserRole{UserID: userID, RoleID: roleIDs[index]}
		},
	); err != nil {
		return err
	}
	return s.ReloadPolicies()
}

func (s *Service) replaceAssociations(modelValue any, ownerColumn string, ownerID uint, count int, rowAt func(int) any) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		// 关联表有唯一索引；软删除会留下占位记录，导致重新绑定同一项时冲突。
		if err := tx.Unscoped().Where(ownerColumn+" = ?", ownerID).Delete(modelValue).Error; err != nil {
			return err
		}
		for index := 0; index < count; index++ {
			if err := tx.Create(rowAt(index)).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ReloadPolicies 从数据库重新加载 Casbin 分组策略和权限策略。
//
// 调用时机：角色、权限或用户角色关系变更后必须调用，否则内存策略不会更新。
func (s *Service) ReloadPolicies() error {
	s.Enforcer.ClearPolicy()

	var policies []struct {
		Username string
		RoleCode string
	}
	if err := s.DB.Table("user_roles").
		Select("users.username, roles.code AS role_code").
		Joins("JOIN users ON users.id = user_roles.user_id").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Scan(&policies).Error; err != nil {
		return err
	}
	for _, policy := range policies {
		if _, err := s.Enforcer.AddGroupingPolicy(policy.Username, policy.RoleCode); err != nil {
			return err
		}
	}

	var permissions []struct {
		RoleCode string
		Object   string
		Action   string
	}
	if err := s.DB.Table("role_permissions").
		Select("roles.code AS role_code, permissions.object, permissions.action").
		Joins("JOIN roles ON roles.id = role_permissions.role_id").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Scan(&permissions).Error; err != nil {
		return err
	}
	for _, permission := range permissions {
		if _, err := s.Enforcer.AddPolicy(permission.RoleCode, permission.Object, permission.Action, "*", "*"); err != nil {
			return err
		}
	}
	return nil
}

// IncludesSystemRole 判断角色 ID 列表中是否包含超级管理员角色。
//
// 参数说明：
// - roleIDs：待检查的角色 ID 列表。
func (s *Service) IncludesSystemRole(roleIDs []uint) bool {
	var count int64
	s.DB.Model(&model.Role{}).Where("id IN ? AND code = ?", roleIDs, SuperAdminCode).Count(&count)
	return count > 0
}

// DefaultPermissions 返回系统默认权限清单。
//
// 参数说明：无。
// 返回说明：返回尚未持久化的 Permission 模型列表。
func DefaultPermissions() []model.Permission {
	defs := []struct {
		name   string
		code   string
		object string
		action string
	}{
		{"组织查看", "system:organizations:read", "/api/v1/system/organizations", "read"},
		{"组织维护", "system:organizations:write", "/api/v1/system/organizations", "write"},
		{"部门查看", "system:departments:read", "/api/v1/system/departments", "read"},
		{"部门维护", "system:departments:write", "/api/v1/system/departments", "write"},
		{"员工查看", "system:employees:read", "/api/v1/system/employees", "read"},
		{"员工维护", "system:employees:write", "/api/v1/system/employees", "write"},
		{"终端查看", "system:terminals:read", "/api/v1/system/terminals", "read"},
		{"终端维护", "system:terminals:write", "/api/v1/system/terminals", "write"},
		{"用户查看", "system:users:read", "/api/v1/system/users", "read"},
		{"用户维护", "system:users:write", "/api/v1/system/users", "write"},
		{"角色查看", "system:roles:read", "/api/v1/system/roles", "read"},
		{"角色维护", "system:roles:write", "/api/v1/system/roles", "write"},
		{"权限查看", "system:permissions:read", "/api/v1/system/permissions", "read"},
		{"审计查看", "system:audits:read", "/api/v1/system/audits", "read"},
		{"更新查看", "system:updates:read", "/api/v1/system/updates", "read"},
		{"更新维护", "system:updates:write", "/api/v1/system/updates", "write"},
		{"客户查看", "customers:read", "/api/v1/customers", "read"},
		{"客户维护", "customers:write", "/api/v1/customers", "write"},
		{"客户批量导入", "customers:import", "/api/v1/customers/import", "import"},
		{"供应商查看", "suppliers:read", "/api/v1/suppliers", "read"},
		{"供应商维护", "suppliers:write", "/api/v1/suppliers", "write"},
		{"仓库查看", "warehouse:read", "/api/v1/warehouse", "read"},
		{"仓库维护", "warehouse:write", "/api/v1/warehouse", "write"},
		{"库存查看", "inventory:read", "/api/v1/inventory", "read"},
		{"库存维护", "inventory:write", "/api/v1/inventory", "write"},
		{"物料查看", "material:read", "/api/v1/material", "read"},
		{"物料维护", "material:write", "/api/v1/material", "write"},
		{"产品查看", "product:read", "/api/v1/product", "read"},
		{"产品维护", "product:write", "/api/v1/product", "write"},
		{"模具查看", "mold:read", "/api/v1/mold", "read"},
		{"模具维护", "mold:write", "/api/v1/mold", "write"},
		{"模具台账查看", "molds:read", "/api/v1/molds", "read"},
		{"模具台账维护", "molds:write", "/api/v1/molds", "write"},
		{"任务查看", "workorder:read", "/api/v1/workorder", "read"},
		{"任务维护", "workorder:write", "/api/v1/workorder", "write"},
		{"生产单临时产品建档", TemporaryProductWriteCode, "/api/v1/workorder/products", "write"},
		{"报表查看", "statistics:read", "/api/v1/statistics", "read"},
		{"报表维护", "statistics:write", "/api/v1/statistics", "write"},
		{"成本查看", CostViewCode, "/api/v1/cost", "read"},
		{"库存单据查看", "inventory:documents:read", "/api/v1/inventory-documents", "read"},
		{"库存单据维护", "inventory:documents:write", "/api/v1/inventory-documents", "write"},
		{"库存余额查看", "inventory:balances:read", "/api/v1/inventory-balances", "read"},
		{"库存流水查看", "inventory:ledgers:read", "/api/v1/inventory-ledgers", "read"},
		// 兼容第一版 API 命名，避免前端或测试仍使用旧路径时直接失效。
		{"旧任务查看", "tasks:read", "/api/v1/tasks", "read"},
		{"旧任务维护", "tasks:write", "/api/v1/tasks", "write"},
	}
	items := make([]model.Permission, 0, len(defs))
	for _, def := range defs {
		items = append(items, model.Permission{Name: def.name, Code: def.code, Object: def.object, Action: def.action})
	}
	return items
}
