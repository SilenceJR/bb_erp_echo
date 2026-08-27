<template>
      <el-drawer v-model="moldDetailDrawerVisible" size="min(720px, 100%)" title="模具详情" :with-header="false" :before-close="handleMoldBeforeClose" destroy-on-close @closed="resetMold">
        <PageState v-if="moldDetailLoading" kind="loading" title="正在加载模具详情" />
        <PageState
          v-else-if="moldDetailError"
          kind="error"
          title="模具详情加载失败"
          :description="moldDetailError"
          action-label="重新加载"
          @action="loadMoldDetail"
        />
        <div v-else-if="selectedMoldDetail" class="item-drawer mold-drawer" aria-label="模具详情">
          <div class="drawer-heading">
            <div><small>{{ selectedMoldDetail.code }}</small><h2>{{ selectedMoldDetail.name }}</h2><span>{{ moldStatusLabel(selectedMoldDetail.status) }} · {{ selectedMoldDetail.current_location || '暂无位置' }}</span></div>
            <el-button circle aria-label="关闭模具详情" :disabled="moldActionSubmitting" @click="closeMold">×</el-button>
          </div>
          <el-alert
            class="drawer-status-alert"
            :title="selectedMoldMaintenanceState.label"
            :description="selectedMoldMaintenanceState.description"
            :type="selectedMoldAlertType"
            :closable="false"
            show-icon
          />
          <div class="stock-summary mold-summary">
            <div><span>穴数</span><strong>{{ formatCell(selectedMoldDetail.cavity_count) }}</strong></div>
            <div><span>成型材料</span><strong>{{ formatCell(selectedMoldDetail.mold_material) }}</strong></div>
            <div><span>钢材</span><strong>{{ formatCell(selectedMoldDetail.steel) }}</strong></div>
            <div><span>存放位置</span><strong>{{ formatCell(selectedMoldDetail.storage_location) }}</strong></div>
            <div><span>保养周期</span><strong>{{ formatCell(selectedMoldDetail.maintenance_cycle_days) }} 天</strong></div>
            <div><span>下次保养</span><strong>{{ formatDate(selectedMoldDetail.next_maintenance_at) }}</strong></div>
          </div>
          <section v-if="hasPermission('mold:write')" class="movement-section mold-lifecycle-section" aria-labelledby="mold-lifecycle-title">
            <h3 id="mold-lifecycle-title">模具状态操作</h3>
            <el-alert v-if="moldActionError" :title="moldActionError" type="error" :closable="false" show-icon />
            <div class="movement-actions">
              <el-button v-if="selectedMoldDetail.status === 'in_stock'" type="primary" plain :loading="moldActionSubmitting" :disabled="moldActionSubmitting" @click="loanMold">借出模具</el-button>
              <el-button v-if="selectedMoldDetail.status === 'loaned'" type="primary" plain :loading="moldActionSubmitting" :disabled="moldActionSubmitting" @click="returnMold">归还入库</el-button>
              <el-button v-if="selectedMoldDetail.status === 'in_stock'" type="warning" plain :loading="moldActionSubmitting" :disabled="moldActionSubmitting" @click="repairMold(false)">开始维修</el-button>
              <el-button v-if="selectedMoldDetail.status === 'repairing'" type="success" plain :loading="moldActionSubmitting" :disabled="moldActionSubmitting" @click="repairMold(true)">完成维修</el-button>
              <el-button v-if="selectedMoldDetail.status === 'in_stock'" plain :loading="moldActionSubmitting" :disabled="moldActionSubmitting" @click="maintainMold(false)">开始保养</el-button>
              <el-button v-if="selectedMoldDetail.status === 'maintenance'" type="success" plain :loading="moldActionSubmitting" :disabled="moldActionSubmitting" @click="maintainMold(true)">完成保养</el-button>
            </div>
            <p v-if="selectedMoldDetail.status === 'scrapped'" class="permission-hint">已报废模具没有可执行的生命周期操作。</p>
          </section>
          <ImageGallery owner-type="mold" :owner-id="selectedMoldDetail.id" :token="token" :can-write="hasPermission('mold:write')" category="mold"/>
          <section class="movement-history">
            <div class="drawer-section-title"><h3>模具履历</h3></div>
            <div v-if="Array.isArray(selectedMoldDetail.events) && selectedMoldDetail.events.length" class="movement-list">
              <article v-for="event in selectedMoldDetail.events" :key="event.id"><span class="movement-kind">{{ event.type || '事件' }}</span><div><strong>{{ event.status_before || '-' }} → {{ event.status_after || '-' }}</strong><small>{{ event.description || event.reason || event.remark || '-' }} · {{ formatDate(event.created_at) }}</small></div></article>
            </div>
            <p v-else class="drawer-empty">暂无模具履历</p>
          </section>
        </div>
      </el-drawer>

</template>

<script setup lang="ts">
import ImageGallery from '../ImageGallery.vue'
import PageState from '../ui/PageState.vue'
import {useWorkspaceContext} from '../../composables/workspaceContext'

const {
  token,
  moldDetailDrawerVisible,
  selectedMoldDetail,
  moldDetailLoading,
  moldDetailError,
  moldActionSubmitting,
  moldActionError,
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
