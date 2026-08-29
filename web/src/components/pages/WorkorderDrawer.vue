<template>
      <el-drawer v-model="workorderDrawerVisible" size="min(720px, 100%)" title="任务单详情" :with-header="false" :before-close="handleWorkOrderBeforeClose" destroy-on-close @closed="resetWorkOrder">
        <div v-if="selectedWorkOrder" class="item-drawer workorder-drawer" aria-label="任务单详情">
          <div class="drawer-heading">
            <div>
              <small>{{ selectedWorkOrder.code }} · {{ workorderTypeLabel(selectedWorkOrder.type) }}</small>
              <h2>{{ selectedWorkOrder.title }}</h2>
              <span>{{ selectedWorkOrder.product_name || '通用任务' }} · {{ workorderStatusLabel(selectedWorkOrder.status) }}</span>
            </div>
            <el-button circle aria-label="关闭任务单详情" @click="closeWorkOrder">×</el-button>
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
            description="产品关联已保留；如需查看实时库存，请联系管理员授予 warehouse:read。"
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
          <ImageGallery owner-type="workorder" :owner-id="selectedWorkOrder.id" :token="token" :can-write="hasPermission('workorder:write')" category="workorder"/>

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
                <ImageGallery owner-type="department_task" :owner-id="task.id" :token="token" :can-write="canOperateDepartmentTask(task)" category="department_task"/>
                <div v-if="canOperateDepartmentTask(task)" class="department-task-actions">
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
      </el-drawer>
      <WorkorderActionDialog />

</template>

<script setup lang="ts">
import ImageGallery from '../ImageGallery.vue'
import PageState from '../ui/PageState.vue'
import StatusTag from '../ui/StatusTag.vue'
import WorkorderStockCard from './WorkorderStockCard.vue'
import WorkorderActionDialog from './WorkorderActionDialog.vue'
import {useWorkspaceContext} from '../../composables/workspaceContext'

const {
  token,
  selectedWorkOrder,
  workorderDrawerVisible,
  workorderLogs,
  workorderLogsLoading,
  workorderLogsError,
  workorderDrawerProductStock,
  workorderDrawerProductStockLoading,
  workorderDrawerProductStockError,
  workorderDrawerProductStockUpdatedAt,
  authRequestGeneration,
  statisticsData,
  selectedMoldMaintenanceState,
  selectedMoldAlertType,
  warehouseTabs,
  warehouseTabOptions,
  movementDefinitions,
  workorderStatusOptions,
  workorderTypeOptions,
  workorderPriorityOptions,
  navItems,
  businessItems,
  systemItems,
  activeModule,
  canWriteActive,
  activePageReadonly,
  hasActiveFilters,
  filteredEmptyTitle,
  filteredEmptyDescription,
  isMasterDataValidationPage,
  hasRenderableData,
  listSearchPlaceholder,
  masterDataEmptyTitle,
  masterDataEmptyDescription,
  genericIdentityColumns,
  genericStatusColumn,
  genericCardColumns,
  hasAssignmentAction,
  assignmentConfig,
  assignmentOptions,
  assignmentOptionsReady,
  assignmentOptionGroups,
  userInitial,
  greeting,
  quickActionDefinitions,
  quickActions,
  businessGroups,
  accountTypeText,
  healthStatusLabel,
  dashboardMetricCards,
  dashboardFocusItems,
  availableMovementDefinitions,
  movementDependencyMessage,
  movementTitle,
  movementIsOutbound,
  expectedStockQuantity,
  warehouseQuantityAvailable,
  selectedWarehouseStockState,
  selectedWarehouseAlertType,
  movementQuantityError,
  movementCounterpartyLabel,
  formatMovementInputQuantity,
  movementCanSubmit,
  movementSubmitLabel,
  movementFormDirty,
  displayedItemMovements,
  eligibleOriginalDocuments,
  warehouseSummaryCards,
  workorderSummaryCards,
  moldSummaryCards,
  operationalSummaryCards,
  statisticsCards,
  compactTrendItems,
  formSchema,
  activeWarehouseTabTitle,
  createEntityTitle,
  rowsFor,
  isPaginatedResponse,
  appendQuery,
  hasPermission,
  canReadModule,
  canWriteModule,
  switchModule,
  selectMobileModule,
  restoreMobileMenuFocus,
  handleUserCommand,
  resetFilters,
  assignmentOptionsRequestToken,
  openAssignment,
  closeAssignment,
  loadAssignmentOptions,
  retryAssignmentOptions,
  isAssignmentOptionDisabled,
  saveAssignment,
  switchWarehouseTab,
  resetListQuery,
  applySearch,
  handlePageChange,
  handlePageSizeChange,
  login,
  clearAuthSession,
  logout,
  openServerSettings,
  testServerSetting,
  saveServerSetting,
  bootstrap,
  loadHealth,
  loadMe,
  loadClientUpdate,
  downloadClientUpdate,
  preloadBaseData,
  loadActiveModule,
  loadStatistics,
  loadList,
  createItem,
  normalizedForm,
  validateActiveForm,
  numericKeys,
  clearForm,
  toggleCreateForm,
  editSupplier,
  inferColumns,
  formatCell,
  genericStatusLabel,
  genericStatusTone,
  isGenericStatusColumn,
  formatGenericCell,
  permissionDomainKey,
  permissionDomainLabel,
  genericRowTitle,
  genericRowSubtitle,
  stockState,
  columnLabels,
  columnLabel,
  warehouseDetailRequestToken,
  itemMovementsRequestToken,
  moldDetailRequestToken,
  workorderLogsRequestToken,
  warehouseDetailAbortController,
  itemMovementsAbortController,
  moldDetailAbortController,
  workorderLogsAbortController,
  invalidateWarehouseRequests,
  isCurrentWarehouseRequest,
  openWarehouseItem,
  warehouseCloseBypass,
  closeWarehouseItem,
  performWarehouseClose,
  requestWarehouseClose,
  handleWarehouseBeforeClose,
  resetWarehouseItem,
  invalidateMoldDetailRequest,
  isCurrentMoldDetailRequest,
  openMold,
  loadMoldDetail,
  closeMold,
  handleMoldBeforeClose,
  resetMold,
  loanMold,
  returnMold,
  repairMold,
  maintainMold,
  runMoldAction,
  loadWarehouseItemDetail,
  loadItemMovements,
  loadAllItemMovements,
  startMovement,
  cancelMovement,
  resetMovementSource,
  clearMovementForm,
  submitMovement,
  createQuickSupplier,
  decimalToScaled,
  moneyToCents,
  formatQuantity,
  formatMoney,
  formatDate,
  businessTypeLabel,
  movementQuantity,
  openWorkOrder,
  invalidateWorkOrderLogsRequest,
  isCurrentWorkOrderLogsRequest,
  closeWorkOrder,
  handleWorkOrderBeforeClose,
  resetWorkOrder,
  loadWorkOrderLogs,
  loadWorkorderDrawerProductStock,
  dispatchWorkOrder,
  pauseWorkOrder,
  resumeWorkOrder,
  toggleWorkOrderUrgent,
  completeWorkOrder,
  startDepartmentTask,
  partialCompleteDepartmentTask,
  completeDepartmentTask,
  runWorkOrderAction,
  promptText,
  promptTextWithDefault,
  promptPositiveInteger,
  departmentTasks,
  departmentProgressMetrics,
  departmentProgressSummary,
  departmentName,
  canOperateDepartmentTask,
  workorderStatusLabel,
  workorderTypeLabel,
  inventoryItemTypeLabel,
  moldStatusLabel,
  moldStatusTone,
  moldMaintenanceState,
  departmentCompletionRate,
  trendNameLabel,
  trendBarPercentage,
  departmentTaskStatusLabel,
  workorderStatusTone,
  workorderDueState,
  departmentTaskStatusTone,
  workorderNextAction,
  workorderActionLabel,
} = useWorkspaceContext()
</script>
