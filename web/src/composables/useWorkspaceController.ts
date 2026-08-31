import {computed, nextTick, onBeforeUnmount, onMounted, ref, watch} from 'vue'
import {appMessageBox} from './useAppMessageBox'
import {useAssignment} from './useAssignment'
import {useAuth} from './useAuth'
import {useMold} from './useMold'
import {useModuleData} from './useModuleData'
import {useOperatorEmployees} from './useOperatorEmployees'
import {useWarehouse} from './useWarehouse'
import {useWorkorder} from './useWorkorder'
import {ElMessage} from 'element-plus'
import {ApiError, apiBaseUrl, configureAuthSession, desktopAppVersion, downloadApiFile, request, saveDesktopServerUrl, testDesktopServerUrl} from '../api/http'
import type {MetricTone} from '../components/ui/MetricCard.vue'
import type {StatusTone} from '../components/ui/StatusTag.vue'
import {type ModuleItem, modules} from '../data/modules'
import type {BasicItem, ClientUpdateStatus, CurrentUser, PaginatedResponse, SkeletonResponse} from '../types'

type AuthResponse = {
  access_token: string
  expires_at: string
  refresh_token: string
  refresh_expires_at: string
  user: CurrentUser
}

let fallbackIdempotencySequence = 0

function formatUuidBytes(bytes: Uint8Array): string {
  const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

function fallbackHex(length: number): string {
  let result = ''
  while (result.length < length) {
    result += Math.floor(Math.random() * 0x100000000).toString(16).padStart(8, '0')
  }
  return result.slice(0, length)
}

function fallbackIdempotencyKey(): string {
  // Idempotency keys are not secrets; preserve uniqueness when Web Crypto is unavailable.
  const timestamp = Date.now().toString(16).padStart(12, '0')
  const sequence = (fallbackIdempotencySequence++ % 0x100000000).toString(16).padStart(8, '0')
  const hex = `${timestamp}${sequence}${fallbackHex(12)}`
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-4${hex.slice(13, 16)}-8${hex.slice(17, 20)}-${hex.slice(20)}`
}

function createIdempotencyKey(): string {
  const webCrypto = typeof crypto !== 'undefined' ? crypto : undefined
  if (webCrypto && typeof webCrypto.randomUUID === 'function') {
    try {
      return webCrypto.randomUUID()
    } catch {
      // Continue with the next available generator for incomplete WebView implementations.
    }
  }
  if (webCrypto && typeof webCrypto.getRandomValues === 'function') {
    try {
      const bytes = new Uint8Array(16)
      webCrypto.getRandomValues(bytes)
      bytes[6] = (bytes[6] & 0x0f) | 0x40
      bytes[8] = (bytes[8] & 0x3f) | 0x80
      return formatUuidBytes(bytes)
    } catch {
      // Fall back to a local unique key if Web Crypto is present but unusable.
    }
  }
  return fallbackIdempotencyKey()
}


/**
 * Creates the authenticated workspace state shared by Web and Tauri.
 *
 * Domain views receive this plain object through Vue injection so refs remain reactive
 * after destructuring, while each API workflow still keeps its cancellation state local.
 */
export function useWorkspaceController() {
let activeModuleLeaveGuard: (() => Promise<boolean>) | null = null
type FormField = {
  key: string
  label: string
  kind?: 'text' | 'password' | 'select' | 'multi-select' | 'textarea' | 'date' | 'workorder-product' | 'workorder-quantity' | 'operator'
  options?: Array<{ label: string; value: string | number; disabled?: boolean }>
  required?: boolean
}

type MovementDefinition = {
  key: string
  title: string
  requiredAny?: string[]
  requiredAll?: string[]
}

type StatisticNameValue = { name: string; value: number; amount?: number }
type MetricCardItem = {label: string; value: string; caption: string; tone: MetricTone; statusLabel?: string; statusTone?: StatusTone}
type StatisticTrendItem = { date: string; name?: string; value: number; quantity?: number; amount?: number }
type DepartmentStatistic = { department_id: number; name: string; total: number; completed: number; processing: number; partial: number; received: number }
type StockStatisticItem = { item_type: string; item_id: number; name: string; code: string; category: string; quantity: number; safety_stock: number; amount?: number }
type MoldStatisticItem = { id: number; code: string; name: string; status: string; current_location?: string; next_maintenance_at?: string }
type StatisticsDashboard = {
  generated_at: string
  can_view_cost: boolean
  summary: Record<string, number>
  inventory: { by_item_type: StatisticNameValue[]; by_material_type: StatisticNameValue[]; low_stock: StockStatisticItem[]; trend: StatisticTrendItem[] }
  workorders: { by_status: StatisticNameValue[]; by_type: StatisticNameValue[]; by_department: DepartmentStatistic[]; trend: StatisticTrendItem[] }
  molds: { by_status: StatisticNameValue[]; need_care: MoldStatisticItem[] }
  business: { by_master_data: StatisticNameValue[] }
  audit: { by_result: StatisticNameValue[]; trend: StatisticTrendItem[] }
  recent_workorders: BasicItem[]
}

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
  serverDialogVisible,
  serverTesting,
  serverUrlInput,
  serverMessage,
  serverMessageType,
  clientUpdate,
  loginForm,
  loginUsernameInput,
  formError,
} = useAuth()

const operatorDirectory = useOperatorEmployees(token)

let authRequestGeneration = 0
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
  mobileNavOpen,
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
} = useWarehouse()

const {
  moldDetailDrawerVisible,
  selectedMoldDetail,
  selectedMoldID,
  moldDetailLoading,
  moldDetailError,
  moldActionSubmitting,
  moldActionError,
} = useMold()

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
} = useWorkorder()

const statisticsData = ref<StatisticsDashboard | null>(null)
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
const selectedMoldMaintenanceState = computed(() => moldMaintenanceState(selectedMoldDetail.value || {}))
const selectedMoldAlertType = computed<'success' | 'warning' | 'error' | 'info'>(() => {
  if (selectedMoldMaintenanceState.value.tone === 'danger') return 'error'
  return selectedMoldMaintenanceState.value.tone
})
const warehouseTabs = [
  {key: 'product', title: '产品'},
  {key: 'production_material', title: '生产物资'},
  {key: 'regular_product', title: '常规产品'},
  {key: 'daily_supply', title: '生活物资'},
]
let confirmedWarehouseTab = 'product'
const warehouseTabOptions = warehouseTabs.map((item) => ({label: item.title, value: item.key}))
const movementDefinitions: MovementDefinition[] = [
  {key: 'purchase_inbound', title: '采购入库', requiredAll: ['suppliers:read']},
  {key: 'return_rework_inbound', title: '退货返工', requiredAny: ['customers:read', 'system:departments:read']},
  {key: 'customer_outbound', title: '客户出库', requiredAll: ['customers:read']},
  {key: 'department_outbound', title: '部门出库', requiredAll: ['system:departments:read']},
]
const workorderStatusOptions = [
  {label: '全部状态', value: ''},
  {label: '草稿', value: 'draft'},
  {label: '正在处理', value: 'processing'},
  {label: '暂停', value: 'paused'},
  {label: '待办公室确认', value: 'pending_close'},
  {label: '正常完成', value: 'completed_normal'},
  {label: '强制完成', value: 'completed_forced'},
  {label: '取消', value: 'cancelled'},
]
const workorderTypeOptions = [
  {label: '全部类型', value: ''},
  {label: '生产单', value: 'production'},
  {label: '通用任务', value: 'general'},
]
const workorderPriorityOptions = [
  {label: '全部优先级', value: ''},
  {label: '普通', value: 'normal'},
  {label: '加急', value: 'urgent'},
]

const navItems = computed(() => modules.filter(canReadModule))
const businessItems = computed(() => navItems.value.filter((item) => item.group === 'business'))
const systemItems = computed(() => navItems.value.filter((item) => item.group === 'system'))
const activeModule = computed(() => modules.find((item) => item.key === activeKey.value))
const canWriteActive = computed(() => !!activeModule.value && canWriteModule(activeModule.value))
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
const userInitial = computed(() => (currentUser.value?.name || currentUser.value?.username || '用户').slice(0, 1))
const greeting = computed(() => {
  const hour = new Date().getHours()
  if (hour < 11) return '早上好'
  if (hour < 14) return '中午好'
  if (hour < 18) return '下午好'
  return '晚上好'
})
const quickActionDefinitions = [
  {key: 'workorder', title: '任务单', description: '查看当前任务与部门处理进度', icon: '✓'},
  {key: 'warehouses', title: '仓库', description: '查询库存并办理物品出入库', icon: '▦'},
  {key: 'customers', title: '客户档案', description: '查找或新增客户资料', icon: '◎'},
  {key: 'suppliers', title: '供应商', description: '维护采购供应商资料', icon: '↙'},
  {key: 'molds', title: '模具台账', description: '查询模具位置与保养状态', icon: '◇'},
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
    icon: '▣',
    items: businessItems.value.filter((item) => ['warehouses'].includes(item.key)),
  },
  {
    title: '客户与生产',
    caption: '客户资料与生产档案',
    icon: '◫',
    items: businessItems.value.filter((item) => ['customers', 'suppliers', 'molds', 'workorder'].includes(item.key)),
  },
  {
    title: '数据与报表',
    caption: '经营数据与统计结果',
    icon: '↗',
    items: businessItems.value.filter((item) => item.key === 'statistics'),
  },
].filter((group) => group.items.length))
const accountTypeText = computed(() => {
  if (!currentUser.value) return '未登录'
  return currentUser.value.account_type === 'department_terminal' ? '部门终端账号' : '个人账号'
})
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
    label: '业务服务',
    value: healthStatus.value === 'checking' ? '检查中' : healthStatus.value === 'healthy' ? '运行正常' : '暂不可用',
    caption: healthStatus.value === 'checking' ? '正在确认服务连接' : healthStatus.value === 'healthy' ? '可以继续办理业务' : '请检查服务连接后重试',
    tone: healthStatus.value === 'checking' ? 'info' : healthStatus.value === 'healthy' ? 'success' : 'danger',
    statusLabel: healthStatus.value === 'checking' ? '检查中' : healthStatus.value === 'healthy' ? '在线' : '异常',
    statusTone: healthStatus.value === 'checking' ? 'info' : healthStatus.value === 'healthy' ? 'success' : 'danger',
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
  {key: 'molds', label: '模具台账', title: '检查位置与保养日期', description: '关注维修、保养中与即将到期的模具。', tone: 'info' as StatusTone},
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
  const delta = decimalToScaled(movementForm.quantity)
  return movementIsOutbound.value ? current - delta : current + delta
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
  const maintenanceStates = rows.value.map(moldMaintenanceState)
  const needsFollowUp = rows.value.filter((row, index) => maintenanceStates[index]?.tone === 'warning' || ['repairing', 'maintenance'].includes(String(row.status))).length
  const needsAttention = rows.value.filter((row, index) => maintenanceStates[index]?.tone === 'danger' || row.status === 'scrapped').length
  return [
    {label: '当前页模具', value: String(rows.value.length), caption: `共 ${pageTotal.value} 条记录`, tone: 'neutral'},
    {label: '在库', value: String(rows.value.filter((row) => row.status === 'in_stock').length), caption: '当前可用台账状态', tone: 'success'},
    {label: '待跟进', value: String(needsFollowUp), caption: '7 天内到期、维修或保养中', tone: needsFollowUp ? 'warning' : 'success'},
    {label: '需立即关注', value: String(needsAttention), caption: '保养已逾期或模具已报废', tone: needsAttention ? 'danger' : 'success'},
  ]
})
const operationalSummaryCards = computed<MetricCardItem[]>(() => {
  if (activeKey.value === 'warehouses') return warehouseSummaryCards.value
  if (activeKey.value === 'workorder') return workorderSummaryCards.value
  if (activeKey.value === 'molds') return moldSummaryCards.value
  return []
})
const statisticsCards = computed<MetricCardItem[]>(() => {
  const summary = statisticsData.value?.summary || {}
  const lowStock = Number(summary.low_stock_items || 0)
  const urgent = Number(summary.urgent_workorders || 0)
  const pendingClose = Number(summary.pending_close_orders || 0)
  const moldsNeedCare = Number(summary.molds_need_care || 0)
  return [
    {label: '库存总量', value: formatQuantity(summary.inventory_quantity), caption: statisticsData.value?.can_view_cost ? `金额 ${formatMoney(summary.inventory_amount)}` : '金额按权限隐藏', tone: 'info'},
    {label: '低库存', value: String(lowStock), caption: '低于或等于安全库存', tone: lowStock ? 'danger' : 'success', statusLabel: lowStock ? '需处理' : '正常', statusTone: lowStock ? 'danger' : 'success'},
    {label: '进行中任务', value: String(summary.open_workorders || 0), caption: `加急 ${urgent} · 待确认 ${pendingClose}`, tone: urgent ? 'danger' : pendingClose ? 'warning' : 'info'},
    {label: '模具关注', value: String(moldsNeedCare), caption: `模具总数 ${summary.molds || 0}`, tone: moldsNeedCare ? 'warning' : 'success'},
    {label: '客户编码', value: String(summary.customers || 0), caption: '稳定关联编码总数', tone: 'neutral'},
    {label: '仓库物品', value: String(summary.warehouse_items || 0), caption: '产品与物资档案', tone: 'neutral'},
  ]
})
const compactTrendItems = computed(() => {
  const inventory = statisticsData.value?.inventory?.trend || []
  const workorders = statisticsData.value?.workorders?.trend || []
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

// formSchema 根据当前模块返回轻量新增表单，和后端已实现能力保持一致。
const formSchema = computed<FormField[]>(() => {
  const departmentOptions = rowsFor('departments').map((item) => ({
    label: item.name || item.code || `#${item.id}`,
    value: item.id
  }))
  const selectedDepartmentID = Number(formState.department_id || 0)
  const terminalOptions = rowsFor('terminals').filter((item) => (
    selectedDepartmentID > 0 && Number(item.department_id) === selectedDepartmentID
  )).map((item) => ({
    label: item.name || item.code || `#${item.id}`,
    value: item.id
  }))
  switch (activeKey.value) {
    case 'departments':
      return [
        {key: 'name', label: '部门名称', required: true},
        {key: 'code', label: '部门编码', required: true},
      ]
    case 'terminals':
      return [
        {key: 'department_id', label: '所属部门', kind: 'select', options: departmentOptions, required: true},
        {key: 'code', label: '终端编码', required: true},
        {key: 'name', label: '终端名称', required: true},
        {key: 'location', label: '位置说明'},
      ]
    case 'users':
      return [
        {key: 'username', label: '账号', required: true},
        {key: 'password', label: '密码', kind: 'password', required: true},
        {
          key: 'account_type',
          label: '账号类型',
          kind: 'select',
          required: true,
          options: [
            {label: '个人账号', value: 'personal'},
            {label: canCreateDepartmentTerminalUser.value ? '部门终端账号' : '部门终端账号（需要部门和终端查看权限）', value: 'department_terminal', disabled: !canCreateDepartmentTerminalUser.value},
          ],
        },
        {key: 'name', label: '姓名/终端名', required: true},
        {key: 'department_id', label: '所属部门', kind: 'select', options: departmentOptions, required: formState.account_type === 'department_terminal'},
        {key: 'terminal_id', label: '所属终端', kind: 'select', options: terminalOptions, required: formState.account_type === 'department_terminal'},
      ]
    case 'roles':
      return [
        {key: 'name', label: '角色名称', required: true},
        {key: 'code', label: '角色编码', required: true},
        {key: 'description', label: '说明'},
      ]
    case 'suppliers':
      return [
        {key: 'name', label: '供应商名称'},
        {key: 'code', label: '供应商编码'},
        {key: 'contact', label: '联系人'},
        {key: 'phone', label: '联系电话'},
        {key: 'address', label: '地址'},
      ]
    case 'warehouses':
      return [
        {key: 'name', label: `${activeWarehouseTabTitle.value}名称`},
        {key: 'code', label: `${activeWarehouseTabTitle.value}编码`},
        {key: 'unit', label: '单位'},
        {key: 'spec', label: '规格'},
        {key: 'safety_stock', label: '安全库存'},
        ...(hasPermission('cost:view') ? [{key: 'default_cost', label: '默认成本（元）'}] : []),
        {key: 'operator_employee_id', label: '本次操作人', kind: 'operator', required: true},
      ]
    case 'molds':
      return [
        {key: 'code', label: '模具编号'},
        {key: 'name', label: '模具名称'},
        {key: 'customer_id', label: '客户ID'},
        {key: 'product_id', label: '产品ID'},
        {key: 'cavity_count', label: '穴数'},
        {key: 'mold_material', label: '成型材料'},
        {key: 'steel', label: '钢材'},
        {key: 'size', label: '尺寸'},
        {key: 'weight_gram', label: '重量g'},
        {key: 'manufacturer', label: '制造商'},
        {key: 'storage_location', label: '存放位置'},
        {key: 'maintenance_cycle_days', label: '保养周期天'},
      ]
    case 'workorder':
      return [
        {
          key: 'type',
          label: '任务类型',
          kind: 'select',
          options: [
            {label: '生产单', value: 'production'},
            {label: '通用任务', value: 'general'},
          ],
          required: true,
        },
        {key: 'code', label: '任务编号'},
        {key: 'title', label: '标题', required: formState.type === 'general'},
        {key: 'customer_id', label: '客户', kind: 'select', options: rowsFor('customers').map((item) => ({label: item.name || item.code || `#${item.id}`, value: item.id}))},
        ...(formState.type === 'production' ? [{key: 'product_id', label: '仓库产品', kind: 'workorder-product' as const, required: true}] : []),
        {key: 'planned_quantity', label: '计划数量', kind: formState.type === 'production' ? 'workorder-quantity' : 'text', required: formState.type === 'production'},
        {key: 'due_at', label: '交期', kind: 'date'},
        {
          key: 'priority',
          label: '优先级',
          kind: 'select',
          options: [
            {label: '普通', value: 'normal'},
            {label: '加急', value: 'urgent'},
          ],
        },
        {key: 'target_department_ids', label: '流转部门', kind: 'multi-select', options: departmentOptions, required: true},
        {key: 'description', label: '说明', kind: 'textarea', required: formState.type === 'general'},
        {key: 'operator_employee_id', label: '本次操作人', kind: 'operator', required: true},
      ]
    default:
      return []
  }
})

const activeWarehouseTabTitle = computed(() => warehouseTabs.find((tab) => tab.key === activeWarehouseTab.value)?.title || '物品')
const createEntityTitle = computed(() => activeKey.value === 'warehouses' ? '物品' : (activeModule.value?.title || ''))

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
  return item.key === 'dashboard' || hasPermission(item.readPermission)
}

function canWriteModule(item: ModuleItem): boolean {
  return !!item.writePermission && hasPermission(item.writePermission)
}

async function switchModule(key: string) {
  const target = modules.find((item) => item.key === key)
  if (!target || !canReadModule(target)) {
    panelMessage.value = '你的账号暂无该功能权限'
    activeKey.value = 'dashboard'
    return
  }
  if (showCreateForm.value && createFormDirty()) {
    try { await appMessageBox.confirm('当前表单尚未保存，确认离开？', '放弃修改', {type: 'warning'}) } catch { return }
  }
  if (activeModuleLeaveGuard && !(await activeModuleLeaveGuard())) return
  if (warehouseDrawerVisible.value) {
    const canClose = await requestWarehouseClose()
    if (!canClose) return
    performWarehouseClose()
  }
  activeKey.value = key
  rows.value = []
  columns.value = []
  pageTotal.value = 0
  skeletonResult.value = null
  listError.value = ''
  closeWorkOrder()
  closeMold()
  closeAssignment()
  showCreateForm.value = false
  editingSupplier.value = null
  resetListQuery()
  clearForm()
  void loadActiveModule()
}

function registerModuleLeaveGuard(guard: (() => Promise<boolean>) | null) {
  activeModuleLeaveGuard = guard
}

function selectMobileModule(key: string) {
  mobileNavOpen.value = false
  void switchModule(key)
}

function restoreMobileMenuFocus() {
  document.getElementById('mobile-menu-button')?.focus()
}

async function handleUserCommand(command: string) {
  if (command === 'server') openServerSettings()
  if (command === 'logout') {
    if (warehouseDrawerVisible.value) {
      const canClose = await requestWarehouseClose()
      if (!canClose) return
      warehouseDrawerVisible.value = false
      resetWarehouseItem()
    }
    logout()
  }
}

function resetFilters() {
  searchKeyword.value = ''
  workorderStatusFilter.value = ''
  workorderTypeFilter.value = ''
  workorderPriorityFilter.value = ''
  applySearch()
}

let assignmentOptionsRequestToken = 0

async function openAssignment(row: any) {
  const config = assignmentConfigs[activeKey.value]
  if (!config) return
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
  return Boolean(
    assignmentConfig.value?.isDisabled
    && assignmentTarget.value
    && assignmentConfig.value.isDisabled(assignmentTarget.value, option)
  )
}

async function saveAssignment() {
  const config = assignmentConfig.value
  if (!assignmentTarget.value || !config || !assignmentOptionsReady.value || assignmentSaving.value) return
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

function openUserAffiliation(row: any) {
  if (!canEditUserAffiliation.value) {
    ElMessage.warning('修正账号归属需要部门查看和终端查看权限。')
    return
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
  if (done) done(); else affiliationTarget.value = null
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

async function switchWarehouseTab(key: string) {
  const previous = confirmedWarehouseTab
  if (key === previous) return
  if (showCreateForm.value && loading.value) {
    activeWarehouseTab.value = previous
    return
  }
  if (showCreateForm.value && createFormDirty()) {
    try { await appMessageBox.confirm('仓库新增表单尚未保存，切换分类将放弃当前内容。', '切换仓库分类？', {type: 'warning'}) } catch {
      activeWarehouseTab.value = previous
      return
    }
  }
  confirmedWarehouseTab = key
  activeWarehouseTab.value = key
  resetListQuery()
  clearForm()
  void loadActiveModule()
}

function resetListQuery() {
  searchKeyword.value = ''
  page.value = 1
  pageTotal.value = 0
  workorderStatusFilter.value = ''
  workorderTypeFilter.value = ''
  workorderPriorityFilter.value = ''
}

function applySearch() {
  page.value = 1
  void loadActiveModule()
}

function handlePageChange(value: number) {
  page.value = value
  void loadActiveModule()
}

function handlePageSizeChange(value: number) {
  pageSize.value = value
  page.value = 1
  void loadActiveModule()
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
  if (loading.value || serverTesting.value) return
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

function openServerSettings() {
  serverUrlInput.value = apiBaseUrl()
  serverMessage.value = ''
  serverDialogVisible.value = true
}

async function testServerSetting() {
  if (serverTesting.value || (!token.value && loading.value)) return
  serverTesting.value = true
  serverMessage.value = ''
  try {
    await testDesktopServerUrl(serverUrlInput.value)
    serverMessageType.value = 'success'
    serverMessage.value = '连接成功，Go 服务可以访问'
  } catch (error) {
    serverMessageType.value = 'error'
    serverMessage.value = error instanceof Error ? error.message : '连接失败，请检查地址和网络'
  } finally {
    serverTesting.value = false
  }
}

function saveServerSetting() {
  if (serverTesting.value || (!token.value && loading.value)) return
  try {
    const previous = apiBaseUrl()
    const saved = saveDesktopServerUrl(serverUrlInput.value)
    serverUrlInput.value = saved
    serverMessageType.value = 'success'
    serverMessage.value = '服务器地址已保存'
    serverDialogVisible.value = false
    if (previous !== saved) {
      authRequestGeneration += 1
      if (token.value) {
        clearAuthSession()
        errorMessage.value = '服务器已切换，请使用新服务器的账号重新登录'
      }
    }
    void loadHealth()
    ElMessage.success('服务器地址已保存')
  } catch (error) {
    serverMessageType.value = 'error'
    serverMessage.value = error instanceof Error ? error.message : '服务器地址保存失败'
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
  const startupTasks: Promise<unknown>[] = [loadHealth(), preloadBaseData()]
  if (desktopClient) startupTasks.push(loadClientUpdate())
  await Promise.allSettled(startupTasks)
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
  }
}

async function loadMe() {
  if (!token.value) return
  currentUser.value = await request<CurrentUser>('/api/v1/auth/me', {}, token.value)
}

async function loadClientUpdate() {
  if (!desktopClient) return
  try {
    const currentVersion = await desktopAppVersion()
    const path = appendQuery('/api/v1/updates/client/status', {current_version: currentVersion})
    clientUpdate.value = await request<ClientUpdateStatus>(path)
  } catch {
    clientUpdate.value = {
      current_version: '',
      available: false,
      cached: false,
    }
  }
}

async function downloadClientUpdate() {
  if (!desktopClient || !clientUpdate.value.download_path) return
  try {
    await downloadApiFile(
      clientUpdate.value.download_path,
      clientUpdate.value.file_name || 'bb-erp-client-windows.zip',
      token.value,
    )
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '客户端安装包下载失败')
  }
}

async function preloadBaseData() {
  const candidates = [
    {key: 'departments', permission: 'system:departments:read'},
    {key: 'terminals', permission: 'system:terminals:read'},
    {key: 'materials', permission: 'material:read'},
    {key: 'products', permission: 'product:read'},
    {key: 'customers', permission: 'customers:read'},
    {key: 'suppliers', permission: 'suppliers:read'},
  ]
  const keys = candidates.filter((item) => hasPermission(item.permission)).map((item) => item.key)
  await Promise.allSettled(keys.map((key) => loadList(key, false)))
}

async function loadActiveModule() {
  const item = activeModule.value
  if (!item || item.key === 'dashboard') return
  if (!canReadModule(item)) {
    activeKey.value = 'dashboard'
    panelMessage.value = '你的账号暂无该功能权限'
    return
  }
  loading.value = true
  panelMessage.value = ''
  listError.value = ''
  skeletonResult.value = null
  try {
    if (item.key === 'customers') {
      // CustomerPage owns its grouped query, filter and paging state. Keep the
      // workspace controller from issuing the retired generic customer request.
      rows.value = []
      columns.value = []
      pageTotal.value = 0
    } else if (item.key === 'updates') {
      rows.value = []
      columns.value = []
      pageTotal.value = 0
    } else if (item.key === 'statistics') {
      await loadStatistics()
    } else {
      await loadList(item.key, true)
    }
    panelMessage.value = '已同步'
  } catch (error) {

    listError.value = error instanceof Error ? error.message : '加载失败'
    panelMessage.value = listError.value
  } finally {
    loading.value = false
  }
}

async function loadStatistics() {
  statisticsData.value = await request<StatisticsDashboard>('/api/v1/statistics', {}, token.value)
  rows.value = []
  columns.value = []
  pageTotal.value = 0
}

async function loadList(key: string, applyToPanel: boolean) {
  // `warehouse_records` is a view over warehouse documents, while `warehouses`
  // is the stock master list; keep this mapping here instead of changing module metadata.
  const item = modules.find((moduleItem) => moduleItem.key === key)
  let path = item?.path
  if (key === 'customers' && !applyToPanel) {
    const options = await request<BasicItem[]>('/api/v1/customers/options', {}, token.value)
    cache[key] = options.map((option) => ({
      ...option,
      name: String(option.short_name || option.name || option.code || `#${option.id}`),
    }))
    return
  }
  if (key === 'warehouse_records') {
    path = '/api/v1/warehouses'
  }
  if (!path) return
  if (key === 'warehouses') {
    path = appendQuery(path, {tab: activeWarehouseTab.value})
  }
  if (applyToPanel) {
    path = appendQuery(path, {
      page: page.value,
      page_size: pageSize.value,
      q: searchKeyword.value,
      status: key === 'workorder' ? workorderStatusFilter.value : undefined,
      type: key === 'workorder' ? workorderTypeFilter.value : undefined,
      priority: key === 'workorder' ? workorderPriorityFilter.value : undefined,
    })
  } else {
    path = appendQuery(path, {page: 1, page_size: 200})
  }
  const data = await request<BasicItem[] | PaginatedResponse<BasicItem> | SkeletonResponse>(path, {}, token.value)
  if (isPaginatedResponse(data)) {
    cache[key] = data.items
    if (applyToPanel) {
      rows.value = data.items
      page.value = data.page
      pageSize.value = data.page_size
      pageTotal.value = data.total
      if (item) {
        columns.value = inferColumns(data.items, item)
      }
    }
    return
  }
  if (!Array.isArray(data)) {
    if (applyToPanel) {
      skeletonResult.value = data
      rows.value = []
      columns.value = []
      pageTotal.value = 0
    }
    return
  }
  cache[key] = data
  if (applyToPanel) {
    rows.value = data
    pageTotal.value = data.length
    if (item) {
      columns.value = inferColumns(data, item)
    }
  }
}

async function createItem() {
  const item = activeModule.value
  if (!item?.path || !canWriteModule(item)) {
    panelMessage.value = '你的账号只有查看权限，不能新增数据'
    showCreateForm.value = false
    return
  }
  formError.value = validateActiveForm()
  if (formError.value) {
    if (['warehouses', 'workorder'].includes(activeKey.value) && !formState.operator_employee_id) {
      await nextTick()
      document.getElementById('create-form-operator')?.focus()
    }
    return
  }
  loading.value = true
  panelMessage.value = ''
  try {
    const isSupplierEdit = activeKey.value === 'suppliers' && editingSupplier.value
    const path = isSupplierEdit ? `${item.path}/${editingSupplier.value?.id}` : item.path
    await request(path, {
      method: isSupplierEdit ? 'PATCH' : 'POST',
      body: normalizedForm(),
    }, token.value)
    if (['warehouses', 'workorder'].includes(activeKey.value)) operatorDirectory.invalidate()
    if (['roles', 'permissions'].includes(item.key)) delete assignmentOptionsCache[item.key]
    clearForm()
    await preloadBaseData()
    await loadActiveModule()
    panelMessage.value = '已新增'
    if (isSupplierEdit) panelMessage.value = '已保存'
    ElMessage.success(isSupplierEdit ? '保存成功' : '新增成功')
    editingSupplier.value = null
    showCreateForm.value = false
  } catch (error) {
    if (['warehouses', 'workorder'].includes(activeKey.value) && operatorDirectory.handleSubmitError(error)) formState.operator_employee_id = undefined
    formError.value = error instanceof Error ? error.message : '保存失败'
    panelMessage.value = formError.value
  } finally {
    loading.value = false
  }
}

function normalizedForm(): Record<string, unknown> {
  const body: Record<string, unknown> = {}
  if (activeKey.value === 'users') {
    body.organization_id = currentUser.value?.organization_id
  }
  if (activeKey.value === 'warehouses') {
    body.tab = activeWarehouseTab.value
  }
  // The API stores quantity in ten-thousandths and money in cents. Convert once
  // at the boundary so display values remain human-readable and calculations stay exact.
  for (const field of formSchema.value) {
    const value = formState[field.key]
    if (value === '' || value === undefined) continue
    if (activeKey.value === 'warehouses' && field.key === 'safety_stock') {
      body[field.key] = decimalToScaled(value)
      continue
    }
    if (activeKey.value === 'warehouses' && field.key === 'default_cost') {
      body[field.key] = moneyToCents(value)
      continue
    }
    if (activeKey.value === 'workorder' && field.key === 'planned_quantity') {
      body[field.key] = decimalToScaled(value)
      continue
    }
    if (activeKey.value === 'workorder' && field.key === 'target_department_ids') {
      body[field.key] = Array.isArray(value) ? value.map(Number) : []
      continue
    }
    body[field.key] = numericKeys.has(field.key) || field.key.endsWith('_id') ? Number(value) : value
  }
  return body
}

function validateActiveForm(): string {
  if (activeKey.value === 'workorder' && formState.type === 'production' && !hasPermission('warehouse:read')) {
    return '当前账号缺少仓库查看权限，无法选择产品或创建生产单；请联系管理员授权或改为通用任务。'
  }
  const missing = formSchema.value.filter((field) => field.required && (
    formState[field.key] === undefined
    || formState[field.key] === null
    || (typeof formState[field.key] === 'string' && !formState[field.key].trim())
  ))
  if (missing.length) return `请填写必填项：${missing.map((field) => field.label).join('、')}`
  if (activeKey.value === 'users') {
    if (!currentUser.value?.organization_id) return '当前登录身份缺少组织信息，请重新登录后再试。'
    if (String(formState.password || '').length < 8) return '密码至少需要 8 个字符。'
    if (formState.account_type === 'department_terminal' && !canCreateDepartmentTerminalUser.value) return '创建部门终端账号需要部门查看和终端查看权限。'
    if (formState.account_type === 'department_terminal') {
      const terminal = rowsFor('terminals').find((item) => Number(item.id) === Number(formState.terminal_id))
      if (!terminal || Number(terminal.department_id) !== Number(formState.department_id)) {
        return '所选终端不属于当前部门，请重新选择部门和终端。'
      }
    }
  }
  if (activeKey.value === 'workorder') {
    if (!Array.isArray(formState.target_department_ids) || !formState.target_department_ids.length) return '请选择至少一个流转部门。'
    if (formState.type === 'production') {
      const quantity = String(formState.planned_quantity || '').trim()
      if (!/^\d+(\.\d{1,4})?$/.test(quantity) || Number(quantity) <= 0) return '生产单计划数量必须大于 0，且最多保留 4 位小数。'
    }
  }
  return ''
}

const numericKeys = new Set(['quantity', 'unit_cost', 'default_cost', 'safety_stock', 'customer_id', 'product_id', 'cavity_count', 'weight_gram', 'maintenance_cycle_days'])

function clearForm() {
  resetWorkorderProductSelection()
  for (const key of Object.keys(formState)) {
    delete formState[key]
  }
  formError.value = ''
}

async function toggleCreateForm() {
  if (showCreateForm.value && loading.value) return
  const formDirty = createFormDirty()
  if (showCreateForm.value && formDirty) {
    try { await appMessageBox.confirm('当前表单尚未保存，确认关闭？', '放弃修改', {type: 'warning'}) } catch { return }
  }
  editingSupplier.value = null
  clearForm()
  showCreateForm.value = !showCreateForm.value
  if (showCreateForm.value && ['warehouses', 'workorder'].includes(activeKey.value)) void operatorDirectory.load(true)
  if (showCreateForm.value && activeKey.value === 'workorder') {
    formState.type = 'production'
    formState.priority = 'normal'
    void searchWorkorderProducts('')
  } else if (!showCreateForm.value) {
    invalidateWorkorderProductSearch()
    closeTemporaryProductDialog()
  }
}

function createFormDirty(): boolean {
  return Object.entries(formState).some(([key, value]) => {
    if (activeKey.value === 'workorder' && ((key === 'type' && value === 'production') || (key === 'priority' && value === 'normal'))) return false
    if (editingSupplier.value && value === editingSupplier.value[key]) return false
    return Array.isArray(value) ? value.length > 0 : value !== '' && value !== undefined && value !== null
  })
}

function editSupplier(item: any) {
  editingSupplier.value = item
  clearForm()
  for (const key of ['name', 'code', 'contact', 'phone', 'address', 'status']) {
    const value = item[key]
    if (typeof value === 'string' || typeof value === 'number') formState[key] = value
  }
  showCreateForm.value = true
  window.scrollTo({top: 0, behavior: 'smooth'})
}

function inferColumns(data: BasicItem[], item: ModuleItem): string[] {
  const preferred: Record<string, string[]> = {
    users: ['id', 'username', 'account_type', 'name', 'organization_id', 'department_id', 'terminal_id', 'status'],
    departments: ['id', 'organization_id', 'name', 'code', 'status'],
    terminals: ['id', 'department_id', 'code', 'name', 'location', 'status'],
    roles: ['id', 'name', 'code', 'description'],
    suppliers: ['id', 'name', 'code', 'contact', 'phone', 'address', 'status'],
    warehouses: ['id', 'item_type', 'category', 'name', 'code', 'unit', 'spec', 'safety_stock', 'status'],
    materials: ['id', 'name', 'code', 'category', 'unit', 'spec', 'safety_stock', 'status'],
    products: ['id', 'name', 'code', 'unit', 'spec', 'safety_stock', 'status'],
    molds: ['id', 'code', 'name', 'status', 'current_location', 'storage_location', 'cavity_count', 'mold_material', 'steel', 'maintenance_cycle_days', 'next_maintenance_at'],
    permissions: ['id', 'name', 'code', 'object', 'action'],
    audits: ['id', 'operator_employee_name', 'operator_department_name', 'actor_username', 'terminal_id', 'action', 'object', 'result', 'created_at'],
  }
  const inferred = preferred[item.key] || Object.keys(data[0] || {})
  if (!hasPermission('cost:view')) {
    return inferred.filter((column) => !['avg_cost', 'amount', 'unit_cost', 'default_cost'].includes(column))
  }
  return inferred
}

function formatCell(value: unknown): string {
  if (value === null || value === undefined || value === '') return '-'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function genericStatusLabel(value: unknown): string {
  const status = String(value || 'unknown').toLowerCase()
  const labels: Record<string, string> = {
    active: '正常',
    enabled: '正常',
    normal: '正常',
    success: '成功',
    succeeded: '成功',
    inactive: '停用',
    disabled: '停用',
    stopped: '停用',
    failed: '失败',
    failure: '失败',
    error: '失败',
    denied: '拒绝',
    unknown: '未设置',
  }
  return labels[status] || formatCell(value)
}

function genericStatusTone(value: unknown): StatusTone {
  const status = String(value || 'unknown').toLowerCase()
  if (['active', 'enabled', 'normal', 'success', 'succeeded'].includes(status)) return 'success'
  if (['failed', 'failure', 'error', 'denied'].includes(status)) return 'danger'
  return 'info'
}

function isGenericStatusColumn(column: string): boolean {
  return column === 'status' || column === 'result'
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

function permissionDomainKey(option: BasicItem): string {
  const codePrefix = String(option.code || '').split(':')[0].trim().toLowerCase()
  const objectPath = String(option.object || '').toLowerCase()
  const candidates = [codePrefix, objectPath.replace(/^\/api\/v1\//, '').split('/')[0]]
  const aliases: Record<string, string> = {
    system: 'system', users: 'system', roles: 'system', permissions: 'system', audits: 'system', updates: 'system', departments: 'system', employees: 'system', terminals: 'system',
    warehouse: 'warehouse', inventory: 'warehouse', material: 'warehouse', materials: 'warehouse', product: 'warehouse', products: 'warehouse',
    workorder: 'workorder', mold: 'mold', molds: 'mold', customer: 'customers', customers: 'customers',
    supplier: 'suppliers', suppliers: 'suppliers', statistics: 'statistics', cost: 'cost',
  }
  for (const candidate of candidates) {
    if (aliases[candidate]) return aliases[candidate]
  }
  const fallback = codePrefix || objectPath.replace(/^\/api\/v1\//, '').replaceAll('/', ' · ') || '未分类'
  return `other:${fallback}`
}

function permissionDomainLabel(value: string): string {
  const labels: Record<string, string> = {
    system: '系统管理',
    warehouse: '仓库',
    inventory: '库存单据',
    workorder: '任务单',
    mold: '模具台账',
    customers: '客户',
    suppliers: '供应商',
    statistics: '统计报表',
    cost: '成本数据',
  }
  if (labels[value]) return labels[value]
  const fallback = value.startsWith('other:') ? value.slice(6) : value
  return `其他业务域 · ${fallback}`
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

function stockState(row: unknown): {label: string; tone: StatusTone} {
  const item = row as Record<string, unknown>
  const quantity = Number(item.quantity || 0)
  const safetyStock = Number(item.safety_stock || 0)
  if (quantity <= 0) return {label: '缺货', tone: 'danger'}
  if (quantity <= safetyStock) return {label: '低于安全库存', tone: 'warning'}
  return {label: '库存正常', tone: 'success'}
}

const columnLabels: Record<string, string> = {
  id: '编号', username: '账号', account_type: '账号类型', name: '名称', organization_id: '组织',
  department_id: '部门', terminal_id: '终端', status: '状态', code: '编码', description: '说明',
  phone: '电话', contact: '联系人', address: '地址', location: '位置', item_type: '对象类型', category: '分类',
  unit: '单位', spec: '规格', safety_stock: '安全库存', type: '业务类型', warehouse_id: '仓库',
  to_warehouse_id: '目标仓库', reason: '原因', location_id: '库位', item_id: '物品',
  quantity: '数量', avg_cost: '平均成本', amount: '金额', document_id: '单据',
  balance_qty: '结存数量', current_location: '当前位置', storage_location: '存放位置',
  cavity_count: '穴数', mold_material: '成型材料', steel: '钢材', maintenance_cycle_days: '保养周期',
  next_maintenance_at: '下次保养', object: '对象', action: '操作', actor_username: '操作账号',
  actor_account_type: '账号类型', person_name: '操作人', result: '结果', created_at: '操作时间',
  operator_employee_name: '操作员工', operator_department_name: '操作部门',
}

function columnLabel(column: string): string {
  return columnLabels[column] || column
}

// Drawer requests use both cancellation and a sequence check: cancellation is
// best-effort, while the sequence protects the UI when a response already won the race.
let warehouseDetailRequestToken = 0
let itemMovementsRequestToken = 0
let moldDetailRequestToken = 0
let workorderLogsRequestToken = 0
let workorderProductSearchRequestToken = 0
let workorderProductStockRequestToken = 0
let workorderDrawerProductStockRequestToken = 0
let warehouseDetailAbortController: AbortController | null = null
let itemMovementsAbortController: AbortController | null = null
let moldDetailAbortController: AbortController | null = null
let workorderLogsAbortController: AbortController | null = null
let workorderProductSearchAbortController: AbortController | null = null
let workorderProductStockAbortController: AbortController | null = null
let workorderDrawerProductStockAbortController: AbortController | null = null

function invalidateWarehouseRequests() {
  warehouseDetailAbortController?.abort()
  itemMovementsAbortController?.abort()
  warehouseDetailAbortController = null
  itemMovementsAbortController = null
  warehouseDetailRequestToken += 1
  itemMovementsRequestToken += 1
  warehouseDetailLoading.value = false
  itemMovementsLoading.value = false
}

function isCurrentWarehouseRequest(requestToken: number, itemType: string, itemID: number, kind: 'detail' | 'movements'): boolean {
  const activeToken = kind === 'detail' ? warehouseDetailRequestToken : itemMovementsRequestToken
  return requestToken === activeToken
    && warehouseDrawerVisible.value
    && String(selectedWarehouseItem.value?.item_type || '') === itemType
    && Number(selectedWarehouseItem.value?.id) === itemID
}

async function openWarehouseItem(item: any) {
  selectedWarehouseItem.value = item
  warehouseDetail.value = null
  warehouseDetailError.value = ''
  warehouseDrawerVisible.value = true
  movementMode.value = ''
  showAllItemMovements.value = false
  itemMovements.value = []
  itemMovementsError.value = ''
  panelMessage.value = ''
  await Promise.allSettled([loadWarehouseItemDetail(), loadItemMovements()])
}

let warehouseCloseBypass = false

async function closeWarehouseItem() {
  if (!await requestWarehouseClose()) return
  performWarehouseClose()
}

function performWarehouseClose() {
  invalidateWarehouseRequests()
  warehouseCloseBypass = true
  warehouseDrawerVisible.value = false
  window.setTimeout(() => { warehouseCloseBypass = false }, 0)
}

async function requestWarehouseClose(): Promise<boolean> {
  if (movementSubmitting.value) {
    ElMessage.warning('正在提交库存变动，请等待办理完成后再关闭。')
    return false
  }
  if (!movementFormDirty.value) return true
  try {
    await appMessageBox.confirm('当前出入库表单尚未提交，关闭后已填写内容将丢失。', '放弃本次办理？', {
      confirmButtonText: '放弃并关闭',
      cancelButtonText: '继续填写',
      type: 'warning',
    })
    return true
  } catch {
    return false
  }
}

function handleWarehouseBeforeClose(done: () => void) {
  if (warehouseCloseBypass) {
    warehouseCloseBypass = false
    invalidateWarehouseRequests()
    done()
    return
  }
  void requestWarehouseClose().then((canClose) => {
    if (canClose) {
      invalidateWarehouseRequests()
      done()
    }
  })
}

function resetWarehouseItem() {
  // Closing must clear both visible data and transient form state; otherwise a
  // reopened drawer can show the previous item or submit stale movement fields.
  invalidateWarehouseRequests()
  warehouseCloseBypass = false
  selectedWarehouseItem.value = null
  warehouseDetail.value = null
  warehouseDetailLoading.value = false
  warehouseDetailError.value = ''
  itemMovements.value = []
  itemMovementsLoading.value = false
  itemMovementsError.value = ''
  movementMode.value = ''
  showQuickSupplier.value = false
  movementFormError.value = ''
  quickSupplierError.value = ''
  clearMovementForm()
}

function openMold(item: any) {
  selectedMoldID.value = Number(item.id)
  selectedMoldDetail.value = null
  moldDetailError.value = ''
  moldDetailDrawerVisible.value = true
  void loadMoldDetail()
}

async function loadMoldDetail() {
  const moldID = selectedMoldID.value
  if (!moldID) return
  moldDetailAbortController?.abort()
  const abortController = new AbortController()
  moldDetailAbortController = abortController
  const requestToken = ++moldDetailRequestToken
  moldDetailLoading.value = true
  moldDetailError.value = ''
  try {
    const data = await request<BasicItem>(`/api/v1/molds/${moldID}`, {signal: abortController.signal}, token.value)
    if (!isCurrentMoldDetailRequest(requestToken, moldID)) return
    selectedMoldDetail.value = data
  } catch (error) {
    if (!isCurrentMoldDetailRequest(requestToken, moldID)) return
    selectedMoldDetail.value = null
    moldDetailError.value = error instanceof Error ? error.message : '模具详情加载失败'
  } finally {
    if (moldDetailAbortController === abortController) moldDetailAbortController = null
    if (isCurrentMoldDetailRequest(requestToken, moldID)) moldDetailLoading.value = false
  }
}

function closeMold() {
  if (moldActionSubmitting.value) return
  invalidateMoldDetailRequest()
  moldDetailDrawerVisible.value = false
}

function handleMoldBeforeClose(done: () => void) {
  if (moldActionSubmitting.value) {
    ElMessage.warning('模具状态正在提交，请稍候。')
    return
  }
  invalidateMoldDetailRequest()
  done()
}

function resetMold() {
  invalidateMoldDetailRequest()
  selectedMoldDetail.value = null
  selectedMoldID.value = null
  moldDetailLoading.value = false
  moldDetailError.value = ''
  moldActionSubmitting.value = false
  moldActionError.value = ''
}

function invalidateMoldDetailRequest() {
  moldDetailAbortController?.abort()
  moldDetailAbortController = null
  moldDetailRequestToken += 1
  moldDetailLoading.value = false
}

function isCurrentMoldDetailRequest(requestToken: number, moldID: number): boolean {
  return requestToken === moldDetailRequestToken
    && moldDetailDrawerVisible.value
    && selectedMoldID.value === moldID
}

async function loanMold() {
  if (selectedMoldDetail.value?.status !== 'in_stock') return
  const location = await promptText('借出位置', '请输入模具借出后的具体位置')
  if (!location) return
  const counterparty = await promptText('借用方', '请输入借用部门或单位')
  if (!counterparty) return
  try {
    await appMessageBox.confirm(`确认将“${selectedMoldDetail.value.name}”借出至 ${location}？`, '确认借出', {
      type: 'warning', confirmButtonText: '确认借出', cancelButtonText: '取消',
    })
  } catch {
    return
  }
  await runMoldAction('loan', {
    location,
    counterparty,
    handler_name: currentUser.value?.name || currentUser.value?.username || '',
    reason: '模具借出',
  }, '模具已借出')
}

async function returnMold() {
  if (selectedMoldDetail.value?.status !== 'loaned') return
  const defaultLocation = String(selectedMoldDetail.value.storage_location || '').trim()
  const location = (await promptTextWithDefault('归还位置', '请确认或修改模具归还后的具体位置', defaultLocation)).trim()
  if (!location) return
  try {
    await appMessageBox.confirm(
      `确认将“${selectedMoldDetail.value.name}”归还至 ${location}？`,
      '确认归还',
      {type: 'warning', confirmButtonText: '确认归还', cancelButtonText: '取消'},
    )
  } catch {
    return
  }
  await runMoldAction('return', {
    location,
    handler_name: currentUser.value?.name || currentUser.value?.username || '',
    reason: '模具归还',
  }, '模具已归还入库')
}

async function repairMold(completed: boolean) {
  const expectedStatus = completed ? 'repairing' : 'in_stock'
  if (selectedMoldDetail.value?.status !== expectedStatus) return
  const reason = await promptText(completed ? '完成维修' : '开始维修', completed ? '请输入本次维修完成说明' : '请输入维修原因')
  if (!reason) return
  try {
    await appMessageBox.confirm(
      completed ? '确认维修已经完成并将模具恢复为在库状态？' : '确认将模具状态变更为维修中？',
      completed ? '确认完成维修' : '确认开始维修',
      {type: completed ? 'success' : 'warning', confirmButtonText: completed ? '完成维修' : '开始维修', cancelButtonText: '取消'},
    )
  } catch {
    return
  }
  await runMoldAction('repair', {
    reason,
    description: reason,
    handler_name: currentUser.value?.name || currentUser.value?.username || '',
    completed,
  }, completed ? '模具维修已完成' : '模具已进入维修状态')
}

async function maintainMold(completed: boolean) {
  const expectedStatus = completed ? 'maintenance' : 'in_stock'
  if (selectedMoldDetail.value?.status !== expectedStatus) return
  let maintenanceCycleDays = Number(selectedMoldDetail.value.maintenance_cycle_days || 0)
  if (completed && (!Number.isInteger(maintenanceCycleDays) || maintenanceCycleDays <= 0)) {
    const enteredCycle = await promptPositiveInteger('补充保养周期', '当前模具未设置有效保养周期，请填写完成保养后的周期天数')
    if (enteredCycle === null) return
    maintenanceCycleDays = enteredCycle
  }
  try {
    await appMessageBox.confirm(
      completed ? `确认保养已经完成？系统将按 ${maintenanceCycleDays} 天周期计算下次保养日期。` : '确认将模具状态变更为保养中？',
      completed ? '确认完成保养' : '确认开始保养',
      {type: completed ? 'success' : 'warning', confirmButtonText: completed ? '完成保养' : '开始保养', cancelButtonText: '取消'},
    )
  } catch {
    return
  }
  await runMoldAction('maintenance', {
    description: completed ? '模具保养完成' : '开始模具保养',
    handler_name: currentUser.value?.name || currentUser.value?.username || '',
    completed,
    ...(completed ? {maintenance_cycle_days: maintenanceCycleDays} : {}),
  }, completed ? '模具保养已完成' : '模具已进入保养状态')
}

async function runMoldAction(action: 'loan' | 'return' | 'repair' | 'maintenance', body: Record<string, unknown>, successMessage: string) {
  if (!selectedMoldDetail.value || !hasPermission('mold:write') || moldActionSubmitting.value) return
  moldActionSubmitting.value = true
  moldActionError.value = ''
  try {
    await request<BasicItem>(`/api/v1/molds/${selectedMoldDetail.value.id}/${action}`, {method: 'POST', body}, token.value)
    await Promise.all([loadActiveModule(), loadMoldDetail()])
    ElMessage.success(successMessage)
  } catch (error) {
    if (error instanceof ApiError && error.status === 409) {
      moldActionError.value = '模具状态已发生变化，已刷新最新详情和列表，请按当前状态重新操作。'
      ElMessage.warning(moldActionError.value)
      await Promise.all([loadActiveModule(), loadMoldDetail()])
      return
    }
    moldActionError.value = error instanceof Error ? error.message : '模具状态操作失败'
    ElMessage.error(moldActionError.value)
  } finally {
    moldActionSubmitting.value = false
  }
}

async function loadWarehouseItemDetail() {
  const item = selectedWarehouseItem.value
  if (!item) return
  const itemType = String(item.item_type || '')
  const itemID = Number(item.id)
  warehouseDetailAbortController?.abort()
  const abortController = new AbortController()
  warehouseDetailAbortController = abortController
  const requestToken = ++warehouseDetailRequestToken
  warehouseDetailLoading.value = true
  warehouseDetailError.value = ''
  warehouseDetail.value = null
  try {
    const data = await request<Record<string, unknown>>(
      `/api/v1/warehouse/items/${itemType}/${itemID}`,
      {signal: abortController.signal},
      token.value,
    )
    if (!isCurrentWarehouseRequest(requestToken, itemType, itemID, 'detail')) return
    warehouseDetail.value = data
  } catch (error) {
    if (!isCurrentWarehouseRequest(requestToken, itemType, itemID, 'detail')) return
    warehouseDetailError.value = error instanceof Error ? error.message : '库存详情加载失败'
  } finally {
    if (warehouseDetailAbortController === abortController) warehouseDetailAbortController = null
    if (isCurrentWarehouseRequest(requestToken, itemType, itemID, 'detail')) warehouseDetailLoading.value = false
  }
}

async function loadItemMovements() {
  const item = selectedWarehouseItem.value
  if (!item || !hasPermission('inventory:documents:read')) return
  const itemType = String(item.item_type || '')
  const itemID = Number(item.id)
  itemMovementsAbortController?.abort()
  const abortController = new AbortController()
  itemMovementsAbortController = abortController
  const requestToken = ++itemMovementsRequestToken
  itemMovementsLoading.value = true
  itemMovementsError.value = ''
  try {
    const data = await request<{items: BasicItem[]}>(
      `/api/v1/warehouse/items/${itemType}/${itemID}/movements?page_size=100`,
      {signal: abortController.signal},
      token.value,
    )
    if (!isCurrentWarehouseRequest(requestToken, itemType, itemID, 'movements')) return
    itemMovements.value = data.items
  } catch (error) {
    if (!isCurrentWarehouseRequest(requestToken, itemType, itemID, 'movements')) return
    itemMovements.value = []
    itemMovementsError.value = error instanceof Error ? error.message : '出入库记录加载失败'
  } finally {
    if (itemMovementsAbortController === abortController) itemMovementsAbortController = null
    if (isCurrentWarehouseRequest(requestToken, itemType, itemID, 'movements')) itemMovementsLoading.value = false
  }
}

async function loadAllItemMovements() {
  showAllItemMovements.value = true
  if (itemMovements.value.length < 100) {
    await loadItemMovements()
  }
}

function startMovement(mode: string) {
  if (movementSubmitting.value || !warehouseDetail.value || !warehouseQuantityAvailable.value || warehouseDetailLoading.value || warehouseDetailError.value) return
  movementMode.value = mode
  showQuickSupplier.value = false
  clearMovementForm()
  movementFormError.value = ''
  void operatorDirectory.load(true)
  if (mode === 'return_rework_inbound') {
    movementForm.source_type = hasPermission('customers:read') ? 'customer' : 'department'
  }
}

async function cancelMovement() {
  if (movementSubmitting.value) return
  if (movementFormDirty.value) {
    try {
      await appMessageBox.confirm('取消后本次填写内容不会保留。', '取消本次办理？', {
        confirmButtonText: '确认取消',
        cancelButtonText: '继续填写',
        type: 'warning',
      })
    } catch {
      return
    }
  }
  movementMode.value = ''
  showQuickSupplier.value = false
  movementFormError.value = ''
  quickSupplierError.value = ''
  clearMovementForm()
}

function resetMovementSource() {
  delete movementForm.customer_id
  delete movementForm.department_id
  delete movementForm.original_document_id
}

function clearMovementForm() {
  for (const key of Object.keys(movementForm)) delete movementForm[key]
  movementRequestSnapshot = ''
  movementRequestIdempotencyKey = ''
}

let movementRequestSnapshot = ''
let movementRequestIdempotencyKey = ''

async function submitMovement() {
  if (movementSubmitting.value) return
  const item = selectedWarehouseItem.value
  if (!item || !movementMode.value) return
  const quantity = decimalToScaled(movementForm.quantity)
  if (quantity <= 0) {
    movementFormError.value = '数量必须大于 0。'
    return
  }
  if (movementIsOutbound.value && expectedStockQuantity.value < 0) {
    movementFormError.value = `当前可用库存为 ${formatQuantity(warehouseDetail.value?.quantity)} ${item.unit}，本次出库数量不能超过可用库存。`
    return
  }
  if (!movementCanSubmit.value) {
    movementFormError.value = '请补全办理对象、数量和必填业务说明后再提交。'
    return
  }
  const body: Record<string, unknown> = {
    business_type: movementMode.value,
    quantity,
    operator_employee_id: Number(movementForm.operator_employee_id),
    reason: movementForm.reason || '',
    remark: movementForm.reason || '',
  }
  for (const key of ['supplier_id', 'customer_id', 'department_id', 'original_document_id']) {
    if (movementForm[key] !== '' && movementForm[key] !== undefined) body[key] = Number(movementForm[key])
  }
  if (movementMode.value === 'purchase_inbound' && movementForm.unit_cost) {
    body.unit_cost = moneyToCents(movementForm.unit_cost)
  }
  const requestSnapshot = JSON.stringify(body)
  if (!movementRequestIdempotencyKey || movementRequestSnapshot !== requestSnapshot) {
    movementRequestSnapshot = requestSnapshot
    movementRequestIdempotencyKey = createIdempotencyKey()
  }
  movementSubmitting.value = true
  movementFormError.value = ''
  try {
    await request(`/api/v1/warehouse/items/${item.item_type}/${item.id}/movements`, {
      method: 'POST',
      headers: {'Idempotency-Key': movementRequestIdempotencyKey},
      body,
    }, token.value)
    movementMode.value = ''
    showQuickSupplier.value = false
    clearMovementForm()
    operatorDirectory.invalidate()
    await Promise.all([loadActiveModule(), loadWarehouseItemDetail(), loadItemMovements()])
    const refreshed = rows.value.find((row) => row.id === item.id && row.item_type === item.item_type)
    if (refreshed) selectedWarehouseItem.value = refreshed
    panelMessage.value = '库存已更新'
    ElMessage.success('库存已更新')
  } catch (error) {
    if (operatorDirectory.handleSubmitError(error)) delete movementForm.operator_employee_id
    if (error instanceof ApiError && error.status < 500) {
      movementRequestSnapshot = ''
      movementRequestIdempotencyKey = ''
    }
    movementFormError.value = error instanceof Error ? error.message : '办理失败，请检查填写内容后重试。'
  } finally {
    movementSubmitting.value = false
  }
}

async function createQuickSupplier() {
  if (movementSubmitting.value || quickSupplierSubmitting.value) return
  if (!quickSupplier.name || !quickSupplier.code) {
    quickSupplierError.value = '请填写供应商名称和唯一编码。'
    return
  }
  quickSupplierSubmitting.value = true
  quickSupplierError.value = ''
  try {
    const created = await request<BasicItem>('/api/v1/suppliers', {method: 'POST', body: {...quickSupplier}}, token.value)
    await loadList('suppliers', false)
    movementForm.supplier_id = created.id
    Object.assign(quickSupplier, {name: '', code: '', contact: '', phone: ''})
    showQuickSupplier.value = false
    panelMessage.value = '供应商已新增'
    ElMessage.success('供应商已新增')
  } catch (error) {
    quickSupplierError.value = error instanceof Error ? error.message : '供应商新增失败，请检查编码是否重复。'
    ElMessage.error(quickSupplierError.value)
  } finally {
    quickSupplierSubmitting.value = false
  }
}

function decimalToScaled(value: unknown): number {
  const number = Number(value)
  return Number.isFinite(number) ? Math.round(number * 10000) : 0
}

function moneyToCents(value: unknown): number {
  const number = Number(value)
  return Number.isFinite(number) ? Math.round(number * 100) : 0
}

function formatQuantity(value: unknown): string {
  const number = Number(value || 0) / 10000
  return number.toLocaleString('zh-CN', {maximumFractionDigits: 4})
}

function formatMoney(value: unknown): string {
  return `¥${(Number(value || 0) / 100).toLocaleString('zh-CN', {minimumFractionDigits: 2, maximumFractionDigits: 2})}`
}

function formatDate(value: unknown): string {
  if (!value) return '-'
  return new Date(String(value)).toLocaleString('zh-CN', {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'})
}

function businessTypeLabel(value: unknown): string {
  return movementDefinitions.find((item) => item.key === value)?.title || (value === 'inbound' ? '入库' : '出库')
}

function movementQuantity(document: BasicItem): string {
  const lines = Array.isArray(document.lines) ? document.lines as Array<Record<string, unknown>> : []
  const quantity = formatQuantity(lines[0]?.quantity)
  return `${document.type === 'outbound' ? '−' : '+'}${quantity} ${selectedWarehouseItem.value?.unit || ''}`
}

function safeWorkorderProduct(item: BasicItem): BasicItem {
  const safe = {...item}
  delete safe.default_cost
  delete safe.avg_cost
  delete safe.amount
  return safe
}

function normalizeWorkorderProductDetail(data: BasicItem, fallback?: BasicItem): BasicItem {
  const nestedItem = data.item && typeof data.item === 'object' && !Array.isArray(data.item)
    ? data.item as BasicItem
    : data
  return safeWorkorderProduct({
    ...(fallback || {}),
    ...nestedItem,
    quantity: data.quantity ?? nestedItem.quantity ?? fallback?.quantity ?? 0,
    safety_stock: nestedItem.safety_stock ?? data.safety_stock ?? fallback?.safety_stock ?? 0,
    item_type: 'product',
  })
}

function invalidateWorkorderProductSearch() {
  workorderProductSearchAbortController?.abort()
  workorderProductSearchAbortController = null
  workorderProductSearchRequestToken += 1
  workorderProductSearchLoading.value = false
}

function isWorkorderProductionCreateFormActive(): boolean {
  return activeKey.value === 'workorder' && showCreateForm.value && formState.type === 'production'
}

async function searchWorkorderProducts(keyword = '') {
  if (!isWorkorderProductionCreateFormActive()) return
  if (!hasPermission('warehouse:read')) {
    workorderProductOptions.value = []
    workorderProductSearchError.value = ''
    return
  }
  workorderProductSearchAbortController?.abort()
  const abortController = new AbortController()
  workorderProductSearchAbortController = abortController
  const requestToken = ++workorderProductSearchRequestToken
  workorderProductSearchLoading.value = true
  workorderProductSearchError.value = ''
  try {
    const path = appendQuery('/api/v1/warehouse/items', {tab: 'product', q: keyword.trim(), page: 1, page_size: 50})
    const data = await request<PaginatedResponse<BasicItem> | BasicItem[]>(path, {signal: abortController.signal}, token.value)
    if (requestToken !== workorderProductSearchRequestToken || !isWorkorderProductionCreateFormActive()) return
    const items = Array.isArray(data) ? data : data.items
    workorderProductOptions.value = items.map(safeWorkorderProduct)
  } catch (error) {
    if (requestToken !== workorderProductSearchRequestToken || !isWorkorderProductionCreateFormActive()) return
    workorderProductSearchError.value = error instanceof Error ? error.message : '仓库产品搜索失败，请重试。'
  } finally {
    if (workorderProductSearchAbortController === abortController) workorderProductSearchAbortController = null
    if (requestToken === workorderProductSearchRequestToken) workorderProductSearchLoading.value = false
  }
}

function handleWorkorderProductSelect(productID: unknown) {
  const id = Number(productID || 0)
  if (!id) {
    resetWorkorderProductSelection()
    return
  }
  const option = workorderProductOptions.value.find((item) => Number(item.id) === id)
  workorderProductStock.value = option ? safeWorkorderProduct(option) : null
  workorderProductStockError.value = ''
  workorderProductStockUpdatedAt.value = ''
  void loadWorkorderProductStock()
}

function resetWorkorderProductSelection() {
  workorderProductStockAbortController?.abort()
  workorderProductStockAbortController = null
  workorderProductStockRequestToken += 1
  delete formState.product_id
  workorderProductStock.value = null
  workorderProductStockLoading.value = false
  workorderProductStockError.value = ''
  workorderProductStockUpdatedAt.value = ''
}

async function loadWorkorderProductStock() {
  const productID = Number(formState.product_id || 0)
  if (!productID || !isWorkorderProductionCreateFormActive()) return
  if (!hasPermission('warehouse:read')) {
    workorderProductStockLoading.value = false
    workorderProductStockError.value = ''
    return
  }
  workorderProductStockAbortController?.abort()
  const abortController = new AbortController()
  workorderProductStockAbortController = abortController
  const requestToken = ++workorderProductStockRequestToken
  workorderProductStockLoading.value = true
  workorderProductStockError.value = ''
  const fallback = workorderProductOptions.value.find((item) => Number(item.id) === productID) || workorderProductStock.value || undefined
  try {
    const data = await request<BasicItem>(`/api/v1/warehouse/items/product/${productID}`, {signal: abortController.signal}, token.value)
    if (requestToken !== workorderProductStockRequestToken || !isWorkorderProductionCreateFormActive() || Number(formState.product_id) !== productID) return
    workorderProductStock.value = normalizeWorkorderProductDetail(data, fallback)
    workorderProductStockUpdatedAt.value = new Date().toISOString()
  } catch (error) {
    if (requestToken !== workorderProductStockRequestToken || !isWorkorderProductionCreateFormActive() || Number(formState.product_id) !== productID) return
    workorderProductStockError.value = error instanceof Error ? error.message : '库存数量加载失败，请重试。'
  } finally {
    if (workorderProductStockAbortController === abortController) workorderProductStockAbortController = null
    if (requestToken === workorderProductStockRequestToken && Number(formState.product_id) === productID) workorderProductStockLoading.value = false
  }
}

function openTemporaryProductDialog() {
  if (!hasPermission('warehouse:read') || !hasPermission('workorder:write') || !hasPermission('workorder:temporary-product:write')) return
  temporaryProductForm.name = ''
  temporaryProductForm.code = ''
  temporaryProductForm.unit = '个'
  temporaryProductForm.spec = ''
  temporaryProductForm.operator_employee_id = undefined
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

async function createTemporaryProduct() {
  if (!hasPermission('warehouse:read') || !hasPermission('workorder:write') || !hasPermission('workorder:temporary-product:write')) {
    temporaryProductError.value = '当前账号没有临时新增产品的权限。'
    return
  }
  const name = temporaryProductForm.name.trim()
  const code = temporaryProductForm.code.trim()
  const unit = temporaryProductForm.unit.trim()
  if (!name || !code || !unit) {
    temporaryProductError.value = '请填写产品名称、产品编码和单位。'
    return
  }
  if (!temporaryProductForm.operator_employee_id || operatorDirectory.unavailableReason.value) {
    temporaryProductError.value = operatorDirectory.unavailableReason.value || '请选择本次操作人。'
    return
  }
  invalidateWorkorderProductSearch()
  temporaryProductSubmitting.value = true
  temporaryProductError.value = ''
  try {
    const created = await request<BasicItem>('/api/v1/workorder/products', {
      method: 'POST',
      body: {name, code, unit, spec: temporaryProductForm.spec.trim(), operator_employee_id: Number(temporaryProductForm.operator_employee_id)},
    }, token.value)
    const product = normalizeWorkorderProductDetail(created)
    if (!isWorkorderProductionCreateFormActive()) return
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
  } finally {
    temporaryProductSubmitting.value = false
  }
}

function invalidateWorkorderDrawerProductStock() {
  workorderDrawerProductStockAbortController?.abort()
  workorderDrawerProductStockAbortController = null
  workorderDrawerProductStockRequestToken += 1
  workorderDrawerProductStockLoading.value = false
}

async function loadWorkorderDrawerProductStock() {
  const workorderID = Number(selectedWorkOrder.value?.id || 0)
  const productID = Number(selectedWorkOrder.value?.product_id || 0)
  if (!workorderID || !productID) return
  if (!hasPermission('warehouse:read')) {
    workorderDrawerProductStockLoading.value = false
    workorderDrawerProductStockError.value = ''
    return
  }
  workorderDrawerProductStockAbortController?.abort()
  const abortController = new AbortController()
  workorderDrawerProductStockAbortController = abortController
  const requestToken = ++workorderDrawerProductStockRequestToken
  workorderDrawerProductStockLoading.value = true
  workorderDrawerProductStockError.value = ''
  const fallback = workorderDrawerProductStock.value || {
    id: productID,
    name: String(selectedWorkOrder.value?.product_name || ''),
    unit: String(selectedWorkOrder.value?.unit || ''),
  }
  try {
    const data = await request<BasicItem>(`/api/v1/warehouse/items/product/${productID}`, {signal: abortController.signal}, token.value)
    if (requestToken !== workorderDrawerProductStockRequestToken
      || !workorderDrawerVisible.value
      || Number(selectedWorkOrder.value?.id) !== workorderID
      || Number(selectedWorkOrder.value?.product_id) !== productID) return
    workorderDrawerProductStock.value = normalizeWorkorderProductDetail(data, fallback)
    workorderDrawerProductStockUpdatedAt.value = new Date().toISOString()
  } catch (error) {
    if (requestToken !== workorderDrawerProductStockRequestToken
      || !workorderDrawerVisible.value
      || Number(selectedWorkOrder.value?.id) !== workorderID
      || Number(selectedWorkOrder.value?.product_id) !== productID) return
    workorderDrawerProductStockError.value = error instanceof Error ? error.message : '库存数量加载失败，请重试。'
  } finally {
    if (workorderDrawerProductStockAbortController === abortController) workorderDrawerProductStockAbortController = null
    if (requestToken === workorderDrawerProductStockRequestToken && Number(selectedWorkOrder.value?.id) === workorderID) {
      workorderDrawerProductStockLoading.value = false
    }
  }
}

function openWorkOrder(item: any) {
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

function invalidateWorkOrderLogsRequest() {
  workorderLogsAbortController?.abort()
  workorderLogsAbortController = null
  workorderLogsRequestToken += 1
  workorderLogsLoading.value = false
}

function isCurrentWorkOrderLogsRequest(requestToken: number, workorderID: number): boolean {
  return requestToken === workorderLogsRequestToken
    && workorderDrawerVisible.value
    && Number(selectedWorkOrder.value?.id) === workorderID
}

function closeWorkOrder() {
  invalidateWorkOrderLogsRequest()
  invalidateWorkorderDrawerProductStock()
  workorderDrawerVisible.value = false
}

function handleWorkOrderBeforeClose(done: () => void) {
  invalidateWorkOrderLogsRequest()
  invalidateWorkorderDrawerProductStock()
  done()
}

function resetWorkOrder() {
  invalidateWorkOrderLogsRequest()
  invalidateWorkorderDrawerProductStock()
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
  workorderLogsAbortController?.abort()
  const abortController = new AbortController()
  workorderLogsAbortController = abortController
  const requestToken = ++workorderLogsRequestToken
  workorderLogsLoading.value = true
  workorderLogsError.value = ''
  try {
    const data = await request<BasicItem[]>(`/api/v1/workorder/${workorderID}/logs`, {signal: abortController.signal}, token.value)
    if (!isCurrentWorkOrderLogsRequest(requestToken, workorderID)) return
    workorderLogs.value = data
  } catch (error) {
    if (!isCurrentWorkOrderLogsRequest(requestToken, workorderID)) return
    workorderLogs.value = []
    workorderLogsError.value = error instanceof Error ? error.message : '任务日志加载失败'
  } finally {
    if (workorderLogsAbortController === abortController) workorderLogsAbortController = null
    if (isCurrentWorkOrderLogsRequest(requestToken, workorderID)) workorderLogsLoading.value = false
  }
}

async function dispatchWorkOrder() {
  openWorkOrderAction('dispatch')
}

async function pauseWorkOrder() {
  openWorkOrderAction('pause')
}

async function resumeWorkOrder() {
  openWorkOrderAction('resume')
}

async function toggleWorkOrderUrgent() {
  openWorkOrderAction('urgent')
}

async function completeWorkOrder(mode: 'normal' | 'forced') {
  openWorkOrderAction(mode === 'normal' ? 'complete_normal' : 'complete_forced')
}

async function startDepartmentTask(task: BasicItem) {
  openWorkOrderAction('department_start', task)
}

async function partialCompleteDepartmentTask(task: BasicItem) {
  openWorkOrderAction('department_partial_complete', task)
}

async function completeDepartmentTask(task: BasicItem) {
  openWorkOrderAction('department_complete', task)
}

function openWorkOrderAction(kind: string, target: BasicItem | null = null) {
  actionKind.value = kind
  actionTarget.value = target
  actionForm.operator_employee_id = undefined
  actionForm.reason = ''
  actionForm.remark = ''
  actionForm.completed_quantity = ''
  actionError.value = ''
  Object.assign(actionFieldErrors, {operator: '', reason: '', quantity: ''})
  actionDialogVisible.value = true
  void operatorDirectory.load(true)
}

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

async function submitWorkOrderAction() {
  if (actionSubmitting.value || !selectedWorkOrder.value) return
  if (!actionForm.operator_employee_id || operatorDirectory.unavailableReason.value) {
    if (operatorDirectory.unavailableReason.value) actionError.value = operatorDirectory.unavailableReason.value
    else actionFieldErrors.operator = '请选择本次操作人。'
    await nextTick()
    document.getElementById('workorder-action-operator')?.focus()
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
    await nextTick()
    document.getElementById('workorder-action-reason')?.focus()
    return
  }
  if (actionKind.value === 'department_partial_complete') {
    const quantityInput = actionForm.completed_quantity.trim()
    if (!quantityInput) {
      actionFieldErrors.quantity = '请输入累计完成数量。'
      await nextTick()
      document.getElementById('workorder-action-quantity')?.focus()
      return
    }
    if (!/^\d+(\.\d{1,4})?$/.test(quantityInput)) {
      actionFieldErrors.quantity = '请输入正数，最多保留 4 位小数。'
      await nextTick()
      document.getElementById('workorder-action-quantity')?.focus()
      return
    }
    const quantity = decimalToScaled(quantityInput)
    const planned = Number(actionTarget.value?.planned_quantity || 0)
    const completed = Number(actionTarget.value?.completed_quantity || 0)
    if (quantity <= 0 || quantity >= planned) {
      actionFieldErrors.quantity = '累计完成数量必须大于 0 且小于计划数量。'
      await nextTick()
      document.getElementById('workorder-action-quantity')?.focus()
      return
    }
    if (quantity <= completed) {
      actionFieldErrors.quantity = '累计完成数量必须大于当前已完成数量。'
      await nextTick()
      document.getElementById('workorder-action-quantity')?.focus()
      return
    }
    body.completed_quantity = quantity
  }
  actionSubmitting.value = true
  actionError.value = ''
  try {
    selectedWorkOrder.value = await request<BasicItem>(path, {method: 'POST', body}, token.value)
    await Promise.all([loadActiveModule(), loadWorkOrderLogs()])
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
  } finally {
    actionSubmitting.value = false
  }
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

async function runWorkOrderAction(path: string, body: Record<string, unknown>, successMessage: string) {
  if (!selectedWorkOrder.value) return
  loading.value = true
  panelMessage.value = ''
  try {
    selectedWorkOrder.value = await request<BasicItem>(path, {method: 'POST', body}, token.value)
    await Promise.all([loadActiveModule(), loadWorkOrderLogs()])
    panelMessage.value = successMessage
    ElMessage.success(successMessage)
  } catch (error) {
    panelMessage.value = error instanceof Error ? error.message : '任务操作失败'
    ElMessage.error(panelMessage.value)
  } finally {
    loading.value = false
  }
}

async function promptText(title: string, message: string, required = true): Promise<string> {
  try {
    const result = await appMessageBox.prompt(message, title, {
      inputType: 'textarea',
      inputValidator: (value) => !required || !!String(value || '').trim(),
      inputErrorMessage: '请填写内容',
    })
    return String(result.value || '').trim()
  } catch {
    return ''
  }
}

async function promptTextWithDefault(title: string, message: string, inputValue: string): Promise<string> {
  try {
    const result = await appMessageBox.prompt(message, title, {
      inputValue,
      inputValidator: (value) => Boolean(String(value || '').trim()),
      inputErrorMessage: '请填写具体位置',
      confirmButtonText: '继续',
      cancelButtonText: '取消',
    })
    return String(result.value || '').trim()
  } catch {
    return ''
  }
}

async function promptPositiveInteger(title: string, message: string): Promise<number | null> {
  try {
    const result = await appMessageBox.prompt(message, title, {
      inputType: 'number',
      inputValidator: (value) => {
        const number = Number(String(value || '').trim())
        return Number.isInteger(number) && number > 0
      },
      inputErrorMessage: '请输入大于 0 的整数天数',
      confirmButtonText: '继续',
      cancelButtonText: '取消',
    })
    const number = Number(String(result.value || '').trim())
    return Number.isInteger(number) && number > 0 ? number : null
  } catch {
    return null
  }
}

function departmentTasks(row: any): BasicItem[] {
  return Array.isArray(row.department_tasks) ? row.department_tasks as BasicItem[] : []
}

function departmentProgressMetrics(row: unknown): {percentage: number; completed: number; total: number} {
  const tasks = departmentTasks(row)
  if (!tasks.length) return {percentage: 0, completed: 0, total: 0}
  const completed = tasks.filter((task) => task.status === 'completed').length
  const totalProgress = tasks.reduce((sum, task) => {
    const explicit = Number(task.progress)
    if (Number.isFinite(explicit)) return sum + Math.min(100, Math.max(0, explicit))
    const planned = Number(task.planned_quantity || 0)
    const finished = Number(task.completed_quantity || 0)
    return sum + (planned > 0 ? Math.min(100, Math.max(0, finished * 100 / planned)) : 0)
  }, 0)
  const percentage = Math.round(totalProgress / tasks.length)
  return {percentage, completed, total: tasks.length}
}

function departmentProgressSummary(row: unknown): string {
  const progress = departmentProgressMetrics(row)
  if (!progress.total) return '0% · 尚未分配部门'
  return `${progress.percentage}% · ${progress.completed}/${progress.total} 个部门已完成`
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

function workorderStatusLabel(value: unknown): string {
  const labels: Record<string, string> = {
    draft: '草稿',
    processing: '正在处理',
    paused: '暂停',
    pending_close: '待办公室确认',
    completed_normal: '正常完成',
    completed_forced: '强制完成',
    cancelled: '取消',
  }
  return labels[String(value)] || String(value || '-')
}

function workorderTypeLabel(value: unknown): string {
  return value === 'general' ? '通用任务' : '生产单'
}

function inventoryItemTypeLabel(value: unknown): string {
  return value === 'product' ? '产品' : '物料'
}

function moldStatusLabel(value: unknown): string {
  const labels: Record<string, string> = {
    in_stock: '在库',
    loaned: '已借出',
    repairing: '维修中',
    maintenance: '保养中',
    scrapped: '报废',
  }
  return labels[String(value)] || String(value || '-')
}

function moldStatusTone(value: unknown): StatusTone {
  if (value === 'in_stock') return 'success'
  if (value === 'repairing' || value === 'maintenance') return 'warning'
  if (value === 'scrapped') return 'danger'
  return 'info'
}

function moldMaintenanceState(row: unknown): {label: string; tone: StatusTone; description: string} {
  const item = row as Record<string, unknown>
  if (!item.next_maintenance_at) return {label: '未设置计划', tone: 'info', description: '建议补充下次保养日期'}
  const rawDate = String(item.next_maintenance_at)
  const dateOnlyMatch = /^(\d{4})-(\d{2})-(\d{2})$/.exec(rawDate)
  const maintenanceDate = dateOnlyMatch
    ? new Date(Number(dateOnlyMatch[1]), Number(dateOnlyMatch[2]) - 1, Number(dateOnlyMatch[3]))
    : new Date(rawDate)
  if (!Number.isFinite(maintenanceDate.getTime())) return {label: '日期异常', tone: 'warning', description: '请核对保养日期'}
  const now = new Date()
  const localDaySerial = (date: Date) => Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()) / 86_400_000
  const dayDifference = localDaySerial(maintenanceDate) - localDaySerial(now)
  if (dayDifference < 0) {
    return {label: `逾期 ${Math.abs(dayDifference)} 天`, tone: 'danger', description: '请尽快安排保养'}
  }
  if (dayDifference <= 7) return {label: dayDifference === 0 ? '今天到期' : `${dayDifference} 天内到期`, tone: 'warning', description: '请提前安排保养'}
  return {label: '保养计划正常', tone: 'success', description: `距下次保养 ${dayDifference} 天`}
}

function departmentCompletionRate(item: DepartmentStatistic): number {
  if (!item.total) return 0
  return Math.round((Number(item.completed || 0) * 100) / Number(item.total))
}

function trendNameLabel(value: unknown): string {
  const labels: Record<string, string> = {
    inbound: '入库',
    outbound: '出库',
    transfer: '调拨',
    draft: '草稿',
    processing: '处理中',
    pending_close: '待确认',
    completed_normal: '正常完成',
    completed_forced: '强制完成',
  }
  return labels[String(value)] || String(value || '趋势')
}

function trendBarPercentage(item: StatisticTrendItem): number {
  const usesQuantity = item.quantity !== undefined
  const peers = compactTrendItems.value.filter((candidate) => (candidate.quantity !== undefined) === usesQuantity)
  const values = peers.map((candidate) => Math.abs(Number(usesQuantity ? candidate.quantity : candidate.value) || 0))
  const maximum = Math.max(...values, 0)
  if (!maximum) return 0
  return Math.max(4, Math.round(Math.abs(Number(usesQuantity ? item.quantity : item.value) || 0) * 100 / maximum))
}

function departmentTaskStatusLabel(value: unknown): string {
  const labels: Record<string, string> = {
    draft: '待派发',
    received: '已收到',
    processing: '正在处理',
    partial_completed: '部分完成',
    completed: '完成',
  }
  return labels[String(value)] || String(value || '-')
}

function workorderStatusTone(value: unknown): StatusTone {
  if (value === 'completed_normal') return 'success'
  if (value === 'completed_forced' || value === 'cancelled') return 'danger'
  if (value === 'pending_close') return 'warning'
  return 'info'
}

function workorderDueState(row: unknown): {overdue: boolean; label: string} {
  const item = row as Record<string, unknown>
  const status = String(item.status || '')
  if (status.startsWith('completed') || status === 'cancelled' || !item.due_at) {
    return {overdue: false, label: ''}
  }
  const dueDay = new Date(String(item.due_at))
  if (!Number.isFinite(dueDay.getTime())) return {overdue: false, label: ''}
  dueDay.setHours(0, 0, 0, 0)
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  if (dueDay.getTime() >= today.getTime()) return {overdue: false, label: ''}
  const days = Math.max(1, Math.round((today.getTime() - dueDay.getTime()) / 86_400_000))
  return {overdue: true, label: `逾期 ${days} 天`}
}

function departmentTaskStatusTone(value: unknown): StatusTone {
  if (value === 'completed') return 'success'
  if (value === 'partial_completed') return 'warning'
  return 'info'
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

function workorderActionLabel(value: unknown): string {
  const labels: Record<string, string> = {
    create: '创建',
    dispatch: '派发',
    dispatch_department: '部门收到',
    department_start: '部门处理',
    department_partial_complete: '部分完成',
    department_complete: '部门完成',
    pending_close: '待确认',
    pause: '暂停',
    resume: '恢复',
    urgent: '加急',
    complete_normal: '正常完成',
    complete_forced: '强制完成',
  }
  return labels[String(value)] || String(value || '-')
}

configureAuthSession({
  getToken: () => token.value,
  refresh: refreshSession,
  onFailure: handleAuthFailure,
})

onMounted(() => {
  window.addEventListener('focus', refreshOnSessionActivity)
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
    void loadClientUpdate()
  }
})

onBeforeUnmount(() => {
  clearSessionRefreshTimer()
  invalidateWorkorderProductSearch()
  resetWorkorderProductSelection()
  invalidateWorkorderDrawerProductStock()
  window.removeEventListener('focus', refreshOnSessionActivity)
  document.removeEventListener('visibilitychange', refreshOnSessionActivity)
  configureAuthSession(null)
})


  return {
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
    temporaryProductForm,
    activeWarehouseTab,
    workorderStatusFilter,
    workorderTypeFilter,
    workorderPriorityFilter,
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
    actionDialogVisible,
    actionKind,
    actionTarget,
    actionSubmitting,
    actionError,
    actionFieldErrors,
    actionForm,
    moldDetailDrawerVisible,
    selectedMoldDetail,
    selectedMoldID,
    moldDetailLoading,
    moldDetailError,
    moldActionSubmitting,
    moldActionError,
    statisticsData,
    affiliationTarget,
    affiliationDepartmentID,
    affiliationTerminalID,
    affiliationTerminalOptions,
    affiliationSaving,
    affiliationError,
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
    registerModuleLeaveGuard,
    selectMobileModule,
    restoreMobileMenuFocus,
    handleUserCommand,
    resetFilters,
    openAssignment,
    assignmentOptionsRequestToken,
    closeAssignment,
    loadAssignmentOptions,
    retryAssignmentOptions,
    isAssignmentOptionDisabled,
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
    closeWarehouseItem,
    requestWarehouseClose,
    warehouseCloseBypass,
    performWarehouseClose,
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
    openWorkOrderAction,
    closeWorkOrderAction,
    submitWorkOrderAction,
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
  }
}
