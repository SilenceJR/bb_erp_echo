package notification

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"bb_erp_echo/internal/auth"
	"bb_erp_echo/internal/model"

	"github.com/labstack/echo/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type notificationFixtures struct {
	db        *gorm.DB
	service   *Service
	actor     model.User
	reader    model.User
	writer    model.User
	disabled  model.User
	foreign   model.User
	readRole  model.Role
	writeRole model.Role
}

func newNotificationFixtures(t *testing.T) notificationFixtures {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, err := db.DB(); err != nil {
		t.Fatalf("get sql db: %v", err)
	} else {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(
		&model.Organization{}, &model.Department{}, &model.Terminal{}, &model.User{},
		&model.Role{}, &model.Permission{}, &model.UserRole{}, &model.RolePermission{},
	); err != nil {
		t.Fatalf("migrate notification fixtures: %v", err)
	}
	org := model.Organization{Name: "通知组织", Code: "NOTIFY-ORG", Status: model.StatusActive}
	otherOrg := model.Organization{Name: "其他组织", Code: "NOTIFY-OTHER", Status: model.StatusActive}
	if err := db.Create(&[]*model.Organization{&org, &otherOrg}).Error; err != nil {
		t.Fatalf("create organizations: %v", err)
	}
	dept := model.Department{OrganizationID: org.ID, Name: "通知部门", Code: "NOTIFY-DEPT", Status: model.StatusActive}
	otherDept := model.Department{OrganizationID: otherOrg.ID, Name: "其他部门", Code: "OTHER-DEPT", Status: model.StatusActive}
	if err := db.Create(&[]*model.Department{&dept, &otherDept}).Error; err != nil {
		t.Fatalf("create departments: %v", err)
	}
	terminal := model.Terminal{DepartmentID: dept.ID, Code: "NOTIFY-TERMINAL", Name: "通知终端", Status: model.StatusActive}
	if err := db.Create(&terminal).Error; err != nil {
		t.Fatalf("create terminal: %v", err)
	}
	passwordHash := "bcrypt-hash-placeholder"
	actor := model.User{Username: "notify-actor", Name: "操作者", OrganizationID: org.ID, DepartmentID: &dept.ID, TerminalID: &terminal.ID, Status: model.StatusActive, PasswordHash: passwordHash, PasswordVersion: 1}
	reader := model.User{Username: "notify-reader", Name: "查看者", OrganizationID: org.ID, DepartmentID: &dept.ID, TerminalID: &terminal.ID, Status: model.StatusActive, PasswordHash: passwordHash, PasswordVersion: 1}
	writer := model.User{Username: "notify-writer", Name: "维护者", OrganizationID: org.ID, Status: model.StatusActive, PasswordHash: passwordHash, PasswordVersion: 1}
	disabled := model.User{Username: "notify-disabled", Name: "停用者", OrganizationID: org.ID, Status: model.StatusDisabled, PasswordHash: passwordHash, PasswordVersion: 1}
	foreign := model.User{Username: "notify-foreign", Name: "外组织", OrganizationID: otherOrg.ID, DepartmentID: &otherDept.ID, Status: model.StatusActive, PasswordHash: passwordHash, PasswordVersion: 1}
	if err := db.Create(&[]*model.User{&actor, &reader, &writer, &disabled, &foreign}).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	readRole := model.Role{Name: "通知查看角色", Code: "notify-reader"}
	writeRole := model.Role{Name: "通知维护角色", Code: "notify-writer"}
	if err := db.Create(&[]*model.Role{&readRole, &writeRole}).Error; err != nil {
		t.Fatalf("create roles: %v", err)
	}
	permission := model.Permission{Name: "供应商查看", Code: "suppliers:read", Object: "/api/v1/suppliers", Action: "read"}
	writePermission := model.Permission{Name: "供应商维护", Code: "suppliers:write", Object: "/api/v1/suppliers", Action: "write"}
	if err := db.Create(&[]*model.Permission{&permission, &writePermission}).Error; err != nil {
		t.Fatalf("create permissions: %v", err)
	}
	if err := db.Create(&[]model.UserRole{{UserID: reader.ID, RoleID: readRole.ID}, {UserID: writer.ID, RoleID: writeRole.ID}, {UserID: disabled.ID, RoleID: readRole.ID}, {UserID: foreign.ID, RoleID: readRole.ID}}).Error; err != nil {
		t.Fatalf("create user roles: %v", err)
	}
	if err := db.Create(&[]model.RolePermission{{RoleID: readRole.ID, PermissionID: permission.ID}, {RoleID: writeRole.ID, PermissionID: writePermission.ID}}).Error; err != nil {
		t.Fatalf("create role permissions: %v", err)
	}
	service := NewService(db, Config{MergeWindow: 5 * time.Millisecond, HeartbeatInterval: time.Hour, QueueSize: 4}, nil)
	return notificationFixtures{db: db, service: service, actor: actor, reader: reader, writer: writer, disabled: disabled, foreign: foreign, readRole: readRole, writeRole: writeRole}
}

func TestServicePublishesOnlyCurrentReadersAndExcludesActor(t *testing.T) {
	fixture := newNotificationFixtures(t)
	readerEvents, cancelReader, err := fixture.service.Subscribe(fixture.reader.ID)
	if err != nil {
		t.Fatalf("subscribe reader: %v", err)
	}
	defer cancelReader()
	actorEvents, cancelActor, err := fixture.service.Subscribe(fixture.actor.ID)
	if err != nil {
		t.Fatalf("subscribe actor: %v", err)
	}
	defer cancelActor()
	actorSecondSession, cancelActorSecond, err := fixture.service.Subscribe(fixture.actor.ID)
	if err != nil {
		t.Fatalf("subscribe second actor session: %v", err)
	}
	defer cancelActorSecond()
	writerEvents, cancelWriter, err := fixture.service.Subscribe(fixture.writer.ID)
	if err != nil {
		t.Fatalf("subscribe writer: %v", err)
	}
	defer cancelWriter()
	disabledEvents, cancelDisabled, err := fixture.service.Subscribe(fixture.disabled.ID)
	if err != nil {
		t.Fatalf("subscribe disabled: %v", err)
	}
	defer cancelDisabled()
	foreignEvents, cancelForeign, err := fixture.service.Subscribe(fixture.foreign.ID)
	if err != nil {
		t.Fatalf("subscribe foreign: %v", err)
	}
	defer cancelForeign()

	change := Change{
		Module:          "suppliers",
		ModuleTitle:     "供应商",
		EntityType:      "supplier",
		OrganizationID:  fixture.actor.OrganizationID,
		ActorUserID:     fixture.actor.ID,
		Operation:       OperationUpdate,
		Count:           1,
		Items:           []ChangeItem{{EntityType: "supplier", EntityID: 9, Operation: OperationUpdate, Label: "SUP-009", Fields: map[string]string{"name": "可见供应商", "phone": "禁止字段"}}},
		ReadPermissions: []PermissionRequirement{{Object: "/api/v1/suppliers", Action: "read"}},
	}
	if err := fixture.service.Publish(change); err != nil {
		t.Fatalf("publish change: %v", err)
	}

	event := receiveNotification(t, readerEvents)
	if event.Module != "suppliers" || event.Count != 1 || event.Action.Type != ActionOpenEntity || event.Action.EntityID != 9 {
		t.Fatalf("reader event = %+v", event)
	}
	if event.Items[0].Fields["phone"] != "" {
		t.Fatalf("unsafe field leaked into notification: %+v", event.Items[0].Fields)
	}
	assertNoNotification(t, actorEvents)
	assertNoNotification(t, actorSecondSession)
	assertNoNotification(t, writerEvents)
	assertNoNotification(t, disabledEvents)
	assertNoNotification(t, foreignEvents)

	// 权限在连接建立后撤销，下一次投递必须基于当前数据库权限重新判断。
	if err := fixture.db.Where("role_id = ?", fixture.readRole.ID).Delete(&model.RolePermission{}).Error; err != nil {
		t.Fatalf("revoke reader permission: %v", err)
	}
	if err := fixture.service.Publish(change); err != nil {
		t.Fatalf("publish after revoke: %v", err)
	}
	assertNoNotification(t, readerEvents)
}

func TestDescriptorForMutationPathsAndOperations(t *testing.T) {
	cases := []struct {
		path       string
		ok         bool
		module     string
		entityType string
		operation  string
	}{
		{path: "/api/v1/customers", ok: true, module: "customers", entityType: "customer_profile", operation: OperationCreate},
		{path: "/api/v1/customer-codes/9", ok: true, module: "customers", entityType: "customer_code", operation: OperationUpdate},
		{path: "/api/v1/suppliers/9", ok: true, module: "suppliers", entityType: "supplier", operation: OperationUpdate},
		{path: "/api/v1/customers/9", ok: true, module: "customers", entityType: "customer_profile", operation: OperationDelete},
		{path: "/api/v1/customers/import/preview", ok: false},
		{path: "/api/v1/customers/import/commit", ok: true, module: "customers", entityType: "customer_profile", operation: OperationImport},
		{path: "/api/v1/inventory-documents/3/post", ok: true, module: "warehouses", entityType: "inventory_document", operation: OperationUpdate},
		{path: "/api/v1/workorder/3/pause", ok: true, module: "workorder", entityType: "workorder", operation: OperationUpdate},
		{path: "/api/v1/system/users/3/reset-password", ok: true, module: "users", entityType: "user", operation: OperationUpdate},
		{path: "/api/v1/system/departments/3/employees", ok: true, module: "departments", entityType: "department", operation: OperationUpdate},
		{path: "/api/v1/molds/3/drawings", ok: true, module: "molds", entityType: "mold", operation: OperationCreate},
		{path: "/api/v1/materials", ok: true, module: "warehouses", entityType: "material", operation: OperationCreate},
		{path: "/api/v1/products", ok: true, module: "products", entityType: "product", operation: OperationCreate},
		{path: "/api/v1/unregistered", ok: false},
	}
	for _, tt := range cases {
		t.Run(tt.path, func(t *testing.T) {
			d, ok := descriptorForPath(tt.path)
			if ok != tt.ok {
				t.Fatalf("descriptor ok = %v, want %v (%+v)", ok, tt.ok, d)
			}
			if !tt.ok {
				return
			}
			if d.module != tt.module || d.entityType != tt.entityType {
				t.Fatalf("descriptor = %+v, want module=%q entity=%q", d, tt.module, tt.entityType)
			}
			method := http.MethodPost
			if strings.HasSuffix(tt.path, "/9") || strings.Contains(tt.path, "/3/") && !strings.HasSuffix(tt.path, "/drawings") {
				method = http.MethodPatch
			}
			if tt.operation == OperationDelete {
				method = http.MethodDelete
			}
			if operation := operationFor(method, tt.path); operation != tt.operation {
				t.Fatalf("operation = %q, want %q", operation, tt.operation)
			}
		})
	}
}

func TestInventoryDocumentNotificationOpensWarehouseModule(t *testing.T) {
	n, err := notificationFromChange(Change{
		Module: "warehouses", ModuleTitle: "仓库", EntityType: "inventory_document",
		OrganizationID: 1, ActorUserID: 2, Operation: OperationUpdate, Count: 1,
		Items: []ChangeItem{{EntityType: "inventory_document", EntityID: 9, Operation: OperationUpdate, Label: "RK-009"}},
	})
	if err != nil {
		t.Fatalf("notificationFromChange returned error: %v", err)
	}
	if n.Action.Type != ActionOpenModule || n.Action.Module != "warehouses" || n.Action.EntityID != 0 {
		t.Fatalf("inventory document action = %+v, want warehouse module", n.Action)
	}
}

func TestAllProtectedBusinessWriteRoutesHaveNotificationDescriptors(t *testing.T) {
	routes := []struct {
		method      string
		path        string
		ownerLookup bool
	}{
		{http.MethodPost, "/api/v1/system/departments", false},
		{http.MethodPut, "/api/v1/system/departments/:id", false},
		{http.MethodPatch, "/api/v1/system/departments/:id", false},
		{http.MethodPatch, "/api/v1/system/departments/:id/status", false},
		{http.MethodPut, "/api/v1/system/departments/:id/employees", false},
		{http.MethodPost, "/api/v1/system/terminals", false},
		{http.MethodPost, "/api/v1/system/employees", false},
		{http.MethodPut, "/api/v1/system/employees/:id", false},
		{http.MethodPatch, "/api/v1/system/employees/:id/status", false},
		{http.MethodDelete, "/api/v1/system/employees/:id", false},
		{http.MethodPost, "/api/v1/system/users", false},
		{http.MethodPatch, "/api/v1/system/users/:id/status", false},
		{http.MethodPatch, "/api/v1/system/users/:id/affiliation", false},
		{http.MethodPost, "/api/v1/system/users/:id/reset-password", false},
		{http.MethodPost, "/api/v1/system/users/:id/roles", false},
		{http.MethodPost, "/api/v1/system/roles", false},
		{http.MethodPost, "/api/v1/system/roles/:id/permissions", false},
		{http.MethodPost, "/api/v1/customer-codes", false},
		{http.MethodPatch, "/api/v1/customer-codes/:id", false},
		{http.MethodDelete, "/api/v1/customer-codes/:id", false},
		{http.MethodPost, "/api/v1/customers", false},
		{http.MethodPatch, "/api/v1/customers/:id", false},
		{http.MethodPut, "/api/v1/customers/:id/default", false},
		{http.MethodDelete, "/api/v1/customers/:id", false},
		{http.MethodPost, "/api/v1/customers/import/commit", false},
		{http.MethodPost, "/api/v1/suppliers", false},
		{http.MethodPatch, "/api/v1/suppliers/:id", false},
		{http.MethodPost, "/api/v1/warehouses", false},
		{http.MethodPost, "/api/v1/warehouse/products", false},
		{http.MethodPost, "/api/v1/locations", false},
		{http.MethodPost, "/api/v1/inventory-documents", false},
		{http.MethodPost, "/api/v1/inventory-documents/:id/post", false},
		{http.MethodPost, "/api/v1/inventory-documents/:id/reverse", false},
		{http.MethodPost, "/api/v1/warehouse/products/:id/movements", true},
		{http.MethodPost, "/api/v1/materials", false},
		{http.MethodPost, "/api/v1/products", false},
		{http.MethodPost, "/api/v1/workorder", false},
		{http.MethodPost, "/api/v1/workorder/:id/dispatch", false},
		{http.MethodPost, "/api/v1/workorder/:id/pause", false},
		{http.MethodPost, "/api/v1/workorder/:id/resume", false},
		{http.MethodPost, "/api/v1/workorder/:id/urgent", false},
		{http.MethodPost, "/api/v1/workorder/:id/complete", false},
		{http.MethodPost, "/api/v1/workorder/department-tasks/:id/start", false},
		{http.MethodPost, "/api/v1/workorder/department-tasks/:id/partial-complete", false},
		{http.MethodPost, "/api/v1/workorder/department-tasks/:id/complete", false},
		{http.MethodPost, "/api/v1/molds/:id/drawings", false},
		{http.MethodDelete, "/api/v1/molds/:id/drawings/:drawing_id", false},
		{http.MethodPost, "/api/v1/molds/import/commit", false},
		{http.MethodPost, "/api/v1/molds", false},
		{http.MethodPatch, "/api/v1/molds/:id", false},
		{http.MethodDelete, "/api/v1/molds/:id", false},
		{http.MethodPost, "/api/v1/molds/bulk-location", false},
		{http.MethodPost, "/api/v1/mold-locations", false},
		{http.MethodPost, "/api/v1/mold-locations/bulk", false},
		{http.MethodPatch, "/api/v1/mold-locations/:id", false},
		{http.MethodPost, "/api/v1/files/images", true},
		{http.MethodPut, "/api/v1/files/:id/content", true},
		{http.MethodDelete, "/api/v1/files/:id", true},
	}
	for _, route := range routes {
		route := route
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			if _, ok := descriptorForPath(route.path); !ok {
				if route.ownerLookup && hasPathPrefix(route.path, "/api/v1/files") {
					// File replacement/deletion resolves the business owner from
					// ImageFile before the response; the owner descriptor cannot
					// be selected from URL text alone.
					return
				}
				t.Fatalf("missing notification descriptor for protected write route")
			}
		})
	}
	for _, path := range []string{
		"/api/v1/system/audits",
		"/api/v1/notifications/stream",
		"/api/v1/auth/change-password",
	} {
		if _, ok := descriptorForPath(path); ok {
			t.Fatalf("non-business route unexpectedly has notification descriptor: %s", path)
		}
	}
}

func TestMutationMiddlewareSkipsInventoryIdempotentReplay(t *testing.T) {
	fixture := newNotificationFixtures(t)
	permission := model.Permission{Name: "库存查看", Code: "inventory-documents:read", Object: "/api/v1/inventory-documents", Action: "read"}
	if err := fixture.db.Create(&permission).Error; err != nil {
		t.Fatalf("create inventory permission: %v", err)
	}
	if err := fixture.db.Create(&model.RolePermission{RoleID: fixture.readRole.ID, PermissionID: permission.ID}).Error; err != nil {
		t.Fatalf("bind inventory permission: %v", err)
	}
	events, cancel, err := fixture.service.Subscribe(fixture.reader.ID)
	if err != nil {
		t.Fatalf("subscribe reader: %v", err)
	}
	defer cancel()
	e := echo.New()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/inventory-documents", nil)
	request.Header.Set("Idempotency-Key", "replayed-key")
	context := e.NewContext(request, httptest.NewRecorder())
	context.Set(auth.ContextUserKey, &auth.CurrentUser{ID: fixture.actor.ID, Username: fixture.actor.Username, OrganizationID: fixture.actor.OrganizationID})
	if err := fixture.service.MutationMiddleware()(func(c *echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{"id": 41})
	})(context); err != nil {
		t.Fatalf("replay returned error: %v", err)
	}
	assertNoNotification(t, events)

	request = httptest.NewRequest(http.MethodPost, "/api/v1/inventory-documents", nil)
	request.Header.Set("Idempotency-Key", "new-key")
	context = e.NewContext(request, httptest.NewRecorder())
	context.Set(auth.ContextUserKey, &auth.CurrentUser{ID: fixture.actor.ID, Username: fixture.actor.Username, OrganizationID: fixture.actor.OrganizationID})
	if err := fixture.service.MutationMiddleware()(func(c *echo.Context) error {
		return c.JSON(http.StatusCreated, map[string]any{"id": 42})
	})(context); err != nil {
		t.Fatalf("new create returned error: %v", err)
	}
	event := receiveNotification(t, events)
	if event.Action.Type != ActionOpenModule || event.Action.EntityID != 0 || len(event.Items) != 1 || event.Items[0].EntityID != 42 || event.Items[0].Operation != OperationCreate {
		t.Fatalf("new create event = %+v", event)
	}
}

func TestHubMergesSameModuleAndBoundsItems(t *testing.T) {
	hub := NewHub(Config{MergeWindow: 5 * time.Millisecond, QueueSize: 2, MaxConnections: 4, MaxConnectionsPerUser: 2})
	events, cancel, err := hub.Subscribe(7)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cancel()
	for id := uint(1); id <= 5; id++ {
		hub.PublishTo([]uint{7}, Notification{
			V: 1, ID: uintString(id), Kind: KindDataChanged, Priority: PriorityNormal, Module: "suppliers", Count: 1,
			Action:  NotificationAction{Type: ActionOpenEntity, Module: "suppliers", EntityType: "supplier", EntityID: id},
			Refresh: RefreshHint{Module: "suppliers", EntityType: "supplier", EntityID: id},
			Items:   []NotificationItem{{EntityType: "supplier", EntityID: id, Operation: OperationUpdate}},
		})
	}
	event := receiveNotification(t, events)
	if event.Count != 5 || len(event.Items) != 3 || !event.Truncated || event.Action.Type != ActionOpenModule || !event.Refresh.InvalidateAll {
		t.Fatalf("merged event = %+v", event)
	}
}

func TestHubDisconnectsSlowConsumerAndExposesMetrics(t *testing.T) {
	hub := NewHub(Config{MergeWindow: time.Millisecond, QueueSize: 1, MaxConnections: 1, MaxConnectionsPerUser: 1})
	events, cancel, err := hub.Subscribe(7)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cancel()
	if _, _, err := hub.Subscribe(7); err == nil {
		t.Fatal("second connection unexpectedly succeeded")
	}
	for id := uint(1); id <= 2; id++ {
		hub.PublishTo([]uint{7}, Notification{V: 1, ID: uintString(id), Kind: KindDataChanged, Module: "suppliers", Count: 1, Action: NotificationAction{Type: ActionOpenModule, Module: "suppliers"}, Refresh: RefreshHint{Module: "suppliers"}})
		time.Sleep(8 * time.Millisecond)
	}
	// The first event remains unread in the queue; the second flush observes a
	// full queue and closes the slow subscription.
	if got := <-events; got.ID == "" {
		t.Fatal("slow consumer received empty event")
	}
	deadline := time.After(time.Second)
	for hub.ConnectionCount() != 0 {
		select {
		case <-deadline:
			t.Fatalf("slow consumer remained connected, metrics=%+v", hub.Metrics())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	metrics := hub.Metrics()
	if metrics.DroppedMessages == 0 || metrics.RejectedConnections == 0 || metrics.ActiveConnections != 0 {
		t.Fatalf("unexpected hub metrics: %+v", metrics)
	}
}

func TestMutationMiddlewarePublishesOnlySuccessfulWrites(t *testing.T) {
	fixture := newNotificationFixtures(t)
	events, cancel, err := fixture.service.Subscribe(fixture.reader.ID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cancel()

	e := echo.New()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/suppliers", nil)
	recorder := httptest.NewRecorder()
	context := e.NewContext(request, recorder)
	context.Set(auth.ContextUserKey, &auth.CurrentUser{ID: fixture.actor.ID, Username: fixture.actor.Username, OrganizationID: fixture.actor.OrganizationID})
	handler := fixture.service.MutationMiddleware()(func(c *echo.Context) error {
		return c.JSON(http.StatusCreated, map[string]any{"id": 12, "code": "SUP-012", "name": "供应商"})
	})(context)
	if err := handler; err != nil {
		t.Fatalf("successful mutation returned error: %v", err)
	}
	event := receiveNotification(t, events)
	if event.Action.Type != ActionOpenEntity || event.Action.EntityID != 12 || event.Items[0].Label != "SUP-012" {
		t.Fatalf("middleware event = %+v", event)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/suppliers", nil)
	recorder = httptest.NewRecorder()
	context = e.NewContext(request, recorder)
	context.Set(auth.ContextUserKey, &auth.CurrentUser{ID: fixture.actor.ID, Username: fixture.actor.Username, OrganizationID: fixture.actor.OrganizationID})
	if err := fixture.service.MutationMiddleware()(func(c *echo.Context) error {
		return echo.NewHTTPError(http.StatusConflict, "失败")
	})(context); err == nil {
		t.Fatal("failed mutation unexpectedly returned nil")
	}
	assertNoNotification(t, events)
}

func TestStreamUsesAuthenticatedSessionAndClosesOnContext(t *testing.T) {
	service := NewService(nil, Config{MergeWindow: time.Millisecond, HeartbeatInterval: time.Hour}, nil)
	handler := NewHandler(service)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/notifications/stream", nil)
	writer := newStreamWriter()
	e := echo.New()
	context := e.NewContext(request, writer)
	context.Set(auth.ContextUserKey, &auth.CurrentUser{ID: 7, Username: "reader", OrganizationID: 1})
	done := make(chan error, 1)
	go func() { done <- handler.Stream(context) }()
	if initial := readStreamChunk(t, writer); initial != "retry: 3000\n\n" {
		t.Fatalf("initial stream chunk = %q", initial)
	}
	service.Hub().PublishTo([]uint{7}, Notification{V: 1, ID: "event", Kind: KindDataChanged, Module: "suppliers", Count: 1, Action: NotificationAction{Type: ActionOpenModule, Module: "suppliers"}, Refresh: RefreshHint{Module: "suppliers", InvalidateAll: true}})
	event := readStreamChunk(t, writer)
	if event == "" || !strings.Contains(event, "event: data_changed") || !strings.Contains(event, `"module":"suppliers"`) {
		t.Fatalf("stream event = %q", event)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stream close error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stream did not close after request context cancellation")
	}
}

func receiveNotification(t *testing.T, events <-chan Notification) Notification {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("notification channel closed")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
		return Notification{}
	}
}

func assertNoNotification(t *testing.T, events <-chan Notification) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("unexpected notification: %+v", event)
	case <-time.After(30 * time.Millisecond):
	}
}

type streamWriter struct {
	mu      sync.Mutex
	header  http.Header
	chunks  chan []byte
	flushed chan struct{}
}

func newStreamWriter() *streamWriter {
	return &streamWriter{header: make(http.Header), chunks: make(chan []byte, 16), flushed: make(chan struct{}, 16)}
}

func (w *streamWriter) Header() http.Header { return w.header }

func (w *streamWriter) Write(p []byte) (int, error) {
	copyOfP := append([]byte(nil), p...)
	w.chunks <- copyOfP
	return len(p), nil
}

func (w *streamWriter) WriteHeader(int) {}

func (w *streamWriter) Flush() {
	select {
	case w.flushed <- struct{}{}:
	default:
	}
}

func readStreamChunk(t *testing.T, writer *streamWriter) string {
	t.Helper()
	select {
	case chunk := <-writer.chunks:
		return string(chunk)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for SSE data")
		return ""
	}
}
