<template>
    <section class="workspace">
      <header class="topbar">
        <el-button
          id="mobile-menu-button"
          class="mobile-menu-button"
          circle
          aria-label="打开系统导航"
          @click="mobileNavOpen = true"
        >
          <span class="mobile-menu-icon" aria-hidden="true"><i></i><i></i><i></i></span>
        </el-button>
        <div class="brand">
          <img src="/bobang-logo-hd.png" alt="博邦光电"/>
          <span class="brand-mark" aria-label="博邦光电">BB</span>
          <div>
            <strong>博邦光电</strong>
            <span>业务工作台</span>
          </div>
        </div>
        <div class="user-chip">
          <div class="user-copy">
            <span>{{ currentUser?.name || currentUser?.username }}</span>
            <small>{{ accountTypeText }}</small>
          </div>
          <el-dropdown trigger="click" @command="handleWorkspaceUserCommand">
            <el-button circle class="user-avatar" :aria-label="`${currentUser?.name || currentUser?.username || '用户'}菜单`">{{ userInitial }}</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-if="desktopClient" command="server">服务器设置</el-dropdown-item>
                <el-dropdown-item command="change-password">修改密码</el-dropdown-item>
                <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>

      <aside class="sidebar" aria-label="系统导航">
        <AppNavigation
          :active-key="activeKey"
          :business-items="businessItems"
          :system-items="systemItems"
          @select="switchModule"
        />
      </aside>

      <el-drawer
        v-model="mobileNavOpen"
        class="mobile-navigation-drawer"
        direction="ltr"
        size="min(320px, 88vw)"
        title="系统导航"
        :with-header="false"
        append-to-body
        @closed="restoreMobileMenuFocus"
      >
        <div class="mobile-navigation">
          <div class="mobile-navigation__brand">
            <img src="/bobang-logo-hd.png" alt=""/>
            <div><strong>博邦光电</strong><span>业务工作台</span></div>
            <el-button aria-label="关闭系统导航" @click="mobileNavOpen = false">关闭</el-button>
          </div>
          <AppNavigation
            :active-key="activeKey"
            :business-items="businessItems"
            :system-items="systemItems"
            aria-label="移动端系统导航"
            @select="selectMobileModule"
          />
        </div>
      </el-drawer>

      <ChangePasswordDialog
        v-model="passwordDialogVisible"
        :token="token"
        @changed="handlePasswordChanged"
      />

      <section class="content">
        <slot name="page"></slot>
      </section>
      <slot name="overlays"></slot>
    </section>
</template>

<script setup lang="ts">
import {ref} from 'vue'
import {ElMessage} from 'element-plus'
import AppNavigation from '../ui/AppNavigation.vue'
import ChangePasswordDialog from './ChangePasswordDialog.vue'
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

const passwordDialogVisible = ref(false)

function handleWorkspaceUserCommand(command: string) {
  if (command === 'change-password') {
    passwordDialogVisible.value = true
    return
  }
  void handleUserCommand(command)
}

function handlePasswordChanged() {
  loginForm.password = ''
  logout()
  ElMessage.success('密码修改成功，请使用新密码重新登录')
}
</script>
