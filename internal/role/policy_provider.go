package role

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	"gorm.io/gorm"
)

// Authorizer 是业务模块使用的权限判断和策略刷新边界。
//
// 业务模块不应直接持有或修改 Casbin 引擎；PolicyProvider 会先构建完整
// 的不可变策略快照，再一次性替换当前快照，保证并发请求不会读到半成品。
type Authorizer interface {
	Enforce(subject, object, action, organization, department string) (bool, error)
	ReloadPolicies() error
}

// PolicyProvider 从数据库加载 Casbin 策略，并以原子指针发布完整快照。
//
// 当前快照发布后不再修改。ReloadPolicies 的所有数据库查询和策略写入都
// 发生在临时引擎上，只有全部成功才会通过 atomic.Pointer 一次性切换，
// 因此请求线程始终只会看到旧快照或新快照。
type PolicyProvider struct {
	db *gorm.DB

	current  atomic.Pointer[casbin.Enforcer]
	reloadMu sync.Mutex

	// buildEnforcer 仅用于隔离模型构建，并让 role 包测试能够验证构建失败
	// 时旧快照仍然可用；生产实例使用 newEnforcer，不对外暴露底层引擎。
	buildEnforcer func() (*casbin.Enforcer, error)
}

var _ Authorizer = (*PolicyProvider)(nil)

// NewPolicyProvider 创建统一权限快照 provider。
//
// provider 初始包含空策略快照；应用完成基础数据初始化后应调用
// ReloadPolicies 发布数据库中的完整策略。
func NewPolicyProvider(db *gorm.DB) (*PolicyProvider, error) {
	if db == nil {
		return nil, errors.New("policy provider database is nil")
	}
	provider := &PolicyProvider{
		db:            db,
		buildEnforcer: newEnforcer,
	}
	initial, err := provider.buildEnforcer()
	if err != nil {
		return nil, err
	}
	provider.current.Store(initial)
	return provider, nil
}

// newEnforcer 创建一个尚未加载业务策略的临时 Casbin 引擎。
//
// 权限模型说明：
// - sub：用户或角色。
// - obj：接口资源路径。
// - act：动作，当前约定为 read/write。
// - org/dept：组织和部门数据范围，* 表示不限制。
func newEnforcer() (*casbin.Enforcer, error) {
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

// Enforce 在当前完整策略快照上执行权限判断。
func (p *PolicyProvider) Enforce(subject, object, action, organization, department string) (bool, error) {
	if p == nil {
		return false, errors.New("policy provider is nil")
	}
	current := p.current.Load()
	if current == nil {
		return false, errors.New("policy provider has no policy snapshot")
	}
	return current.Enforce(subject, object, action, organization, department)
}

// ReloadPolicies 在临时 Casbin 引擎上构建数据库完整快照，并原子发布。
//
// 数据查询、临时引擎构建或任一策略写入失败时，当前快照保持不变，已有
// 请求不会因为一次刷新失败而全部失去权限。
func (p *PolicyProvider) ReloadPolicies() error {
	if p == nil || p.db == nil {
		return errors.New("policy provider database is nil")
	}
	p.reloadMu.Lock()
	defer p.reloadMu.Unlock()
	if p.buildEnforcer == nil {
		return errors.New("policy provider builder is nil")
	}

	var policies []struct {
		Username string
		RoleCode string
	}
	var permissions []struct {
		RoleCode string
		Object   string
		Action   string
	}
	// 在同一个只读事务中读取两组策略，避免刷新过程中观察到关联表的
	// 中间状态。事务提交后才开始构建临时引擎。
	if err := p.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("user_roles").
			Select("users.username, roles.code AS role_code").
			Joins("JOIN users ON users.id = user_roles.user_id").
			Joins("JOIN roles ON roles.id = user_roles.role_id").
			Scan(&policies).Error; err != nil {
			return err
		}
		return tx.Table("role_permissions").
			Select("roles.code AS role_code, permissions.object, permissions.action").
			Joins("JOIN roles ON roles.id = role_permissions.role_id").
			Joins("JOIN permissions ON permissions.id = role_permissions.permission_id").
			Scan(&permissions).Error
	}); err != nil {
		return err
	}

	next, err := p.buildEnforcer()
	if err != nil {
		return fmt.Errorf("build policy snapshot: %w", err)
	}
	if next == nil {
		return errors.New("build policy snapshot: builder returned nil enforcer")
	}
	for _, policy := range policies {
		if _, err := next.AddGroupingPolicy(policy.Username, policy.RoleCode); err != nil {
			return fmt.Errorf("add grouping policy: %w", err)
		}
	}
	for _, permission := range permissions {
		if _, err := next.AddPolicy(permission.RoleCode, permission.Object, permission.Action, "*", "*"); err != nil {
			return fmt.Errorf("add permission policy: %w", err)
		}
	}
	// next 在此之前从未对外可见；发布只需一次原子指针替换。
	p.current.Store(next)
	return nil
}
