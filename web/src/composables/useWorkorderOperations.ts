import {nextTick, type Ref} from 'vue'
import {ElMessage} from 'element-plus'
import {request} from '../api/http'
import type {BasicItem, PaginatedResponse} from '../types'
import {appMessageBox} from './useAppMessageBox'
import type {useWorkorder} from './useWorkorder'

type WorkorderState = ReturnType<typeof useWorkorder>
type WorkorderDependencies = {
  token: Ref<string>
  activeKey: Ref<string>
  showCreateForm: Ref<boolean>
  formState: Record<string, any>
  hasPermission: (code?: string) => boolean
  operatorDirectory: any
  appendQuery: (path: string, params: Record<string, string | number | undefined>) => string
  panelMessage: Ref<string>
  reloadActiveModule: () => Promise<void>
}

/** Owns task-product lookup, task detail requests and their cancellation boundaries. */
export function useWorkorderOperations(state: WorkorderState, dependencies: WorkorderDependencies) {
  const {
    token, activeKey, showCreateForm, formState, hasPermission, operatorDirectory,
    appendQuery, panelMessage, reloadActiveModule,
  } = dependencies
  const {
    selectedWorkOrder, workorderDrawerVisible, workorderLogs, workorderLogsLoading, workorderLogsError,
    workorderProductOptions, workorderProductSearchLoading, workorderProductSearchError,
    workorderProductStock, workorderProductStockLoading, workorderProductStockError, workorderProductStockUpdatedAt,
    workorderDrawerProductStock, workorderDrawerProductStockLoading, workorderDrawerProductStockError,
    workorderDrawerProductStockUpdatedAt, temporaryProductDialogVisible, temporaryProductSubmitting,
    temporaryProductError, temporaryProductForm,
    actionDialogVisible, actionKind, actionTarget, actionSubmitting, actionError,
    actionFieldErrors, actionForm,
  } = state

  let logsToken = 0
  let searchToken = 0
  let stockToken = 0
  let drawerStockToken = 0
  let logsAbort: AbortController | null = null
  let searchAbort: AbortController | null = null
  let stockAbort: AbortController | null = null
  let drawerStockAbort: AbortController | null = null

  const decimalToScaled = (value: unknown) => Math.round(Number(value || 0) * 10_000)

  function safeProduct(item: BasicItem): BasicItem {
    const safe = {...item}
    delete safe.default_cost
    delete safe.avg_cost
    delete safe.amount
    return safe
  }

  function normalizeProduct(data: BasicItem, fallback?: BasicItem): BasicItem {
    const nested = data.item && typeof data.item === 'object' && !Array.isArray(data.item) ? data.item as BasicItem : data
    return safeProduct({
      ...(fallback || {}), ...nested,
      quantity: data.quantity ?? nested.quantity ?? fallback?.quantity ?? 0,
      safety_stock: nested.safety_stock ?? data.safety_stock ?? fallback?.safety_stock ?? 0,
      item_type: 'product',
    })
  }

  function isProductionCreateActive() {
    return activeKey.value === 'workorder' && showCreateForm.value && formState.type === 'production'
  }

  function invalidateWorkorderProductSearch() {
    searchAbort?.abort()
    searchAbort = null
    searchToken += 1
    workorderProductSearchLoading.value = false
  }

  async function searchWorkorderProducts(keyword = '') {
    if (!isProductionCreateActive()) return
    if (!hasPermission('warehouse:read')) {
      workorderProductOptions.value = []
      workorderProductSearchError.value = ''
      return
    }
    searchAbort?.abort()
    const abortController = new AbortController()
    searchAbort = abortController
    const requestToken = ++searchToken
    workorderProductSearchLoading.value = true
    workorderProductSearchError.value = ''
    try {
      const path = appendQuery('/api/v1/warehouse/items', {tab: 'product', q: keyword.trim(), page: 1, page_size: 50})
      const data = await request<PaginatedResponse<BasicItem> | BasicItem[]>(path, {signal: abortController.signal}, token.value)
      if (requestToken !== searchToken || !isProductionCreateActive()) return
      workorderProductOptions.value = (Array.isArray(data) ? data : data.items).map(safeProduct)
    } catch (error) {
      if (requestToken === searchToken && isProductionCreateActive()) workorderProductSearchError.value = error instanceof Error ? error.message : '仓库产品搜索失败，请重试。'
    } finally {
      if (searchAbort === abortController) searchAbort = null
      if (requestToken === searchToken) workorderProductSearchLoading.value = false
    }
  }

  function handleWorkorderProductSelect(productID: unknown) {
    const id = Number(productID || 0)
    if (!id) { resetWorkorderProductSelection(); return }
    const option = workorderProductOptions.value.find((item) => Number(item.id) === id)
    workorderProductStock.value = option ? safeProduct(option) : null
    workorderProductStockError.value = ''
    workorderProductStockUpdatedAt.value = ''
    void loadWorkorderProductStock()
  }

  function resetWorkorderProductSelection() {
    stockAbort?.abort()
    stockAbort = null
    stockToken += 1
    delete formState.product_id
    workorderProductStock.value = null
    workorderProductStockLoading.value = false
    workorderProductStockError.value = ''
    workorderProductStockUpdatedAt.value = ''
  }

  async function loadWorkorderProductStock() {
    const productID = Number(formState.product_id || 0)
    if (!productID || !isProductionCreateActive()) return
    if (!hasPermission('warehouse:read')) { workorderProductStockLoading.value = false; workorderProductStockError.value = ''; return }
    stockAbort?.abort()
    const abortController = new AbortController()
    stockAbort = abortController
    const requestToken = ++stockToken
    workorderProductStockLoading.value = true
    workorderProductStockError.value = ''
    const fallback = workorderProductOptions.value.find((item) => Number(item.id) === productID) || workorderProductStock.value || undefined
    try {
      const data = await request<BasicItem>(`/api/v1/warehouse/items/product/${productID}`, {signal: abortController.signal}, token.value)
      if (requestToken !== stockToken || !isProductionCreateActive() || Number(formState.product_id) !== productID) return
      workorderProductStock.value = normalizeProduct(data, fallback)
      workorderProductStockUpdatedAt.value = new Date().toISOString()
    } catch (error) {
      if (requestToken === stockToken && isProductionCreateActive() && Number(formState.product_id) === productID) workorderProductStockError.value = error instanceof Error ? error.message : '库存数量加载失败，请重试。'
    } finally {
      if (stockAbort === abortController) stockAbort = null
      if (requestToken === stockToken && Number(formState.product_id) === productID) workorderProductStockLoading.value = false
    }
  }

  function openTemporaryProductDialog() {
    if (!hasPermission('warehouse:read') || !hasPermission('workorder:write') || !hasPermission('workorder:temporary-product:write')) return
    Object.assign(temporaryProductForm, {name: '', code: '', unit: '个', spec: '', operator_employee_id: undefined})
    formState.operator_employee_id = undefined
    temporaryProductError.value = ''
    temporaryProductDialogVisible.value = true
    void operatorDirectory.load(true)
  }

  function closeTemporaryProductDialog() {
    if (temporaryProductSubmitting.value) return
    temporaryProductDialogVisible.value = false
    temporaryProductError.value = ''
  }

  async function closeTemporaryProductWithGuard(done?: () => void) {
    if (temporaryProductSubmitting.value) return
    const dirty = Boolean(temporaryProductForm.name || temporaryProductForm.code || temporaryProductForm.spec || temporaryProductForm.operator_employee_id || temporaryProductForm.unit !== '个')
    if (dirty) {
      try { await appMessageBox.confirm('临时产品信息尚未保存，确认关闭？', '放弃修改', {type: 'warning'}) } catch { return }
    }
    temporaryProductError.value = ''
    if (done) done(); else temporaryProductDialogVisible.value = false
  }

  async function createTemporaryProduct() {
    if (!hasPermission('warehouse:read') || !hasPermission('workorder:write') || !hasPermission('workorder:temporary-product:write')) { temporaryProductError.value = '当前账号没有临时新增产品的权限。'; return }
    const name = temporaryProductForm.name.trim()
    const code = temporaryProductForm.code.trim()
    const unit = temporaryProductForm.unit.trim()
    if (!name || !code || !unit) { temporaryProductError.value = '请填写产品名称、产品编码和单位。'; return }
    if (!temporaryProductForm.operator_employee_id || operatorDirectory.unavailableReason.value) { temporaryProductError.value = operatorDirectory.unavailableReason.value || '请选择本次操作人。'; return }
    invalidateWorkorderProductSearch()
    temporaryProductSubmitting.value = true
    temporaryProductError.value = ''
    try {
      const created = await request<BasicItem>('/api/v1/workorder/products', {method: 'POST', body: {name, code, unit, spec: temporaryProductForm.spec.trim(), operator_employee_id: Number(temporaryProductForm.operator_employee_id)}}, token.value)
      const product = normalizeProduct(created)
      if (!isProductionCreateActive()) return
      invalidateWorkorderProductSearch()
      workorderProductOptions.value = [product, ...workorderProductOptions.value.filter((item) => Number(item.id) !== Number(product.id))]
      formState.product_id = Number(product.id)
      workorderProductStock.value = product
      temporaryProductDialogVisible.value = false
      operatorDirectory.invalidate()
      ElMessage.success('产品档案已新增并选中，初始库存为 0。')
      await loadWorkorderProductStock()
    } catch (error) {
      if (operatorDirectory.handleSubmitError(error)) temporaryProductForm.operator_employee_id = undefined
      temporaryProductError.value = error instanceof Error ? error.message : '临时产品建档失败，请检查编码后重试。'
    } finally { temporaryProductSubmitting.value = false }
  }

  function invalidateWorkorderDrawerProductStock() {
    drawerStockAbort?.abort()
    drawerStockAbort = null
    drawerStockToken += 1
    workorderDrawerProductStockLoading.value = false
  }

  async function loadWorkorderDrawerProductStock() {
    const workorderID = Number(selectedWorkOrder.value?.id || 0)
    const productID = Number(selectedWorkOrder.value?.product_id || 0)
    if (!workorderID || !productID) return
    if (!hasPermission('warehouse:read')) { workorderDrawerProductStockLoading.value = false; workorderDrawerProductStockError.value = ''; return }
    drawerStockAbort?.abort()
    const abortController = new AbortController()
    drawerStockAbort = abortController
    const requestToken = ++drawerStockToken
    workorderDrawerProductStockLoading.value = true
    workorderDrawerProductStockError.value = ''
    const fallback = workorderDrawerProductStock.value || {id: productID, name: String(selectedWorkOrder.value?.product_name || ''), unit: String(selectedWorkOrder.value?.unit || '')}
    try {
      const data = await request<BasicItem>(`/api/v1/warehouse/items/product/${productID}`, {signal: abortController.signal}, token.value)
      if (requestToken !== drawerStockToken || !workorderDrawerVisible.value || Number(selectedWorkOrder.value?.id) !== workorderID || Number(selectedWorkOrder.value?.product_id) !== productID) return
      workorderDrawerProductStock.value = normalizeProduct(data, fallback)
      workorderDrawerProductStockUpdatedAt.value = new Date().toISOString()
    } catch (error) {
      if (requestToken === drawerStockToken && workorderDrawerVisible.value && Number(selectedWorkOrder.value?.id) === workorderID && Number(selectedWorkOrder.value?.product_id) === productID) workorderDrawerProductStockError.value = error instanceof Error ? error.message : '库存数量加载失败，请重试。'
    } finally {
      if (drawerStockAbort === abortController) drawerStockAbort = null
      if (requestToken === drawerStockToken && Number(selectedWorkOrder.value?.id) === workorderID) workorderDrawerProductStockLoading.value = false
    }
  }

  function invalidateWorkOrderLogsRequest() {
    logsAbort?.abort()
    logsAbort = null
    logsToken += 1
    workorderLogsLoading.value = false
  }

  function isCurrentLogsRequest(requestToken: number, workorderID: number) {
    return requestToken === logsToken && workorderDrawerVisible.value && Number(selectedWorkOrder.value?.id) === workorderID
  }

  async function openWorkOrder(value: unknown) {
    const item = value as BasicItem
    if (workorderDrawerVisible.value && Number(selectedWorkOrder.value?.id) === Number(item.id) && !actionDialogVisible.value) return
    if (actionDialogVisible.value) {
      let allowed = false
      await closeWorkOrderAction(() => {
        allowed = true
        actionDialogVisible.value = false
      })
      if (!allowed) return
    }
    selectedWorkOrder.value = item
    workorderLogs.value = []
    workorderLogsError.value = ''
    workorderDrawerProductStock.value = null
    workorderDrawerProductStockError.value = ''
    workorderDrawerProductStockUpdatedAt.value = ''
    workorderDrawerVisible.value = true
    void loadWorkOrderLogs()
    if (Number(item.product_id || 0) && hasPermission('warehouse:read')) void loadWorkorderDrawerProductStock()
  }

  function closeWorkOrder() { invalidateWorkOrderLogsRequest(); invalidateWorkorderDrawerProductStock(); workorderDrawerVisible.value = false }
  function handleWorkOrderBeforeClose(done: () => void) { invalidateWorkOrderLogsRequest(); invalidateWorkorderDrawerProductStock(); done() }
  function resetWorkOrder() {
    invalidateWorkOrderLogsRequest(); invalidateWorkorderDrawerProductStock()
    selectedWorkOrder.value = null
    workorderLogs.value = []
    workorderLogsLoading.value = false
    workorderLogsError.value = ''
    workorderDrawerProductStock.value = null
    workorderDrawerProductStockLoading.value = false
    workorderDrawerProductStockError.value = ''
    workorderDrawerProductStockUpdatedAt.value = ''
  }

  async function loadWorkOrderLogs() {
    const workorderID = Number(selectedWorkOrder.value?.id)
    if (!workorderID) return
    logsAbort?.abort()
    const abortController = new AbortController()
    logsAbort = abortController
    const requestToken = ++logsToken
    workorderLogsLoading.value = true
    workorderLogsError.value = ''
    try {
      const data = await request<BasicItem[]>(`/api/v1/workorder/${workorderID}/logs`, {signal: abortController.signal}, token.value)
      if (isCurrentLogsRequest(requestToken, workorderID)) workorderLogs.value = data
    } catch (error) {
      if (isCurrentLogsRequest(requestToken, workorderID)) { workorderLogs.value = []; workorderLogsError.value = error instanceof Error ? error.message : '任务日志加载失败' }
    } finally {
      if (logsAbort === abortController) logsAbort = null
      if (isCurrentLogsRequest(requestToken, workorderID)) workorderLogsLoading.value = false
    }
  }

  function openWorkOrderAction(kind: string, target: BasicItem | null = null) {
    actionKind.value = kind
    actionTarget.value = target
    Object.assign(actionForm, {operator_employee_id: undefined, reason: '', remark: '', completed_quantity: ''})
    actionError.value = ''
    Object.assign(actionFieldErrors, {operator: '', reason: '', quantity: ''})
    actionDialogVisible.value = true
    void operatorDirectory.load(true)
  }

  const dispatchWorkOrder = () => openWorkOrderAction('dispatch')
  const pauseWorkOrder = () => openWorkOrderAction('pause')
  const resumeWorkOrder = () => openWorkOrderAction('resume')
  const toggleWorkOrderUrgent = () => openWorkOrderAction('urgent')
  const completeWorkOrder = (mode: 'normal' | 'forced') => openWorkOrderAction(mode === 'normal' ? 'complete_normal' : 'complete_forced')
  const startDepartmentTask = (task: BasicItem) => openWorkOrderAction('department_start', task)
  const partialCompleteDepartmentTask = (task: BasicItem) => openWorkOrderAction('department_partial_complete', task)
  const completeDepartmentTask = (task: BasicItem) => openWorkOrderAction('department_complete', task)

  async function closeWorkOrderAction(done?: () => void) {
    if (actionSubmitting.value) return
    if (actionForm.operator_employee_id || actionForm.reason || actionForm.remark || actionForm.completed_quantity) {
      try { await appMessageBox.confirm('本次任务操作尚未提交，确认关闭？', '放弃操作', {type: 'warning'}) } catch { return }
    }
    actionTarget.value = null
    actionForm.operator_employee_id = undefined
    actionError.value = ''
    Object.assign(actionFieldErrors, {operator: '', reason: '', quantity: ''})
    if (done) done(); else actionDialogVisible.value = false
  }

  async function focusActionField(id: string) {
    await nextTick()
    document.getElementById(id)?.focus()
  }

  async function submitWorkOrderAction() {
    if (actionSubmitting.value || !selectedWorkOrder.value) return
    if (!actionForm.operator_employee_id || operatorDirectory.unavailableReason.value) {
      if (operatorDirectory.unavailableReason.value) actionError.value = operatorDirectory.unavailableReason.value
      else actionFieldErrors.operator = '请选择本次操作人。'
      await focusActionField('workorder-action-operator')
      return
    }
    const id = Number(selectedWorkOrder.value.id)
    const taskID = Number(actionTarget.value?.id || 0)
    let path = ''
    let success = ''
    const body: Record<string, unknown> = {operator_employee_id: Number(actionForm.operator_employee_id)}
    switch (actionKind.value) {
      case 'dispatch': path = `/api/v1/workorder/${id}/dispatch`; success = '任务已派发'; break
      case 'pause': path = `/api/v1/workorder/${id}/pause`; body.reason = actionForm.reason.trim(); success = '任务已暂停'; break
      case 'resume': path = `/api/v1/workorder/${id}/resume`; success = '任务已恢复'; break
      case 'urgent': path = `/api/v1/workorder/${id}/urgent`; body.urgent = selectedWorkOrder.value.priority !== 'urgent'; success = body.urgent ? '已设为加急' : '已取消加急'; break
      case 'complete_normal': path = `/api/v1/workorder/${id}/complete`; body.mode = 'normal'; body.reason = ''; success = '任务已正常完成'; break
      case 'complete_forced': path = `/api/v1/workorder/${id}/complete`; body.mode = 'forced'; body.reason = actionForm.reason.trim(); success = '任务已强制完成'; break
      case 'department_start': path = `/api/v1/workorder/department-tasks/${taskID}/start`; success = '已开始处理'; break
      case 'department_partial_complete': path = `/api/v1/workorder/department-tasks/${taskID}/partial-complete`; body.remark = actionForm.remark.trim(); success = '部分完成已提交'; break
      case 'department_complete': path = `/api/v1/workorder/department-tasks/${taskID}/complete`; body.remark = actionForm.remark.trim(); success = '部门任务已完成'; break
    }
    if (!path) return
    if (['pause', 'complete_forced'].includes(actionKind.value) && !String(body.reason || '').trim()) {
      actionFieldErrors.reason = '请填写原因。'
      await focusActionField('workorder-action-reason')
      return
    }
    if (actionKind.value === 'department_partial_complete') {
      const quantityInput = actionForm.completed_quantity.trim()
      if (!quantityInput) { actionFieldErrors.quantity = '请输入累计完成数量。'; await focusActionField('workorder-action-quantity'); return }
      if (!/^\d+(\.\d{1,4})?$/.test(quantityInput)) { actionFieldErrors.quantity = '请输入正数，最多保留 4 位小数。'; await focusActionField('workorder-action-quantity'); return }
      const quantity = decimalToScaled(quantityInput)
      const planned = Number(actionTarget.value?.planned_quantity || 0)
      const completed = Number(actionTarget.value?.completed_quantity || 0)
      if (quantity <= 0 || quantity >= planned) { actionFieldErrors.quantity = '累计完成数量必须大于 0 且小于计划数量。'; await focusActionField('workorder-action-quantity'); return }
      if (quantity <= completed) { actionFieldErrors.quantity = '累计完成数量必须大于当前已完成数量。'; await focusActionField('workorder-action-quantity'); return }
      body.completed_quantity = quantity
    }
    actionSubmitting.value = true
    actionError.value = ''
    try {
      selectedWorkOrder.value = await request<BasicItem>(path, {method: 'POST', body}, token.value)
      await Promise.all([reloadActiveModule(), loadWorkOrderLogs()])
      panelMessage.value = success
      actionDialogVisible.value = false
      actionTarget.value = null
      actionForm.operator_employee_id = undefined
      Object.assign(actionFieldErrors, {operator: '', reason: '', quantity: ''})
      operatorDirectory.invalidate()
      ElMessage.success(success)
    } catch (error) {
      if (operatorDirectory.handleSubmitError(error)) actionForm.operator_employee_id = undefined
      actionError.value = error instanceof Error ? error.message : '任务操作失败'
    } finally { actionSubmitting.value = false }
  }

  function dispose() {
    invalidateWorkorderProductSearch()
    resetWorkorderProductSelection()
    invalidateWorkorderDrawerProductStock()
    invalidateWorkOrderLogsRequest()
  }

  return {
    invalidateWorkorderProductSearch, searchWorkorderProducts, handleWorkorderProductSelect, resetWorkorderProductSelection, loadWorkorderProductStock,
    openTemporaryProductDialog, closeTemporaryProductDialog, closeTemporaryProductWithGuard, createTemporaryProduct,
    loadWorkorderDrawerProductStock, openWorkOrder, closeWorkOrder, handleWorkOrderBeforeClose, resetWorkOrder,
    loadWorkOrderLogs, dispatchWorkOrder, pauseWorkOrder, resumeWorkOrder, toggleWorkOrderUrgent,
    completeWorkOrder, startDepartmentTask, partialCompleteDepartmentTask, completeDepartmentTask,
    openWorkOrderAction, closeWorkOrderAction, submitWorkOrderAction, dispose,
  }
}
