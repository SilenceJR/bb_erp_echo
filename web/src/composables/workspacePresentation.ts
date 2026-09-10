import type {StatusTone} from '../components/ui/StatusTag.vue'
import type {BasicItem} from '../types'

let fallbackIdempotencySequence = 0

function formatUuidBytes(bytes: Uint8Array): string {
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

function fallbackHex(length: number): string {
  let result = ''
  while (result.length < length) result += Math.floor(Math.random() * 0x100000000).toString(16).padStart(8, '0')
  return result.slice(0, length)
}

function fallbackIdempotencyKey(): string {
  const timestamp = Date.now().toString(16).padStart(12, '0')
  const sequence = (fallbackIdempotencySequence++ % 0x100000000).toString(16).padStart(8, '0')
  const hex = `${timestamp}${sequence}${fallbackHex(12)}`
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-4${hex.slice(13, 16)}-8${hex.slice(17, 20)}-${hex.slice(20)}`
}

export function createIdempotencyKey(): string {
  const webCrypto = typeof crypto !== 'undefined' ? crypto : undefined
  if (webCrypto && typeof webCrypto.randomUUID === 'function') {
    try { return webCrypto.randomUUID() } catch { /* incomplete WebView */ }
  }
  if (webCrypto && typeof webCrypto.getRandomValues === 'function') {
    try {
      const bytes = new Uint8Array(16)
      webCrypto.getRandomValues(bytes)
      bytes[6] = (bytes[6] & 0x0f) | 0x40
      bytes[8] = (bytes[8] & 0x3f) | 0x80
      return formatUuidBytes(bytes)
    } catch { /* fallback below */ }
  }
  return fallbackIdempotencyKey()
}

export function formatCell(value: unknown): string {
  if (value === null || value === undefined || value === '') return '-'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

export function genericStatusLabel(value: unknown): string {
  const labels: Record<string, string> = {active: '正常', enabled: '正常', normal: '正常', success: '成功', succeeded: '成功', inactive: '停用', disabled: '停用', stopped: '停用', failed: '失败', failure: '失败', error: '失败', denied: '拒绝', unknown: '未设置'}
  return labels[String(value || 'unknown').toLowerCase()] || formatCell(value)
}

export function genericStatusTone(value: unknown): StatusTone {
  const status = String(value || 'unknown').toLowerCase()
  if (['active', 'enabled', 'normal', 'success', 'succeeded'].includes(status)) return 'success'
  if (['failed', 'failure', 'error', 'denied'].includes(status)) return 'danger'
  return 'info'
}

const auditModuleLabels: Record<string, string> = {
  departments: '部门', employees: '员工档案', terminals: '终端', users: '用户账号', roles: '角色',
  customers: '客户资料', products: '产品资料', suppliers: '供应商', warehouses: '仓库', molds: '模具', workorder: '任务单', files: '文件', api: '系统',
}
const auditActionLabels: Record<string, string> = {
  create: '新增', update: '修改', delete: '删除', import: '导入', bulk_create: '批量新增',
  bulk_move: '批量移动', reset_password: '重置密码', assign_permissions: '配置权限', assign_roles: '分配角色',
  replace_employees: '更新成员', set_default: '切换默认资料', movement: '办理出入库', drawing_upload: '上传图纸',
  replace_content: '替换图纸内容', status: '变更状态', post: '提交', reverse: '撤回', dispatch: '派发',
  pause: '暂停', resume: '恢复', urgent: '加急', start: '开始处理', complete: '完成', partial_complete: '部分完成',
}

/** Map the backend's stable module:action audit code to operator-facing text. */
export function auditActionLabel(value: unknown): string {
  const raw = String(value || '').trim()
  if (!raw) return '-'
  const separator = raw.indexOf(':')
  const module = separator > 0 ? raw.slice(0, separator) : ''
  const action = separator > 0 ? raw.slice(separator + 1) : raw
  const moduleLabel = auditModuleLabels[module] || module
  const actionLabel = auditActionLabels[action] || action
  return module ? `${moduleLabel} · ${actionLabel}` : actionLabel
}

export function isGenericStatusColumn(column: string): boolean {
  return column === 'status' || column === 'result'
}

export function permissionDomainKey(option: BasicItem): string {
  const codePrefix = String(option.code || '').split(':')[0].trim().toLowerCase()
  const objectPath = String(option.object || '').toLowerCase()
  const aliases: Record<string, string> = {
    system: 'system', users: 'system', roles: 'system', audits: 'system', departments: 'system', employees: 'system', terminals: 'system',
    warehouse: 'warehouse', inventory: 'warehouse', material: 'warehouse', materials: 'warehouse', product: 'products', products: 'products',
    workorder: 'workorder', mold: 'mold', molds: 'mold', customer: 'customers', customers: 'customers', supplier: 'suppliers', suppliers: 'suppliers', statistics: 'statistics', cost: 'cost',
  }
  for (const candidate of [codePrefix, objectPath.replace(/^\/api\/v1\//, '').split('/')[0]]) {
    if (aliases[candidate]) return aliases[candidate]
  }
  const fallback = codePrefix || objectPath.replace(/^\/api\/v1\//, '').replaceAll('/', ' · ') || '未分类'
  return `other:${fallback}`
}

export function permissionDomainLabel(value: string): string {
  const labels: Record<string, string> = {system: '系统管理', warehouse: '仓库', inventory: '库存单据', workorder: '任务单', mold: '模具', customers: '客户', suppliers: '供应商', statistics: '统计报表', cost: '成本数据'}
  if (labels[value]) return labels[value]
  return `其他业务域 · ${value.startsWith('other:') ? value.slice(6) : value}`
}

export function stockState(row: unknown): {label: string; tone: StatusTone} {
  const item = row as Record<string, unknown>
  const quantity = Number(item.quantity || 0)
  const safetyStock = Number(item.safety_stock || 0)
  if (quantity <= 0) return {label: '缺货', tone: 'danger'}
  if (quantity <= safetyStock) return {label: '低于安全库存', tone: 'warning'}
  return {label: '库存正常', tone: 'success'}
}

const columnLabels: Record<string, string> = {
  id: '编号', username: '账号', account_type: '账号类型', name: '名称', organization_id: '组织', department_id: '部门', terminal_id: '终端', status: '状态', code: '编码', description: '说明',
  phone: '电话', contact: '联系人', address: '地址', location: '位置', item_type: '对象类型', category: '分类', unit: '单位', spec: '规格', safety_stock: '安全库存', type: '业务类型', warehouse_id: '仓库',
  to_warehouse_id: '目标仓库', reason: '原因', location_id: '库位', item_id: '物品', quantity: '数量', avg_cost: '平均成本', amount: '金额', document_id: '单据', balance_qty: '结存数量',
  product_model: '产品型号', customer_model: '客户型号', material: '产品材料', ink_required: '是否刷墨', mold_type: '模具类型', cavity_count: '模穴数', common_group_no: '共模组号', image_count: '图片总数', drawing_count: '图纸总数',
  object: '对象', action: '操作', actor_username: '操作账号', actor_account_type: '账号类型', person_name: '操作人', result: '结果', created_at: '操作时间', operator_employee_name: '操作员工', operator_department_name: '操作部门',
}

export function columnLabel(column: string): string { return columnLabels[column] || column }

export function departmentTasks(row: unknown): BasicItem[] {
  const value = (row as {department_tasks?: unknown})?.department_tasks
  return Array.isArray(value) ? value as BasicItem[] : []
}

export function departmentProgressMetrics(row: unknown): {percentage: number; completed: number; total: number} {
  const tasks = departmentTasks(row)
  if (!tasks.length) return {percentage: 0, completed: 0, total: 0}
  const completed = tasks.filter((task) => task.status === 'completed').length
  const totalProgress = tasks.reduce((sum, task) => {
    const explicit = Number(task.progress)
    if (Number.isFinite(explicit)) return sum + Math.min(100, Math.max(0, explicit))
    const planned = Number(task.planned_quantity || 0)
    const finished = Number(task.completed_quantity || 0)
    return sum + (planned > 0 ? Math.min(100, Math.max(0, finished * 100 / planned)) : 0)
  }, 0)
  return {percentage: Math.round(totalProgress / tasks.length), completed, total: tasks.length}
}

export function departmentProgressSummary(row: unknown): string {
  const progress = departmentProgressMetrics(row)
  return progress.total ? `${progress.percentage}% · ${progress.completed}/${progress.total} 个部门已完成` : '0% · 尚未分配部门'
}

export function workorderStatusLabel(value: unknown): string {
  const labels: Record<string, string> = {draft: '草稿', processing: '正在处理', paused: '暂停', pending_close: '待办公室确认', completed_normal: '正常完成', completed_forced: '强制完成', cancelled: '取消'}
  return labels[String(value)] || String(value || '-')
}

export const workorderTypeLabel = (value: unknown) => value === 'general' ? '通用任务' : '生产单'
export const inventoryItemTypeLabel = (value: unknown) => value === 'product' ? '产品' : '物料'

export function departmentCompletionRate(item: {total: number; completed: number}): number {
  return item.total ? Math.round(Number(item.completed || 0) * 100 / Number(item.total)) : 0
}

export function trendNameLabel(value: unknown): string {
  const labels: Record<string, string> = {inbound: '入库', outbound: '出库', transfer: '调拨', draft: '草稿', processing: '处理中', pending_close: '待确认', completed_normal: '正常完成', completed_forced: '强制完成'}
  return labels[String(value)] || String(value || '趋势')
}

export function departmentTaskStatusLabel(value: unknown): string {
  const labels: Record<string, string> = {draft: '待派发', received: '已收到', processing: '正在处理', partial_completed: '部分完成', completed: '完成'}
  return labels[String(value)] || String(value || '-')
}

export function workorderStatusTone(value: unknown): StatusTone {
  if (value === 'completed_normal') return 'success'
  if (value === 'completed_forced' || value === 'cancelled') return 'danger'
  if (value === 'pending_close') return 'warning'
  return 'info'
}

export function workorderDueState(row: unknown): {overdue: boolean; label: string} {
  const item = row as Record<string, unknown>
  const status = String(item.status || '')
  if (status.startsWith('completed') || status === 'cancelled' || !item.due_at) return {overdue: false, label: ''}
  const dueDay = new Date(String(item.due_at))
  if (!Number.isFinite(dueDay.getTime())) return {overdue: false, label: ''}
  dueDay.setHours(0, 0, 0, 0)
  const today = new Date(); today.setHours(0, 0, 0, 0)
  if (dueDay.getTime() >= today.getTime()) return {overdue: false, label: ''}
  return {overdue: true, label: `逾期 ${Math.max(1, Math.round((today.getTime() - dueDay.getTime()) / 86_400_000))} 天`}
}

export function departmentTaskStatusTone(value: unknown): StatusTone {
  if (value === 'completed') return 'success'
  if (value === 'partial_completed') return 'warning'
  return 'info'
}

export function workorderActionLabel(value: unknown): string {
  const labels: Record<string, string> = {create: '创建', dispatch: '派发', dispatch_department: '部门收到', department_start: '部门处理', department_partial_complete: '部分完成', department_complete: '部门完成', pending_close: '待确认', pause: '暂停', resume: '恢复', urgent: '加急', complete_normal: '正常完成', complete_forced: '强制完成'}
  return labels[String(value)] || String(value || '-')
}
