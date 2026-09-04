package role

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"bb_erp_echo/internal/config"
	"bb_erp_echo/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAssignmentTestService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.Organization{},
		&model.Department{},
		&model.Terminal{},
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.UserRole{},
		&model.RolePermission{},
		&model.Warehouse{},
	); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	authorizer, err := NewPolicyProvider(db)
	if err != nil {
		t.Fatalf("create policy provider: %v", err)
	}
	return NewService(db, authorizer)
}

func TestAssignmentServiceReplacesAndBatchLoadsAssociations(t *testing.T) {
	service := newAssignmentTestService(t)
	roles := []model.Role{
		{Name: "测试角色一", Code: "test_role_1"},
		{Name: "测试角色二", Code: "test_role_2"},
	}
	permissions := []model.Permission{
		{Name: "测试查看", Code: "test:read", Object: "/test", Action: "read"},
		{Name: "测试维护", Code: "test:write", Object: "/test", Action: "write"},
	}
	if err := service.DB.Create(&roles).Error; err != nil {
		t.Fatalf("create roles: %v", err)
	}
	if err := service.DB.Create(&permissions).Error; err != nil {
		t.Fatalf("create permissions: %v", err)
	}

	if err := service.ReplaceRolePermissions(roles[0].ID, []uint{permissions[0].ID, permissions[1].ID}); err != nil {
		t.Fatalf("replace role permissions: %v", err)
	}
	if err := service.ReplaceRolePermissions(roles[0].ID, []uint{permissions[1].ID}); err != nil {
		t.Fatalf("replace role permissions again: %v", err)
	}

	idsByRole, err := service.RolePermissionIDs([]uint{roles[0].ID, roles[1].ID})
	if err != nil {
		t.Fatalf("load role permission ids: %v", err)
	}
	if got := idsByRole[roles[0].ID]; len(got) != 1 || got[0] != permissions[1].ID {
		t.Fatalf("unexpected first role permissions: %v", got)
	}
	if got := idsByRole[roles[1].ID]; len(got) != 0 {
		t.Fatalf("unassigned role should return an empty slice: %v", got)
	}
}

func TestAssignmentServiceRejectsSuperAdminForTerminalAccount(t *testing.T) {
	service := newAssignmentTestService(t)
	superAdmin := model.Role{Name: "测试超级管理员", Code: SuperAdminCode}
	if err := service.DB.Create(&superAdmin).Error; err != nil {
		t.Fatalf("create super admin role: %v", err)
	}

	terminal := model.User{Username: "terminal", AccountType: model.AccountTypeDepartmentTerminal, Name: "终端", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	if err := service.DB.Create(&terminal).Error; err != nil {
		t.Fatalf("create terminal account: %v", err)
	}
	err := service.ReplaceUserRoles(terminal.ID, []uint{superAdmin.ID}, false)
	if !errors.Is(err, ErrSuperAdminNotAllowed) {
		t.Fatalf("expected ErrSuperAdminNotAllowed, got %v", err)
	}
	var count int64
	if err := service.DB.Model(&model.UserRole{}).Where("user_id = ?", terminal.ID).Count(&count).Error; err != nil {
		t.Fatalf("count user roles: %v", err)
	}
	if count != 0 {
		t.Fatalf("rejected assignment should not write rows, count=%d", count)
	}
}

func TestBackfillUpdateReadPermission(t *testing.T) {
	service := newAssignmentTestService(t)
	role := model.Role{Name: "更新管理员", Code: "update_admin"}
	permissions := []model.Permission{
		{Name: "更新查看", Code: "system:updates:read", Object: "/api/v1/system/updates", Action: "read"},
		{Name: "更新维护", Code: "system:updates:write", Object: "/api/v1/system/updates", Action: "write"},
	}
	if err := service.DB.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := service.DB.Create(&permissions).Error; err != nil {
		t.Fatalf("create permissions: %v", err)
	}
	if err := service.DB.Create(&model.RolePermission{RoleID: role.ID, PermissionID: permissions[1].ID}).Error; err != nil {
		t.Fatalf("bind write permission: %v", err)
	}
	if err := service.backfillUpdateReadPermission(); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	var count int64
	if err := service.DB.Model(&model.RolePermission{}).
		Where("role_id = ? AND permission_id = ?", role.ID, permissions[0].ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count read binding: %v", err)
	}
	if count != 1 {
		t.Fatalf("read permission binding count = %d", count)
	}
}

func TestDefaultPermissionsIncludeTemporaryProductWrite(t *testing.T) {
	permissions := DefaultPermissions()
	for _, permission := range permissions {
		if permission.Code != TemporaryProductWriteCode {
			continue
		}
		if permission.Object != "/api/v1/workorder/products" || permission.Action != "write" {
			t.Fatalf("temporary product permission = %+v", permission)
		}
		return
	}
	t.Fatalf("default permissions do not include %q", TemporaryProductWriteCode)
}

func TestDefaultPermissionsUseNewCustomerMatrix(t *testing.T) {
	want := map[string]struct {
		object string
		action string
	}{
		"customers:read":   {object: "/api/v1/customers", action: "read"},
		"customers:write":  {object: "/api/v1/customers", action: "write"},
		"customers:import": {object: "/api/v1/customers/import", action: "import"},
	}
	for _, permission := range DefaultPermissions() {
		if strings.HasPrefix(permission.Code, "contacts:") {
			t.Fatalf("legacy contact permission remains: %+v", permission)
		}
		if expected, ok := want[permission.Code]; ok {
			if permission.Object != expected.object || permission.Action != expected.action {
				t.Fatalf("permission %s = %+v", permission.Code, permission)
			}
			delete(want, permission.Code)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing customer permissions: %+v", want)
	}
}

func TestDefaultPermissionsUseCanonicalMoldPermissions(t *testing.T) {
	want := map[string]struct {
		object string
		action string
	}{
		"mold:read":  {object: "/api/v1/molds", action: "read"},
		"mold:write": {object: "/api/v1/molds", action: "write"},
	}
	seen := make(map[string]int)
	for _, permission := range DefaultPermissions() {
		if strings.HasPrefix(permission.Code, "molds:") {
			t.Fatalf("duplicate plural mold permission remains: %+v", permission)
		}
		expected, ok := want[permission.Code]
		if !ok {
			continue
		}
		seen[permission.Code]++
		if permission.Object != expected.object || permission.Action != expected.action {
			t.Fatalf("permission %s = %+v, want object=%s action=%s", permission.Code, permission, expected.object, expected.action)
		}
	}
	for code := range want {
		if seen[code] != 1 {
			t.Fatalf("permission %s count = %d, want exactly one", code, seen[code])
		}
	}
}

func TestAttachPermissionCodesFailsClosedForUnknownCode(t *testing.T) {
	service := newAssignmentTestService(t)
	role := model.Role{Name: "权限绑定测试角色", Code: "permission_assignment_test"}
	if err := service.DB.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	permission := model.Permission{Name: "任务查看", Code: "workorder:read", Object: "/api/v1/workorder", Action: "read"}
	if err := service.DB.Create(&permission).Error; err != nil {
		t.Fatalf("create permission: %v", err)
	}

	err := service.AttachPermissionCodes(role.ID, []string{"workorder:read", "workorder:reed"})
	if !errors.Is(err, ErrPermissionCodeNotFound) {
		t.Fatalf("unknown permission code error = %v, want ErrPermissionCodeNotFound", err)
	}
	var count int64
	if err := service.DB.Model(&model.RolePermission{}).Where("role_id = ?", role.ID).Count(&count).Error; err != nil {
		t.Fatalf("count role permissions: %v", err)
	}
	if count != 0 {
		t.Fatalf("unknown permission code must not grant any permission, count=%d", count)
	}

	if err := service.AttachPermissionCodes(role.ID, nil); !errors.Is(err, ErrPermissionCodeNotFound) {
		t.Fatalf("empty permission code error = %v, want ErrPermissionCodeNotFound", err)
	}
}

func TestAttachPermissionsRequiresExplicitIDsAndAttachAllIsExplicit(t *testing.T) {
	service := newAssignmentTestService(t)
	role := model.Role{Name: "全量权限测试角色", Code: "attach_all_test"}
	if err := service.DB.Create(&role).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	permissions := []model.Permission{
		{Name: "全量查看一", Code: "attach-all:read", Object: "/attach-all/one", Action: "read"},
		{Name: "全量查看二", Code: "attach-all:write", Object: "/attach-all/two", Action: "write"},
	}
	if err := service.DB.Create(&permissions).Error; err != nil {
		t.Fatalf("create permissions: %v", err)
	}

	if err := service.AttachPermissions(role.ID, nil); !errors.Is(err, ErrNoPermissions) {
		t.Fatalf("empty permission IDs error = %v, want ErrNoPermissions", err)
	}
	if err := service.AttachAllPermissions(role.ID); err != nil {
		t.Fatalf("attach all permissions: %v", err)
	}
	var count int64
	if err := service.DB.Model(&model.RolePermission{}).Where("role_id = ?", role.ID).Count(&count).Error; err != nil {
		t.Fatalf("count attached permissions: %v", err)
	}
	if count != int64(len(permissions)) {
		t.Fatalf("attached permission count = %d, want %d", count, len(permissions))
	}
}

func TestSeedSystemDataCreatesAdminAndAdditionalSilenceSuperAdmin(t *testing.T) {
	service := newAssignmentTestService(t)
	cfg := &config.Config{Admin: config.AdminConfig{
		Username: "seed-admin",
		Password: "seed-password",
		Name:     "种子管理员",
	}, Silence: config.SilenceConfig{Password: "silence-password"}}
	if err := service.SeedSystemData(cfg); err != nil {
		t.Fatalf("seed system data: %v", err)
	}

	var users []model.User
	if err := service.DB.Order("username").Find(&users).Error; err != nil {
		t.Fatalf("load seeded users: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("seeded users = %d, want admin and Silence", len(users))
	}
	byName := map[string]model.User{}
	for _, item := range users {
		byName[item.Username] = item
	}
	if _, ok := byName[cfg.Admin.Username]; !ok {
		t.Fatalf("original admin was not preserved: %v", byName)
	}
	silence, ok := byName[SilenceUsername]
	if !ok {
		t.Fatalf("additional Silence account missing: %v", byName)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(silence.PasswordHash), []byte("silence-password")); err != nil {
		t.Fatalf("Silence password was not bcrypt hashed: %v", err)
	}
	var super model.Role
	if err := service.DB.Where("code = ?", SuperAdminCode).First(&super).Error; err != nil {
		t.Fatalf("load super admin role: %v", err)
	}
	if !super.System {
		t.Fatal("super_admin must be the locked system role")
	}
	var costPermission model.Permission
	if err := service.DB.Where("code = ?", CostViewCode).First(&costPermission).Error; err != nil {
		t.Fatalf("load cost permission: %v", err)
	}
	var costBindingCount int64
	if err := service.DB.Model(&model.RolePermission{}).Where("role_id = ? AND permission_id = ?", super.ID, costPermission.ID).Count(&costBindingCount).Error; err != nil || costBindingCount != 1 {
		t.Fatalf("super admin cost permission binding count=%d err=%v", costBindingCount, err)
	}
	for _, item := range []model.User{byName[cfg.Admin.Username], silence} {
		var count int64
		if err := service.DB.Model(&model.UserRole{}).Where("user_id = ? AND role_id = ?", item.ID, super.ID).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("user %s super_admin binding count=%d err=%v", item.Username, count, err)
		}
	}
	var legacyRoleCount int64
	if err := service.DB.Model(&model.Role{}).Where("code IN ?", []string{BossCode, TerminalOperatorCode}).Count(&legacyRoleCount).Error; err != nil {
		t.Fatalf("count legacy roles: %v", err)
	}
	if legacyRoleCount != 0 {
		t.Fatalf("fresh database created %d legacy roles", legacyRoleCount)
	}
	var warehouseCount int64
	if err := service.DB.Model(&model.Warehouse{}).Count(&warehouseCount).Error; err != nil {
		t.Fatalf("count warehouses: %v", err)
	}
	if warehouseCount != 0 {
		t.Fatalf("fresh database seeded deferred warehouse rows: %d", warehouseCount)
	}
}

func TestSeedSystemDataRequiresConfiguredSilencePasswordForFreshDatabase(t *testing.T) {
	service := newAssignmentTestService(t)
	cfg := &config.Config{Admin: config.AdminConfig{
		Username: "seed-admin",
		Password: "seed-password",
		Name:     "种子管理员",
	}}

	err := service.SeedSystemData(cfg)
	if err == nil || !strings.Contains(err.Error(), "BB_ERP_SILENCE_PASSWORD") {
		t.Fatalf("missing Silence password error = %v", err)
	}
	var userCount int64
	if countErr := service.DB.Model(&model.User{}).Count(&userCount).Error; countErr != nil || userCount != 0 {
		t.Fatalf("fresh database received partial initialization users: count=%d err=%v", userCount, countErr)
	}
	for name, item := range map[string]any{"organizations": &model.Organization{}, "roles": &model.Role{}, "permissions": &model.Permission{}} {
		var count int64
		if countErr := service.DB.Model(item).Count(&count).Error; countErr != nil || count != 0 {
			t.Fatalf("fresh database received partial %s seed data: count=%d err=%v", name, count, countErr)
		}
	}
}

func TestSeedSystemDataDoesNotAddSilenceOrResetAdminInExistingDatabase(t *testing.T) {
	service := newAssignmentTestService(t)
	existingHash := "existing-password-hash"
	admin := model.User{Username: "admin", AccountType: model.AccountTypePersonal, Name: "已有管理员", OrganizationID: 1, Status: model.StatusActive, PasswordHash: existingHash, PasswordVersion: 7}
	if err := service.DB.Create(&admin).Error; err != nil {
		t.Fatalf("create existing admin: %v", err)
	}
	legacyRoles := []model.Role{{Name: "老板", Code: BossCode, System: true}, {Name: "终端操作员", Code: TerminalOperatorCode, System: true}}
	if err := service.DB.Create(&legacyRoles).Error; err != nil {
		t.Fatalf("create legacy roles: %v", err)
	}
	if err := service.DB.Create(&model.UserRole{UserID: admin.ID, RoleID: legacyRoles[0].ID}).Error; err != nil {
		t.Fatalf("bind legacy boss role: %v", err)
	}
	cfg := &config.Config{Admin: config.AdminConfig{Username: "replacement", Password: "replacement-password", Name: "不应创建"}}
	if err := service.SeedSystemData(cfg); err != nil {
		t.Fatalf("seed existing database: %v", err)
	}
	var reloaded model.User
	if err := service.DB.First(&reloaded, admin.ID).Error; err != nil {
		t.Fatalf("reload existing admin: %v", err)
	}
	if reloaded.PasswordHash != existingHash || reloaded.PasswordVersion != 7 || reloaded.Name != admin.Name {
		t.Fatalf("existing admin was changed: %+v", reloaded)
	}
	var silenceCount int64
	if err := service.DB.Model(&model.User{}).Where("username = ?", SilenceUsername).Count(&silenceCount).Error; err != nil {
		t.Fatalf("count Silence users: %v", err)
	}
	if silenceCount != 0 {
		t.Fatalf("existing database received %d Silence accounts", silenceCount)
	}
	var replacement model.User
	if err := service.DB.Where("username = ?", cfg.Admin.Username).First(&replacement).Error; err != nil {
		t.Fatalf("configured admin was not created by preserved initialization behavior: %v", err)
	}
	var replacementSuper int64
	if err := service.DB.Table("user_roles").Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.code = ?", replacement.ID, SuperAdminCode).Count(&replacementSuper).Error; err != nil || replacementSuper != 1 {
		t.Fatalf("configured admin super role count=%d err=%v", replacementSuper, err)
	}
	var unlocked []model.Role
	if err := service.DB.Where("code IN ?", []string{BossCode, TerminalOperatorCode}).Order("code").Find(&unlocked).Error; err != nil {
		t.Fatalf("load upgraded legacy roles: %v", err)
	}
	if len(unlocked) != 2 || unlocked[0].System || unlocked[1].System {
		t.Fatalf("legacy roles were not converted to custom roles: %+v", unlocked)
	}
	var preserved int64
	if err := service.DB.Model(&model.UserRole{}).Where("user_id = ? AND role_id = ?", admin.ID, legacyRoles[0].ID).Count(&preserved).Error; err != nil || preserved != 1 {
		t.Fatalf("legacy assignment count=%d err=%v", preserved, err)
	}
}

func TestSeedSystemDataDoesNotTreatSoftDeletedUsersAsFresh(t *testing.T) {
	service := newAssignmentTestService(t)
	historical := model.User{Username: "historical", AccountType: model.AccountTypePersonal, Name: "历史账号", OrganizationID: 1, Status: model.StatusActive, PasswordHash: "hash"}
	if err := service.DB.Create(&historical).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.DB.Delete(&historical).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Admin: config.AdminConfig{Username: "admin", Password: "admin-password", Name: "管理员"}}
	if err := service.SeedSystemData(cfg); err != nil {
		t.Fatalf("existing database should not require Silence configuration: %v", err)
	}
	var silenceCount int64
	if err := service.DB.Unscoped().Model(&model.User{}).Where("username = ?", SilenceUsername).Count(&silenceCount).Error; err != nil || silenceCount != 0 {
		t.Fatalf("Silence count=%d err=%v", silenceCount, err)
	}
}
