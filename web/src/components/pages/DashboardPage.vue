<template>
        <div class="dashboard">
          <div class="welcome-block">
            <div>
              <p class="eyebrow">{{ greeting }}</p>
              <h1>{{ currentUser?.name || currentUser?.username }}，今天要处理什么？</h1>
              <p>从常用功能开始，快速完成手头的工作。</p>
            </div>
            <div class="service-status" :class="`is-${healthStatus}`" role="status" aria-live="polite">
              <span></span> {{ healthStatusLabel }}
            </div>
            <DesktopUpdatePanel
              v-if="desktopClient"
              compact
              :legacy-status="clientUpdate"
              @download-recovery="downloadClientUpdate"
            />
          </div>

          <section class="home-section dashboard-overview" aria-labelledby="dashboard-overview-title">
            <div class="home-section-title">
              <div>
                <h2 id="dashboard-overview-title">工作概览</h2>
                <p>先确认服务与当前账号可用范围，再进入业务办理</p>
              </div>
            </div>
            <div class="dashboard-metrics">
              <MetricCard
                v-for="card in dashboardMetricCards"
                :key="card.label"
                :label="card.label"
                :value="card.value"
                :caption="card.caption"
                :tone="card.tone"
                :status-label="card.statusLabel"
                :status-tone="card.statusTone"
              />
            </div>
          </section>

          <section v-if="dashboardFocusItems.length" class="home-section" aria-labelledby="dashboard-focus-title">
            <div class="home-section-title">
              <div>
                <h2 id="dashboard-focus-title">今日关注</h2>
                <p>按高频 ERP 场景快速核对异常、进度和保养事项</p>
              </div>
            </div>
            <div class="dashboard-focus-grid">
              <button
                v-for="item in dashboardFocusItems"
                :key="item.key"
                class="dashboard-focus-card"
                type="button"
                @click="switchModule(item.key)"
              >
                <span class="dashboard-focus-card__heading">
                  <StatusTag :label="item.label" :tone="item.tone" />
                  <span aria-hidden="true">→</span>
                </span>
                <strong>{{ item.title }}</strong>
                <small>{{ item.description }}</small>
              </button>
            </div>
          </section>

          <section v-if="quickActions.length" class="home-section">
            <div class="home-section-title">
              <div>
                <h2>常用功能</h2>
                <p>一步直达最常办理的业务</p>
              </div>
            </div>
            <div class="quick-grid">
              <el-card
                  v-for="item in quickActions"
                  :key="item.key"
                  class="quick-card"
                  shadow="hover"
                  role="button"
                  tabindex="0"
                  :aria-label="`打开${item.title}：${item.description}`"
                  @click="switchModule(item.key)"
                  @keyup.enter="switchModule(item.key)"
                  @keyup.space.prevent="switchModule(item.key)"
              >
                <span class="quick-icon" aria-hidden="true">{{ item.icon }}</span>
                <span class="quick-copy">
                  <strong>{{ item.title }}</strong>
                  <small>{{ item.description }}</small>
                </span>
                <span class="quick-arrow" aria-hidden="true">→</span>
              </el-card>
            </div>
          </section>

          <section v-if="businessGroups.length" class="home-section">
            <div class="home-section-title">
              <div>
                <h2>全部业务</h2>
                <p>按工作场景查找功能</p>
              </div>
            </div>
            <div class="business-grid">
              <el-card v-for="group in businessGroups" :key="group.title" class="business-card" shadow="never">
                <div class="business-card-heading">
                  <span aria-hidden="true">{{ group.icon }}</span>
                  <div>
                    <strong>{{ group.title }}</strong>
                    <small>{{ group.caption }}</small>
                  </div>
                </div>
                <el-button
                    v-for="item in group.items"
                    :key="item.key"
                    link
                    @click="switchModule(item.key)"
                >
                  {{ item.title }} <span>→</span>
                </el-button>
              </el-card>
            </div>
          </section>
          <PageState
            v-if="!businessItems.length"
            kind="permission"
            title="当前账号没有可用的日常业务"
            description="如需使用其他功能，请联系管理员为账号配置对应的查看权限。"
          />
        </div>

</template>

<script setup lang="ts">
import DesktopUpdatePanel from '../DesktopUpdatePanel.vue'
import MetricCard from '../ui/MetricCard.vue'
import PageState from '../ui/PageState.vue'
import StatusTag from '../ui/StatusTag.vue'
import {useWorkspaceContext} from '../../composables/workspaceContext'

const {
  assignmentConfigs,
  tokenKey,
  desktopClient,
  authRequestGeneration,
  token,
  currentUser,
  activeKey,
  showCreateForm,
  editingSupplier,
  rows,
  columns,
  skeletonResult,
  searchKeyword,
  page,
  pageSize,
  pageTotal,
  loading,
  errorMessage,
  panelMessage,
  listError,
  assignmentTarget,
  assignmentModuleKey,
  selectedAssignmentIDs,
  assignmentOptionsCache,
  assignmentOptionsLoading,
  assignmentOptionsError,
  assignmentSaving,
  assignmentSaveError,
  selectedWarehouseItem,
  warehouseDrawerVisible,
  warehouseDetail,
  warehouseDetailLoading,
  warehouseDetailError,
  itemMovements,
  itemMovementsLoading,
  itemMovementsError,
  showAllItemMovements,
  movementMode,
  showQuickSupplier,
  movementSubmitting,
  movementFormError,
  quickSupplierSubmitting,
  quickSupplierError,
  healthStatus,
  mobileNavOpen,
  serverDialogVisible,
  serverTesting,
  serverUrlInput,
  serverMessage,
  serverMessageType,
  clientUpdate,
  loginForm,
  loginUsernameInput,
  formError,
  formState,
  movementForm,
  quickSupplier,
  activeWarehouseTab,
  workorderStatusFilter,
  workorderTypeFilter,
  workorderPriorityFilter,
  selectedWorkOrder,
  workorderDrawerVisible,
  workorderLogs,
  workorderLogsLoading,
  workorderLogsError,
  moldDetailDrawerVisible,
  selectedMoldDetail,
  selectedMoldID,
  moldDetailLoading,
  moldDetailError,
  moldActionSubmitting,
  moldActionError,
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
  cache,
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
