import type {ComputedRef, Ref} from 'vue'
import {ElMessage} from 'element-plus'
import {ApiError, request} from '../api/http'
import type {BasicItem} from '../types'
import {createIdempotencyKey} from './workspacePresentation'
import {appMessageBox} from './useAppMessageBox'
import {movementDefinitions} from './useModuleConfiguration'
import type {useWarehouse} from './useWarehouse'

type WarehouseState = ReturnType<typeof useWarehouse>
type Dependencies = {
  token: Ref<string>
  panelMessage: Ref<string>
  rows: Ref<BasicItem[]>
  hasPermission: (code: string) => boolean
  movementFormDirty: ComputedRef<boolean>
  warehouseQuantityAvailable: ComputedRef<boolean>
  movementIsOutbound: ComputedRef<boolean>
  expectedStockQuantity: ComputedRef<number>
  movementCanSubmit: ComputedRef<boolean>
  operatorDirectory: any
  reloadActiveModule: () => Promise<unknown>
  loadList: (key: string, applyToPanel: boolean) => Promise<unknown>
}

/** Complete warehouse drawer request, close-guard and inventory-posting workflow. */
export function useWarehouseOperations(state: WarehouseState, deps: Dependencies) {
  const {
    selectedWarehouseItem, warehouseDrawerVisible, warehouseDetail, warehouseDetailLoading, warehouseDetailError,
    itemMovements, itemMovementsLoading, itemMovementsError, showAllItemMovements, movementMode,
    showQuickSupplier, movementSubmitting, movementFormError, quickSupplierSubmitting, quickSupplierError,
    movementForm, quickSupplier,
  } = state
  let detailToken = 0; let movementsToken = 0
  let detailController: AbortController | null = null; let movementsController: AbortController | null = null
  let closeBypass = false; let requestSnapshot = ''; let idempotencyKey = ''

  function invalidateRequests() { detailController?.abort(); movementsController?.abort(); detailController = null; movementsController = null; detailToken += 1; movementsToken += 1; warehouseDetailLoading.value = false; itemMovementsLoading.value = false }
  function isCurrent(token: number, itemType: string, itemID: number, kind: 'detail' | 'movements') { return token === (kind === 'detail' ? detailToken : movementsToken) && warehouseDrawerVisible.value && String(selectedWarehouseItem.value?.item_type || '') === itemType && Number(selectedWarehouseItem.value?.id) === itemID }

  async function openWarehouseItem(item: any) {
    selectedWarehouseItem.value = item; warehouseDetail.value = null; warehouseDetailError.value = ''; warehouseDrawerVisible.value = true
    movementMode.value = ''; showAllItemMovements.value = false; itemMovements.value = []; itemMovementsError.value = ''; deps.panelMessage.value = ''
    await Promise.allSettled([loadWarehouseItemDetail(), loadItemMovements()])
  }
  async function closeWarehouseItem() { if (await requestWarehouseClose()) performWarehouseClose() }
  function performWarehouseClose() { invalidateRequests(); closeBypass = true; warehouseDrawerVisible.value = false; window.setTimeout(() => { closeBypass = false }, 0) }
  async function requestWarehouseClose() {
    if (movementSubmitting.value) { ElMessage.warning('正在提交库存变动，请等待办理完成后再关闭。'); return false }
    if (!deps.movementFormDirty.value) return true
    try { await appMessageBox.confirm('当前出入库表单尚未提交，关闭后已填写内容将丢失。', '放弃本次办理？', {confirmButtonText: '放弃并关闭', cancelButtonText: '继续填写', type: 'warning'}); return true } catch { return false }
  }
  function handleWarehouseBeforeClose(done: () => void) { if (closeBypass) { closeBypass = false; invalidateRequests(); done(); return }; void requestWarehouseClose().then((allowed) => { if (allowed) { invalidateRequests(); done() } }) }
  function resetWarehouseItem() { invalidateRequests(); closeBypass = false; selectedWarehouseItem.value = null; warehouseDetail.value = null; warehouseDetailLoading.value = false; warehouseDetailError.value = ''; itemMovements.value = []; itemMovementsLoading.value = false; itemMovementsError.value = ''; movementMode.value = ''; showQuickSupplier.value = false; movementFormError.value = ''; quickSupplierError.value = ''; clearMovementForm() }

  async function loadWarehouseItemDetail() {
    const item = selectedWarehouseItem.value; if (!item) return
    const itemType = String(item.item_type || ''); const itemID = Number(item.id)
    detailController?.abort(); const controller = new AbortController(); detailController = controller; const token = ++detailToken
    warehouseDetailLoading.value = true; warehouseDetailError.value = ''; warehouseDetail.value = null
    try { const data = await request<Record<string, unknown>>(`/api/v1/warehouse/items/${itemType}/${itemID}`, {signal: controller.signal}, deps.token.value); if (isCurrent(token, itemType, itemID, 'detail')) warehouseDetail.value = data }
    catch (error) { if (isCurrent(token, itemType, itemID, 'detail')) warehouseDetailError.value = error instanceof Error ? error.message : '库存详情加载失败' }
    finally { if (detailController === controller) detailController = null; if (isCurrent(token, itemType, itemID, 'detail')) warehouseDetailLoading.value = false }
  }
  async function loadItemMovements() {
    const item = selectedWarehouseItem.value; if (!item || !deps.hasPermission('inventory:documents:read')) return
    const itemType = String(item.item_type || ''); const itemID = Number(item.id)
    movementsController?.abort(); const controller = new AbortController(); movementsController = controller; const token = ++movementsToken
    itemMovementsLoading.value = true; itemMovementsError.value = ''
    try { const data = await request<{items: BasicItem[]}>(`/api/v1/warehouse/items/${itemType}/${itemID}/movements?page_size=100`, {signal: controller.signal}, deps.token.value); if (isCurrent(token, itemType, itemID, 'movements')) itemMovements.value = data.items }
    catch (error) { if (isCurrent(token, itemType, itemID, 'movements')) { itemMovements.value = []; itemMovementsError.value = error instanceof Error ? error.message : '出入库记录加载失败' } }
    finally { if (movementsController === controller) movementsController = null; if (isCurrent(token, itemType, itemID, 'movements')) itemMovementsLoading.value = false }
  }
  async function loadAllItemMovements() { showAllItemMovements.value = true; if (itemMovements.value.length < 100) await loadItemMovements() }
  function startMovement(mode: string) { if (movementSubmitting.value || !warehouseDetail.value || !deps.warehouseQuantityAvailable.value || warehouseDetailLoading.value || warehouseDetailError.value) return; movementMode.value = mode; showQuickSupplier.value = false; clearMovementForm(); movementFormError.value = ''; void deps.operatorDirectory.load(true); if (mode === 'return_rework_inbound') movementForm.source_type = deps.hasPermission('customers:read') ? 'customer' : 'department' }
  async function cancelMovement() { if (movementSubmitting.value) return; if (deps.movementFormDirty.value) { try { await appMessageBox.confirm('取消后本次填写内容不会保留。', '取消本次办理？', {confirmButtonText: '确认取消', cancelButtonText: '继续填写', type: 'warning'}) } catch { return } }; movementMode.value = ''; showQuickSupplier.value = false; movementFormError.value = ''; quickSupplierError.value = ''; clearMovementForm() }
  function resetMovementSource() { delete movementForm.customer_id; delete movementForm.department_id; delete movementForm.original_document_id }
  function clearMovementForm() { for (const key of Object.keys(movementForm)) delete movementForm[key]; requestSnapshot = ''; idempotencyKey = '' }

  async function submitMovement() {
    if (movementSubmitting.value) return
    const item = selectedWarehouseItem.value; if (!item || !movementMode.value) return
    const quantity = decimalToScaled(movementForm.quantity)
    if (quantity <= 0) { movementFormError.value = '数量必须大于 0。'; return }
    if (deps.movementIsOutbound.value && deps.expectedStockQuantity.value < 0) { movementFormError.value = `当前可用库存为 ${formatQuantity(warehouseDetail.value?.quantity)} ${item.unit}，本次出库数量不能超过可用库存。`; return }
    if (!deps.movementCanSubmit.value) { movementFormError.value = '请补全办理对象、数量和必填业务说明后再提交。'; return }
    const body: Record<string, unknown> = {business_type: movementMode.value, quantity, operator_employee_id: Number(movementForm.operator_employee_id), reason: movementForm.reason || '', remark: movementForm.reason || ''}
    for (const key of ['supplier_id', 'customer_id', 'department_id', 'original_document_id']) if (movementForm[key] !== '' && movementForm[key] !== undefined) body[key] = Number(movementForm[key])
    if (movementMode.value === 'purchase_inbound' && movementForm.unit_cost) body.unit_cost = moneyToCents(movementForm.unit_cost)
    const snapshot = JSON.stringify(body); if (!idempotencyKey || requestSnapshot !== snapshot) { requestSnapshot = snapshot; idempotencyKey = createIdempotencyKey() }
    movementSubmitting.value = true; movementFormError.value = ''
    try {
      await request(`/api/v1/warehouse/items/${item.item_type}/${item.id}/movements`, {method: 'POST', headers: {'Idempotency-Key': idempotencyKey}, body}, deps.token.value)
      movementMode.value = ''; showQuickSupplier.value = false; clearMovementForm(); deps.operatorDirectory.invalidate()
      await Promise.all([deps.reloadActiveModule(), loadWarehouseItemDetail(), loadItemMovements()])
      const refreshed = deps.rows.value.find((row) => row.id === item.id && row.item_type === item.item_type); if (refreshed) selectedWarehouseItem.value = refreshed
      deps.panelMessage.value = '库存已更新'; ElMessage.success('库存已更新')
    } catch (error) {
      if (deps.operatorDirectory.handleSubmitError(error)) delete movementForm.operator_employee_id
      if (error instanceof ApiError && error.status < 500) { requestSnapshot = ''; idempotencyKey = '' }
      movementFormError.value = error instanceof Error ? error.message : '办理失败，请检查填写内容后重试。'
    } finally { movementSubmitting.value = false }
  }
  async function createQuickSupplier() {
    if (movementSubmitting.value || quickSupplierSubmitting.value) return
    if (!quickSupplier.name || !quickSupplier.code) { quickSupplierError.value = '请填写供应商名称和唯一编码。'; return }
    quickSupplierSubmitting.value = true; quickSupplierError.value = ''
    try { const created = await request<BasicItem>('/api/v1/suppliers', {method: 'POST', body: {...quickSupplier}}, deps.token.value); await deps.loadList('suppliers', false); movementForm.supplier_id = created.id; Object.assign(quickSupplier, {name: '', code: '', contact: '', phone: ''}); showQuickSupplier.value = false; deps.panelMessage.value = '供应商已新增'; ElMessage.success('供应商已新增') }
    catch (error) { quickSupplierError.value = error instanceof Error ? error.message : '供应商新增失败，请检查编码是否重复。'; ElMessage.error(quickSupplierError.value) }
    finally { quickSupplierSubmitting.value = false }
  }

  function decimalToScaled(value: unknown) { const number = Number(value); return Number.isFinite(number) ? Math.round(number * 10000) : 0 }
  function moneyToCents(value: unknown) { const number = Number(value); return Number.isFinite(number) ? Math.round(number * 100) : 0 }
  function formatQuantity(value: unknown) { return (Number(value || 0) / 10000).toLocaleString('zh-CN', {maximumFractionDigits: 4}) }
  function formatMoney(value: unknown) { return `¥${(Number(value || 0) / 100).toLocaleString('zh-CN', {minimumFractionDigits: 2, maximumFractionDigits: 2})}` }
  function formatDate(value: unknown) { return value ? new Date(String(value)).toLocaleString('zh-CN', {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'}) : '-' }
  function businessTypeLabel(value: unknown) { return movementDefinitions.find((item) => item.key === value)?.title || (value === 'inbound' ? '入库' : '出库') }
  function movementQuantity(document: BasicItem) { const lines = Array.isArray(document.lines) ? document.lines as Array<Record<string, unknown>> : []; return `${document.type === 'outbound' ? '−' : '+'}${formatQuantity(lines[0]?.quantity)} ${selectedWarehouseItem.value?.unit || ''}` }

  return {invalidateWarehouseRequests: invalidateRequests, openWarehouseItem, closeWarehouseItem, performWarehouseClose, requestWarehouseClose, handleWarehouseBeforeClose, resetWarehouseItem, loadWarehouseItemDetail, loadItemMovements, loadAllItemMovements, startMovement, cancelMovement, resetMovementSource, clearMovementForm, submitMovement, createQuickSupplier, decimalToScaled, moneyToCents, formatQuantity, formatMoney, formatDate, businessTypeLabel, movementQuantity}
}
