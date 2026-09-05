package role

import (
	"errors"
	"sync"
	"testing"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"
)

func TestAuthorizedUserRoleAssignmentEnforcesActorBoundary(t *testing.T) {
	service := newAssignmentTestService(t)
	permissions := []model.Permission{
		{Name: "用户维护", Code: "system:users:write", Object: "/api/v1/system/users", Action: "write"},
		{Name: "客户查看", Code: "customers:read", Object: "/api/v1/customers", Action: "read"},
		{Name: "成本查看", Code: CostViewCode, Object: "/api/v1/cost", Action: "read"},
	}
	if err := service.DB.Create(&permissions).Error; err != nil {
		t.Fatalf("create permissions: %v", err)
	}
	managerRole := model.Role{Name: "用户管理员", Code: "user_manager"}
	allowedRole := model.Role{Name: "客户只读", Code: "customer_reader"}
	forbiddenRole := model.Role{Name: "成本角色", Code: "cost_reader"}
	superRole := model.Role{Name: "超级管理员", Code: SuperAdminCode, System: true}
	if err := service.DB.Create(&[]*model.Role{&managerRole, &allowedRole, &forbiddenRole, &superRole}).Error; err != nil {
		t.Fatalf("create roles: %v", err)
	}
	bindings := []model.RolePermission{
		{RoleID: managerRole.ID, PermissionID: permissions[0].ID},
		{RoleID: managerRole.ID, PermissionID: permissions[1].ID},
		{RoleID: allowedRole.ID, PermissionID: permissions[1].ID},
		{RoleID: forbiddenRole.ID, PermissionID: permissions[2].ID},
	}
	if err := service.DB.Create(&bindings).Error; err != nil {
		t.Fatalf("bind permissions: %v", err)
	}
	actor := model.User{Username: "manager", AccountType: model.AccountTypePersonal, Name: "管理员", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	target := model.User{Username: "target", AccountType: model.AccountTypePersonal, Name: "目标", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	foreign := model.User{Username: "foreign", AccountType: model.AccountTypePersonal, Name: "跨组织", OrganizationID: 2, Status: model.StatusActive, PasswordHash: "hash"}
	anchorSuper := model.User{Username: "anchor-super", AccountType: model.AccountTypePersonal, Name: "保底超管", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	if err := service.DB.Create(&[]*model.User{&actor, &target, &foreign, &anchorSuper}).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	if err := service.DB.Create(&[]model.UserRole{{UserID: actor.ID, RoleID: managerRole.ID}, {UserID: actor.ID, RoleID: allowedRole.ID}}).Error; err != nil {
		t.Fatalf("bind manager role: %v", err)
	}
	if err := service.DB.Create(&model.UserRole{UserID: anchorSuper.ID, RoleID: superRole.ID}).Error; err != nil {
		t.Fatalf("bind anchor super role: %v", err)
	}
	current := &auth.CurrentUser{ID: actor.ID, Username: actor.Username, OrganizationID: actor.OrganizationID}
	if err := service.ReplaceUserRolesForActor(current, target.ID, []uint{allowedRole.ID}); err != nil {
		t.Fatalf("assign subset role: %v", err)
	}
	if err := service.ReplaceUserRolesForActor(current, target.ID, []uint{forbiddenRole.ID}); !errors.Is(err, ErrManagerCannotGrant) {
		t.Fatalf("over-grant error = %v, want ErrManagerCannotGrant", err)
	}
	if err := service.ReplaceUserRolesForActor(current, actor.ID, nil); !errors.Is(err, ErrAssignmentSelfDenied) {
		t.Fatalf("self-assignment error = %v, want ErrAssignmentSelfDenied", err)
	}
	if err := service.ReplaceUserRolesForActor(current, foreign.ID, nil); !errors.Is(err, ErrAssignmentOrganizationDenied) {
		t.Fatalf("cross-org error = %v, want ErrAssignmentOrganizationDenied", err)
	}
	if err := service.ReplaceUserRolesForActor(current, target.ID, []uint{999999}); !errors.Is(err, ErrInvalidRoleID) {
		t.Fatalf("invalid role error = %v, want ErrInvalidRoleID", err)
	}
}

func TestAuthorizedAssignmentsProtectSystemRoleAndLastSuperAdmin(t *testing.T) {
	service := newAssignmentTestService(t)
	permissions := []model.Permission{
		{Name: "用户维护", Code: "system:users:write", Object: "/api/v1/system/users", Action: "write"},
		{Name: "角色维护", Code: "system:roles:write", Object: "/api/v1/system/roles", Action: "write"},
	}
	if err := service.DB.Create(&permissions).Error; err != nil {
		t.Fatalf("create permissions: %v", err)
	}
	superRole := model.Role{Name: "超级管理员", Code: SuperAdminCode, System: true}
	managerRole := model.Role{Name: "全权限管理员", Code: "all_permissions_manager"}
	if err := service.DB.Create(&[]*model.Role{&superRole, &managerRole}).Error; err != nil {
		t.Fatalf("create roles: %v", err)
	}
	for _, permission := range permissions {
		for _, roleID := range []uint{superRole.ID, managerRole.ID} {
			if err := service.DB.Create(&model.RolePermission{RoleID: roleID, PermissionID: permission.ID}).Error; err != nil {
				t.Fatalf("bind permission: %v", err)
			}
		}
	}
	manager := model.User{Username: "manager", AccountType: model.AccountTypePersonal, Name: "管理员", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	lastSuper := model.User{Username: "last-super", AccountType: model.AccountTypePersonal, Name: "最后超管", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	if err := service.DB.Create(&[]*model.User{&manager, &lastSuper}).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	if err := service.DB.Create(&[]model.UserRole{{UserID: manager.ID, RoleID: managerRole.ID}, {UserID: lastSuper.ID, RoleID: superRole.ID}}).Error; err != nil {
		t.Fatalf("bind roles: %v", err)
	}
	current := &auth.CurrentUser{ID: manager.ID, Username: manager.Username, OrganizationID: manager.OrganizationID}
	assignTarget := model.User{Username: "assign-target", AccountType: model.AccountTypePersonal, Name: "授权目标", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	if err := service.DB.Create(&assignTarget).Error; err != nil {
		t.Fatalf("create assignment target: %v", err)
	}
	if err := service.ReplaceUserRolesForActor(current, assignTarget.ID, []uint{superRole.ID}); !errors.Is(err, ErrManagerCannotGrant) {
		t.Fatalf("unowned super role grant error = %v, want ErrManagerCannotGrant", err)
	}
	if err := service.ReplaceUserRolesForActor(current, lastSuper.ID, nil); !errors.Is(err, ErrLastSuperAdmin) {
		t.Fatalf("last super admin removal error = %v, want ErrLastSuperAdmin", err)
	}
	if err := service.ReplaceRolePermissionsForActor(current, superRole.ID, []uint{permissions[0].ID}); !errors.Is(err, ErrSystemRoleLocked) {
		t.Fatalf("super role modification error = %v, want ErrSystemRoleLocked", err)
	}
}

func TestActualSuperAdminCanGrantAnyRole(t *testing.T) {
	service := newAssignmentTestService(t)
	permission := model.Permission{Name: "用户维护", Code: "system:users:write", Object: "/api/v1/system/users", Action: "write"}
	superRole := model.Role{Name: "超级管理员", Code: SuperAdminCode, System: true}
	customRole := model.Role{Name: "业务角色", Code: "business_role"}
	if err := service.DB.Create(&permission).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.DB.Create(&[]*model.Role{&superRole, &customRole}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.DB.Create(&model.RolePermission{RoleID: superRole.ID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatal(err)
	}
	actor := model.User{Username: "actual-super", AccountType: model.AccountTypePersonal, Name: "超级管理员", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	target := model.User{Username: "super-target", AccountType: model.AccountTypePersonal, Name: "目标", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	if err := service.DB.Create(&[]*model.User{&actor, &target}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.DB.Create(&model.UserRole{UserID: actor.ID, RoleID: superRole.ID}).Error; err != nil {
		t.Fatal(err)
	}
	current := &auth.CurrentUser{ID: actor.ID, Username: actor.Username, OrganizationID: actor.OrganizationID}
	if err := service.ReplaceUserRolesForActor(current, target.ID, []uint{customRole.ID}); err != nil {
		t.Fatalf("actual super admin grant custom role: %v", err)
	}
}

func TestRolePermissionUpdateRejectsRoleSharedWithAnotherOrganization(t *testing.T) {
	service := newAssignmentTestService(t)
	permission := model.Permission{Name: "角色维护", Code: "system:roles:write", Object: "/api/v1/system/roles", Action: "write"}
	managerRole := model.Role{Name: "角色管理员", Code: "role_manager"}
	sharedRole := model.Role{Name: "共享角色", Code: "shared_role"}
	if err := service.DB.Create(&permission).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.DB.Create(&[]*model.Role{&managerRole, &sharedRole}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.DB.Create(&model.RolePermission{RoleID: managerRole.ID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatal(err)
	}
	actor := model.User{Username: "role-manager", AccountType: model.AccountTypePersonal, Name: "角色管理员", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	foreign := model.User{Username: "foreign-role-user", AccountType: model.AccountTypePersonal, Name: "外部组织账号", OrganizationID: 2, Status: model.StatusActive, PasswordHash: "hash"}
	if err := service.DB.Create(&[]*model.User{&actor, &foreign}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.DB.Create(&[]model.UserRole{{UserID: actor.ID, RoleID: managerRole.ID}, {UserID: foreign.ID, RoleID: sharedRole.ID}}).Error; err != nil {
		t.Fatal(err)
	}
	current := &auth.CurrentUser{ID: actor.ID, Username: actor.Username, OrganizationID: actor.OrganizationID}
	if err := service.ReplaceRolePermissionsForActor(current, sharedRole.ID, []uint{permission.ID}); !errors.Is(err, ErrAssignmentOrganizationDenied) {
		t.Fatalf("cross-organization shared role update error=%v", err)
	}
}

func TestAuthorizedRolePermissionUpdateRejectsOverGrantAndPreservesBindings(t *testing.T) {
	service := newAssignmentTestService(t)
	permissions := []model.Permission{
		{Name: "角色维护", Code: "system:roles:write", Object: "/api/v1/system/roles", Action: "write"},
		{Name: "允许查看", Code: "authorized:test:read", Object: "/authorized/test", Action: "read"},
		{Name: "禁止维护", Code: "authorized:test:write", Object: "/authorized/test", Action: "write"},
	}
	if err := service.DB.Create(&permissions).Error; err != nil {
		t.Fatalf("create permissions: %v", err)
	}
	managerRole := model.Role{Name: "角色管理员", Code: "authorized_role_manager"}
	targetRole := model.Role{Name: "业务角色", Code: "authorized_target_role"}
	if err := service.DB.Create(&[]*model.Role{&managerRole, &targetRole}).Error; err != nil {
		t.Fatalf("create roles: %v", err)
	}
	if err := service.DB.Create(&[]model.RolePermission{
		{RoleID: managerRole.ID, PermissionID: permissions[0].ID},
		{RoleID: managerRole.ID, PermissionID: permissions[1].ID},
		{RoleID: targetRole.ID, PermissionID: permissions[1].ID},
	}).Error; err != nil {
		t.Fatalf("create role permissions: %v", err)
	}
	actor := model.User{Username: "authorized-role-manager", AccountType: model.AccountTypePersonal, Name: "角色管理员", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	if err := service.DB.Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if err := service.DB.Create(&model.UserRole{UserID: actor.ID, RoleID: managerRole.ID}).Error; err != nil {
		t.Fatalf("bind actor role: %v", err)
	}
	current := &auth.CurrentUser{ID: actor.ID, Username: actor.Username, OrganizationID: actor.OrganizationID}

	err := service.ReplaceRolePermissionsForActor(current, targetRole.ID, []uint{permissions[2].ID})
	if !errors.Is(err, ErrManagerCannotGrant) {
		t.Fatalf("over-grant error = %v, want ErrManagerCannotGrant", err)
	}
	var bindings []model.RolePermission
	if err := service.DB.Where("role_id = ?", targetRole.ID).Order("permission_id").Find(&bindings).Error; err != nil {
		t.Fatalf("load target role bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].PermissionID != permissions[1].ID {
		t.Fatalf("target role bindings changed after over-grant rejection: %+v", bindings)
	}
}

func TestAuthorizedRolePermissionUpdateRejectsInvalidAndDuplicateIDsAtomically(t *testing.T) {
	service := newAssignmentTestService(t)
	permissions := []model.Permission{
		{Name: "角色维护", Code: "system:roles:write", Object: "/api/v1/system/roles", Action: "write"},
		{Name: "允许查看", Code: "atomic:test:read", Object: "/atomic/test", Action: "read"},
		{Name: "允许写入", Code: "atomic:test:write", Object: "/atomic/test", Action: "write"},
	}
	if err := service.DB.Create(&permissions).Error; err != nil {
		t.Fatalf("create permissions: %v", err)
	}
	managerRole := model.Role{Name: "角色管理员", Code: "atomic_role_manager"}
	targetRole := model.Role{Name: "业务角色", Code: "atomic_target_role"}
	if err := service.DB.Create(&[]*model.Role{&managerRole, &targetRole}).Error; err != nil {
		t.Fatalf("create roles: %v", err)
	}
	if err := service.DB.Create(&[]model.RolePermission{
		{RoleID: managerRole.ID, PermissionID: permissions[0].ID},
		{RoleID: managerRole.ID, PermissionID: permissions[1].ID},
		{RoleID: managerRole.ID, PermissionID: permissions[2].ID},
		{RoleID: targetRole.ID, PermissionID: permissions[1].ID},
	}).Error; err != nil {
		t.Fatalf("create role permissions: %v", err)
	}
	actor := model.User{Username: "atomic-role-manager", AccountType: model.AccountTypePersonal, Name: "角色管理员", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	if err := service.DB.Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if err := service.DB.Create(&model.UserRole{UserID: actor.ID, RoleID: managerRole.ID}).Error; err != nil {
		t.Fatalf("bind actor role: %v", err)
	}
	current := &auth.CurrentUser{ID: actor.ID, Username: actor.Username, OrganizationID: actor.OrganizationID}

	err := service.ReplaceRolePermissionsForActor(current, targetRole.ID, []uint{permissions[2].ID, 999999})
	if !errors.Is(err, ErrInvalidPermissionID) {
		t.Fatalf("invalid permission error = %v, want ErrInvalidPermissionID", err)
	}
	assertRolePermissionIDs(t, service, targetRole.ID, []uint{permissions[1].ID})

	err = service.ReplaceRolePermissionsForActor(current, targetRole.ID, []uint{permissions[1].ID, permissions[1].ID})
	if !errors.Is(err, ErrDuplicateAssignmentID) {
		t.Fatalf("duplicate permission error = %v, want ErrDuplicateAssignmentID", err)
	}
	assertRolePermissionIDs(t, service, targetRole.ID, []uint{permissions[1].ID})
}

func assertRolePermissionIDs(t *testing.T, service *Service, roleID uint, want []uint) {
	t.Helper()
	got, err := service.RolePermissionIDs([]uint{roleID})
	if err != nil {
		t.Fatalf("load role permissions: %v", err)
	}
	if len(got[roleID]) != len(want) {
		t.Fatalf("role %d permission IDs = %v, want %v", roleID, got[roleID], want)
	}
	for index := range want {
		if got[roleID][index] != want[index] {
			t.Fatalf("role %d permission IDs = %v, want %v", roleID, got[roleID], want)
		}
	}
}

func TestConcurrentSuperAdminDisableKeepsOneActive(t *testing.T) {
	service := newAssignmentTestService(t)
	permission := model.Permission{Name: "用户维护", Code: "system:users:write", Object: "/api/v1/system/users", Action: "write"}
	managerRole := model.Role{Name: "用户管理员", Code: "status_manager"}
	superRole := model.Role{Name: "超级管理员", Code: SuperAdminCode, System: true}
	if err := service.DB.Create(&permission).Error; err != nil {
		t.Fatalf("create permission: %v", err)
	}
	if err := service.DB.Create(&[]*model.Role{&managerRole, &superRole}).Error; err != nil {
		t.Fatalf("create roles: %v", err)
	}
	if err := service.DB.Create(&model.RolePermission{RoleID: managerRole.ID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatalf("bind manager permission: %v", err)
	}
	manager := model.User{Username: "status-manager", AccountType: model.AccountTypePersonal, Name: "状态管理员", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	first := model.User{Username: "first-super", AccountType: model.AccountTypePersonal, Name: "第一超管", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	second := model.User{Username: "second-super", AccountType: model.AccountTypePersonal, Name: "第二超管", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	if err := service.DB.Create(&[]*model.User{&manager, &first, &second}).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	if err := service.DB.Create(&[]model.UserRole{{UserID: manager.ID, RoleID: managerRole.ID}, {UserID: first.ID, RoleID: superRole.ID}, {UserID: second.ID, RoleID: superRole.ID}}).Error; err != nil {
		t.Fatalf("bind roles: %v", err)
	}
	current := &auth.CurrentUser{ID: manager.ID, Username: manager.Username, OrganizationID: manager.OrganizationID}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, userID := range []uint{first.ID, second.ID} {
		wait.Add(1)
		go func(id uint) {
			defer wait.Done()
			results <- service.UpdateUserStatusForActor(current, id, model.StatusDisabled)
		}(userID)
	}
	wait.Wait()
	close(results)
	succeeded := 0
	protected := 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrLastSuperAdmin):
			protected++
		default:
			t.Fatalf("unexpected concurrent disable error: %v", err)
		}
	}
	if succeeded != 1 || protected != 1 {
		t.Fatalf("concurrent results succeeded=%d protected=%d", succeeded, protected)
	}
	var activeSuper int64
	if err := service.DB.Table("user_roles").
		Joins("JOIN users ON users.id = user_roles.user_id").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("users.status = ? AND roles.code = ?", model.StatusActive, SuperAdminCode).
		Count(&activeSuper).Error; err != nil || activeSuper != 1 {
		t.Fatalf("active super admins=%d err=%v", activeSuper, err)
	}
}
