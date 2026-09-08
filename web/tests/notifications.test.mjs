import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'
import {
  aggregateNotificationEvents,
  normalizeNotification,
  parseNotificationSseText,
} from '../src/platform/notifications.ts'
import {auditActionLabel} from '../src/composables/workspacePresentation.ts'

const change = (id, module, entityID, operation = 'update') => ({
  v: 1,
  id,
  kind: 'data_changed',
  priority: 'normal',
  occurred_at: '2026-09-09T08:00:00Z',
  display_for_ms: 8000,
  module,
  title: `${module}有数据更新`,
  summary: '数据更新',
  count: 1,
  items: [{entity_type: 'record', entity_id: entityID, operation, name: `${module}-${entityID}` }],
  action: {type: 'open_entity', module, entity_type: 'record', entity_id: entityID},
  refresh: {module, entity_type: 'record', entity_id: entityID, invalidate_all: false},
  truncated: false,
})

test('SSE data block 只解析 data 行并忽略心跳和无效 JSON', () => {
  const events = parseNotificationSseText([
    ': keep-alive',
    '',
    'event: data_changed',
    'data: {"kind":"data_changed","module":"customers","count":1}',
    '',
    'data: {not-json}',
    '',
  ].join('\n'))
  assert.deepEqual(events, [{kind: 'data_changed', module: 'customers', count: 1}])
})

test('通知规范化只保留白名单字段并兼容最终 action/refresh 对象', () => {
  const notification = normalizeNotification({
    ...change('n-1', 'customer', 7, 'create'),
    items: [{entity_type: 'customer_profile', entity_id: 7, operation: 'create', fields: {name: '华东客户', status: 'active', cost: 'secret', password: 'secret'}}],
    action: {type: 'open_entity', module: 'customer', entity_type: 'customer_profile', entity_id: 7},
    refresh: {module: 'customer', entity_type: 'customer_profile', entity_id: 7, invalidate_all: true},
  })
  assert.equal(notification.module, 'customers')
  assert.equal(notification.items[0].name, '华东客户')
  assert.equal(notification.items[0].status, 'active')
  assert.equal('cost' in notification.items[0], false)
  assert.equal('password' in notification.items[0], false)
  assert.deepEqual(notification.action, {type: 'open_entity', module: 'customers', entity_type: 'customer_profile', entity_id: 7})
  assert.deepEqual(notification.refresh, {module: 'customers', entity_type: 'customer_profile', entity_id: 7, invalidate_all: true})
})

test('单对象摘要展示后端提供的安全名称与状态', () => {
  const merged = aggregateNotificationEvents([{
    ...change('n-status', 'workorder', 12, 'update'),
    items: [{entity_type: 'workorder', entity_id: 12, operation: 'update', label: 'WO-012', status: 'processing'}],
  }], 'batch-status')
  assert.match(merged.summary, /修改WO-012/)
  assert.match(merged.summary, /状态：处理中/)
})

test('两秒窗口合并跨模块事件并保留准确总数和模块摘要', () => {
  const merged = aggregateNotificationEvents([
    change('n-1', 'customers', 1, 'create'),
    change('n-2', 'warehouses', 2, 'delete'),
    change('n-3', 'molds', 3, 'import'),
    change('n-4', 'users', 4, 'update'),
  ], 'batch-1')
  assert.equal(merged.id, 'batch-1')
  assert.equal(merged.count, 4)
  assert.equal(merged.modules.length, 4)
  assert.match(merged.summary, /4 条数据有更新/)
  assert.match(merged.summary, /另有 1 类更新/)
  assert.deepEqual(merged.modules.map((item) => item.action.type), ['open_module', 'open_module', 'open_module', 'open_module'])
})

test('审计稳定动作编码显示为业务动作，不把 HTTP 方法或路径当作操作名', () => {
  assert.equal(auditActionLabel('users:assign_roles'), '用户账号 · 分配角色')
  assert.equal(auditActionLabel('molds:bulk_move'), '模具 · 批量移动')
  assert.equal(auditActionLabel('warehouses:movement'), '仓库 · 办理出入库')
  assert.equal(auditActionLabel('molds:drawing_upload'), '模具 · 上传图纸')
  assert.equal(auditActionLabel('files:replace_content'), '文件 · 替换图纸内容')
  assert.equal(auditActionLabel('users:status'), '用户账号 · 变更状态')
  assert.equal(auditActionLabel(''), '-')
})

test('图片大图预览存在时，Escape 不会穿透关闭底层详情', () => {
  const carrier = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  const mold = readFileSync(new URL('../src/components/pages/module/MoldModuleContent.vue', import.meta.url), 'utf8')
  assert.match(carrier, /\.el-image-viewer__wrapper/)
  assert.match(carrier, /\.el-image-viewer__mask/)
  assert.match(mold, /\.el-image-viewer__wrapper/)
  assert.match(mold, /\.el-image-viewer__mask/)
})

test('通知打开实体前等待列表，并对分页外 warehouse/workorder 目标按 ID 兜底', () => {
  const realtime = readFileSync(new URL('../src/composables/useRealtimeNotifications.ts', import.meta.url), 'utf8')
  const warehouse = readFileSync(new URL('../src/composables/useWarehouseOperations.ts', import.meta.url), 'utf8')
  const workorder = readFileSync(new URL('../src/composables/useWorkorderOperations.ts', import.meta.url), 'utf8')
  assert.match(realtime, /await host\.loadActiveModule\(\)/)
  assert.match(realtime, /loadWarehouseItemByID/)
  assert.match(realtime, /loadWorkOrderByID/)
  assert.match(warehouse, /\/api\/v1\/warehouse\/items\/\$\{itemType\}\/\$\{itemID\}/)
  assert.match(workorder, /while \(currentPage <= 100/)
  assert.match(workorder, /workOrderContainsID/)
})

test('账号角色/权限缓存不可见时使用明确的不可用文案', () => {
  const controller = readFileSync(new URL('../src/composables/useWorkspaceController.ts', import.meta.url), 'utf8')
  assert.match(controller, /当前账号无权查看权限详情/)
  assert.match(controller, /权限信息暂不可用/)
  assert.doesNotMatch(controller, /已配置 \$\{roleIDs\.length\} 个角色来源/)
})

test('通知面板具备 Teleport 后聚焦、键盘循环及 40px 操作目标', () => {
  const notification = readFileSync(new URL('../src/components/ui/NotificationCenter.vue', import.meta.url), 'utf8')
  assert.match(notification, /aria-haspopup="dialog"/)
  assert.match(notification, /:aria-label="triggerLabel"/)
  assert.match(notification, /function handlePanelKeydown/)
  assert.match(notification, /restoreTriggerFocus/)
  assert.match(notification, /notification-panel/)
  assert.match(notification, /notification-list-item__close \{[^}]*width: 40px/)
  assert.match(notification, /notification-toast__view \{[^}]*min-height: 40px/)
})

test('客户只读详情允许同步，创建/编辑/导入状态由页面暴露给通知层', () => {
  const customer = readFileSync(new URL('../src/components/pages/CustomerPage.vue', import.meta.url), 'utf8')
  const realtime = readFileSync(new URL('../src/composables/useRealtimeNotifications.ts', import.meta.url), 'utf8')
  const app = readFileSync(new URL('../src/components/app/AppWorkspace.vue', import.meta.url), 'utf8')
  assert.match(customer, /setCustomerNotificationState/)
  assert.match(customer, /const customerEditing = computed/)
  assert.match(customer, /const customerDirty = computed/)
  assert.match(customer, /importVisible\.value/)
  assert.match(realtime, /host\.customerEditing\?\.value \|\| host\.customerDirty\?\.value/)
  assert.doesNotMatch(realtime, /module === 'customers' && host\.pageDetailPanelVisible/)
  assert.match(app, /customerEditing, customerDirty/)
})

test('顶栏将铃铛与用户菜单收进右侧 actions 容器，审计列和查看列文案收敛', () => {
  const app = readFileSync(new URL('../src/components/app/AppWorkspace.vue', import.meta.url), 'utf8')
  const shell = readFileSync(new URL('../src/styles/shell.css', import.meta.url), 'utf8')
  const generic = readFileSync(new URL('../src/components/pages/module/GenericModuleContent.vue', import.meta.url), 'utf8')
  const workorder = readFileSync(new URL('../src/components/pages/module/WorkorderModuleContent.vue', import.meta.url), 'utf8')
  const mold = readFileSync(new URL('../src/components/pages/module/MoldModuleContent.vue', import.meta.url), 'utf8')
  assert.match(app, /class="topbar-actions"/)
  assert.match(shell, /\.topbar-actions \{/)
  assert.match(generic, /activeKey\.value !== 'audits'/)
  assert.match(generic, /actionColumnLabel/)
  assert.match(workorder, /label="查看"[^>]*fixed="right"/)
  assert.match(mold, /label="查看"[^>]*fixed="right"/)
})
