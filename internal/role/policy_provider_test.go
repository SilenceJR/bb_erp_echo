package role

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"bb_erp_echo/internal/model"

	"github.com/casbin/casbin/v2"
)

func addPolicyProviderFixture(t *testing.T, service *Service) {
	t.Helper()
	policyRole := model.Role{Name: "并发策略角色", Code: "concurrent_policy_role"}
	policyPermission := model.Permission{
		Name:   "并发接口读取",
		Code:   "concurrent:read",
		Object: "/api/v1/workorder",
		Action: "read",
	}
	policyUser := model.User{
		Username:        "concurrent-policy-user",
		AccountType:     model.AccountTypePersonal,
		Name:            "并发策略用户",
		OrganizationID:  1,
		Status:          model.StatusActive,
		PasswordHash:    "test-password-hash",
		PasswordVersion: 1,
	}
	if err := service.DB.Create(&policyRole).Error; err != nil {
		t.Fatalf("create policy role: %v", err)
	}
	if err := service.DB.Create(&policyPermission).Error; err != nil {
		t.Fatalf("create policy permission: %v", err)
	}
	if err := service.DB.Create(&policyUser).Error; err != nil {
		t.Fatalf("create policy user: %v", err)
	}
	if err := service.DB.Create(&model.UserRole{UserID: policyUser.ID, RoleID: policyRole.ID}).Error; err != nil {
		t.Fatalf("create user role: %v", err)
	}
	if err := service.DB.Create(&model.RolePermission{RoleID: policyRole.ID, PermissionID: policyPermission.ID}).Error; err != nil {
		t.Fatalf("create role permission: %v", err)
	}
}

func TestPolicyProviderEnforceAndReloadAreRaceFree(t *testing.T) {
	service := newAssignmentTestService(t)
	addPolicyProviderFixture(t, service)
	if err := service.ReloadPolicies(); err != nil {
		t.Fatalf("initial policy reload: %v", err)
	}
	provider, ok := service.Authorizer.(*PolicyProvider)
	if !ok {
		t.Fatalf("service authorizer type = %T, want *PolicyProvider", service.Authorizer)
	}

	const (
		workers       = 12
		enforceRounds = 500
		reloadRounds  = 80
	)
	failures := make(chan error, 1)
	reportFailure := func(err error) {
		select {
		case failures <- err:
		default:
		}
	}
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for round := 0; round < enforceRounds; round++ {
				allowed, err := provider.Enforce("concurrent-policy-user", "/api/v1/workorder", "read", "1", "*")
				if err != nil {
					reportFailure(fmt.Errorf("enforce: %w", err))
					return
				}
				if !allowed {
					reportFailure(errors.New("enforce unexpectedly denied during reload"))
					return
				}
			}
		}()
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		for round := 0; round < reloadRounds; round++ {
			if err := provider.ReloadPolicies(); err != nil {
				reportFailure(fmt.Errorf("reload: %w", err))
				return
			}
		}
	}()
	wait.Wait()

	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}
}

func TestPolicyProviderReloadFailureKeepsPreviousSnapshot(t *testing.T) {
	service := newAssignmentTestService(t)
	addPolicyProviderFixture(t, service)
	if err := service.ReloadPolicies(); err != nil {
		t.Fatalf("initial policy reload: %v", err)
	}
	provider, ok := service.Authorizer.(*PolicyProvider)
	if !ok {
		t.Fatalf("service authorizer type = %T, want *PolicyProvider", service.Authorizer)
	}
	allowed, err := provider.Enforce("concurrent-policy-user", "/api/v1/workorder", "read", "1", "*")
	if err != nil || !allowed {
		t.Fatalf("initial policy allowed=%v err=%v", allowed, err)
	}

	previousFactory := provider.buildEnforcer
	provider.buildEnforcer = func() (*casbin.Enforcer, error) {
		return nil, errors.New("injected policy snapshot build failure")
	}
	err = provider.ReloadPolicies()
	provider.buildEnforcer = previousFactory
	if err == nil || !strings.Contains(err.Error(), "injected policy snapshot build failure") {
		t.Fatalf("reload error = %v, want injected build failure", err)
	}
	allowed, err = provider.Enforce("concurrent-policy-user", "/api/v1/workorder", "read", "1", "*")
	if err != nil || !allowed {
		t.Fatalf("old policy snapshot allowed=%v err=%v after failed reload", allowed, err)
	}
}
