<template>
  <el-dialog v-model="actionDialogVisible" :title="title" width="min(560px, calc(100vw - 28px))" append-to-body :close-on-click-modal="!actionSubmitting" :close-on-press-escape="!actionSubmitting" :show-close="!actionSubmitting" :before-close="beforeClose">
    <AnimatePresence mode="wait" initial>
      <motion.div
        v-if="actionDialogVisible"
        key="workorder-action-content"
        class="workorder-dialog-motion"
        :initial="{opacity: 0, y: 10, scale: 0.985}"
        :animate="{opacity: 1, y: 0, scale: 1}"
        :exit="{opacity: 0, y: -8, scale: 0.99}"
        :transition="{duration: 0.18, ease: [0.2, 0, 0, 1]}"
      >
    <el-form id="workorder-action-form" label-position="top" :disabled="actionSubmitting" @submit.prevent="submitWorkOrderAction">
      <el-alert v-if="impactMessage" :title="impactMessage" :type="dangerous ? 'warning' : 'info'" :closable="false" show-icon />
      <el-alert v-if="actionError" :title="actionError" type="error" :closable="false" show-icon />
      <div class="workorder-action-summary"><strong>{{ selectedWorkOrder?.title }}</strong><small>{{ actionTarget ? `部门：${departmentName(actionTarget.department_id)}` : `任务编号：${selectedWorkOrder?.code}` }}</small></div>
      <el-form-item v-if="needsQuantity" :label="`累计完成数量（计划 ${formatQuantity(actionTarget?.planned_quantity)}）`" required :error="quantityError"><el-input id="workorder-action-quantity" v-model="actionForm.completed_quantity" inputmode="decimal" :aria-invalid="Boolean(quantityError)" placeholder="请输入大于当前完成量且小于计划数量的累计值" /><small class="field-help">当前已完成 {{ formatQuantity(actionTarget?.completed_quantity) }}，提交后状态变为“部分完成”。</small></el-form-item>
      <el-form-item v-if="needsReason" :label="actionKind === 'pause' ? '暂停原因' : '强制完成原因'" required :error="reasonError"><el-input id="workorder-action-reason" v-model.trim="actionForm.reason" type="textarea" :rows="3" maxlength="500" show-word-limit :aria-invalid="Boolean(reasonError)" /></el-form-item>
      <el-form-item v-if="needsRemark" label="本次备注"><el-input v-model.trim="actionForm.remark" type="textarea" :rows="3" maxlength="500" show-word-limit placeholder="选填" /></el-form-item>
      <OperatorSelect id="workorder-action-operator" v-model="actionForm.operator_employee_id" :department="operatorDirectory.department.value" :employees="operatorDirectory.employees.value" :loading="operatorDirectory.loading.value" :unavailable-reason="operatorDirectory.unavailableReason.value" :retryable="operatorDirectory.retryable.value" :invalid="Boolean(actionFieldErrors.operator)" :validation-error="actionFieldErrors.operator" @update:model-value="clearOperatorError" @load="operatorDirectory.load" @retry="operatorDirectory.load(true)" />
    </el-form>
      </motion.div>
    </AnimatePresence>
    <template #footer><div class="form-actions"><el-button :disabled="actionSubmitting" @click="closeWorkOrderAction()">取消</el-button><el-button native-type="submit" form="workorder-action-form" :type="confirmType" :loading="actionSubmitting" :disabled="confirmDisabled">{{ confirmText }}</el-button></div></template>
  </el-dialog>
</template>
<script setup lang="ts">
import {computed, watch} from 'vue'
import {AnimatePresence, motion} from 'motion-v'
import {useWorkorderContext} from '../../composables/workorderContext'
import OperatorSelect from '../ui/OperatorSelect.vue'

const {operatorDirectory, selectedWorkOrder, actionDialogVisible, actionKind, actionTarget, actionSubmitting, actionError, actionFieldErrors, actionForm, closeWorkOrderAction, submitWorkOrderAction, departmentName, formatQuantity} = useWorkorderContext().action
const titles: Record<string, string> = {dispatch: '派发任务', pause: '暂停任务', resume: '恢复任务', urgent: '调整加急状态', complete_normal: '确认正常完成', complete_forced: '强制完成任务', department_start: '开始处理部门任务', department_partial_complete: '提交部分完成', department_complete: '完成部门任务'}
const title = computed(() => titles[actionKind.value] || '任务操作确认')
const dangerous = computed(() => actionKind.value === 'complete_forced')
const statusLabels: Record<string, string> = {draft: '草稿', received: '已收到', processing: '处理中', paused: '已暂停', partial_completed: '部分完成', pending_close: '待办公室确认', completed: '完成', completed_normal: '正常完成', completed_forced: '强制完成'}
const currentStatus = computed(() => String(actionTarget.value?.status || selectedWorkOrder.value?.status || ''))
const currentStatusLabel = computed(() => statusLabels[currentStatus.value] || currentStatus.value || '未知')
const targetStatusLabel = computed(() => ({complete_normal: '正常完成', complete_forced: '强制完成', department_partial_complete: '部分完成', department_complete: '完成'}[actionKind.value] || ''))
const impactDetail = computed(() => ({complete_normal: '任务流程结束。', complete_forced: '未完成的部门进度会保留在记录中。', department_partial_complete: '主任务仍继续流转。', department_complete: '本部门进度变为 100%；若全部部门完成，主任务进入“待办公室确认”。'}[actionKind.value] || ''))
const impactMessage = computed(() => targetStatusLabel.value ? `状态：${currentStatusLabel.value} → ${targetStatusLabel.value}。${impactDetail.value}` : '')
const needsReason = computed(() => ['pause', 'complete_forced'].includes(actionKind.value))
const needsQuantity = computed(() => actionKind.value === 'department_partial_complete')
const needsRemark = computed(() => ['department_partial_complete', 'department_complete'].includes(actionKind.value))
const quantityError = computed(() => {
  if (!needsQuantity.value) return ''
  const quantityInput = actionForm.completed_quantity.trim()
  if (!quantityInput) return actionFieldErrors.quantity
  if (!/^\d+(\.\d{1,4})?$/.test(quantityInput)) return '请输入正数，最多保留 4 位小数。'
  const value = Math.round(Number(quantityInput) * 10000)
  const planned = Number(actionTarget.value?.planned_quantity || 0)
  const completed = Number(actionTarget.value?.completed_quantity || 0)
  if (value <= 0 || value >= planned) return `累计完成数量必须大于 0 且小于 ${formatQuantity(planned)}。`
  if (value <= completed) return `累计完成数量必须大于当前已完成的 ${formatQuantity(completed)}。`
  return ''
})
const reasonError = computed(() => needsReason.value && !actionForm.reason.trim() ? actionFieldErrors.reason : '')
const confirmDisabled = computed(() => Boolean(
  operatorDirectory.unavailableReason.value
  || (needsQuantity.value && (!actionForm.completed_quantity.trim() || quantityError.value)),
))
const confirmType = computed<'primary' | 'success' | 'warning' | 'danger'>(() => {
  if (actionKind.value === 'complete_forced') return 'danger'
  if (actionKind.value === 'urgent') return selectedWorkOrder.value?.priority === 'urgent' ? 'primary' : 'warning'
  if (['complete_normal', 'department_complete'].includes(actionKind.value)) return 'success'
  if (actionKind.value === 'pause') return 'warning'
  return 'primary'
})
const confirmText = computed(() => {
  if (actionKind.value === 'urgent') return selectedWorkOrder.value?.priority === 'urgent' ? '确认取消加急' : '确认加急'
  const labels: Record<string, string> = {dispatch: '确认派发', pause: '确认暂停', resume: '确认恢复', complete_normal: '确认正常完成', complete_forced: '确认强制完成', department_start: '确认开始处理', department_partial_complete: '提交部分完成', department_complete: '确认部门完成'}
  return labels[actionKind.value] || '确认操作'
})
function beforeClose(done: () => void) { void closeWorkOrderAction(done) }
function clearOperatorError() { actionFieldErrors.operator = '' }
watch(() => actionForm.reason, () => { actionFieldErrors.reason = '' })
watch(() => actionForm.completed_quantity, () => { actionFieldErrors.quantity = '' })
</script>
