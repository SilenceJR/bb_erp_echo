import {nextTick} from 'vue'
import {ElMessage} from 'element-plus'
import {request} from '../api/http'
import {modules, type ModuleItem} from '../data/modules'
import type {BasicItem, CurrentUser, PaginatedResponse, SkeletonResponse} from '../types'
import {appMessageBox} from './useAppMessageBox'
import {isModuleNotInitialized} from '../platform/moduleAvailability'

type Dependencies = Record<string, any>

/** Owns generic directory listing, paging and configurable create/edit forms. */
export function useDirectoryOperations(d: Dependencies) {
  let confirmedWarehouseTab = 'product'
  const numericKeys = new Set(['quantity', 'unit_cost', 'default_cost', 'safety_stock', 'customer_id', 'product_id'])

  function inferColumns(data: BasicItem[], item: ModuleItem): string[] {
    const preferred: Record<string, string[]> = {
      users: ['id', 'username', 'account_type', 'name', 'organization_id', 'department_id', 'terminal_id', 'status'],
      departments: ['id', 'organization_id', 'name', 'code', 'status'],
      terminals: ['id', 'department_id', 'code', 'name', 'location', 'status'],
      roles: ['id', 'name', 'code', 'description'],
      suppliers: ['id', 'name', 'code', 'contact', 'phone', 'address', 'status'],
      warehouses: ['id', 'item_type', 'category', 'name', 'code', 'unit', 'spec', 'safety_stock', 'status'],
      materials: ['id', 'name', 'code', 'category', 'unit', 'spec', 'safety_stock', 'status'],
      products: ['id', 'name', 'code', 'unit', 'spec', 'safety_stock', 'status'],
      audits: ['id', 'operator_employee_name', 'operator_department_name', 'actor_username', 'terminal_id', 'action', 'object', 'result', 'created_at'],
    }
    const inferred = preferred[item.key] || Object.keys(data[0] || {})
    return d.hasPermission('cost:view') ? inferred : inferred.filter((column) => !['avg_cost', 'amount', 'unit_cost', 'default_cost'].includes(column))
  }

  function resetListQuery() {
    d.searchKeyword.value = ''
    d.page.value = 1
    d.pageTotal.value = 0
    d.workorderStatusFilter.value = ''
    d.workorderTypeFilter.value = ''
    d.workorderPriorityFilter.value = ''
  }

  function applySearch() { d.page.value = 1; void loadActiveModule() }
  function handlePageChange(value: number) { d.page.value = value; void loadActiveModule() }
  function handlePageSizeChange(value: number) { d.pageSize.value = value; d.page.value = 1; void loadActiveModule() }
  function resetFilters() {
    d.searchKeyword.value = ''
    d.workorderStatusFilter.value = ''
    d.workorderTypeFilter.value = ''
    d.workorderPriorityFilter.value = ''
    applySearch()
  }

  async function switchWarehouseTab(key: string) {
    const previous = confirmedWarehouseTab
    if (key === previous) return
    if (d.showCreateForm.value && d.loading.value) { d.activeWarehouseTab.value = previous; return }
    if (d.showCreateForm.value && createFormDirty()) {
      try { await appMessageBox.confirm('仓库新增表单尚未保存，切换分类将放弃当前内容。', '切换仓库分类？', {type: 'warning'}) } catch { d.activeWarehouseTab.value = previous; return }
    }
    confirmedWarehouseTab = key
    d.activeWarehouseTab.value = key
    resetListQuery()
    clearForm()
    void loadActiveModule()
  }

  async function preloadBaseData() {
    const candidates = [
      {key: 'departments', permission: 'system:departments:read'}, {key: 'terminals', permission: 'system:terminals:read'},
      {key: 'materials', permission: 'material:read'}, {key: 'products', permission: 'product:read'},
      {key: 'customers', permission: 'customers:read'}, {key: 'suppliers', permission: 'suppliers:read'},
    ]
    await Promise.allSettled(candidates.filter((item) => d.hasPermission(item.permission)).map((item) => loadList(item.key, false)))
  }

  async function loadActiveModule() {
    const item = d.activeModule.value as ModuleItem | undefined
    if (!item || item.key === 'dashboard') return
    if (!d.canReadModule(item)) { d.activeKey.value = 'dashboard'; d.panelMessage.value = '你的账号暂无该功能权限'; return }
    d.loading.value = true
    d.panelMessage.value = ''
    d.listError.value = ''
    d.moduleUnavailable.value = null
    d.skeletonResult.value = null
    try {
      if (['customers', 'updates'].includes(item.key)) {
        d.rows.value = []; d.columns.value = []; d.pageTotal.value = 0
      } else if (item.key === 'statistics') await d.loadStatistics()
      else await loadList(item.key, true)
      d.panelMessage.value = '已刷新'
    } catch (error) {
      if (isModuleNotInitialized(error)) {
        d.moduleUnavailable.value = {module: item.key, message: error.message || '此功能暂不可用'}
        d.panelMessage.value = '此功能暂不可用'
        return
      }
      d.listError.value = error instanceof Error ? error.message : '加载失败'
      d.panelMessage.value = d.listError.value
    } finally { d.loading.value = false }
  }

  async function loadList(key: string, applyToPanel: boolean) {
    const item = modules.find((candidate) => candidate.key === key)
    let path = String(item?.path || '')
    if (key === 'customers' && !applyToPanel) {
      const options = await request<BasicItem[]>('/api/v1/customers/options', {}, d.token.value)
      d.cache[key] = options.map((option) => ({...option, name: String(option.short_name || option.name || option.code || `#${option.id}`)}))
      return
    }
    if (key === 'warehouse_records') path = '/api/v1/warehouses'
    if (!path) return
    if (key === 'warehouses') path = d.appendQuery(path, {tab: d.activeWarehouseTab.value})
    path = applyToPanel
      ? d.appendQuery(path, {page: d.page.value, page_size: d.pageSize.value, q: d.searchKeyword.value, status: key === 'workorder' ? d.workorderStatusFilter.value : undefined, type: key === 'workorder' ? d.workorderTypeFilter.value : undefined, priority: key === 'workorder' ? d.workorderPriorityFilter.value : undefined})
      : d.appendQuery(path, {page: 1, page_size: 200})
    const data = await request<BasicItem[] | PaginatedResponse<BasicItem> | SkeletonResponse>(path, {}, d.token.value)
    if (!Array.isArray(data) && Array.isArray((data as PaginatedResponse<BasicItem>).items)) {
      const paged = data as PaginatedResponse<BasicItem>
      d.cache[key] = paged.items
      if (applyToPanel) {
        d.rows.value = paged.items; d.page.value = paged.page; d.pageSize.value = paged.page_size; d.pageTotal.value = paged.total
        if (item) d.columns.value = inferColumns(paged.items, item)
      }
      return
    }
    if (!Array.isArray(data)) {
      if (applyToPanel) { d.skeletonResult.value = data; d.rows.value = []; d.columns.value = []; d.pageTotal.value = 0 }
      return
    }
    d.cache[key] = data
    if (applyToPanel) { d.rows.value = data; d.pageTotal.value = data.length; if (item) d.columns.value = inferColumns(data, item) }
  }

  function normalizedForm(): Record<string, unknown> {
    const body: Record<string, unknown> = {}
    if (d.activeKey.value === 'users') body.organization_id = (d.currentUser.value as CurrentUser | null)?.organization_id
    if (d.activeKey.value === 'warehouses') body.tab = d.activeWarehouseTab.value
    for (const field of d.formSchema.value) {
      const value = d.formState[field.key]
      if (value === '' || value === undefined) continue
      if (d.activeKey.value === 'warehouses' && field.key === 'safety_stock') { body[field.key] = d.decimalToScaled(value); continue }
      if (d.activeKey.value === 'warehouses' && field.key === 'default_cost') { body[field.key] = d.moneyToCents(value); continue }
      if (d.activeKey.value === 'workorder' && field.key === 'planned_quantity') { body[field.key] = d.decimalToScaled(value); continue }
      if (d.activeKey.value === 'workorder' && field.key === 'target_department_ids') { body[field.key] = Array.isArray(value) ? value.map(Number) : []; continue }
      body[field.key] = numericKeys.has(field.key) || field.key.endsWith('_id') ? Number(value) : value
    }
    return body
  }

  function validateActiveForm(): string {
    if (d.activeKey.value === 'workorder' && d.formState.type === 'production' && !d.hasPermission('warehouse:read')) return '当前账号缺少仓库查看权限，无法选择产品或创建生产单；请联系管理员授权或改为通用任务。'
    const missing = d.formSchema.value.filter((field: any) => field.required && (d.formState[field.key] === undefined || d.formState[field.key] === null || (typeof d.formState[field.key] === 'string' && !d.formState[field.key].trim())))
    if (missing.length) return `请填写必填项：${missing.map((field: any) => field.label).join('、')}`
    if (d.activeKey.value === 'users') {
      if (!d.currentUser.value?.organization_id) return '当前登录身份缺少组织信息，请重新登录后再试。'
      if (String(d.formState.password || '').length < 8) return '密码至少需要 8 个字符。'
      if (d.formState.account_type === 'department_terminal' && !d.canCreateDepartmentTerminalUser.value) return '创建部门终端账号需要部门查看和终端查看权限。'
      if (d.formState.account_type === 'department_terminal') {
        const terminal = d.rowsFor('terminals').find((item: BasicItem) => Number(item.id) === Number(d.formState.terminal_id))
        if (!terminal || Number(terminal.department_id) !== Number(d.formState.department_id)) return '所选终端不属于当前部门，请重新选择部门和终端。'
      }
    }
    if (d.activeKey.value === 'workorder') {
      if (!Array.isArray(d.formState.target_department_ids) || !d.formState.target_department_ids.length) return '请选择至少一个流转部门。'
      if (d.formState.type === 'production') {
        const quantity = String(d.formState.planned_quantity || '').trim()
        if (!/^\d+(\.\d{1,4})?$/.test(quantity) || Number(quantity) <= 0) return '生产单计划数量必须大于 0，且最多保留 4 位小数。'
      }
    }
    return ''
  }

  function clearForm() {
    d.resetWorkorderProductSelection()
    for (const key of Object.keys(d.formState)) delete d.formState[key]
    d.formError.value = ''
  }

  function createFormDirty() {
    return Object.entries(d.formState).some(([key, value]) => {
      if (d.activeKey.value === 'workorder' && ((key === 'type' && value === 'production') || (key === 'priority' && value === 'normal'))) return false
      if (d.editingSupplier.value && value === d.editingSupplier.value[key]) return false
      return Array.isArray(value) ? value.length > 0 : value !== '' && value !== undefined && value !== null
    })
  }

  async function toggleCreateForm() {
    if (d.showCreateForm.value && d.loading.value) return
    if (d.showCreateForm.value && createFormDirty()) {
      try { await appMessageBox.confirm('当前表单尚未保存，确认关闭？', '放弃修改', {type: 'warning'}) } catch { return }
    }
    d.editingSupplier.value = null
    clearForm()
    d.showCreateForm.value = !d.showCreateForm.value
    if (d.showCreateForm.value && ['warehouses', 'workorder'].includes(d.activeKey.value)) void d.operatorDirectory.load(true)
    if (d.showCreateForm.value && d.activeKey.value === 'workorder') {
      d.formState.type = 'production'; d.formState.priority = 'normal'; void d.searchWorkorderProducts('')
    } else if (!d.showCreateForm.value) { d.invalidateWorkorderProductSearch(); d.closeTemporaryProductDialog() }
  }

  async function editSupplier(value: unknown) {
    const item = value as BasicItem
    if (d.showCreateForm.value && Number(d.editingSupplier.value?.id || 0) === Number(item.id)) return
    if (d.showCreateForm.value && d.loading.value) {
      ElMessage.warning('资料正在保存，请等待完成后再切换。')
      return
    }
    if (d.showCreateForm.value && createFormDirty()) {
      try { await appMessageBox.confirm('当前供应商资料尚未保存，切换后修改将丢失。', '切换编辑对象？', {confirmButtonText: '放弃并切换', cancelButtonText: '继续编辑', type: 'warning'}) } catch { return }
    }
    d.editingSupplier.value = item
    clearForm()
    for (const key of ['name', 'code', 'contact', 'phone', 'address', 'status']) {
      const value = item[key]
      if (typeof value === 'string' || typeof value === 'number') d.formState[key] = value
    }
    d.showCreateForm.value = true
  }

  async function createItem(): Promise<BasicItem | null> {
    const item = d.activeModule.value as ModuleItem | undefined
    if (d.moduleUnavailable.value) {
      d.formError.value = d.moduleUnavailable.value.message || '此功能暂不可用，无法保存'
      d.panelMessage.value = d.formError.value
      return null
    }
    if (!item?.path || !d.canWriteModule(item)) { d.panelMessage.value = '你的账号只有查看权限，不能新增数据'; d.showCreateForm.value = false; return null }
    d.formError.value = validateActiveForm()
    if (d.formError.value) {
      if (['warehouses', 'workorder'].includes(d.activeKey.value) && !d.formState.operator_employee_id) { await nextTick(); document.getElementById('create-form-operator')?.focus() }
      return null
    }
    d.loading.value = true
    d.panelMessage.value = ''
    try {
      const isSupplierEdit = d.activeKey.value === 'suppliers' && d.editingSupplier.value
      const saved = await request<BasicItem>(isSupplierEdit ? `${item.path}/${d.editingSupplier.value.id}` : item.path, {method: isSupplierEdit ? 'PATCH' : 'POST', body: normalizedForm()}, d.token.value)
      if (['warehouses', 'workorder'].includes(d.activeKey.value)) d.operatorDirectory.invalidate()
      if (item.key === 'roles') delete d.assignmentOptionsCache[item.key]
      clearForm(); await preloadBaseData(); await loadActiveModule()
      d.panelMessage.value = isSupplierEdit ? '已保存' : '已新增'
      ElMessage.success(isSupplierEdit ? '保存成功' : '新增成功')
      d.editingSupplier.value = null
      return saved
    } catch (error) {
      if (['warehouses', 'workorder'].includes(d.activeKey.value) && d.operatorDirectory.handleSubmitError(error)) d.formState.operator_employee_id = undefined
      d.formError.value = error instanceof Error ? error.message : '保存失败'; d.panelMessage.value = d.formError.value
      return null
    } finally { d.loading.value = false }
  }

  return {resetFilters, switchWarehouseTab, resetListQuery, applySearch, handlePageChange, handlePageSizeChange, preloadBaseData, loadActiveModule, loadList, createItem, clearForm, toggleCreateForm, editSupplier, createFormDirty}
}
