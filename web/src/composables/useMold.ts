import {ref, type Ref} from 'vue'
import {ElMessage} from 'element-plus'
import {ApiError, request} from '../api/http'
import type {BasicItem, CurrentUser} from '../types'
import {appMessageBox} from './useAppMessageBox'

type MoldDependencies = {
  token: Ref<string>
  currentUser: Ref<CurrentUser | null>
  hasPermission: (code: string) => boolean
  reloadList: () => Promise<unknown>
  promptText: (title: string, message: string) => Promise<string>
  promptTextWithDefault: (title: string, message: string, value: string) => Promise<string>
  promptPositiveInteger: (title: string, message: string) => Promise<number | null>
}

/** Owns the complete mold detail request lifecycle and lifecycle actions. */
export function useMold(deps: MoldDependencies) {
  const moldDetailDrawerVisible = ref(false)
  const selectedMoldDetail = ref<BasicItem | null>(null)
  const selectedMoldID = ref<number | null>(null)
  const moldDetailLoading = ref(false)
  const moldDetailError = ref('')
  const moldActionSubmitting = ref(false)
  const moldActionError = ref('')
  let detailRequestToken = 0
  let detailAbortController: AbortController | null = null

  function openMold(item: {id?: unknown}) {
    selectedMoldID.value = Number(item.id); selectedMoldDetail.value = null; moldDetailError.value = ''; moldDetailDrawerVisible.value = true; void loadMoldDetail()
  }
  async function loadMoldDetail() {
    const moldID = selectedMoldID.value
    if (!moldID) return
    detailAbortController?.abort()
    const controller = new AbortController(); detailAbortController = controller; const requestToken = ++detailRequestToken
    moldDetailLoading.value = true; moldDetailError.value = ''
    try {
      const data = await request<BasicItem>(`/api/v1/molds/${moldID}`, {signal: controller.signal}, deps.token.value)
      if (isCurrentRequest(requestToken, moldID)) selectedMoldDetail.value = data
    } catch (error) {
      if (!isCurrentRequest(requestToken, moldID)) return
      selectedMoldDetail.value = null; moldDetailError.value = error instanceof Error ? error.message : '模具详情加载失败'
    } finally {
      if (detailAbortController === controller) detailAbortController = null
      if (isCurrentRequest(requestToken, moldID)) moldDetailLoading.value = false
    }
  }
  function invalidateRequest() { detailAbortController?.abort(); detailAbortController = null; detailRequestToken += 1; moldDetailLoading.value = false }
  function isCurrentRequest(requestToken: number, moldID: number) { return requestToken === detailRequestToken && moldDetailDrawerVisible.value && selectedMoldID.value === moldID }
  function closeMold() { if (!moldActionSubmitting.value) { invalidateRequest(); moldDetailDrawerVisible.value = false } }
  function handleMoldBeforeClose(done: () => void) { if (moldActionSubmitting.value) { ElMessage.warning('模具状态正在提交，请稍候。'); return }; invalidateRequest(); done() }
  function resetMold() { invalidateRequest(); selectedMoldDetail.value = null; selectedMoldID.value = null; moldDetailError.value = ''; moldActionSubmitting.value = false; moldActionError.value = '' }
  function handlerName() { return deps.currentUser.value?.name || deps.currentUser.value?.username || '' }

  async function loanMold() {
    if (selectedMoldDetail.value?.status !== 'in_stock') return
    const location = await deps.promptText('借出位置', '请输入模具借出后的具体位置'); if (!location) return
    const counterparty = await deps.promptText('借用方', '请输入借用部门或单位'); if (!counterparty) return
    try { await appMessageBox.confirm(`确认将“${selectedMoldDetail.value.name}”借出至 ${location}？`, '确认借出', {type: 'warning', confirmButtonText: '确认借出'}) } catch { return }
    await runAction('loan', {location, counterparty, handler_name: handlerName(), reason: '模具借出'}, '模具已借出')
  }
  async function returnMold() {
    if (selectedMoldDetail.value?.status !== 'loaned') return
    const location = (await deps.promptTextWithDefault('归还位置', '请确认或修改模具归还后的具体位置', String(selectedMoldDetail.value.storage_location || '').trim())).trim(); if (!location) return
    try { await appMessageBox.confirm(`确认将“${selectedMoldDetail.value.name}”归还至 ${location}？`, '确认归还', {type: 'warning', confirmButtonText: '确认归还'}) } catch { return }
    await runAction('return', {location, handler_name: handlerName(), reason: '模具归还'}, '模具已归还入库')
  }
  async function repairMold(completed: boolean) {
    if (selectedMoldDetail.value?.status !== (completed ? 'repairing' : 'in_stock')) return
    const reason = await deps.promptText(completed ? '完成维修' : '开始维修', completed ? '请输入本次维修完成说明' : '请输入维修原因'); if (!reason) return
    try { await appMessageBox.confirm(completed ? '确认维修已经完成并将模具恢复为在库状态？' : '确认将模具状态变更为维修中？', completed ? '确认完成维修' : '确认开始维修', {type: completed ? 'success' : 'warning', confirmButtonText: completed ? '完成维修' : '开始维修'}) } catch { return }
    await runAction('repair', {reason, description: reason, handler_name: handlerName(), completed}, completed ? '模具维修已完成' : '模具已进入维修状态')
  }
  async function maintainMold(completed: boolean) {
    if (selectedMoldDetail.value?.status !== (completed ? 'maintenance' : 'in_stock')) return
    let cycle = Number(selectedMoldDetail.value.maintenance_cycle_days || 0)
    if (completed && (!Number.isInteger(cycle) || cycle <= 0)) { const entered = await deps.promptPositiveInteger('补充保养周期', '当前模具未设置有效保养周期，请填写完成保养后的周期天数'); if (entered === null) return; cycle = entered }
    try { await appMessageBox.confirm(completed ? `确认保养已经完成？系统将按 ${cycle} 天周期计算下次保养日期。` : '确认将模具状态变更为保养中？', completed ? '确认完成保养' : '确认开始保养', {type: completed ? 'success' : 'warning', confirmButtonText: completed ? '完成保养' : '开始保养'}) } catch { return }
    await runAction('maintenance', {description: completed ? '模具保养完成' : '开始模具保养', handler_name: handlerName(), completed, ...(completed ? {maintenance_cycle_days: cycle} : {})}, completed ? '模具保养已完成' : '模具已进入保养状态')
  }
  async function runAction(action: 'loan' | 'return' | 'repair' | 'maintenance', body: Record<string, unknown>, successMessage: string) {
    if (!selectedMoldDetail.value || !deps.hasPermission('mold:write') || moldActionSubmitting.value) return
    moldActionSubmitting.value = true; moldActionError.value = ''
    try { await request(`/api/v1/molds/${selectedMoldDetail.value.id}/${action}`, {method: 'POST', body}, deps.token.value); await Promise.all([deps.reloadList(), loadMoldDetail()]); ElMessage.success(successMessage) }
    catch (error) {
      if (error instanceof ApiError && error.status === 409) { moldActionError.value = '模具状态已发生变化，已刷新最新详情和列表，请按当前状态重新操作。'; ElMessage.warning(moldActionError.value); await Promise.all([deps.reloadList(), loadMoldDetail()]) }
      else { moldActionError.value = error instanceof Error ? error.message : '模具状态操作失败'; ElMessage.error(moldActionError.value) }
    } finally { moldActionSubmitting.value = false }
  }
  return {
    moldDetailDrawerVisible,
    selectedMoldDetail,
    selectedMoldID,
    moldDetailLoading,
    moldDetailError,
    moldActionSubmitting,
    moldActionError,
    openMold,
    loadMoldDetail,
    closeMold,
    handleMoldBeforeClose,
    resetMold,
    loanMold,
    returnMold,
    repairMold,
    maintainMold,
  }
}
