/**
 * Shared, versioned client-side shape for best-effort realtime data changes.
 *
 * The server remains responsible for authorization and for filtering sensitive
 * fields.  The client applies a second, deliberately small allow-list before
 * displaying anything in a toast or notification list.
 */

export const notificationStreamPath = '/api/v1/notifications/stream'
export const notificationRefreshEvent = 'bb:notification-refresh'
export const notificationOpenEvent = 'bb:notification-open'

export type NotificationOperation = 'create' | 'update' | 'delete' | 'import' | 'created' | 'updated' | 'deleted' | 'imported' | 'status_changed' | 'changed' | string
export type NotificationActionType = 'open_module' | 'open_entity'

export interface NotificationAction {
  type: NotificationActionType
  module: string
  entity_type?: string
  entity_id?: string | number
}

export interface NotificationRefresh {
  module: string
  entity_type?: string
  entity_id?: string | number
  invalidate_all?: boolean
}

/** Fields permitted in the UI.  Arbitrary server fields are intentionally not
 * retained, even if a future server version includes them in `key_fields`. */
export interface NotificationItem {
  entity_type: string
  entity_id: string | number
  operation: NotificationOperation
  label?: string
  name?: string
  code?: string
  number?: string
  model?: string
  status?: string
  type?: string
}

export interface DataChangeNotification {
  v: number
  id: string
  kind: 'data_changed' | string
  priority: 'normal' | 'high'
  occurred_at: string
  display_for_ms: number
  module: string
  title: string
  summary: string
  count: number
  items: NotificationItem[]
  action: NotificationAction
  refresh: NotificationRefresh
  truncated: boolean
  /** Optional defense-in-depth field.  The server should already omit the actor. */
  actor_user_id?: string | number
}

export interface NotificationModuleSummary {
  module: string
  title: string
  count: number
  items: NotificationItem[]
  action: NotificationAction
  refresh: NotificationRefresh
  truncated: boolean
}

export interface RealtimeNotification {
  id: string
  kind: 'data_changed'
  priority: 'normal' | 'high'
  occurredAt: string
  displayForMs: number
  title: string
  summary: string
  count: number
  modules: NotificationModuleSummary[]
  read: boolean
}

export interface NotificationRefreshEventDetail {
  module: NotificationModuleSummary
  notification: RealtimeNotification
  deferred: boolean
  /** Page adapters may claim the refresh after preserving local state. */
  handled?: boolean
  /** Claimed adapters expose whether their asynchronous refresh succeeded. */
  completion?: Promise<boolean>
}

export interface NotificationOpenEventDetail {
  module: NotificationModuleSummary
  notification: RealtimeNotification
}

const moduleAliases: Record<string, string> = {
  customer: 'customers',
  customers: 'customers',
  customer_code: 'customers',
  customer_codes: 'customers',
  supplier: 'suppliers',
  suppliers: 'suppliers',
  warehouse: 'warehouses',
  warehouses: 'warehouses',
  inventory: 'warehouses',
  material: 'materials',
  materials: 'materials',
  product: 'products',
  products: 'products',
  mold: 'molds',
  molds: 'molds',
  workorder: 'workorder',
  workorders: 'workorder',
  work_order: 'workorder',
  work_orders: 'workorder',
  department: 'departments',
  departments: 'departments',
  employee: 'employees',
  employees: 'employees',
  user: 'users',
  users: 'users',
  terminal: 'terminals',
  terminals: 'terminals',
  role: 'roles',
  roles: 'roles',
  audit: 'audits',
  audits: 'audits',
  statistics: 'statistics',
}

const moduleTitles: Record<string, string> = {
  customers: '客户资料',
  suppliers: '供应商',
  warehouses: '仓库',
  materials: '物料',
  products: '产品',
  molds: '模具',
  workorder: '任务单',
  departments: '部门',
  employees: '员工档案',
  users: '用户账号',
  terminals: '终端',
  roles: '角色',
  audits: '操作审计',
  statistics: '统计报表',
}

const itemFieldAllowList = new Set(['label', 'name', 'code', 'number', 'model', 'status', 'type'])

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function text(value: unknown, max = 160): string {
  if (typeof value !== 'string' && typeof value !== 'number') return ''
  return String(value).replace(/[\u0000-\u001f\u007f]/g, '').trim().slice(0, max)
}

function scalar(value: unknown): string | number | undefined {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  const normalized = text(value, 80)
  return normalized || undefined
}

function notificationDataLines(block: string): string[] {
  const lines: string[] = []
  for (const line of block.split(/\r?\n/)) {
    if (line.startsWith('data:')) lines.push(line.slice(5).replace(/^ /, ''))
  }
  return lines
}

export function parseNotificationSseBlock(block: string): unknown | null {
  const data = notificationDataLines(block).join('\n').trim()
  if (!data) return null
  try { return JSON.parse(data) as unknown } catch { return null }
}

/** Parse complete SSE blocks; the streaming consumer handles partial chunks. */
export function parseNotificationSseText(value: string): unknown[] {
  return value.split(/\r?\n\r?\n/).map(parseNotificationSseBlock).filter((item): item is unknown => item !== null)
}

export function normalizeModuleKey(value: unknown): string {
  const normalized = text(value, 80).toLowerCase().replace(/[\s-]+/g, '_')
  return moduleAliases[normalized] || normalized
}

export function notificationModuleTitle(module: string): string {
  return moduleTitles[module] || moduleTitles[normalizeModuleKey(module)] || (module ? module : '业务数据')
}

function normalizeItem(value: unknown): NotificationItem | null {
  const source = record(value)
  const fields = {...record(source.key_fields), ...record(source.fields), ...record(source.display)}
  const entityID = scalar(source.entity_id ?? source.id ?? fields.entity_id ?? fields.id)
  if (entityID === undefined) return null
  const item: NotificationItem = {
    entity_type: text(source.entity_type ?? source.type_name ?? fields.entity_type, 80) || 'record',
    entity_id: entityID,
    operation: text(source.operation ?? source.action ?? source.change_type, 40) || 'updated',
  }
  for (const key of itemFieldAllowList) {
    const value = text(source[key] ?? fields[key])
    if (value) item[key as keyof NotificationItem] = value as never
  }
  return item
}

function normalizeAction(value: unknown, module: string, item?: NotificationItem): NotificationAction {
  const source = record(value)
  const typeValue = text(typeof value === 'string' ? value : source.type ?? source.kind, 40)
  const type: NotificationActionType = typeValue === 'open_entity' || typeValue === 'entity' ? 'open_entity' : 'open_module'
  const actionModule = normalizeModuleKey(source.module || module)
  return {
    type,
    module: actionModule,
    entity_type: text(source.entity_type ?? source.type_name ?? item?.entity_type, 80) || undefined,
    entity_id: scalar(source.entity_id ?? source.id ?? item?.entity_id),
  }
}

function normalizeRefresh(value: unknown, module: string, item?: NotificationItem): NotificationRefresh {
  const source = record(value)
  return {
    module: normalizeModuleKey(source.module || module),
    entity_type: text(source.entity_type ?? source.type_name ?? item?.entity_type, 80) || undefined,
    entity_id: scalar(source.entity_id ?? source.id ?? item?.entity_id),
    invalidate_all: Boolean(source.invalidate_all ?? source.all),
  }
}

export function normalizeNotification(value: unknown): DataChangeNotification | null {
  const source = record(value)
  const kind = text(source.kind, 40) || 'data_changed'
  if (kind !== 'data_changed') return null
  const module = normalizeModuleKey(source.module ?? source.resource ?? source.domain)
  if (!module) return null
  const rawItems = Array.isArray(source.items) ? source.items : []
  const items = rawItems.map(normalizeItem).filter((item): item is NotificationItem => !!item)
  const firstItem = items[0]
  const countValue = Number(source.count)
  const count = Math.max(items.length, Number.isFinite(countValue) && countValue > 0 ? Math.floor(countValue) : 1)
  const rawID = text(source.id, 120)
  const id = rawID || `${module}:${text(source.occurred_at, 80) || Date.now()}:${firstItem?.entity_id || count}`
  const title = text(source.title, 120) || `${notificationModuleTitle(module)}有数据更新`
  const summary = text(source.summary, 240) || `${notificationModuleTitle(module)}有 ${count} 条数据更新`
  const occurredAt = text(source.occurred_at ?? source.occurredAt, 80) || new Date().toISOString()
  const display = Number(source.display_for_ms)
  return {
    v: Number.isFinite(Number(source.v)) ? Number(source.v) : 1,
    id,
    kind,
    priority: source.priority === 'high' || source.priority === 'urgent' ? 'high' : 'normal',
    occurred_at: occurredAt,
    display_for_ms: Math.min(30000, Math.max(3000, Number.isFinite(display) && display > 0 ? display : 8000)),
    module,
    title,
    summary,
    count,
    items,
    action: normalizeAction(source.action, module, firstItem),
    refresh: normalizeRefresh(source.refresh, module, firstItem),
    truncated: Boolean(source.truncated) || items.length < rawItems.length,
    actor_user_id: scalar(source.actor_user_id),
  }
}

function operationLabel(operation: string): string {
  const labels: Record<string, string> = {
    create: '新增',
    update: '修改',
    delete: '删除',
    import: '导入',
    created: '新增',
    updated: '修改',
    deleted: '删除',
    imported: '导入',
    status_changed: '状态更新',
    changed: '更新',
  }
  return labels[operation] || '更新'
}

function itemLabel(item: NotificationItem): string {
  return item.label || item.name || item.code || item.number || item.model || `#${item.entity_id}`
}

function itemStatusLabel(value: string): string {
  const labels: Record<string, string> = {
    active: '正常', enabled: '正常', normal: '正常', success: '成功', succeeded: '成功',
    inactive: '停用', disabled: '停用', stopped: '停用', failed: '失败', failure: '失败',
    error: '失败', denied: '拒绝', draft: '草稿', processing: '处理中', paused: '暂停',
    pending_close: '待确认', completed: '已完成', completed_normal: '正常完成',
    completed_forced: '强制完成', cancelled: '已取消',
  }
  return labels[value.toLowerCase()] || value
}

function itemSummary(item: NotificationItem): string {
  const status = item.status ? ` · 状态：${itemStatusLabel(item.status)}` : ''
  return `${operationLabel(item.operation)}${itemLabel(item)}${status}`
}

function moduleSummary(events: DataChangeNotification[], module: string): NotificationModuleSummary {
  const source = events.filter((event) => event.module === module)
  const count = source.reduce((total, event) => total + event.count, 0)
  const unique = new Map<string, NotificationItem>()
  for (const event of source) {
    for (const item of event.items) {
      const key = `${item.entity_type}:${item.entity_id}:${item.operation}`
      if (!unique.has(key)) unique.set(key, item)
    }
  }
  const items = [...unique.values()].slice(0, 3)
  const first = source[0]
  const firstItem = items[0]
  const canOpenEntity = count === 1 && items.length === 1 && firstItem?.operation !== 'delete' && firstItem?.operation !== 'import' && firstItem?.operation !== 'deleted' && firstItem?.operation !== 'imported'
  return {
    module,
    title: notificationModuleTitle(module),
    count,
    items,
    action: canOpenEntity ? first.action : {type: 'open_module', module},
    refresh: first.refresh,
    truncated: source.some((event) => event.truncated) || unique.size > items.length,
  }
}

/** Merge the short burst collected by the SSE client into one display card. */
export function aggregateNotificationEvents(events: DataChangeNotification[], id = ''): RealtimeNotification | null {
  if (!events.length) return null
  const byModule = [...new Set(events.map((event) => event.module))]
  const modules = byModule.map((module) => moduleSummary(events, module))
  if (modules.length > 1) {
    for (const module of modules) module.action = {type: 'open_module', module: module.module}
  }
  const count = modules.reduce((total, module) => total + module.count, 0)
  const first = events[0]
  const title = modules.length === 1 ? (first.title || modules[0].title) : '业务数据有更新'
  let summary = ''
  if (modules.length === 1) {
    const module = modules[0]
    if (count === 1 && module.items[0]) {
      summary = itemSummary(module.items[0])
    } else {
      summary = `${module.title}有 ${count} 条数据更新`
    }
  } else {
    summary = `${count} 条数据有更新 · ${modules.slice(0, 3).map((module) => `${module.title} ${module.count} 条`).join('、')}`
    if (modules.length > 3) summary += `，另有 ${modules.length - 3} 类更新`
  }
  return {
    id: id || `${first.id}:${events.length}`,
    kind: 'data_changed',
    priority: events.some((event) => event.priority === 'high') ? 'high' : 'normal',
    occurredAt: first.occurred_at,
    displayForMs: Math.max(...events.map((event) => event.display_for_ms), 8000),
    title,
    summary,
    count,
    modules,
    read: false,
  }
}
