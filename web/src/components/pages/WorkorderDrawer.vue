<template>
      <ResponsiveDetailCarrier
        v-model="workorderDrawerVisible"
        drawer-class="workspace-detail-drawer"
        :docked="detailPanelDocked"
        :size="detailPanelSize"
        :title="workorderPanelTitle"
        :before-close="handleWorkorderPanelBeforeClose"
        :close-on-press-escape="!actionSubmitting"
        :show-close="!actionSubmitting"
        :docked-auto-focus="actionDialogVisible ? 'first-editable' : 'panel'"
        destroy-on-close
        @closed="resetWorkOrder"
      >
        <WorkorderActionDialog v-if="actionDialogVisible" />
        <div v-else-if="selectedWorkOrder" class="item-drawer workorder-drawer" aria-label="任务单详情">
          <el-alert v-if="moduleUnavailable" title="任务单暂不可用" description="当前内容仍保留，暂不能办理业务。请确认后再关闭。" type="warning" :closable="false" show-icon />
          <div class="drawer-heading">
            <div>
              <small>{{ selectedWorkOrder.code }} · {{ workorderTypeLabel(selectedWorkOrder.type) }}</small>
              <h2>{{ selectedWorkOrder.title }}</h2>
              <span>{{ selectedWorkOrder.product_name || '通用任务' }} · {{ workorderStatusLabel(selectedWorkOrder.status) }}</span>
            </div>
          </div>

          <div class="stock-summary">
            <div><span>计划数量</span><strong>{{ formatQuantity(selectedWorkOrder.planned_quantity) }} {{ selectedWorkOrder.unit || '' }}</strong></div>
            <div><span>优先级</span><strong>{{ selectedWorkOrder.priority === 'urgent' ? '加急' : '普通' }}</strong></div>
            <div><span>交期</span><strong>{{ formatDate(selectedWorkOrder.due_at) }}</strong><StatusTag v-if="workorderDueState(selectedWorkOrder).overdue" :label="workorderDueState(selectedWorkOrder).label" tone="danger"/></div>
          </div>
          <WorkorderStockCard
            v-if="selectedWorkOrder.product_id && hasPermission('warehouse:read')"
            class="workorder-drawer-stock"
            :product="workorderDrawerProductStock"
            :loading="workorderDrawerProductStockLoading"
            :error="workorderDrawerProductStockError"
            :updated-at="workorderDrawerProductStockUpdatedAt"
            @refresh="loadWorkorderDrawerProductStock"
          />
          <el-alert
            v-else-if="selectedWorkOrder.product_id"
            class="workorder-drawer-stock"
            title="当前账号无权查看仓库库存"
            description="产品关联已保留；如需查看实时库存，请联系管理员开通库存查看权限。"
            type="info"
            :closable="false"
            show-icon
          />
          <el-alert
            v-else-if="selectedWorkOrder.type === 'production'"
            class="workorder-drawer-stock"
            title="历史生产单未关联仓库产品"
            :description="`保留创建时产品名称“${selectedWorkOrder.product_name || '未记录'}”，无法据此查询实时库存。`"
            type="info"
            :closable="false"
            show-icon
          />
          <p v-if="selectedWorkOrder.description" class="drawer-message">{{ selectedWorkOrder.description }}</p>
          <section class="workorder-stage-card" aria-label="任务当前阶段">
            <div class="workorder-stage-card__heading">
              <span>当前阶段</span>
              <div>
                <StatusTag :label="selectedWorkOrder.priority === 'urgent' ? '加急' : '普通'" :tone="selectedWorkOrder.priority === 'urgent' ? 'danger' : 'info'"/>
                <StatusTag :label="workorderStatusLabel(selectedWorkOrder.status)" :tone="workorderStatusTone(selectedWorkOrder.status)"/>
              </div>
            </div>
            <strong>下一步：{{ workorderNextAction(selectedWorkOrder) }}</strong>
            <div class="workorder-stage-card__progress">
              <span>{{ departmentProgressSummary(selectedWorkOrder) }}</span>
              <el-progress :percentage="departmentProgressMetrics(selectedWorkOrder).percentage" :stroke-width="8"/>
            </div>
          </section>
          <ImageGallery owner-type="workorder" :owner-id="selectedWorkOrder.id" :token="token" :can-write="!moduleUnavailable && hasPermission('workorder:write')" category="workorder"/>

          <section v-if="canWriteActive" class="movement-section">
            <h3>办公室操作</h3>
            <div class="movement-actions">
              <el-button v-if="selectedWorkOrder.status === 'draft'" type="primary" plain @click="dispatchWorkOrder">派发</el-button>
              <el-button v-if="selectedWorkOrder.status === 'processing' || selectedWorkOrder.status === 'pending_close'" plain @click="pauseWorkOrder">暂停</el-button>
              <el-button v-if="selectedWorkOrder.status === 'paused'" plain @click="resumeWorkOrder">恢复</el-button>
              <el-button :type="selectedWorkOrder.priority === 'urgent' ? 'warning' : 'danger'" plain @click="toggleWorkOrderUrgent">
                {{ selectedWorkOrder.priority === 'urgent' ? '取消加急' : '加急' }}
              </el-button>
              <el-button v-if="selectedWorkOrder.status === 'pending_close'" type="success" plain @click="completeWorkOrder('normal')">确认正常完成</el-button>
              <el-button v-if="['processing', 'paused', 'pending_close'].includes(String(selectedWorkOrder.status))" type="danger" plain @click="completeWorkOrder('forced')">强制完成</el-button>
            </div>
          </section>

          <section class="workorder-department-section">
            <h3>部门子任务</h3>
            <div class="department-task-grid">
              <article v-for="task in departmentTasks(selectedWorkOrder)" :key="task.id" class="department-task-card">
                <div>
                  <strong>{{ departmentName(task.department_id) }}</strong>
                  <StatusTag :label="departmentTaskStatusLabel(task.status)" :tone="departmentTaskStatusTone(task.status)"/>
                </div>
                <p>{{ formatQuantity(task.completed_quantity) }} / {{ formatQuantity(task.planned_quantity) }} {{ selectedWorkOrder.unit || '' }}</p>
                <el-progress :percentage="Number(task.progress || 0)" :stroke-width="8"/>
                <small>{{ task.remark || '暂无备注' }}</small>
                <ImageGallery owner-type="department_task" :owner-id="task.id" :token="token" :can-write="!moduleUnavailable && canOperateDepartmentTask(task)" category="department_task"/>
                <div v-if="!moduleUnavailable && canOperateDepartmentTask(task)" class="department-task-actions">
                  <el-button v-if="task.status === 'received'" link type="primary" @click="startDepartmentTask(task)">开始处理</el-button>
                  <el-button v-if="['received', 'processing', 'partial_completed'].includes(String(task.status))" link type="warning" @click="partialCompleteDepartmentTask(task)">部分完成</el-button>
                  <el-button v-if="['received', 'processing', 'partial_completed'].includes(String(task.status))" link type="success" @click="completeDepartmentTask(task)">完成</el-button>
                </div>
              </article>
            </div>
          </section>

          <section class="movement-history">
            <div class="drawer-section-title"><h3>流转日志</h3><el-button link type="primary" :loading="workorderLogsLoading" :disabled="workorderLogsLoading" @click="loadWorkOrderLogs">刷新</el-button></div>
            <PageState v-if="workorderLogsLoading" kind="loading" title="正在加载任务日志" />
            <PageState
              v-else-if="workorderLogsError"
              kind="error"
              title="任务日志加载失败"
              :description="workorderLogsError"
              action-label="重新加载"
              @action="loadWorkOrderLogs"
            />
            <div v-else-if="workorderLogs.length" class="movement-list">
              <article v-for="item in workorderLogs" :key="item.id">
                <span class="movement-kind">{{ workorderActionLabel(item.action) }}</span>
                <div><strong>{{ item.operator_employee_name || '历史记录未记录员工' }}</strong><small>{{ item.actor_username || '系统账号' }}{{ item.actor_terminal_id ? ` · 终端#${item.actor_terminal_id}` : '' }} · {{ item.remark || item.reason || `${item.status_before || '-'} → ${item.status_after || '-'}` }} · {{ formatDate(item.created_at) }}</small></div>
              </article>
            </div>
            <p v-else class="drawer-empty">暂无流转日志</p>
          </section>
        </div>
        <template #footer><div v-if="actionDialogVisible" id="workorder-action-footer"></div><el-button v-else @click="closeWorkOrder">关闭</el-button></template>
      </ResponsiveDetailCarrier>

</template>

<script setup lang="ts">
import {computed, nextTick, watch} from 'vue'
import ImageGallery from '../ImageGallery.vue'
import PageState from '../ui/PageState.vue'
import StatusTag from '../ui/StatusTag.vue'
import WorkorderStockCard from './WorkorderStockCard.vue'
import WorkorderActionDialog from './WorkorderActionDialog.vue'
import {useWorkorderContext} from '../../composables/workorderContext'
import {useResponsiveDetailPanel} from '../../composables/useResponsiveDetailPanel'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import ResponsiveDetailCarrier from '../ui/ResponsiveDetailCarrier.vue'

const {moduleUnavailable} = useWorkspaceContext()
const {actionDialogVisible, actionKind, actionSubmitting, closeWorkOrderAction} = useWorkorderContext().action

const {
  token, selectedWorkOrder, workorderDrawerVisible, workorderLogs,
  workorderLogsLoading, workorderLogsError, workorderDrawerProductStock,
  workorderDrawerProductStockLoading, workorderDrawerProductStockError,
  workorderDrawerProductStockUpdatedAt, hasPermission, canWriteActive,
  workorderTypeLabel, workorderStatusLabel, workorderStatusTone,
  formatQuantity, formatDate, workorderDueState, workorderNextAction,
  departmentProgressSummary, departmentProgressMetrics, dispatchWorkOrder,
  pauseWorkOrder, resumeWorkOrder, toggleWorkOrderUrgent, completeWorkOrder,
  departmentTasks, departmentName, departmentTaskStatusLabel,
  departmentTaskStatusTone, canOperateDepartmentTask, startDepartmentTask,
  partialCompleteDepartmentTask, completeDepartmentTask, workorderActionLabel,
  loadWorkorderDrawerProductStock, loadWorkOrderLogs, closeWorkOrder,
  handleWorkOrderBeforeClose, resetWorkOrder,
} = useWorkorderContext().detail
const {docked: detailPanelDocked, size: detailPanelSize} = useResponsiveDetailPanel(workorderDrawerVisible, true)
const actionTitles: Record<string, string> = {dispatch: '派发任务', pause: '暂停任务', resume: '恢复任务', urgent: '调整加急状态', complete_normal: '确认正常完成', complete_forced: '强制完成任务', department_start: '开始处理部门任务', department_partial_complete: '提交部分完成', department_complete: '完成部门任务'}
const workorderPanelTitle = computed(() => actionDialogVisible.value ? actionTitles[actionKind.value] || '任务操作确认' : '任务单详情')
let workorderDetailScrollTop = 0

async function handleWorkorderPanelBeforeClose(done: () => void) {
  if (actionDialogVisible.value) {
    let allowed = false
    await closeWorkOrderAction(() => {
      allowed = true
      actionDialogVisible.value = false
    })
    if (!allowed) return
  }
  handleWorkOrderBeforeClose(done)
}

watch(actionDialogVisible, async (open, wasOpen) => {
  const body = document.querySelector<HTMLElement>('.workspace-detail-aside .detail-body')
  if (open) workorderDetailScrollTop = body?.scrollTop || 0
  await nextTick()
  if (open) return
  if (!wasOpen) return
  const restoredBody = document.querySelector<HTMLElement>('.workspace-detail-aside .detail-body')
  if (restoredBody) restoredBody.scrollTop = workorderDetailScrollTop
  document.querySelector<HTMLElement>('.workspace-detail-aside')?.focus({preventScroll: true})
})
</script>
