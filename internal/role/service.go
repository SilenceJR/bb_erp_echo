// Package role 负责角色、权限和 Casbin 策略装载。
package role

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/config"
	"bb_erp_echo/internal/model"

	"gorm.io/gorm"
)

const (
	// SuperAdminCode 是系统内置超级管理员角色编码。
	SuperAdminCode = "super_admin"
	// BossCode 是升级兼容用的历史老板角色编码。新数据库不会创建它。
	BossCode = "boss"
	// TerminalOperatorCode 是升级兼容用的历史部门终端角色编码。新数据库
	// 不会创建它，新建终端账号也不会自动绑定它。
	TerminalOperatorCode = "department_terminal_operator"
	// SilenceUsername 是全新数据库额外注入的超级管理员账号名。
	SilenceUsername = "Silence"
	// CostViewCode 是成本字段查看权限编码。
	CostViewCode = "cost:view"
	// TemporaryProductWriteCode 是生产单内临时建立仓库产品档案的权限编码。
	TemporaryProductWriteCode = "workorder:temporary-product:write"
)

var (
	// ErrNoPermissions 表示调用方没有提供任何明确的权限 ID。
	ErrNoPermissions = errors.New("permission IDs must not be empty")
	// ErrPermissionCodeNotFound 表示至少一个权限编码不存在。
	ErrPermissionCodeNotFound = errors.New("permission code not found")
	// ErrRoleCodeNotFound 表示至少一个角色编码不存在。
	ErrRoleCodeNotFound = errors.New("role code not found")
)

// Service 封装角色、权限和策略操作。
type Service struct {
	// DB 是角色、权限和关联关系持久化连接。
	DB *gorm.DB
	// Authorizer 是统一的权限快照 provider。
	Authorizer Authorizer
	// assignmentMu 串行化本进程内的角色/权限替换和最后超级管理员检查。
	// SQLite 本身只允许串行写入，但显式互斥可以避免两个事务在检查后
	// 同时删除最后一个超级管理员角色。
	assignmentMu sync.Mutex
}

// AssignmentService 描述角色与权限、用户与角色的分配能力。
// Handler 依赖此接口而不是具体实现，便于替换持久层或策略引擎。
type AssignmentService interface {
	RolePermissionIDs(roleIDs []uint) (map[uint][]uint, error)
	ReplaceRolePermissions(roleID uint, permissionIDs []uint) error
}

// AuthorizedAssignmentService 是角色权限管理接口的带操作者安全边界版本。
// 保留 AssignmentService 的旧方法用于初始化和内部兼容，HTTP 管理接口会
// 优先使用本接口，在事务内重新读取操作者和目标对象。
type AuthorizedAssignmentService interface {
	ReplaceRolePermissionsForActor(actor *auth.CurrentUser, roleID uint, permissionIDs []uint) error
}

// UserRoleService 是用户模块所需的最小角色服务接口。
type UserRoleService interface {
	UserRoleIDs(userIDs []uint) (map[uint][]uint, error)
	ReplaceUserRoles(userID uint, roleIDs []uint, allowSuperAdmin bool) error
	AssignRoleCodes(userID uint, codes []string) error
	AssignRoleCodesTx(tx *gorm.DB, userID uint, codes []string) error
	ReloadPolicies() error
}

// AuthorizedUserRoleService 是用户角色管理接口的带操作者安全边界版本。
type AuthorizedUserRoleService interface {
	ReplaceUserRolesForActor(actor *auth.CurrentUser, userID uint, roleIDs []uint) error
}

// AuthorizedUserStatusService 将账号启停与角色替换放进同一串行边界，
// 保证并发操作也不能停用最后一个有效超级管理员。
type AuthorizedUserStatusService interface {
	UpdateUserStatusForActor(actor *auth.CurrentUser, userID uint, status string) error
}

// NewService 创建角色权限服务。
//
// 参数说明：
// - db：GORM 数据库连接。
// - authorizer：统一权限快照 provider。
func NewService(db *gorm.DB, authorizer Authorizer) *Service {
	return &Service{DB: db, Authorizer: authorizer}
}

// SeedSystemData 初始化系统运行必需的基础数据。
//
// 参数说明：
// - cfg：系统配置，读取默认管理员账号。
func (s *Service) SeedSystemData(cfg *config.Config) error {
	if cfg == nil {
		return errors.New("system seed config is nil")
	}
	freshDatabase, err := s.isFreshDatabase()
	if err != nil {
		return err
	}
	silencePassword := ""
	if freshDatabase {
		silencePassword = strings.TrimSpace(cfg.Silence.Password)
		if silencePassword != "" {
			if err := auth.ValidatePassword(silencePassword); err != nil {
				return fmt.Errorf("validate Silence administrator password: %w", err)
			}
		}
	}

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

	for _, permission := range DefaultPermissions() {
		if err := s.DB.FirstOrCreate(&permission, model.Permission{Code: permission.Code}).Error; err != nil {
			return err
		}
	}
	if err := s.backfillUpdateReadPermission(); err != nil {
		return err
	}

	superDefinition, ok := fixedRoleDefinitionByCode(SuperAdminCode)
	if !ok {
		return errors.New("super admin fixed role definition is missing")
	}
	var super model.Role
	result := s.DB.Where("code = ?", superDefinition.Code).First(&super)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		super = model.Role{
			Name:        superDefinition.Name,
			Code:        superDefinition.Code,
			Description: superDefinition.Description,
			System:      true,
		}
		if err := s.DB.Create(&super).Error; err != nil {
			return err
		}
	} else if result.Error != nil {
		return result.Error
	} else if !isFixedRole(super) || !super.System {
		// super_admin 是唯一锁定系统角色；升级旧库时强制恢复锁定。
		if err := s.DB.Model(&super).Update("system", true).Error; err != nil {
			return err
		}
		super.System = true
	}

	// 老的 boss 和部门终端系统角色只解除锁定，不删除角色或关联，保留
	// 升级库中的用户授权；重复启动时该更新天然幂等。
	if err := s.unlockLegacySystemRoles(); err != nil {
		return err
	}
	// 超级管理员必须拥有完整权限，包括成本查看权限。这里只追加缺失
	// 绑定，不移除历史额外绑定。
	if err := s.AttachAllPermissions(super.ID); err != nil {
		return err
	}

	return s.DB.Transaction(func(tx *gorm.DB) error {
		var admin model.User
		result := tx.Where("username = ?", cfg.Admin.Username).First(&admin)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			adminHash, err := auth.HashPassword(cfg.Admin.Password)
			if err != nil {
				return err
			}
			admin = model.User{
				Username:        cfg.Admin.Username,
				AccountType:     model.AccountTypePersonal,
				Name:            cfg.Admin.Name,
				OrganizationID:  org.ID,
				DepartmentID:    &dept.ID,
				Status:          model.StatusActive,
				PasswordHash:    adminHash,
				PasswordVersion: auth.InitialPasswordVersion,
			}
			if err := tx.Create(&admin).Error; err != nil {
				return err
			}
			if err := s.AssignRoleCodesTx(tx, admin.ID, []string{SuperAdminCode}); err != nil {
				return err
			}
		} else if result.Error != nil {
			return result.Error
		}
		if !freshDatabase {
			// Existing accounts, including a pre-existing admin or Silence, are
			// never reset or rebound. A missing original admin is still created by
			// the historical FirstOrCreate-compatible initialization behavior.
			return nil
		}

		if silencePassword == "" {
			// The optional Silence administrator is only created when an explicit
			// password is configured. The original admin remains sufficient for a
			// fresh installation to complete initialization out of the box.
			return nil
		}

		silenceHash, err := auth.HashPassword(silencePassword)
		if err != nil {
			return err
		}
		silence := model.User{
			Username:        SilenceUsername,
			AccountType:     model.AccountTypePersonal,
			Name:            SilenceUsername,
			OrganizationID:  org.ID,
			DepartmentID:    &dept.ID,
			Status:          model.StatusActive,
			SystemManaged:   true,
			PasswordHash:    silenceHash,
			PasswordVersion: auth.InitialPasswordVersion,
		}
		if err := tx.Create(&silence).Error; err != nil {
			return err
		}
		return s.AssignRoleCodesTx(tx, silence.ID, []string{SuperAdminCode})
	})
}

// isFreshDatabase only treats completely empty core identity data as fresh.
// Unscoped user counting prevents a database containing only soft-deleted
// historical accounts from receiving initialization credentials again.
func (s *Service) isFreshDatabase() (bool, error) {
	checks := []struct {
		model    any
		unscoped bool
	}{
		{model: &model.User{}, unscoped: true},
		{model: &model.Organization{}},
		{model: &model.Role{}},
		{model: &model.Permission{}},
	}
	for _, check := range checks {
		query := s.DB.Model(check.model)
		if check.unscoped {
			query = query.Unscoped()
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return false, nil
		}
	}
	return true, nil
}

// unlockLegacySystemRoles converts the historical boss and terminal roles to
// ordinary custom roles while retaining all role and user associations.
func (s *Service) unlockLegacySystemRoles() error {
	return s.DB.Model(&model.Role{}).
		Where("code IN ? AND code <> ?", []string{BossCode, TerminalOperatorCode}, SuperAdminCode).
		Update("system", false).Error
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
	if len(excludedCodes) == 0 {
		return s.AttachAllPermissions(roleID)
	}

	var permissions []model.Permission
	query := s.DB
	query = query.Where("code NOT IN ?", excludedCodes)
	if err := s.DB.Where("role_id = ? AND permission_id IN (SELECT id FROM permissions WHERE code IN ?)", roleID, excludedCodes).
		Delete(&model.RolePermission{}).Error; err != nil {
		return err
	}
	if err := query.Find(&permissions).Error; err != nil {
		return err
	}
	ids := make([]uint, 0, len(permissions))
	for _, permission := range permissions {
		ids = append(ids, permission.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	return s.AttachPermissions(roleID, ids)
}

// AttachAllPermissions 为角色追加当前数据库中的全部权限。
//
// 需要显式绑定全部权限时调用本方法，避免把 AttachPermissions 的空切片
// 解释成“全部”，从而让权限编码查询为空时错误地放大为全量授权。
func (s *Service) AttachAllPermissions(roleID uint) error {
	var permissions []model.Permission
	if err := s.DB.Find(&permissions).Error; err != nil {
		return err
	}
	if len(permissions) == 0 {
		return ErrNoPermissions
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
// - permissionIDs：明确指定的权限 ID 列表；为空时返回错误。
func (s *Service) AttachPermissions(roleID uint, permissionIDs []uint) error {
	if len(permissionIDs) == 0 {
		return ErrNoPermissions
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
// - codes：权限编码列表，例如 workorder:read。
func (s *Service) AttachPermissionCodes(roleID uint, codes []string) error {
	if len(codes) == 0 {
		return ErrPermissionCodeNotFound
	}
	for _, code := range codes {
		if strings.TrimSpace(code) == "" {
			return fmt.Errorf("%w: empty code", ErrPermissionCodeNotFound)
		}
	}

	var permissions []model.Permission
	if err := s.DB.Where("code IN ?", codes).Find(&permissions).Error; err != nil {
		return err
	}
	found := make(map[string]struct{}, len(permissions))
	var ids []uint
	for _, permission := range permissions {
		found[permission.Code] = struct{}{}
		ids = append(ids, permission.ID)
	}
	missing := make([]string, 0)
	missingSet := make(map[string]struct{})
	for _, code := range codes {
		if _, ok := found[code]; ok {
			continue
		}
		if _, seen := missingSet[code]; seen {
			continue
		}
		missing = append(missing, code)
		missingSet[code] = struct{}{}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrPermissionCodeNotFound, strings.Join(missing, ", "))
	}
	if len(ids) == 0 {
		return ErrPermissionCodeNotFound
	}
	return s.AttachPermissions(roleID, ids)
}

// AssignRoleCodes 根据角色编码为用户追加角色。
//
// 参数说明：
// - userID：用户 ID。
// - codes：角色编码列表，例如 super_admin。
func (s *Service) AssignRoleCodes(userID uint, codes []string) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		return s.AssignRoleCodesTx(tx, userID, codes)
	})
}

// AssignRoleCodesTx 在调用方事务中为用户追加角色。
//
// 角色编码解析和关联写入都使用传入事务，供创建部门终端账号时与用户
// 主记录保持原子性。任何编码缺失都会失败，不会静默创建无角色账号。
func (s *Service) AssignRoleCodesTx(tx *gorm.DB, userID uint, codes []string) error {
	if tx == nil {
		return errors.New("role assignment transaction is nil")
	}
	if len(codes) == 0 {
		return nil
	}

	var roles []model.Role
	if err := tx.Where("code IN ?", codes).Find(&roles).Error; err != nil {
		return err
	}
	found := make(map[string]struct{}, len(roles))
	for _, item := range roles {
		found[item.Code] = struct{}{}
	}
	missing := make([]string, 0)
	missingSet := make(map[string]struct{})
	for _, code := range codes {
		if _, ok := found[code]; ok {
			continue
		}
		if _, seen := missingSet[code]; seen {
			continue
		}
		missing = append(missing, code)
		missingSet[code] = struct{}{}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrRoleCodeNotFound, strings.Join(missing, ", "))
	}
	for _, item := range roles {
		row := model.UserRole{UserID: userID, RoleID: item.ID}
		if err := tx.FirstOrCreate(&row, model.UserRole{UserID: userID, RoleID: item.ID}).Error; err != nil {
			return err
		}
	}
	return nil
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
	s.assignmentMu.Lock()
	defer s.assignmentMu.Unlock()
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		return s.replaceRolePermissionsTx(tx, roleID, permissionIDs)
	}); err != nil {
		return err
	}
	return s.ReloadPolicies()
}

// ReplaceUserRoles 原子替换用户角色并刷新运行时策略。
func (s *Service) ReplaceUserRoles(userID uint, roleIDs []uint, allowSuperAdmin bool) error {
	s.assignmentMu.Lock()
	defer s.assignmentMu.Unlock()
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		var item model.User
		if err := tx.First(&item, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidUserID
			}
			return err
		}
		if !allowSuperAdmin && s.includesSuperAdminTx(tx, roleIDs) {
			return ErrSuperAdminNotAllowed
		}
		if err := s.validateRoleIDsTx(tx, roleIDs); err != nil {
			return err
		}
		if err := s.ensureSuperAdminLifecycleTx(tx, item, roleIDs); err != nil {
			return err
		}
		return s.replaceUserRolesTx(tx, userID, roleIDs)
	}); err != nil {
		return err
	}
	return s.ReloadPolicies()
}

// ReplaceRolePermissionsForActor 在事务内重新确认操作者的角色权限，防止
// 仅凭前端快照越权修改角色。超级管理员角色是唯一锁定系统角色，不能被
// 该接口直接改写。
func (s *Service) ReplaceRolePermissionsForActor(actor *auth.CurrentUser, roleID uint, permissionIDs []uint) error {
	s.assignmentMu.Lock()
	defer s.assignmentMu.Unlock()
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		effective, err := s.validateAssignmentActorTx(tx, actor, "system:roles:write")
		if err != nil {
			return err
		}
		isSuper, err := s.userHasRoleCodeTx(tx, actor.ID, SuperAdminCode)
		if err != nil {
			return err
		}
		if !isSuper {
			var foreignUsers int64
			if err := tx.Table("user_roles").
				Joins("JOIN users ON users.id = user_roles.user_id").
				Where("user_roles.role_id = ? AND users.organization_id <> ?", roleID, actor.OrganizationID).
				Count(&foreignUsers).Error; err != nil {
				return err
			}
			if foreignUsers > 0 {
				return ErrAssignmentOrganizationDenied
			}
		}
		requested, err := s.permissionCodesByIDsTx(tx, permissionIDs)
		if err != nil {
			return err
		}
		for code := range requested {
			if _, ok := effective[code]; !ok {
				return ErrManagerCannotGrant
			}
		}
		return s.replaceRolePermissionsTx(tx, roleID, permissionIDs)
	}); err != nil {
		return err
	}
	return s.ReloadPolicies()
}

// ReplaceUserRolesForActor 在单一事务内完成组织、自身账号、权限边界和
// 最后超级管理员检查，再替换目标用户角色。
func (s *Service) ReplaceUserRolesForActor(actor *auth.CurrentUser, userID uint, roleIDs []uint) error {
	s.assignmentMu.Lock()
	defer s.assignmentMu.Unlock()
	if err := s.DB.Transaction(func(tx *gorm.DB) error {
		actorPermissions, err := s.validateAssignmentActorTx(tx, actor, "system:users:write")
		if err != nil {
			return err
		}
		var target model.User
		if err := tx.First(&target, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidUserID
			}
			return err
		}
		if target.SystemManaged {
			return ErrInvalidUserID
		}
		if actor == nil || target.ID == actor.ID {
			return ErrAssignmentSelfDenied
		}
		if actor.OrganizationID != 0 && target.OrganizationID != actor.OrganizationID {
			return ErrAssignmentOrganizationDenied
		}
		if err := s.validateRoleIDsTx(tx, roleIDs); err != nil {
			return err
		}
		var actorRoleIDs []uint
		if err := tx.Model(&model.UserRole{}).Where("user_id = ?", actor.ID).Pluck("role_id", &actorRoleIDs).Error; err != nil {
			return err
		}
		actorRoles := make(map[uint]struct{}, len(actorRoleIDs))
		for _, roleID := range actorRoleIDs {
			actorRoles[roleID] = struct{}{}
		}
		isSuper, err := s.userHasRoleCodeTx(tx, actor.ID, SuperAdminCode)
		if err != nil {
			return err
		}
		if !isSuper {
			for _, roleID := range roleIDs {
				if _, ok := actorRoles[roleID]; !ok {
					return ErrManagerCannotGrant
				}
			}
		}
		roles, err := s.rolesByIDsTx(tx, roleIDs)
		if err != nil {
			return err
		}
		includesSuper := false
		for _, item := range roles {
			if item.Code == SuperAdminCode {
				includesSuper = true
				if !s.hasAllPermissionsTx(tx, actorPermissions) {
					return ErrManagerCannotGrant
				}
			} else {
				rolePermissions, err := s.permissionCodesForRoleTx(tx, item.ID)
				if err != nil {
					return err
				}
				for code := range rolePermissions {
					if _, ok := actorPermissions[code]; !ok {
						return ErrManagerCannotGrant
					}
				}
			}
		}
		if target.AccountType == model.AccountTypeDepartmentTerminal && includesSuper {
			return ErrSuperAdminNotAllowed
		}
		if err := s.ensureSuperAdminLifecycleTx(tx, target, roleIDs); err != nil {
			return err
		}
		return s.replaceUserRolesTx(tx, userID, roleIDs)
	}); err != nil {
		return err
	}
	return s.ReloadPolicies()
}

func (s *Service) userHasRoleCodeTx(tx *gorm.DB, userID uint, code string) (bool, error) {
	var count int64
	err := tx.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.code = ?", userID, code).
		Count(&count).Error
	return count > 0, err
}

// UpdateUserStatusForActor 在事务内重新确认操作者写权限和组织边界，随后
// 与角色替换共用 assignmentMu 检查并维护最后一个有效超级管理员约束。
func (s *Service) UpdateUserStatusForActor(actor *auth.CurrentUser, userID uint, status string) error {
	s.assignmentMu.Lock()
	defer s.assignmentMu.Unlock()
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := s.validateAssignmentActorTx(tx, actor, "system:users:write"); err != nil {
			return err
		}
		var target model.User
		if err := tx.First(&target, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidUserID
			}
			return err
		}
		if target.SystemManaged {
			return ErrInvalidUserID
		}
		if actor == nil || target.OrganizationID != actor.OrganizationID {
			return ErrAssignmentOrganizationDenied
		}
		if target.Status == model.StatusActive && status == model.StatusDisabled {
			var isSuper int64
			if err := tx.Table("user_roles").
				Joins("JOIN roles ON roles.id = user_roles.role_id").
				Where("user_roles.user_id = ? AND roles.code = ?", target.ID, SuperAdminCode).
				Count(&isSuper).Error; err != nil {
				return err
			}
			if isSuper > 0 {
				var activeSuper int64
				if err := tx.Table("user_roles").
					Joins("JOIN users ON users.id = user_roles.user_id").
					Joins("JOIN roles ON roles.id = user_roles.role_id").
					Where("users.status = ? AND roles.code = ?", model.StatusActive, SuperAdminCode).
					Count(&activeSuper).Error; err != nil {
					return err
				}
				if activeSuper <= 1 {
					return ErrLastSuperAdmin
				}
			}
		}
		result := tx.Model(&model.User{}).
			Where("id = ? AND organization_id = ? AND status = ?", target.ID, target.OrganizationID, target.Status).
			Update("status", status)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAssignmentConflict
		}
		return nil
	})
}

func (s *Service) replaceRolePermissionsTx(tx *gorm.DB, roleID uint, permissionIDs []uint) error {
	var item model.Role
	if err := tx.First(&item, roleID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidRoleID
		}
		return err
	}
	if isFixedRole(item) {
		return ErrSystemRoleLocked
	}
	if err := s.validatePermissionIDsTx(tx, permissionIDs); err != nil {
		return err
	}
	return s.replaceAssociationsTx(tx, &model.RolePermission{}, "role_id", roleID, len(permissionIDs), func(index int) any {
		return &model.RolePermission{RoleID: roleID, PermissionID: permissionIDs[index]}
	})
}

func (s *Service) replaceUserRolesTx(tx *gorm.DB, userID uint, roleIDs []uint) error {
	return s.replaceAssociationsTx(tx, &model.UserRole{}, "user_id", userID, len(roleIDs), func(index int) any {
		return &model.UserRole{UserID: userID, RoleID: roleIDs[index]}
	})
}

func (s *Service) replaceAssociationsTx(tx *gorm.DB, modelValue any, ownerColumn string, ownerID uint, count int, rowAt func(int) any) error {
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
}

func (s *Service) validateRoleIDsTx(tx *gorm.DB, roleIDs []uint) error {
	if err := validateAssignmentIDs(roleIDs, ErrInvalidRoleID); err != nil {
		return err
	}
	if len(roleIDs) == 0 {
		return nil
	}
	var count int64
	if err := tx.Model(&model.Role{}).Where("id IN ?", roleIDs).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(roleIDs)) {
		return ErrInvalidRoleID
	}
	return nil
}

func (s *Service) validatePermissionIDsTx(tx *gorm.DB, permissionIDs []uint) error {
	if err := validateAssignmentIDs(permissionIDs, ErrInvalidPermissionID); err != nil {
		return err
	}
	if len(permissionIDs) == 0 {
		return nil
	}
	var count int64
	if err := tx.Model(&model.Permission{}).Where("id IN ?", permissionIDs).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(permissionIDs)) {
		return ErrInvalidPermissionID
	}
	return nil
}

func validateAssignmentIDs(ids []uint, invalid error) error {
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			return invalid
		}
		if _, ok := seen[id]; ok {
			return ErrDuplicateAssignmentID
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (s *Service) rolesByIDsTx(tx *gorm.DB, roleIDs []uint) ([]model.Role, error) {
	if len(roleIDs) == 0 {
		return []model.Role{}, nil
	}
	var roles []model.Role
	if err := tx.Where("id IN ?", roleIDs).Find(&roles).Error; err != nil {
		return nil, err
	}
	if len(roles) != len(roleIDs) {
		return nil, ErrInvalidRoleID
	}
	return roles, nil
}

func (s *Service) permissionCodesByIDsTx(tx *gorm.DB, permissionIDs []uint) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(permissionIDs))
	if err := s.validatePermissionIDsTx(tx, permissionIDs); err != nil {
		return nil, err
	}
	if len(permissionIDs) == 0 {
		return result, nil
	}
	var permissions []model.Permission
	if err := tx.Where("id IN ?", permissionIDs).Find(&permissions).Error; err != nil {
		return nil, err
	}
	for _, permission := range permissions {
		result[permission.Code] = struct{}{}
	}
	return result, nil
}

func (s *Service) permissionCodesForRoleTx(tx *gorm.DB, roleID uint) (map[string]struct{}, error) {
	var codes []string
	if err := tx.Table("role_permissions").
		Select("permissions.code").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Where("role_permissions.role_id = ?", roleID).
		Scan(&codes).Error; err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		result[code] = struct{}{}
	}
	return result, nil
}

func (s *Service) hasAllPermissionsTx(tx *gorm.DB, effective map[string]struct{}) bool {
	var codes []string
	if err := tx.Model(&model.Permission{}).Pluck("code", &codes).Error; err != nil {
		return false
	}
	for _, code := range codes {
		if _, ok := effective[code]; !ok {
			return false
		}
	}
	return len(codes) > 0
}

// validateAssignmentActorTx loads the actor by ID inside the mutation
// transaction. CurrentUser is only a request hint; the database snapshot is
// authoritative so a stale JWT cannot retain management rights after a role
// change.
func (s *Service) validateAssignmentActorTx(tx *gorm.DB, actor *auth.CurrentUser, requiredPermission string) (map[string]struct{}, error) {
	if actor == nil || actor.ID == 0 {
		return nil, ErrAssignmentActorRequired
	}
	var current model.User
	if err := tx.First(&current, actor.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAssignmentPermissionDenied
		}
		return nil, err
	}
	if current.Status != model.StatusActive {
		return nil, ErrAssignmentPermissionDenied
	}
	if actor.OrganizationID != 0 && current.OrganizationID != actor.OrganizationID {
		return nil, ErrAssignmentOrganizationDenied
	}

	var rows []struct {
		Code string
	}
	if err := tx.Table("user_roles").
		Select("DISTINCT permissions.code").
		Joins("JOIN role_permissions ON role_permissions.role_id = user_roles.role_id").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
		Where("user_roles.user_id = ?", current.ID).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	effective := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		effective[row.Code] = struct{}{}
	}
	var isSuper int64
	if err := tx.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.code = ?", current.ID, SuperAdminCode).
		Count(&isSuper).Error; err != nil {
		return nil, err
	}
	if isSuper > 0 {
		// Keep the invariant true even if an old database had an incomplete
		// super_admin binding before startup reconciliation.
		var all []string
		if err := tx.Model(&model.Permission{}).Pluck("code", &all).Error; err != nil {
			return nil, err
		}
		for _, code := range all {
			effective[code] = struct{}{}
		}
	}
	if _, ok := effective[requiredPermission]; !ok {
		return nil, ErrAssignmentPermissionDenied
	}
	return effective, nil
}

func (s *Service) includesSuperAdminTx(tx *gorm.DB, roleIDs []uint) bool {
	if len(roleIDs) == 0 {
		return false
	}
	var count int64
	if err := tx.Model(&model.Role{}).Where("id IN ? AND code = ?", roleIDs, SuperAdminCode).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func (s *Service) ensureSuperAdminLifecycleTx(tx *gorm.DB, target model.User, roleIDs []uint) error {
	var currentCount int64
	if err := tx.Table("user_roles").
		Joins("JOIN users ON users.id = user_roles.user_id").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("users.status = ? AND roles.code = ?", model.StatusActive, SuperAdminCode).
		Count(&currentCount).Error; err != nil {
		return err
	}
	var targetCurrent int64
	if err := tx.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.code = ?", target.ID, SuperAdminCode).
		Count(&targetCurrent).Error; err != nil {
		return err
	}
	targetNext := s.includesSuperAdminTx(tx, roleIDs)
	if target.Status == model.StatusActive {
		switch {
		case targetCurrent > 0 && !targetNext:
			currentCount--
		case targetCurrent == 0 && targetNext:
			currentCount++
		}
	}
	if currentCount < 1 {
		return ErrLastSuperAdmin
	}
	return nil
}

// ReloadPolicies 从数据库重新加载 Casbin 分组策略和权限策略。
//
// 调用时机：角色、权限或用户角色关系变更后必须调用，否则内存策略不会更新。
// 刷新由统一 provider 完成，避免业务服务直接读写当前 Casbin 引擎。
func (s *Service) ReloadPolicies() error {
	if s.Authorizer == nil {
		return errors.New("role service authorizer is nil")
	}
	return s.Authorizer.ReloadPolicies()
}

// Enforce 使用统一 provider 判断当前用户是否具有指定权限。
func (s *Service) Enforce(subject, object, action, organization, department string) (bool, error) {
	if s.Authorizer == nil {
		return false, errors.New("role service authorizer is nil")
	}
	return s.Authorizer.Enforce(subject, object, action, organization, department)
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
		{"物料查看", "material:read", "/api/v1/materials", "read"},
		{"物料维护", "material:write", "/api/v1/materials", "write"},
		{"产品查看", "product:read", "/api/v1/products", "read"},
		{"产品维护", "product:write", "/api/v1/products", "write"},
		{"模具查看", "mold:read", "/api/v1/molds", "read"},
		{"模具维护", "mold:write", "/api/v1/molds", "write"},
		{"模具资料导入", "mold:import", "/api/v1/molds/import", "import"},
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
	}
	items := make([]model.Permission, 0, len(defs))
	for _, def := range defs {
		items = append(items, model.Permission{Name: def.name, Code: def.code, Object: def.object, Action: def.action})
	}
	return items
}
