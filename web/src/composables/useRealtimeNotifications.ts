import {computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref} from 'vue'
import {ElMessage} from 'element-plus'
import {streamRequest} from '../api/http'
import {modules} from '../data/modules'
import type {BasicItem, CurrentUser} from '../types'
import {
  aggregateNotificationEvents,
  normalizeModuleKey,
  normalizeNotification,
  parseNotificationSseBlock,
  parseNotificationSseText,
  notificationModuleTitle,
  notificationOpenEvent,
  notificationRefreshEvent,
  notificationStreamPath,
  type DataChangeNotification,
  type NotificationModuleSummary,
  type NotificationOpenEventDetail,
  type NotificationRefreshEventDetail,
  type RealtimeNotification,
} from '../platform/notifications'

type RefLike<T> = Pick<Ref<T>, 'value'>

interface WorkorderDetailHost {
  selectedWorkOrder?: RefLike<BasicItem | null>
  workorderDrawerVisible?: RefLike<boolean>
  actionDialogVisible?: RefLike<boolean>
  loadWorkOrderLogs?: () => Promise<unknown>
  loadWorkorderDrawerProductStock?: () => Promise<unknown>
}

interface WorkorderListHost {
  openWorkOrder?: (value: unknown) => Promise<unknown>
  loadWorkOrderByID?: (entityID: unknown) => Promise<BasicItem | null>
}

/** Minimum shape of the controller needed by the shared notification layer. */
export interface RealtimeWorkspaceHost {
  token: RefLike<string>
  currentUser: RefLike<CurrentUser | null>
  activeKey: RefLike<string>
  panelMessage?: RefLike<string>
  showCreateForm?: RefLike<boolean>
  editingSupplier?: RefLike<BasicItem | null>
  movementFormDirty?: RefLike<boolean>
  warehouseDrawerVisible?: RefLike<boolean>
  selectedWarehouseItem?: RefLike<BasicItem | null>
  showQuickSupplier?: RefLike<boolean>
  rows?: RefLike<BasicItem[]>
  pageDetailPanelVisible?: RefLike<boolean>
  customerEditing?: RefLike<boolean>
  customerDirty?: RefLike<boolean>
  assignmentTarget?: RefLike<BasicItem | null>
  affiliationTarget?: RefLike<BasicItem | null>
  loadActiveModule: () => Promise<unknown>
  switchModule: (key: string) => Promise<unknown>
  openWarehouseItem?: (value: unknown) => Promise<unknown>
  loadWarehouseItemByID?: (entityType: unknown, entityID: unknown) => Promise<BasicItem | null>
  loadWarehouseItemDetail?: () => Promise<unknown>
  loadItemMovements?: () => Promise<unknown>
  loadPermissionCaches?: () => Promise<unknown>
  workorderContext?: {
    list?: WorkorderListHost
    detail?: WorkorderDetailHost
  }
}

export type NotificationConnectionState = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'stopped'

export interface NotificationCenterState {
  notifications: Ref<RealtimeNotification[]>
  visibleToast: Ref<RealtimeNotification | null>
  panelOpen: Ref<boolean>
  unreadCount: Readonly<Ref<number>>
  connectionState: Readonly<Ref<NotificationConnectionState>>
  togglePanel: () => void
  closePanel: () => void
  markRead: (id: string) => void
  markAllRead: () => void
  dismiss: (id: string) => void
  clearAll: () => void
  openNotification: (notification: RealtimeNotification) => Promise<void>
  pauseToast: () => void
  resumeToast: () => void
}

const maxSessionNotifications = 20
const batchWindowMs = 2000
const defaultToastMs = 8000
const maxReconnectMs = 30000

/** Small pure parser used by the stream consumer and node tests. */
export function parseSseText(text: string): unknown[] {
  return parseNotificationSseText(text)
}

/** Consume an SSE response without buffering the lifetime of the stream. */
export async function consumeSseResponse(response: Response, onData: (value: unknown) => void): Promise<void> {
  if (!response.body) {
    for (const value of parseSseText(await response.text())) onData(value)
    return
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let dataLines: string[] = []
  const flush = () => {
    if (!dataLines.length) return
    const value = parseNotificationSseBlock(`data: ${dataLines.join('\n')}`)
    if (value !== null) onData(value)
    dataLines = []
  }
  const processLine = (line: string) => {
    if (!line) { flush(); return }
    if (line.startsWith(':')) return
    if (line.startsWith('data:')) dataLines.push(line.slice(5).replace(/^ /, ''))
  }
  try {
    while (true) {
      const next = await reader.read()
      if (next.done) break
      buffer += decoder.decode(next.value, {stream: true})
      let lineBreak = buffer.indexOf('\n')
      while (lineBreak >= 0) {
        processLine(buffer.slice(0, lineBreak).replace(/\r$/, ''))
        buffer = buffer.slice(lineBreak + 1)
        lineBreak = buffer.indexOf('\n')
      }
    }
    buffer += decoder.decode()
    if (buffer) processLine(buffer.replace(/\r$/, ''))
    flush()
  } finally {
    reader.releaseLock()
  }
}

function waitFor(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}

function sameEntity(left: unknown, right: unknown): boolean {
  return left !== undefined && left !== null && right !== undefined && right !== null && String(left) === String(right)
}

function hasActionDraft(host: RealtimeWorkspaceHost, module: string): boolean {
  if (host.activeKey.value !== module) return false
  if (host.showCreateForm?.value) return true
  if (module === 'customers' && (host.customerEditing?.value || host.customerDirty?.value)) return true
  if (module === 'warehouses' && (host.movementFormDirty?.value || host.showQuickSupplier?.value)) return true
  if (module === 'workorder' && host.workorderContext?.detail?.actionDialogVisible?.value) return true
  // Assignment and account-affiliation surfaces can contain values which are
  // not represented in the generic form state. Preserve them conservatively.
  if (host.assignmentTarget?.value || host.affiliationTarget?.value) return true
  return false
}

function canSilentlyRefresh(host: RealtimeWorkspaceHost, module: string): boolean {
  if (hasActionDraft(host, module)) return false
  return true
}

function isKnownModule(module: string): boolean {
  return modules.some((item) => item.key === module)
}

function dispatchRefresh(notification: RealtimeNotification, module: NotificationModuleSummary, deferred: boolean): NotificationRefreshEventDetail | null {
  if (typeof window === 'undefined') return null
  const detail: NotificationRefreshEventDetail = {notification, module, deferred}
  window.dispatchEvent(new CustomEvent(notificationRefreshEvent, {detail}))
  return detail
}

function dispatchOpen(notification: RealtimeNotification, module: NotificationModuleSummary): void {
  if (typeof window === 'undefined') return
  const detail: NotificationOpenEventDetail = {notification, module}
  window.dispatchEvent(new CustomEvent(notificationOpenEvent, {detail}))
}

export function useRealtimeNotifications(host: RealtimeWorkspaceHost): NotificationCenterState {
  const notifications = ref<RealtimeNotification[]>([])
  const visibleToast = ref<RealtimeNotification | null>(null)
  const panelOpen = ref(false)
  const unreadCount = computed(() => notifications.value.reduce((count, item) => count + (item.read ? 0 : 1), 0))
  const connectionState = ref<NotificationConnectionState>('idle')
  const pendingEvents: DataChangeNotification[] = []
  const seenEventIDs = new Set<string>()
  let batchTimer: number | undefined
  let toastTimer: number | undefined
  let toastPaused = false
  let toastRemaining = defaultToastMs
  let toastStartedAt = 0
  let panelMessageTimer: number | undefined
  let streamAbort: AbortController | null = null
  let connectionGeneration = 0
  let disposed = false

  function clearToastTimer() {
    if (toastTimer !== undefined) window.clearTimeout(toastTimer)
    toastTimer = undefined
  }

  function hideToast() {
    clearToastTimer()
    visibleToast.value = null
    toastPaused = false
    toastRemaining = defaultToastMs
  }

  function scheduleToast() {
    clearToastTimer()
    if (!visibleToast.value || toastPaused) return
    toastStartedAt = Date.now()
    toastTimer = window.setTimeout(() => {
      toastTimer = undefined
      visibleToast.value = null
      toastRemaining = defaultToastMs
    }, Math.max(1000, toastRemaining))
  }

  function showToast(notification: RealtimeNotification) {
    visibleToast.value = notification
    toastRemaining = notification.displayForMs || defaultToastMs
    scheduleToast()
  }

  function pushNotification(notification: RealtimeNotification) {
    notifications.value = [notification, ...notifications.value.filter((item) => item.id !== notification.id)].slice(0, maxSessionNotifications)
    showToast(notification)
  }

  function markRead(id: string) {
    notifications.value = notifications.value.map((item) => item.id === id ? {...item, read: true} : item)
  }

  function markAllRead() {
    notifications.value = notifications.value.map((item) => item.read ? item : {...item, read: true})
  }

  function dismiss(id: string) {
    notifications.value = notifications.value.filter((item) => item.id !== id)
    if (visibleToast.value?.id === id) hideToast()
  }

  function clearAll() {
    notifications.value = []
    hideToast()
  }

  function togglePanel() {
    panelOpen.value = !panelOpen.value
    if (panelOpen.value) markAllRead()
  }

  function closePanel() { panelOpen.value = false }

  function setPanelMessage(message: string) {
    if (!host.panelMessage) return
    host.panelMessage.value = message
    if (panelMessageTimer !== undefined) window.clearTimeout(panelMessageTimer)
    panelMessageTimer = window.setTimeout(() => {
      if (host.panelMessage?.value === message) host.panelMessage.value = ''
      panelMessageTimer = undefined
    }, 1800)
  }

  async function refreshModule(module: NotificationModuleSummary, notification: RealtimeNotification): Promise<boolean> {
    const refreshDetail = dispatchRefresh(notification, module, false)
    // Customer and mold own their list/detail request lifecycle. Their page
    // listeners consume the event above so a shared controller request cannot
    // clear a local cache or switch a nested editor to the wrong shape.
    if (module.module === 'customers' || module.module === 'molds') {
      if (!refreshDetail?.handled) await host.loadActiveModule()
      if (refreshDetail?.completion && !(await refreshDetail.completion)) return false
      setPanelMessage('已同步外部更新')
      return !refreshDetail?.deferred
    }
    if (module.module === 'products' || module.module === 'warehouses') {
      window.dispatchEvent(new CustomEvent(module.module === 'products' ? 'bb-product-refresh' : 'bb-warehouse-refresh'))
      setPanelMessage('已同步外部更新')
      return true
    }
    if (!refreshDetail?.handled) await host.loadActiveModule()
    if (module.module === 'warehouses') {
      const selected = host.selectedWarehouseItem?.value
      const selectedItem = module.items.find((item) => sameEntity(item.entity_id, selected?.id))
      if (selected && (!module.items.length || selectedItem || module.refresh.invalidate_all)) {
        await Promise.allSettled([
          host.loadWarehouseItemDetail?.() || Promise.resolve(),
          host.loadItemMovements?.() || Promise.resolve(),
        ])
      }
    }
    if (module.module === 'workorder') {
      const detail = host.workorderContext?.detail
      const selected = detail?.selectedWorkOrder?.value
      const selectedItem = module.items.find((item) => sameEntity(item.entity_id, selected?.id))
      if (selected && (!module.items.length || selectedItem || module.refresh.invalidate_all)) {
        if (selectedItem) detail?.selectedWorkOrder && (detail.selectedWorkOrder.value = {...selected, ...selectedItem})
        await Promise.allSettled([
          detail?.loadWorkOrderLogs?.() || Promise.resolve(),
          detail?.loadWorkorderDrawerProductStock?.() || Promise.resolve(),
        ])
      }
    }
    if (module.module === 'roles' || module.module === 'users') {
      await host.loadPermissionCaches?.()
    }
    setPanelMessage('已同步外部更新')
    return true
  }

  async function synchronizeAfterReconnect() {
    const moduleKey = normalizeModuleKey(host.activeKey.value)
    if (!moduleKey || moduleKey === 'dashboard' || !isKnownModule(moduleKey)) return
    const now = new Date().toISOString()
    const module: NotificationModuleSummary = {
      module: moduleKey,
      title: notificationModuleTitle(moduleKey),
      count: 0,
      items: [],
      action: {type: 'open_module', module: moduleKey},
      refresh: {module: moduleKey, invalidate_all: true},
      truncated: false,
    }
    const notification: RealtimeNotification = {
      id: `reconnect-${now}`,
      kind: 'data_changed',
      priority: 'normal',
      occurredAt: now,
      displayForMs: defaultToastMs,
      title: '实时连接已恢复',
      summary: `${module.title}正在重新同步`,
      count: 0,
      modules: [module],
      read: true,
    }
    const safe = canSilentlyRefresh(host, moduleKey)
    const detail = dispatchRefresh(notification, module, !safe)
    if (!safe) {
      setPanelMessage('实时连接已恢复，请保存或关闭当前修改后刷新')
      return
    }
    if (!detail?.handled) await host.loadActiveModule()
    setPanelMessage('已重新同步外部更新')
  }

  async function processBatch(events: DataChangeNotification[]) {
    const notification = aggregateNotificationEvents(events, `batch-${++messageSequence}`)
    if (!notification) return
    const activeModule = normalizeModuleKey(host.activeKey.value)
    const activeChange = notification.modules.find((module) => module.module === activeModule)
    if (notification.modules.some((module) => module.module === 'roles' || module.module === 'users')
      && (!activeChange || !canSilentlyRefresh(host, activeModule))) {
      void host.loadPermissionCaches?.()
    }
    let attemptedSilentRefresh = false
    if (activeChange && canSilentlyRefresh(host, activeModule) && notification.modules.length === 1) {
      attemptedSilentRefresh = true
      try {
        if (await refreshModule(activeChange, notification)) return
      } catch {
        // A failed silent refresh becomes an actionable notification below.
        setPanelMessage('外部更新同步失败，请手动刷新')
      }
    }
    pushNotification(notification)
    if (activeChange) {
      if (!attemptedSilentRefresh && canSilentlyRefresh(host, activeModule)) {
        void refreshModule(activeChange, notification).catch(() => setPanelMessage('外部更新同步失败，请手动刷新'))
      } else if (!attemptedSilentRefresh) {
        dispatchRefresh(notification, activeChange, true)
        setPanelMessage('数据已由其他用户更新，请保存或关闭后刷新')
      }
    }
  }

  let messageSequence = 0
  function queueEvent(event: DataChangeNotification) {
    pendingEvents.push(event)
    if (batchTimer !== undefined) return
    batchTimer = window.setTimeout(() => {
      batchTimer = undefined
      const events = pendingEvents.splice(0, pendingEvents.length)
      void processBatch(events)
    }, batchWindowMs)
  }

  function handleStreamData(value: unknown) {
    const event = normalizeNotification(value)
    if (!event || event.kind !== 'data_changed' || seenEventIDs.has(event.id)) return
    // Backend filtering is authoritative; this protects against a stale or
    // misconfigured server accidentally echoing the actor to their own session.
    if (event.actor_user_id !== undefined && sameEntity(event.actor_user_id, host.currentUser.value?.id)) return
    seenEventIDs.add(event.id)
    if (seenEventIDs.size > 500) {
      const first = seenEventIDs.values().next().value
      if (first) seenEventIDs.delete(first)
    }
    queueEvent(event)
  }

  async function connectLoop(generation: number) {
    let retryMs = 1000
    let hadConnection = false
    connectionState.value = 'connecting'
    while (!disposed && generation === connectionGeneration && host.token.value) {
      const controller = new AbortController()
      streamAbort = controller
      try {
        const response = await streamRequest(notificationStreamPath, {signal: controller.signal}, host.token.value)
        if (!response.ok) throw new Error(`通知通道响应 ${response.status}`)
        retryMs = 1000
        connectionState.value = 'connected'
        if (hadConnection) void synchronizeAfterReconnect().catch(() => setPanelMessage('连接已恢复，请手动刷新当前页面'))
        hadConnection = true
        await consumeSseResponse(response, handleStreamData)
        if (!controller.signal.aborted) throw new Error('通知通道已关闭')
      } catch (error) {
        if (controller.signal.aborted || disposed || generation !== connectionGeneration) break
        connectionState.value = 'reconnecting'
        await waitFor(retryMs)
        retryMs = Math.min(maxReconnectMs, retryMs * 2)
        // Keep the error local: normal connectivity state belongs in设置，
        // while this best-effort channel should not cover the business page.
        void error
      } finally {
        if (streamAbort === controller) streamAbort = null
      }
    }
    if (!disposed) connectionState.value = host.token.value ? 'idle' : 'stopped'
  }

  function startStream() {
    if (disposed || !host.token.value) return
    connectionGeneration += 1
    streamAbort?.abort()
    void connectLoop(connectionGeneration)
  }

  function stopStream() {
    connectionGeneration += 1
    streamAbort?.abort()
    streamAbort = null
    connectionState.value = 'stopped'
  }

  async function openNotification(notification: RealtimeNotification): Promise<void> {
    markAllRead()
    panelOpen.value = false
    hideToast()
    const requestedModule = normalizeModuleKey(notification.modules[0]?.action.module || '')
    const target = notification.modules.find((item) => item.module === requestedModule) || notification.modules[0]
    if (!target) return
    if (!isKnownModule(target.module)) {
      ElMessage.info('该更新暂时无法定位，请刷新后查看')
      return
    }
    if (host.activeKey.value !== target.module) {
      await host.switchModule(target.module)
      if (host.activeKey.value !== target.module) {
        ElMessage.info('当前修改尚未完成，暂时无法打开通知目标')
        return
      }
    } else {
      // switchModule starts the list request asynchronously for normal
      // navigation. Notifications must await the request before looking up
      // an entity; the visible page may still be on another filtered page.
      await host.loadActiveModule()
    }
    await nextTick()
    if (target.module === 'products' || target.module === 'warehouses') return
    if (target.action.type !== 'open_entity') return
    const targetID = target.action.entity_id ?? target.items[0]?.entity_id
    const item = target.items.find((candidate) => sameEntity(candidate.entity_id, targetID)) || target.items[0]
    const visibleRow = host.rows?.value.find((candidate) => sameEntity(candidate.id, targetID))
    if (target.module === 'warehouses') {
      let row: BasicItem | undefined
      if (host.loadWarehouseItemByID) {
        try {
          // The detail endpoint is authoritative for delete/permission
          // downgrade. A stale visible row must not be opened when this
          // lookup says the target is gone or no longer visible.
          row = await host.loadWarehouseItemByID(target.action.entity_type ?? item?.entity_type, targetID) || undefined
        } catch {
          ElMessage.warning('通知目标加载失败，请刷新后重试')
          return
        }
      } else row = visibleRow
      if (row && host.openWarehouseItem) {
        await host.openWarehouseItem(row)
        return
      }
      ElMessage.info('该库存物品已不存在或当前账号无权查看')
      return
    }
    if (target.module === 'workorder') {
      let row: BasicItem | undefined
      const loadByID = host.workorderContext?.list?.loadWorkOrderByID
      if (loadByID) {
        try { row = await loadByID(targetID) || undefined } catch {
          ElMessage.warning('通知目标加载失败，请刷新后重试')
          return
        }
      } else row = visibleRow
      if (row && host.workorderContext?.list?.openWorkOrder) {
        await host.workorderContext.list.openWorkOrder(row)
        return
      }
      ElMessage.info('该任务单已不存在或当前账号无权查看')
      return
    }
    const detail: NotificationOpenEventDetail = {notification, module: target}
    if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent(notificationOpenEvent, {detail}))
  }

  function pauseToast() {
    if (!visibleToast.value || toastPaused) return
    toastPaused = true
    if (toastTimer !== undefined) {
      toastRemaining = Math.max(1000, toastRemaining - (Date.now() - toastStartedAt))
      window.clearTimeout(toastTimer)
      toastTimer = undefined
    }
  }

  function resumeToast() {
    if (!visibleToast.value || !toastPaused) return
    toastPaused = false
    scheduleToast()
  }

  function clearSession() {
    pendingEvents.splice(0, pendingEvents.length)
    seenEventIDs.clear()
    if (batchTimer !== undefined) window.clearTimeout(batchTimer)
    batchTimer = undefined
    notifications.value = []
    hideToast()
    panelOpen.value = false
  }

  function handleServerChanged() {
    clearSession()
    if (host.token.value) startStream()
  }

  watch(host.token, (value, previous) => {
    if (value === previous) return
    clearSession()
    if (value) startStream()
    else stopStream()
  })

  onMounted(() => {
    window.addEventListener('bb-erp-server-changed', handleServerChanged)
    if (host.token.value) startStream()
  })
  onBeforeUnmount(() => {
    disposed = true
    stopStream()
    clearSession()
    if (panelMessageTimer !== undefined) window.clearTimeout(panelMessageTimer)
    window.removeEventListener('bb-erp-server-changed', handleServerChanged)
  })

  return {
    notifications,
    visibleToast,
    panelOpen,
    unreadCount,
    connectionState,
    togglePanel,
    closePanel,
    markRead,
    markAllRead,
    dismiss,
    clearAll,
    openNotification,
    pauseToast,
    resumeToast,
  }
}
