package role

import (
	"errors"
	"fmt"
	"testing"

	"bb_erp_echo/internal/config"
	"bb_erp_echo/internal/model"

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
	enforcer, err := NewEnforcer()
	if err != nil {
		t.Fatalf("create enforcer: %v", err)
	}
	return NewService(db, enforcer)
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

	err := service.ReplaceUserRoles(99, []uint{superAdmin.ID}, false)
	if !errors.Is(err, ErrSuperAdminNotAllowed) {
		t.Fatalf("expected ErrSuperAdminNotAllowed, got %v", err)
	}
	var count int64
	if err := service.DB.Model(&model.UserRole{}).Where("user_id = ?", 99).Count(&count).Error; err != nil {
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

func TestSeedSystemDataGivesTerminalOperatorWarehouseRead(t *testing.T) {
	service := newAssignmentTestService(t)
	cfg := &config.Config{Admin: config.AdminConfig{
		Username: "seed-admin",
		Password: "seed-password",
		Name:     "种子管理员",
	}}
	if err := service.SeedSystemData(cfg); err != nil {
		t.Fatalf("seed system data: %v", err)
	}

	var terminalRole model.Role
	if err := service.DB.Where("code = ?", TerminalOperatorCode).First(&terminalRole).Error; err != nil {
		t.Fatalf("load terminal operator role: %v", err)
	}
	var warehouseRead model.Permission
	if err := service.DB.Where("code = ?", "warehouse:read").First(&warehouseRead).Error; err != nil {
		t.Fatalf("load warehouse read permission: %v", err)
	}
	var binding model.RolePermission
	if err := service.DB.Where("role_id = ? AND permission_id = ?", terminalRole.ID, warehouseRead.ID).First(&binding).Error; err != nil {
		t.Fatalf("terminal operator should have warehouse read permission: %v", err)
	}
}
