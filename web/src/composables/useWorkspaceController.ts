import {computed, nextTick, onBeforeUnmount, onMounted, ref, watch} from 'vue'
import {appMessageBox} from './useAppMessageBox'
import {useAssignment} from './useAssignment'
import {useAuth} from './useAuth'
import {useModuleData} from './useModuleData'
import {useOperatorEmployees} from './useOperatorEmployees'
import {useWarehouse} from './useWarehouse'
import {useWarehouseOperations} from './useWarehouseOperations'
import {useWorkorder} from './useWorkorder'
import {useWorkorderOperations} from './useWorkorderOperations'
import {ElMessage} from 'element-plus'
import {Box, Coin, TrendCharts, Tickets, UserFilled, Van} from '@element-plus/icons-vue'
import {ApiError, apiBaseUrl, configureAuthSession, request} from '../api/http'
import type {MetricTone} from '../components/ui/MetricCard.vue'
import type {StatusTone} from '../components/ui/StatusTag.vue'
import {type ModuleItem, modules} from '../data/modules'
import type {BasicItem, CurrentUser, PaginatedResponse, SkeletonResponse} from '../types'
import {dirtyGuardRegistry} from '../platform/dirtyGuard'
import {deferredModuleForPath, moduleUnavailableEvent, statisticsSourceIsUnavailable, type StatisticsSource} from '../platform/moduleAvailability'
import {useDirtyGuard} from './useDirtyGuard'
import {useDirectoryOperations} from './useDirectoryOperations'
import {
  movementDefinitions,
  useModuleConfiguration,
  warehouseTabOptions,
  warehouseTabs,
  workorderPriorityOptions,
  workorderStatusOptions,
  workorderTypeOptions,
} from './useModuleConfiguration'
import {
  columnLabel,
  departmentCompletionRate,
  departmentProgressMetrics,
  departmentProgressSummary,
  departmentTasks,
  departmentTaskStatusLabel,
  departmentTaskStatusTone,
  formatCell,
  genericStatusLabel,
  genericStatusTone,
  inventoryItemTypeLabel,
  isGenericStatusColumn,
  permissionDomainKey,
  permissionDomainLabel,
  stockState,
  trendNameLabel,
  workorderActionLabel,
  workorderDueState,
  workorderStatusLabel,
  workorderStatusTone,
  workorderTypeLabel,
} from './workspacePresentation'

type AuthResponse = {
  access_token: string
  expires_at: string
  refresh_token: string
  refresh_expires_at: string
  user: CurrentUser
}

/**
 * Creates the authenticated workspace state shared by Web and Tauri.
 *
 * Domain views receive this plain object through Vue injection so refs remain reactive
 * after destructuring, while each API workflow still keeps its cancellation state local.
 */
export function useWorkspaceController() {
type StatisticNameValue = { name: string; value: number; amount?: number }
type MetricCardItem = {label: string; value: string; caption: string; tone: MetricTone; statusLabel?: string; statusTone?: StatusTone}
type StatisticTrendItem = { date: string; name?: string; value: number; quantity?: number; amount?: number }
type DepartmentStatistic = { department_id: number; name: string; total: number; completed: number; processing: number; partial: number; received: number }
type StockStatisticItem = { item_type: string; item_id: number; name: string; code: string; category: string; quantity: number; safety_stock: number; amount?: number }
type StatisticsDashboard = {
  data_status?: 'ready' | 'sources_unavailable'
  unavailable_sources?: string[]
  message?: string
  generated_at: string
  can_view_cost: boolean
  summary: Record<string, number>
  inventory: { by_item_type: StatisticNameValue[]; by_material_type: StatisticNameValue[]; low_stock: StockStatisticItem[]; trend: StatisticTrendItem[] }
  workorders: { by_status: StatisticNameValue[]; by_type: StatisticNameValue[]; by_department: DepartmentStatistic[]; trend: StatisticTrendItem[] }
  molds: { by_type: StatisticNameValue[]; by_location: StatisticNameValue[] }
  business: { by_master_data: StatisticNameValue[] }
  audit: { by_result: StatisticNameValue[]; trend: StatisticTrendItem[] }
  recent_workorders: BasicItem[]
}

const warehouseState = useWarehouse()
const workorderState = useWorkorder()
const {
  assignmentConfigs,
  assignmentTarget,
  assignmentModuleKey,
  selectedAssignmentIDs,
  assignmentOptionsCache,
  assignmentOptionsLoading,
  assignmentOptionsError,
  assignmentSaving,
  assignmentSaveError,
} = useAssignment()

const {
  tokenKey,
  refreshTokenKey,
  tokenExpiresAtKey,
  desktopClient,
  token,
  refreshToken,
  tokenExpiresAt,
  currentUser,
  errorMessage,
  healthStatus,
  loginForm,
  loginUsernameInput,
  formError,
} = useAuth()

const operatorDirectory = useOperatorEmployees(token)

let authRequestGeneration = 0
let directoryOperations: ReturnType<typeof useDirectoryOperations>
let refreshInFlight: Promise<string> | null = null
let sessionRefreshTimer: number | undefined
const sessionRefreshLeadMs = 5 * 60 * 1000

const {
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
  panelMessage,
  listError,
  moduleUnavailable,
  formState,
  activeWarehouseTab,
  workorderStatusFilter,
  workorderTypeFilter,
  workorderPriorityFilter,
  cache,
} = useModuleData()

const {
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
  movementForm,
  quickSupplier,
} = warehouseState

const {
  selectedWorkOrder,
  workorderDrawerVisible,
  workorderLogs,
  workorderLogsLoading,
  workorderLogsError,
  workorderProductOptions,
  workorderProductSearchLoading,
  workorderProductSearchError,
  workorderProductStock,
  workorderProductStockLoading,
  workorderProductStockError,
  workorderProductStockUpdatedAt,
  workorderDrawerProductStock,
  workorderDrawerProductStockLoading,
  workorderDrawerProductStockError,
  workorderDrawerProductStockUpdatedAt,
  temporaryProductDialogVisible,
  temporaryProductSubmitting,
  temporaryProductError,
  temporaryProductForm,
  actionDialogVisible,
  actionKind,
  actionTarget,
  actionSubmitting,
  actionError,
  actionFieldErrors,
  actionForm,
} = workorderState

const {
  invalidateWorkorderProductSearch,
  searchWorkorderProducts,
  handleWorkorderProductSelect,
  resetWorkorderProductSelection,
  loadWorkorderProductStock,
  openTemporaryProductDialog,
  closeTemporaryProductDialog,
  closeTemporaryProductWithGuard,
  createTemporaryProduct,
  loadWorkorderDrawerProductStock,
  openWorkOrder,
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
  openWorkOrderAction,
  closeWorkOrderAction,
  submitWorkOrderAction,
  dispose: disposeWorkorderOperations,
} = useWorkorderOperations(workorderState, {
  token,
  activeKey,
  showCreateForm,
  formState,
  hasPermission,
  operatorDirectory,
  appendQuery,
  panelMessage,
  reloadActiveModule: loadActiveModule,
})

const statisticsData = ref<StatisticsDashboard | null>(null)
const pageDetailPanelVisible = ref(false)
const affiliationTarget = ref<BasicItem | null>(null)
const affiliationDepartmentID = ref<number | undefined>()
const affiliationTerminalID = ref<number | undefined>()
const affiliationSaving = ref(false)
const affiliationError = ref('')
const affiliationInitial = ref({departmentID: undefined as number | undefined, terminalID: undefined as number | undefined})
const affiliationTerminalOptions = computed(() => rowsFor('terminals').filter((item) => Number(item.department_id) === Number(affiliationDepartmentID.value)))
watch(() => formState.department_id, (departmentID) => {
  if (activeKey.value !== 'users' || !formState.terminal_id) return
  const terminal = rowsFor('terminals').find((item) => Number(item.id) === Number(formState.terminal_id))
  if (!terminal || Number(terminal.department_id) !== Number(departmentID)) delete formState.terminal_id
})
watch(affiliationDepartmentID, () => {
  if (!affiliationTerminalOptions.value.some((item) => Number(item.id) === Number(affiliationTerminalID.value))) affiliationTerminalID.value = undefined
})
watch(() => formState.type, (type) => {
  if (activeKey.value === 'workorder' && type !== 'production') {
    invalidateWorkorderProductSearch()
    resetWorkorderProductSelection()
  }
})
watch(activeKey, (key) => {
  if (key !== 'workorder') {
    invalidateWorkorderProductSearch()
    resetWorkorderProductSelection()
    closeTemporaryProductDialog()
  }
})
let confirmedWarehouseTab = 'product'
const navItems = computed(() => modules.filter(canReadModule))
const businessItems = computed(() => navItems.value.filter((item) => item.group === 'business'))
const systemItems = computed(() => navItems.value.filter((item) => item.group === 'system'))
const activeModule = computed(() => modules.find((item) => item.key === activeKey.value))
const canWriteActive = computed(() => !moduleUnavailable.value && !!activeModule.value && canWriteModule(activeModule.value))
const canEditUserAffiliation = computed(() => canWriteActive.value && hasPermission('system:departments:read') && hasPermission('system:terminals:read'))
const canCreateDepartmentTerminalUser = computed(() => hasPermission('system:departments:read') && hasPermission('system:terminals:read'))
const activePageReadonly = computed(() => {
  if (activeKey.value === 'warehouses') {
    return !hasPermission('warehouse:write') && !hasPermission('inventory:documents:write')
  }
  return !activeModule.value?.writePermission || !canWriteActive.value
})
const hasActiveFilters = computed(() => Boolean(
  searchKeyword.value || workorderStatusFilter.value || workorderTypeFilter.value || workorderPriorityFilter.value,
))
const filteredEmptyTitle = computed(() => hasActiveFilters.value ? '没有符合当前条件的结果' : `还没有${activeModule.value?.title || '业务'}记录`)
const filteredEmptyDescription = computed(() => hasActiveFilters.value
  ? '请调整筛选条件或点击重置查看全部记录。'
  : canWriteActive.value && formSchema.value.length
    ? '可以使用页面右上角的新增操作创建第一条记录。'
    : activePageReadonly.value
      ? '当前账号仅可查看，暂无可显示的记录；如需新增，请联系具备编辑权限的人员。'
      : '当前暂无可显示的记录。')
const isMasterDataValidationPage = computed(() => activeKey.value === 'suppliers')
const hasRenderableData = computed(() => activeKey.value === 'statistics'
  ? Boolean(statisticsData.value)
  : rows.value.length > 0)
const listSearchPlaceholder = computed(() => {
  if (activeKey.value === 'suppliers') return '搜索供应商名称、编码、联系人或电话'
  return '输入名称、编号、电话等关键字'
})
const masterDataEmptyTitle = computed(() => searchKeyword.value
  ? '没有符合当前条件的结果'
  : `还没有${activeModule.value?.title || '档案'}记录`)
const masterDataEmptyDescription = computed(() => searchKeyword.value
  ? '请尝试缩短关键词或清空筛选条件。'
  : canWriteActive.value
    ? '可以使用页面右上角的新增操作创建第一条记录。'
    : '当前账号仅可查看，暂无可显示的记录；如需新增，请联系具备编辑权限的人员。')
const genericIdentityColumns = computed(() => {
  const preferred: Record<string, [string, string]> = {
    users: ['username', 'name'],
    departments: ['name', 'code'],
    terminals: ['name', 'code'],
    roles: ['name', 'code'],
    permissions: ['name', 'code'],
    audits: ['operator_employee_name', 'action'],
  }
  const configured = preferred[activeKey.value] || [columns.value[0] || 'id', columns.value[1] || '']
  const primary = columns.value.includes(configured[0]) ? configured[0] : (columns.value[0] || 'id')
  const secondary = columns.value.includes(configured[1]) ? configured[1] : (columns.value.find((column) => column !== primary) || '')
  return {primary, secondary}
})
const genericStatusColumn = computed(() => columns.value.find(isGenericStatusColumn) || '')
const genericCardColumns = computed(() => columns.value.filter((column) => ![
  genericIdentityColumns.value.primary,
  genericIdentityColumns.value.secondary,
  genericStatusColumn.value,
].includes(column)))
const hasAssignmentAction = computed(() => {
  const config = assignmentConfigs[activeKey.value]
  return Boolean(config?.requiredPermissions.every(hasPermission))
})
const assignmentConfig = computed(() => assignmentConfigs[assignmentModuleKey.value])
const assignmentOptions = computed(() => {
  return assignmentConfig.value ? assignmentOptionsCache[assignmentConfig.value.optionKey] || [] : []
})
const assignmentOptionsReady = computed(() => Boolean(
  assignmentConfig.value
  && Object.prototype.hasOwnProperty.call(assignmentOptionsCache, assignmentConfig.value.optionKey)
  && !assignmentOptionsLoading.value
  && !assignmentOptionsError.value,
))
const assignmentOptionGroups = computed(() => {
  if (assignmentConfig.value?.optionKey !== 'permissions') {
    return [{key: 'roles', label: '可分配角色', items: assignmentOptions.value}]
  }
  const groups = new Map<string, BasicItem[]>()
  for (const option of assignmentOptions.value) {
    const key = permissionDomainKey(option)
    const items = groups.get(key) || []
    items.push(option)
    groups.set(key, items)
  }
  return [...groups.entries()].map(([key, items]) => ({key, label: permissionDomainLabel(key), items}))
})
const assignmentScopeBlockedReason = computed(() => {
  const config = assignmentConfig.value
  const target = assignmentTarget.value
  if (!config || !target || !assignmentOptionsReady.value) return ''
  const originalIDs = Array.isArray(target[config.selectedKey])
    ? (target[config.selectedKey] as unknown[]).map(Number)
    : []
  const optionsByID = new Map(assignmentOptions.value.map((option) => [Number(option.id), option]))
  for (const id of originalIDs) {
    const option = optionsByID.get(id)
    if (!option) return '目标当前包含无法核验的授权项。为避免误删或越权，当前配置已锁定，请刷新后重试。'
    const reason = assignmentOptionScopeReason(option, target)
    if (reason) return `目标当前包含超出你管理范围的授权（${option.name || option.code || `#${id}`}）。为避免越权，不能修改该目标。`
  }
  return ''
})
const userInitial = computed(() => (currentUser.value?.name || currentUser.value?.username || '用户').slice(0, 1))
const greeting = computed(() => {
  const hour = new Date().getHours()
  if (hour < 11) return '早上好'
  if (hour < 14) return '中午好'
  if (hour < 18) return '下午好'
  return '晚上好'
})
const quickActionDefinitions = [
  {key: 'workorder', title: '任务单', description: '查看当前任务与部门处理进度', icon: Tickets},
  {key: 'warehouses', title: '仓库', description: '查询库存并办理物品出入库', icon: Box},
  {key: 'customers', title: '客户档案', description: '查找或新增客户资料', icon: UserFilled},
  {key: 'suppliers', title: '供应商', description: '维护采购供应商资料', icon: Van},
  {key: 'molds', title: '模具', description: '查询模具位置与图片资料', icon: Coin},
]
const quickActions = computed(() => quickActionDefinitions.filter((item) => {
  const moduleItem = modules.find((candidate) => candidate.key === item.key)
  return !!moduleItem && canReadModule(moduleItem)
}).sort((left, right) => {
  const departmentTerminal = currentUser.value?.account_type === 'department_terminal'
  const order = departmentTerminal
    ? ['workorder', 'warehouses', 'molds', 'customers', 'suppliers']
    : ['warehouses', 'workorder', 'customers', 'suppliers', 'molds']
  return order.indexOf(left.key) - order.indexOf(right.key)
}))

const businessGroups = computed(() => [
  {
    title: '库存与仓储',
    caption: '物品、库存与出入库',
    icon: Box,
    items: businessItems.value.filter((item) => ['warehouses'].includes(item.key)),
  },
  {
    title: '客户与生产',
    caption: '客户资料与生产档案',
    icon: UserFilled,
    items: businessItems.value.filter((item) => ['customers', 'suppliers', 'molds', 'workorder'].includes(item.key)),
  },
  {
    title: '数据与报表',
    caption: '经营数据与统计结果',
    icon: TrendCharts,
    items: businessItems.value.filter((item) => item.key === 'statistics'),
  },
].filter((group) => group.items.length))
const accountTypeText = computed(() => {
  if (!currentUser.value) return '未登录'
  return currentUser.value.account_type === 'department_terminal' ? '部门终端账号' : '个人账号'
})
const lastHealthCheckAt = ref('')
const healthStatusLabel = computed(() => ({
  checking: '正在检查服务',
  healthy: '服务正常',
  error: '服务暂不可用',
})[healthStatus.value])
const dashboardMetricCards = computed<MetricCardItem[]>(() => [
  {
    label: '可用业务',
    value: `${businessItems.value.length} 项`,
    caption: '已按当前账号权限显示',
    tone: 'info',
  },
  {
    label: '常用入口',
    value: `${quickActions.value.length} 个`,
    caption: '按账号类型排序',
    tone: 'neutral',
  },
  {
    label: '当前账号',
    value: accountTypeText.value,
    caption: '字段与操作继续按权限控制',
    tone: 'neutral',
  },
])
const dashboardFocusItems = computed(() => [
  {key: 'warehouses', label: '库存核对', title: '核对安全库存', description: '查看缺货与低于安全库存的物品。', tone: 'info' as StatusTone},
  {key: 'workorder', label: '任务进度', title: '跟进交期与部门进度', description: '优先处理加急、暂停和待确认任务。', tone: 'info' as StatusTone},
  {key: 'molds', label: '模具', title: '检查位置与图片资料', description: '关注模具位置、图片资料和图纸。', tone: 'info' as StatusTone},
  {key: 'statistics', label: '经营概览', title: '查看汇总与趋势', description: '从统计报表复核异常和近期变化。', tone: 'info' as StatusTone},
].filter((focus) => {
  const item = modules.find((candidate) => candidate.key === focus.key)
  return !!item && canReadModule(item)
}))
const availableMovementDefinitions = computed(() => movementDefinitions.filter((definition) => {
  const allAllowed = (definition.requiredAll || []).every(hasPermission)
  const anyAllowed = !definition.requiredAny?.length || definition.requiredAny.some(hasPermission)
  return allAllowed && anyAllowed
}))
const movementDependencyMessage = computed(() => {
  if (!hasPermission('inventory:documents:write')) return ''
  const missing: string[] = []
  if (!hasPermission('suppliers:read')) missing.push('采购入库需要供应商查看权限')
  if (!hasPermission('customers:read') && !hasPermission('system:departments:read')) missing.push('退货返工需要客户或部门查看权限')
  if (!hasPermission('customers:read')) missing.push('客户出库需要客户查看权限')
  if (!hasPermission('system:departments:read')) missing.push('部门出库需要部门查看权限')
  return missing.length ? `${missing.join('；')}。请联系管理员配置所需权限。` : ''
})
const movementTitle = computed(() => movementDefinitions.find((item) => item.key === movementMode.value)?.title || '办理出入库')
const movementIsOutbound = computed(() => ['customer_outbound', 'department_outbound'].includes(movementMode.value))
const expectedStockQuantity = computed(() => {
  const current = Number(warehouseDetail.value?.quantity || 0)
  const quantityChange = decimalToScaled(movementForm.quantity)
  return movementIsOutbound.value ? current - quantityChange : current + quantityChange
})
const warehouseQuantityAvailable = computed(() => Boolean(
  warehouseDetail.value
  && Object.prototype.hasOwnProperty.call(warehouseDetail.value, 'quantity')
  && warehouseDetail.value.quantity !== null
  && warehouseDetail.value.quantity !== undefined,
))
const selectedWarehouseStockState = computed(() => stockState({
  ...(selectedWarehouseItem.value || {}),
  ...(warehouseDetail.value || {}),
}))
const selectedWarehouseAlertType = computed<'success' | 'warning' | 'error' | 'info'>(() => {
  if (selectedWarehouseStockState.value.tone === 'danger') return 'error'
  return selectedWarehouseStockState.value.tone
})
const movementQuantityError = computed(() => {
  if (movementForm.quantity === undefined || movementForm.quantity === null || movementForm.quantity === '') return ''
  if (decimalToScaled(movementForm.quantity) <= 0) return '数量必须大于 0。'
  if (movementIsOutbound.value && expectedStockQuantity.value < 0) return '出库数量超过当前可用库存，请减少数量。'
  return ''
})
const movementCounterpartyLabel = computed(() => {
  const source = movementMode.value === 'purchase_inbound' ? rowsFor('suppliers').find((item) => Number(item.id) === Number(movementForm.supplier_id))
    : movementMode.value === 'customer_outbound' || movementForm.source_type === 'customer' ? rowsFor('customers').find((item) => Number(item.id) === Number(movementForm.customer_id))
      : rowsFor('departments').find((item) => Number(item.id) === Number(movementForm.department_id))
  return String(source?.name || '尚未选择')
})
const formatMovementInputQuantity = computed(() => {
  const quantity = Number(movementForm.quantity || 0)
  return Number.isFinite(quantity) && quantity > 0 ? quantity.toLocaleString('zh-CN', {maximumFractionDigits: 4}) : '0'
})
const movementCanSubmit = computed(() => {
  if (!warehouseDetail.value || !warehouseQuantityAvailable.value || warehouseDetailLoading.value || warehouseDetailError.value) return false
  if (movementSubmitting.value || decimalToScaled(movementForm.quantity) <= 0 || !movementForm.operator_employee_id || operatorDirectory.unavailableReason.value) return false
  if (movementIsOutbound.value && expectedStockQuantity.value < 0) return false
  if (movementMode.value === 'purchase_inbound') return Boolean(movementForm.supplier_id)
  if (movementMode.value === 'customer_outbound') return Boolean(movementForm.customer_id)
  if (movementMode.value === 'department_outbound') return Boolean(movementForm.department_id)
  if (movementMode.value === 'return_rework_inbound') {
    const sourceSelected = movementForm.source_type === 'customer' ? movementForm.customer_id : movementForm.department_id
    return Boolean(sourceSelected && String(movementForm.reason || '').trim())
  }
  return false
})
const movementSubmitLabel = computed(() => `确认${movementTitle.value}并过账`)
const movementFormDirty = computed(() => Boolean(movementMode.value && (
  Object.keys(movementForm).some((key) => key !== 'source_type') || showQuickSupplier.value || Object.values(quickSupplier).some(Boolean)
)))
const displayedItemMovements = computed(() => showAllItemMovements.value ? itemMovements.value : itemMovements.value.slice(0, 10))
const eligibleOriginalDocuments = computed(() => itemMovements.value.filter((item) => {
  if (item.type !== 'outbound' || item.status !== 'posted') return false
  if (movementForm.source_type === 'customer') return Number(item.customer_id) === Number(movementForm.customer_id)
  if (movementForm.source_type === 'department') return Number(item.department_id) === Number(movementForm.department_id)
  return false
}))
const {
  invalidateWarehouseRequests,
  openWarehouseItem,
  closeWarehouseItem,
  performWarehouseClose,
  requestWarehouseClose,
  handleWarehouseBeforeClose,
  resetWarehouseItem,
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
} = useWarehouseOperations(warehouseState, {
  token,
  panelMessage,
  rows,
  hasPermission,
  movementFormDirty,
  warehouseQuantityAvailable,
  movementIsOutbound,
  expectedStockQuantity,
  movementCanSubmit,
  operatorDirectory,
  reloadActiveModule: loadActiveModule,
  loadList,
})
const warehouseSummaryCards = computed<MetricCardItem[]>(() => {
  const states = rows.value.map(stockState)
  const danger = states.filter((state) => state.tone === 'danger').length
  const warning = states.filter((state) => state.tone === 'warning').length
  return [
    {label: '当前页物品', value: String(rows.value.length), caption: `共 ${pageTotal.value} 条记录`, tone: 'neutral'},
    {label: '库存正常', value: String(states.filter((state) => state.tone === 'success').length), caption: '高于安全库存', tone: 'success'},
    {label: '低库存', value: String(warning), caption: '低于或等于安全库存', tone: warning ? 'warning' : 'success'},
    {label: '缺货', value: String(danger), caption: '当前库存小于或等于零', tone: danger ? 'danger' : 'success'},
  ]
})
const workorderSummaryCards = computed<MetricCardItem[]>(() => {
  const urgent = rows.value.filter((row) => row.priority === 'urgent').length
  const pendingClose = rows.value.filter((row) => row.status === 'pending_close').length
  const paused = rows.value.filter((row) => row.status === 'paused').length
  return [
    {label: '当前页任务', value: String(rows.value.length), caption: `共 ${pageTotal.value} 条记录`, tone: 'neutral'},
    {label: '加急任务', value: String(urgent), caption: '需要优先跟进', tone: urgent ? 'danger' : 'success'},
    {label: '待办公室确认', value: String(pendingClose), caption: '等待核对并关闭', tone: pendingClose ? 'warning' : 'success'},
    {label: '暂停任务', value: String(paused), caption: '需确认后恢复', tone: paused ? 'info' : 'success'},
  ]
})
const moldSummaryCards = computed<MetricCardItem[]>(() => {
  return [
    {label: '当前页模具', value: String(rows.value.length), caption: `共 ${pageTotal.value} 条记录`, tone: 'neutral'},
    {label: '共模', value: String(rows.value.filter((row) => row.mold_type === 'common').length), caption: '当前页共模产品', tone: 'info'},
    {label: '单模', value: String(rows.value.filter((row) => row.mold_type === 'single').length), caption: '当前页单模产品', tone: 'success'},
    {label: '图片', value: String(rows.value.reduce((sum, row) => sum + Number(row.image_count || 0), 0)), caption: '当前页图片总数', tone: 'neutral'},
  ]
})
const operationalSummaryCards = computed<MetricCardItem[]>(() => {
  if (activeKey.value === 'warehouses') return warehouseSummaryCards.value
  if (activeKey.value === 'workorder') return workorderSummaryCards.value
  if (activeKey.value === 'molds') return moldSummaryCards.value
  return []
})
const statisticsCards = computed<MetricCardItem[]>(() => {
  if (!statisticsData.value) return []
  const summary = statisticsData.value?.summary || {}
  const lowStock = Number(summary.low_stock_items || 0)
  const urgent = Number(summary.urgent_workorders || 0)
  const pendingClose = Number(summary.pending_close_orders || 0)
  return [
    {label: '库存总量', value: statisticsSourceUnavailable('inventory') ? '—' : formatQuantity(summary.inventory_quantity), caption: statisticsSourceUnavailable('inventory') ? '库存数据源尚未初始化' : statisticsData.value?.can_view_cost ? `金额 ${formatMoney(summary.inventory_amount)}` : '金额按权限隐藏', tone: 'info'},
    {label: '低库存', value: statisticsSourceUnavailable('inventory') ? '—' : String(lowStock), caption: statisticsSourceUnavailable('inventory') ? '库存数据源尚未初始化' : '低于或等于安全库存', tone: statisticsSourceUnavailable('inventory') ? 'neutral' : lowStock ? 'danger' : 'success', statusLabel: statisticsSourceUnavailable('inventory') ? '不可用' : lowStock ? '需处理' : '正常', statusTone: statisticsSourceUnavailable('inventory') ? 'info' : lowStock ? 'danger' : 'success'},
    {label: '进行中任务', value: statisticsSourceUnavailable('workorders') ? '—' : String(summary.open_workorders || 0), caption: statisticsSourceUnavailable('workorders') ? '任务数据源尚未初始化' : `加急 ${urgent} · 待确认 ${pendingClose}`, tone: statisticsSourceUnavailable('workorders') ? 'neutral' : urgent ? 'danger' : pendingClose ? 'warning' : 'info'},
    {label: '模具档案', value: String(summary.molds || 0), caption: '模具产品记录总数', tone: 'info'},
    {label: '客户编码', value: String(summary.customers || 0), caption: '客户编码总数', tone: 'neutral'},
    {label: '仓库物品', value: statisticsSourceUnavailable('inventory') ? '—' : String(summary.warehouse_items || 0), caption: '产品与物资档案', tone: 'neutral'},
  ].filter((card) => card.value !== '—') as MetricCardItem[]
})
const statisticsSourcesUnavailable = computed(() => statisticsData.value?.data_status === 'sources_unavailable')
function statisticsSourceUnavailable(source: StatisticsSource): boolean {
  return statisticsSourceIsUnavailable(
    statisticsData.value?.data_status,
    statisticsData.value?.unavailable_sources,
    source,
  )
}
const compactTrendItems = computed(() => {
  const inventory = statisticsSourceUnavailable('inventory') ? [] : statisticsData.value?.inventory?.trend || []
  const workorders = statisticsSourceUnavailable('workorders') ? [] : statisticsData.value?.workorders?.trend || []
  const sorted = [...inventory, ...workorders]
    .sort((left, right) => {
      const leftTime = new Date(String(left.date)).getTime()
      const rightTime = new Date(String(right.date)).getTime()
      if (Number.isFinite(leftTime) && Number.isFinite(rightTime)) return leftTime - rightTime
      return String(left.date).localeCompare(String(right.date), 'zh-CN')
    })
  const recentDates = new Set([...new Set(sorted.map((item) => String(item.date)))].slice(-14))
  return sorted.filter((item) => recentDates.has(String(item.date)))
})

const {activeWarehouseTabTitle, createEntityTitle, formSchema} = useModuleConfiguration({
  activeKey, activeWarehouseTab, activeModule, formState,
  canCreateDepartmentTerminalUser, rowsFor, hasPermission,
})

function rowsFor(key: string): BasicItem[] {
  return cache[key] || []
}

function isPaginatedResponse(data: BasicItem[] | PaginatedResponse<BasicItem> | SkeletonResponse): data is PaginatedResponse<BasicItem> {
  return !Array.isArray(data) && Array.isArray((data as PaginatedResponse<BasicItem>).items)
}

function appendQuery(path: string, params: Record<string, string | number | undefined>): string {
  const urlParams = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && String(value) !== '') {

      urlParams.set(key, String(value))
    }
  }
  const query = urlParams.toString()
  if (!query) return path
  return `${path}${path.includes('?') ? '&' : '?'}${query}`
}

function hasPermission(code?: string): boolean {
  // Navigation and write actions share the backend permission codes so hidden UI
  // is only a convenience; the API remains the final authorization boundary.
  return !code || !!currentUser.value?.permissions?.includes(code)
}

function canReadModule(item: ModuleItem): boolean {
  if (item.key === 'dashboard') return true
  if (item.key === 'users') return hasPermission('system:users:read') || hasPermission('system:users:write')
  if (item.key === 'roles') return hasPermission('system:roles:read') || hasPermission('system:roles:write') || hasPermission('system:users:write')
  if (item.key === 'permissions') return hasPermission('system:permissions:read') || hasPermission('system:roles:write')
  return hasPermission(item.readPermission)
}

function canWriteModule(item: ModuleItem): boolean {
  return !!item.writePermission && hasPermission(item.writePermission)
}

directoryOperations = useDirectoryOperations({
  activeKey, showCreateForm, editingSupplier, rows, columns, skeletonResult, searchKeyword,
  page, pageSize, pageTotal, loading, panelMessage, listError, formState, formError,
  activeWarehouseTab, workorderStatusFilter, workorderTypeFilter, workorderPriorityFilter,
  cache, token, currentUser, activeModule, canCreateDepartmentTerminalUser, formSchema,
  moduleUnavailable, ApiError,
  assignmentOptionsCache, operatorDirectory, hasPermission, canReadModule, canWriteModule,
  rowsFor, appendQuery, isPaginatedResponse, loadStatistics, decimalToScaled, moneyToCents,
  resetWorkorderProductSelection, searchWorkorderProducts, invalidateWorkorderProductSearch,
  closeTemporaryProductDialog,
})
const {
  resetFilters, switchWarehouseTab, resetListQuery, applySearch, handlePageChange,
  handlePageSizeChange, createItem, clearForm, toggleCreateForm, editSupplier, createFormDirty,
} = directoryOperations

function preloadBaseData() { return directoryOperations.preloadBaseData() }
function loadActiveModule() { return directoryOperations.loadActiveModule() }
function loadList(key: string, applyToPanel: boolean) { return directoryOperations.loadList(key, applyToPanel) }

async function switchModule(key: string) {
  const target = modules.find((item) => item.key === key)
  if (!target || !canReadModule(target)) {
    panelMessage.value = '你的账号暂无该功能权限'
    activeKey.value = 'dashboard'
    return
  }
  if (!(await dirtyGuardRegistry.confirmLeave('module-switch'))) return
  if (warehouseDrawerVisible.value) {
    performWarehouseClose()
  }
  activeKey.value = key
  rows.value = []
  columns.value = []
  pageTotal.value = 0
  skeletonResult.value = null
  listError.value = ''
  moduleUnavailable.value = null
  closeWorkOrder()
  closeAssignment()
  showCreateForm.value = false
  editingSupplier.value = null
  resetListQuery()
  clearForm()
  void loadActiveModule()
}

function registerModuleLeaveGuard(guard: (() => Promise<boolean>) | null) {
  if (!guard) {
    dirtyGuardRegistry.remove('active-module')
    return
  }
  dirtyGuardRegistry.register({id: 'active-module', confirmLeave: guard})
}

async function handleUserCommand(command: string) {
  if (command === 'logout') {
    logout()
  }
}


let assignmentOptionsRequestToken = 0

async function openAssignment(row: any) {
  const config = assignmentConfigs[activeKey.value]
  if (!config || assignmentTargetDisabled(row)) {
    const hint = assignmentTargetHint(row)
    if (hint) ElMessage.warning(hint)
    return
  }
  if (affiliationTarget.value) {
    if (affiliationSaving.value) {
      ElMessage.warning('账号归属正在保存，请等待完成后再切换。')
      return
    }
    if (affiliationDepartmentID.value !== affiliationInitial.value.departmentID || affiliationTerminalID.value !== affiliationInitial.value.terminalID) {
      try { await appMessageBox.confirm('当前账号归属尚未保存，切换后修改将丢失。', '切换操作？', {confirmButtonText: '放弃并切换', cancelButtonText: '继续编辑', type: 'warning'}) } catch { return }
    }
    affiliationError.value = ''
    affiliationTarget.value = null
  }
  if (assignmentTarget.value && Number(assignmentTarget.value.id) === Number(row.id) && assignmentModuleKey.value === activeKey.value) return
  if (assignmentTarget.value) {
    if (assignmentSaving.value) {
      ElMessage.warning('权限配置正在保存，请等待完成后再切换。')
      return
    }
    const currentConfig = assignmentConfig.value
    const original = currentConfig && Array.isArray(assignmentTarget.value[currentConfig.selectedKey])
      ? (assignmentTarget.value[currentConfig.selectedKey] as unknown[]).map(Number).sort((left, right) => left - right).join(',')
      : ''
    const selected = [...selectedAssignmentIDs.value].map(Number).sort((left, right) => left - right).join(',')
    if (original !== selected) {
      try { await appMessageBox.confirm('当前权限配置尚未保存，切换后修改将丢失。', '切换配置对象？', {confirmButtonText: '放弃并切换', cancelButtonText: '继续配置', type: 'warning'}) } catch { return }
    }
  }
  assignmentTarget.value = row
  assignmentModuleKey.value = activeKey.value
  assignmentOptionsError.value = ''
  assignmentSaveError.value = ''
  selectedAssignmentIDs.value = Array.isArray(row[config.selectedKey])
    ? (row[config.selectedKey] as unknown[]).map(Number)
    : []
  if (!Object.prototype.hasOwnProperty.call(assignmentOptionsCache, config.optionKey)) await loadAssignmentOptions()
}

function closeAssignment() {
  assignmentOptionsRequestToken += 1
  assignmentTarget.value = null
  assignmentModuleKey.value = ''
  selectedAssignmentIDs.value = []
  assignmentOptionsLoading.value = false
  assignmentOptionsError.value = ''
  assignmentSaveError.value = ''
}

async function loadAssignmentOptions(force = false) {
  const config = assignmentConfig.value
  const targetModule = assignmentModuleKey.value
  if (!config || !assignmentTarget.value) return
  if (!force && Object.prototype.hasOwnProperty.call(assignmentOptionsCache, config.optionKey)) return
  const requestToken = ++assignmentOptionsRequestToken
  assignmentOptionsLoading.value = true
  assignmentOptionsError.value = ''
  try {
    const moduleItem = modules.find((item) => item.key === config.optionKey)
    if (!moduleItem?.path) throw new Error('未找到配置项接口')
    const allItems: BasicItem[] = []
    let currentPage = 1
    let total = Number.POSITIVE_INFINITY
    while (allItems.length < total && currentPage <= 100) {
      const data = await request<BasicItem[] | PaginatedResponse<BasicItem>>(
        appendQuery(moduleItem.path, {page: currentPage, page_size: 200}), {}, token.value,
      )
      if (requestToken !== assignmentOptionsRequestToken || assignmentModuleKey.value !== targetModule) return
      if (Array.isArray(data)) {
        allItems.push(...data)
        total = allItems.length
        break
      }
      allItems.push(...data.items)
      total = data.total
      if (!data.items.length) break
      currentPage += 1
    }
    if (allItems.length < total) throw new Error('配置项数量超过安全加载上限，请联系管理员缩小数据范围')
    assignmentOptionsCache[config.optionKey] = allItems
  } catch (error) {
    if (requestToken !== assignmentOptionsRequestToken || assignmentModuleKey.value !== targetModule) return
    assignmentOptionsError.value = error instanceof Error ? error.message : '配置项加载失败'
  } finally {
    if (requestToken === assignmentOptionsRequestToken && assignmentModuleKey.value === targetModule) assignmentOptionsLoading.value = false
  }
}

function retryAssignmentOptions() {
  void loadAssignmentOptions(true)
}

function isAssignmentOptionDisabled(option: BasicItem): boolean {
  return Boolean(assignmentScopeBlockedReason.value || assignmentOptionDisabledReason(option))
}

function assignmentOptionScopeReason(option: BasicItem, target: BasicItem): string {
  const config = assignmentConfig.value
  if (!config) return ''
  if (config.isDisabled?.(target, option)) return '终端账号不能授予超级管理员角色'
  const isSuperAdmin = currentUser.value?.roles?.includes('super_admin')
  if (isSuperAdmin) return ''
  if (config.optionKey === 'roles' && !currentUser.value?.roles?.includes(String(option.code || ''))) {
    return '只能分配当前管理员自己实际持有的角色'
  }
  if (config.optionKey === 'permissions' && !currentUser.value?.permissions?.includes(String(option.code || ''))) {
    return '只能授予当前管理员自己拥有的有效权限'
  }
  return ''
}

function assignmentOptionDisabledReason(option: BasicItem): string {
  if (assignmentScopeBlockedReason.value) return assignmentScopeBlockedReason.value
  return assignmentTarget.value ? assignmentOptionScopeReason(option, assignmentTarget.value) : ''
}

async function saveAssignment() {
  const config = assignmentConfig.value
  if (!assignmentTarget.value || !config || !assignmentOptionsReady.value || assignmentSaving.value || assignmentScopeBlockedReason.value) return
  assignmentSaving.value = true
  assignmentSaveError.value = ''
  try {
    await request(config.endpoint(assignmentTarget.value.id), {
      method: 'POST',
      body: {[config.payloadKey]: selectedAssignmentIDs.value},
    }, token.value)
    closeAssignment()
    await loadActiveModule()
    panelMessage.value = '权限配置已保存'
    ElMessage.success('权限配置已保存')
  } catch (error) {
    assignmentSaveError.value = error instanceof Error ? error.message : '权限配置保存失败'
  } finally {
    assignmentSaving.value = false
  }
}

async function openUserAffiliation(row: any) {
  if (!canEditUserAffiliation.value) {
    ElMessage.warning('修正账号归属需要部门查看和终端查看权限。')
    return
  }
  if (assignmentTarget.value) {
    if (assignmentSaving.value) {
      ElMessage.warning('权限配置正在保存，请等待完成后再切换。')
      return
    }
    const currentConfig = assignmentConfig.value
    const original = currentConfig && Array.isArray(assignmentTarget.value[currentConfig.selectedKey])
      ? (assignmentTarget.value[currentConfig.selectedKey] as unknown[]).map(Number).sort((left, right) => left - right).join(',')
      : ''
    const selected = [...selectedAssignmentIDs.value].map(Number).sort((left, right) => left - right).join(',')
    if (original !== selected) {
      try { await appMessageBox.confirm('当前权限配置尚未保存，切换后修改将丢失。', '切换操作？', {confirmButtonText: '放弃并切换', cancelButtonText: '继续配置', type: 'warning'}) } catch { return }
    }
    closeAssignment()
  }
  if (affiliationTarget.value && Number(affiliationTarget.value.id) === Number(row.id)) return
  if (affiliationTarget.value) {
    if (affiliationSaving.value) {
      ElMessage.warning('账号归属正在保存，请等待完成后再切换。')
      return
    }
    if (affiliationDepartmentID.value !== affiliationInitial.value.departmentID || affiliationTerminalID.value !== affiliationInitial.value.terminalID) {
      try { await appMessageBox.confirm('当前账号归属尚未保存，切换后修改将丢失。', '切换账号？', {confirmButtonText: '放弃并切换', cancelButtonText: '继续编辑', type: 'warning'}) } catch { return }
    }
  }
  affiliationTarget.value = row
  affiliationDepartmentID.value = row.department_id ? Number(row.department_id) : undefined
  affiliationTerminalID.value = row.terminal_id ? Number(row.terminal_id) : undefined
  affiliationInitial.value = {departmentID: affiliationDepartmentID.value, terminalID: affiliationTerminalID.value}
  affiliationError.value = ''
}

async function closeUserAffiliation(done?: () => void) {
  if (affiliationSaving.value) return
  if (affiliationDepartmentID.value !== affiliationInitial.value.departmentID || affiliationTerminalID.value !== affiliationInitial.value.terminalID) {
    try { await appMessageBox.confirm('账号归属修改尚未保存，确认关闭？', '放弃修改', {type: 'warning'}) } catch { return }
  }
  affiliationError.value = ''
  affiliationTarget.value = null
  done?.()
}

async function saveUserAffiliation() {
  if (!affiliationTarget.value || affiliationSaving.value) return
  affiliationSaving.value = true
  affiliationError.value = ''
  try {
    await request(`/api/v1/system/users/${affiliationTarget.value.id}/affiliation`, {method: 'PATCH', body: {department_id: affiliationDepartmentID.value || null, terminal_id: affiliationTerminalID.value || null}}, token.value)
    const changedCurrentUser = Number(affiliationTarget.value.id) === Number(currentUser.value?.id)
    affiliationTarget.value = null
    await loadActiveModule()
    if (changedCurrentUser) await loadMe()
    ElMessage.success('账号归属已更新')
  } catch (error) { affiliationError.value = error instanceof Error ? error.message : '账号归属保存失败' } finally { affiliationSaving.value = false }
}

function clearSessionRefreshTimer() {
  if (sessionRefreshTimer !== undefined) {
    window.clearTimeout(sessionRefreshTimer)
    sessionRefreshTimer = undefined
  }
}

function scheduleSessionRefresh() {
  clearSessionRefreshTimer()
  if (!token.value || !refreshToken.value) return

  const expiresAt = Date.parse(tokenExpiresAt.value)
  if (Number.isNaN(expiresAt)) return
  const delay = Math.max(0, expiresAt - Date.now() - sessionRefreshLeadMs)
  sessionRefreshTimer = window.setTimeout(() => {
    void refreshSession().catch(() => handleAuthFailure())
  }, delay)
}

function applyAuthResponse(data: AuthResponse) {
  token.value = data.access_token
  tokenExpiresAt.value = data.expires_at
  refreshToken.value = data.refresh_token
  currentUser.value = data.user
  localStorage.setItem(tokenKey, data.access_token)
  localStorage.setItem(tokenExpiresAtKey, data.expires_at)
  localStorage.setItem(refreshTokenKey, data.refresh_token)
  scheduleSessionRefresh()
}

async function refreshSession(): Promise<string> {
  if (refreshInFlight) return refreshInFlight
  const expectedRefreshToken = refreshToken.value
  const expectedGeneration = authRequestGeneration
  if (!expectedRefreshToken) throw new Error('登录会话已失效，请重新登录')

  refreshInFlight = (async () => {
    const data = await request<AuthResponse>('/api/v1/auth/refresh', {
      method: 'POST',
      body: {refresh_token: expectedRefreshToken},
    })
    if (expectedGeneration !== authRequestGeneration || refreshToken.value !== expectedRefreshToken) {
      throw new Error('登录会话已结束，请重新登录')
    }
    applyAuthResponse(data)
    return data.access_token
  })().finally(() => {
    refreshInFlight = null
  })
  return refreshInFlight
}

function handleAuthFailure() {
  authRequestGeneration += 1
  clearAuthSession()
  errorMessage.value = '登录已失效，请重新登录'
}

function refreshOnSessionActivity() {
  if (document.visibilityState !== 'visible') return
  if (!token.value || !refreshToken.value) return
  const expiresAt = Date.parse(tokenExpiresAt.value)
  if (!Number.isNaN(expiresAt) && expiresAt > Date.now() + sessionRefreshLeadMs) {
    scheduleSessionRefresh()
    return
  }
  void refreshSession().catch(() => handleAuthFailure())
}

async function login() {
  if (loading.value) return
  const requestGeneration = ++authRequestGeneration
  const requestServerAddress = apiBaseUrl()
  loading.value = true
  errorMessage.value = ''
  let loginFailed = false
  try {
    const data = await request<AuthResponse>('/api/v1/auth/login', {
      method: 'POST',
      body: loginForm,
    })
    if (requestGeneration !== authRequestGeneration || requestServerAddress !== apiBaseUrl()) {
      throw new Error('服务器地址已变化，本次登录结果已失效，请重新登录')
    }
    applyAuthResponse(data)
    await bootstrap(requestGeneration, requestServerAddress)
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '登录失败'
    loginFailed = true
  } finally {
    loading.value = false
    if (loginFailed) {
      await nextTick()
      loginUsernameInput.value?.focus()
    }
  }
}

function clearAuthSession() {
  clearSessionRefreshTimer()
  token.value = ''
  refreshToken.value = ''
  tokenExpiresAt.value = ''
  currentUser.value = null
  localStorage.removeItem(tokenKey)
  localStorage.removeItem(refreshTokenKey)
  localStorage.removeItem(tokenExpiresAtKey)
}

async function logout() {
  if (!(await dirtyGuardRegistry.confirmLeave('logout'))) return
  const activeRefreshToken = refreshToken.value
  authRequestGeneration += 1
  clearAuthSession()
  if (!activeRefreshToken) return
  try {
    await request<void>('/api/v1/auth/logout', {
      method: 'POST',
      body: {refresh_token: activeRefreshToken},
    })
  } catch {
    // 本地会话已经清理，服务端撤销失败不阻止用户退出当前设备。
  }
}

async function bootstrap(expectedGeneration = authRequestGeneration, expectedServerAddress = apiBaseUrl()) {
  const sessionToken = token.value
  try {
    await loadMe()
  } catch (error) {
    if (expectedGeneration === authRequestGeneration) {
      logout()
      errorMessage.value = error instanceof Error ? `登录身份校验失败：${error.message}` : '登录身份校验失败，请重新登录'
    }
    throw error
  }
  if (expectedGeneration !== authRequestGeneration || expectedServerAddress !== apiBaseUrl() || !token.value || !currentUser.value) {
    if (token.value === sessionToken) clearAuthSession()
    throw new Error('登录会话已失效，请重新登录')
  }
  await Promise.allSettled([loadHealth(), preloadBaseData()])
  if (expectedGeneration !== authRequestGeneration || expectedServerAddress !== apiBaseUrl()) {
    if (token.value === sessionToken) clearAuthSession()
    throw new Error('登录会话已失效，请重新登录')
  }
  await loadActiveModule()
}

async function loadHealth() {
  healthStatus.value = 'checking'
  try {
    await request('/health')
    healthStatus.value = 'healthy'
  } catch {
    healthStatus.value = 'error'
  } finally {
    lastHealthCheckAt.value = new Date().toISOString()
  }
}

function assignmentTargetDisabled(row: Record<string, unknown>): boolean {
  if (activeKey.value === 'users') return Number(row.id) === Number(currentUser.value?.id)
  if (activeKey.value === 'roles') return row.code === 'super_admin'
  return false
}

function assignmentTargetHint(row: Record<string, unknown>): string {
  if (activeKey.value === 'users' && Number(row.id) === Number(currentUser.value?.id)) return '不能修改当前登录账号自己的角色'
  if (activeKey.value === 'roles' && row.code === 'super_admin') return '超级管理员是锁定的系统角色'
  return ''
}

function setPageDetailPanelVisible(visible: boolean) {
  pageDetailPanelVisible.value = visible
}

function handleModuleUnavailableEvent(event: Event) {
  const detail = (event as CustomEvent<{path?: string; message?: string}>).detail
  const moduleKey = deferredModuleForPath(detail?.path || '')
  if (!moduleKey || activeKey.value !== moduleKey) return
  moduleUnavailable.value = {module: moduleKey, message: detail?.message || '此功能暂不可用'}
  // A failing request must not discard an open draft. The owning panel keeps
  // its error and values visible until the user explicitly leaves it.
  panelMessage.value = '此功能暂不可用，已填写内容仍保留，请确认后再关闭'
}

async function loadMe() {
  if (!token.value) return
  currentUser.value = await request<CurrentUser>('/api/v1/auth/me', {}, token.value)
}

async function loadStatistics() {
  statisticsData.value = await request<StatisticsDashboard>('/api/v1/statistics', {}, token.value)
  rows.value = []
  columns.value = []
  pageTotal.value = 0
}

function formatGenericCell(column: string, value: unknown): string {
  if (column === 'operator_employee_name' && !value) return '历史记录未记录员工'
  if (column === 'account_type') {
    if (value === 'personal') return '个人账号'
    if (value === 'department_terminal') return '部门终端账号'
  }
  if (column === 'department_id' && value) {
    const department = rowsFor('departments').find((item) => Number(item.id) === Number(value))
    return department ? `${department.name || department.code}（#${value}）` : `#${value}`
  }
  if (column === 'terminal_id' && value) {
    const terminal = rowsFor('terminals').find((item) => Number(item.id) === Number(value))
    return terminal ? `${terminal.name || terminal.code}（#${value}）` : `#${value}`
  }
  if (column === 'organization_id' && value) return `#${value}`
  if (column === 'created_at' && value) {
    const date = new Date(String(value))
    return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString('zh-CN', {hour12: false})
  }
  if (column.endsWith('_at') && value) return formatDate(value)
  return formatCell(value)
}

function genericRowTitle(row: BasicItem): string {
  const value = formatGenericCell(genericIdentityColumns.value.primary, row[genericIdentityColumns.value.primary])
  return value === '-' ? `${activeModule.value?.title || '记录'} #${row.id}` : value
}

function genericRowSubtitle(row: BasicItem): string {
  const column = genericIdentityColumns.value.secondary
  if (!column) return `编号 ${row.id}`
  const value = formatGenericCell(column, row[column])
  return value === '-' ? `编号 ${row.id}` : `${columnLabel(column)}：${value}`
}

function departmentName(id: unknown): string {
  const item = rowsFor('departments').find((department) => Number(department.id) === Number(id))
  return String(item?.name || `部门#${id}`)
}

function canOperateDepartmentTask(task: BasicItem): boolean {
  if (!canWriteActive.value || ['draft', 'completed'].includes(String(task.status))) return false
  if (!selectedWorkOrder.value || selectedWorkOrder.value.status === 'paused') return false
  if (currentUser.value?.department_id) {
    return Number(currentUser.value.department_id) === Number(task.department_id)
  }
  return true
}

function trendBarPercentage(item: StatisticTrendItem): number {
  const usesQuantity = item.quantity !== undefined
  const peers = compactTrendItems.value.filter((candidate) => (candidate.quantity !== undefined) === usesQuantity)
  const values = peers.map((candidate) => Math.abs(Number(usesQuantity ? candidate.quantity : candidate.value) || 0))
  const maximum = Math.max(...values, 0)
  if (!maximum) return 0
  return Math.max(4, Math.round(Math.abs(Number(usesQuantity ? item.quantity : item.value) || 0) * 100 / maximum))
}

function workorderNextAction(item: BasicItem): string {
  const status = String(item.status || '')
  if (status === 'draft') return hasPermission('workorder:write') ? '办公室派发任务' : '等待办公室派发'
  if (status === 'processing') return '各部门继续处理并回报进度'
  if (status === 'paused') return hasPermission('workorder:write') ? '办公室确认后恢复任务' : '等待办公室恢复任务'
  if (status === 'pending_close') return hasPermission('workorder:write') ? '办公室核对并确认完成' : '等待办公室确认完成'
  if (status.startsWith('completed')) return '任务已结束，可查看流转日志'
  return '查看部门进度与流转日志'
}

configureAuthSession({
  getToken: () => token.value,
  refresh: refreshSession,
  onFailure: handleAuthFailure,
})

function workspaceSubmitInProgress(): boolean {
  return Boolean(
    (showCreateForm.value && loading.value)
    || affiliationSaving.value
    || movementSubmitting.value
    || quickSupplierSubmitting.value
    || temporaryProductSubmitting.value
    || actionSubmitting.value,
  )
}

function affiliationDirty(): boolean {
  return Boolean(affiliationTarget.value) && (
    affiliationDepartmentID.value !== affiliationInitial.value.departmentID
    || affiliationTerminalID.value !== affiliationInitial.value.terminalID
  )
}

function temporaryProductDirty(): boolean {
  if (!temporaryProductDialogVisible.value) return false
  return Object.values(temporaryProductForm).some((value) => String(value ?? '').trim() !== '')
}

function actionFormDirty(): boolean {
  if (!actionDialogVisible.value) return false
  return Object.values(actionForm).some((value) => String(value ?? '').trim() !== '')
}

function quickSupplierDirty(): boolean {
  return showQuickSupplier.value && Object.values(quickSupplier).some((value) => String(value ?? '').trim() !== '')
}

function workspaceHasUnsavedChanges(): boolean {
  return Boolean(
    (showCreateForm.value && createFormDirty())
    || (warehouseDrawerVisible.value && movementFormDirty.value)
    || quickSupplierDirty()
    || affiliationDirty()
    || temporaryProductDirty()
    || actionFormDirty(),
  )
}

useDirtyGuard('workspace-business-state', {
  busy: workspaceSubmitInProgress,
  dirty: workspaceHasUnsavedChanges,
  busyMessage: '业务操作正在提交，请等待完成后再离开当前页面',
  dirtyMessage: '当前页面仍有未保存的表单或配置，离开后填写内容将丢失。',
})

onMounted(() => {
  window.addEventListener('focus', refreshOnSessionActivity)
  window.addEventListener(moduleUnavailableEvent, handleModuleUnavailableEvent)
  document.addEventListener('visibilitychange', refreshOnSessionActivity)
  scheduleSessionRefresh()
  if (token.value) {
    const requestGeneration = ++authRequestGeneration
    const requestServerAddress = apiBaseUrl()
    void bootstrap(requestGeneration, requestServerAddress).catch((error) => {
      if (!token.value) errorMessage.value = error instanceof Error ? `登录已失效：${error.message}` : '登录已失效，请重新登录'
    })
  } else {
    void loadHealth()
  }
})

onBeforeUnmount(() => {
  clearSessionRefreshTimer()
  disposeWorkorderOperations()
  window.removeEventListener('focus', refreshOnSessionActivity)
  window.removeEventListener(moduleUnavailableEvent, handleModuleUnavailableEvent)
  document.removeEventListener('visibilitychange', refreshOnSessionActivity)
  configureAuthSession(null)
})

const workorderContext = {
  list: {
    rows, loading, listError, pageTotal, page, pageSize, filteredEmptyTitle, filteredEmptyDescription,
    loadActiveModule, handlePageChange, handlePageSizeChange, applySearch,
    workorderStatusFilter, workorderTypeFilter, workorderPriorityFilter,
    workorderStatusOptions, workorderTypeOptions, workorderPriorityOptions,
    workorderTypeLabel, workorderStatusLabel,
    workorderStatusTone, formatQuantity, departmentProgressSummary, departmentProgressMetrics,
    formatDate, workorderDueState, openWorkOrder,
  },
  product: {
    operatorDirectory, formState, formError, loading, temporaryProductForm,
    temporaryProductDialogVisible, temporaryProductSubmitting, temporaryProductError,
    workorderProductOptions, workorderProductSearchLoading, workorderProductSearchError,
    workorderProductStock, workorderProductStockLoading, workorderProductStockError,
    workorderProductStockUpdatedAt, hasPermission, stockState, formatQuantity,
    searchWorkorderProducts, handleWorkorderProductSelect, loadWorkorderProductStock,
    openTemporaryProductDialog, closeTemporaryProductWithGuard, createTemporaryProduct,
  },
  detail: {
    token, selectedWorkOrder, workorderDrawerVisible, workorderLogs, workorderLogsLoading,
    workorderLogsError, workorderDrawerProductStock, workorderDrawerProductStockLoading,
    workorderDrawerProductStockError, workorderDrawerProductStockUpdatedAt, hasPermission,
    canWriteActive, workorderTypeLabel, workorderStatusLabel, workorderStatusTone,
    formatQuantity, formatDate, workorderDueState, workorderNextAction,
    departmentProgressSummary, departmentProgressMetrics, departmentTasks, departmentName,
    departmentTaskStatusLabel, departmentTaskStatusTone, canOperateDepartmentTask,
    workorderActionLabel, loadWorkorderDrawerProductStock, loadWorkOrderLogs,
    closeWorkOrder, handleWorkOrderBeforeClose, resetWorkOrder, dispatchWorkOrder,
    pauseWorkOrder, resumeWorkOrder, toggleWorkOrderUrgent, completeWorkOrder,
    startDepartmentTask, partialCompleteDepartmentTask, completeDepartmentTask,
  },
  action: {
    operatorDirectory, selectedWorkOrder, actionDialogVisible, actionKind, actionTarget,
    actionSubmitting, actionError, actionFieldErrors, actionForm, closeWorkOrderAction,
    submitWorkOrderAction, departmentName, formatQuantity,
  },
}

  return {
    workorderContext,
    assignmentConfigs,
    operatorDirectory,
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
    moduleUnavailable,
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
    workorderDrawerVisible,
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
    lastHealthCheckAt,
    loginForm,
    loginUsernameInput,
    formError,
    formState,
    movementForm,
    quickSupplier,
    activeWarehouseTab,
    statisticsData,
    pageDetailPanelVisible,
    statisticsSourcesUnavailable,
    statisticsSourceUnavailable,
    affiliationTarget,
    affiliationDepartmentID,
    affiliationTerminalID,
    affiliationTerminalOptions,
    affiliationSaving,
    affiliationError,
    warehouseTabs,
    warehouseTabOptions,
    movementDefinitions,
    navItems,
    businessItems,
    systemItems,
    activeModule,
    canWriteActive,
    canEditUserAffiliation,
    canCreateDepartmentTerminalUser,
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
    assignmentScopeBlockedReason,
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
    registerModuleLeaveGuard,
    handleUserCommand,
    resetFilters,
    openAssignment,
    assignmentOptionsRequestToken,
    closeAssignment,
    loadAssignmentOptions,
    retryAssignmentOptions,
    isAssignmentOptionDisabled,
    assignmentOptionDisabledReason,
    assignmentTargetDisabled,
    assignmentTargetHint,
    setPageDetailPanelVisible,
    saveAssignment,
    openUserAffiliation,
    closeUserAffiliation,
    saveUserAffiliation,
    switchWarehouseTab,
    resetListQuery,
    applySearch,
    handlePageChange,
    handlePageSizeChange,
    login,
    clearAuthSession,
    logout,
    bootstrap,
    loadHealth,
    loadMe,
    preloadBaseData,
    loadActiveModule,
    loadStatistics,
    loadList,
    createItem,
    clearForm,
    toggleCreateForm,
    editSupplier,
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
    columnLabel,
    invalidateWarehouseRequests,
    openWarehouseItem,
    closeWarehouseItem,
    requestWarehouseClose,
    performWarehouseClose,
    handleWarehouseBeforeClose,
    resetWarehouseItem,
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
    departmentName,
    inventoryItemTypeLabel,
    departmentCompletionRate,
    trendNameLabel,
    trendBarPercentage,
  }
}
