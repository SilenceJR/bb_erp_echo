import {computed, readonly, ref} from 'vue'
import {ElMessage} from 'element-plus'
import {desktopBridge} from '../api/transport'
import {dirtyGuardRegistry} from '../platform/dirtyGuard'
import type {
  DesktopUpdateApplyResult,
  DesktopUpdateErrorCode,
  DesktopUpdatePlan,
  DesktopUpdateProgress,
  DesktopUpdateState,
} from '../types'

// The updater is shared by the login screen, the dashboard notice and Settings.
// Keeping the state at module scope prevents two visible surfaces from starting
// competing downloads or rendering two different plans for the same server.
const state = ref<DesktopUpdateState>('Idle')
const plan = ref<DesktopUpdatePlan | null>(null)
const currentVersion = ref('')
const message = ref('')
const error = ref('')
const errorCode = ref<DesktopUpdateErrorCode | null>(null)
const requestId = ref('')
const downloadedBytes = ref(0)
const totalBytes = ref(0)
const operationPending = ref(false)
const hasChecked = ref(false)
const lastCheckedAt = ref('')
const serverOrigin = ref('')

let initialized = false
let initializePromise: Promise<void> | null = null
let generation = 0
let operation: Promise<void> | null = null
let autoCheckedOrigin = ''
let updatedNoticeShown = false
let rolledBackNoticeShown = false

const knownErrorCodes = new Set<DesktopUpdateErrorCode>([
  'server_unreachable',
  'server_unavailable',
  'not_published',
  'integrity_failure',
  'directory_not_writable',
  'unsupported_platform',
  'plan_changed',
  'rolled_back',
  'unknown',
])

function normalizeState(value: unknown): DesktopUpdateState {
  const normalized = String(value || '').trim().toLowerCase().replaceAll('-', '_')
  return ({
    idle: 'Idle', latest: 'Idle', up_to_date: 'Idle',
    checking: 'Checking', ready: 'Ready', available: 'Ready',
    downloading: 'Downloading', verifying: 'Verifying', applying: 'Applying',
    restarting: 'Restarting', rolledback: 'RolledBack', rolled_back: 'RolledBack',
    updated: 'Updated', succeeded: 'Updated', completed: 'Updated',
    failed: 'Failed', error: 'Failed',
  } as Record<string, DesktopUpdateState>)[normalized] || 'Idle'
}

function resetProgress() {
  downloadedBytes.value = 0
  totalBytes.value = 0
}

function resetForServerChange() {
  generation += 1
  operation = null
  operationPending.value = false
  autoCheckedOrigin = ''
  updatedNoticeShown = false
  rolledBackNoticeShown = false
  plan.value = null
  state.value = 'Idle'
  error.value = ''
  errorCode.value = null
  requestId.value = ''
  message.value = ''
  hasChecked.value = false
  lastCheckedAt.value = ''
  serverOrigin.value = ''
  resetProgress()
}

function field(value: unknown, key: string): unknown {
  if (!value || typeof value !== 'object') return undefined
  return (value as Record<string, unknown>)[key]
}

function inferErrorCode(text: string): DesktopUpdateErrorCode {
  const normalized = text.toLowerCase()
  if (normalized.includes('http 503') || normalized.includes('服务暂不可用') || normalized.includes('更新服务')) return 'server_unavailable'
  if (normalized.includes('不可写') || normalized.includes('writable') || normalized.includes('permission')) return 'directory_not_writable'
  if (normalized.includes('签名') || normalized.includes('sha-256') || normalized.includes('sha256') || normalized.includes('完整性') || normalized.includes('哈希')) return 'integrity_failure'
  if (normalized.includes('未发布') || normalized.includes('暂无有效') || normalized.includes('no update') || normalized.includes('not published')) return 'not_published'
  if (normalized.includes('平台') || normalized.includes('portable') || normalized.includes('windows')) return 'unsupported_platform'
  if (normalized.includes('计划已变化') || normalized.includes('plan changed')) return 'plan_changed'
  if (normalized.includes('恢复旧版本') || normalized.includes('回滚') || normalized.includes('rolled back')) return 'rolled_back'
  if (normalized.includes('连接') || normalized.includes('超时') || normalized.includes('网络') || normalized.includes('server') || normalized.includes('验证服务器') || normalized.includes('健康检查')) return 'server_unreachable'
  return 'unknown'
}

function updateError(value: unknown, fallback: string): {message: string; code: DesktopUpdateErrorCode; requestId: string} {
  const text = value instanceof Error ? value.message : typeof value === 'string' ? value : ''
  const rawCode = String(field(value, 'error_code') || field(value, 'code') || '').trim().toLowerCase()
  const code = knownErrorCodes.has(rawCode as DesktopUpdateErrorCode)
    ? rawCode as DesktopUpdateErrorCode
    : inferErrorCode(text || fallback)
  const id = String(field(value, 'request_id') || '').trim()
  return {message: text || fallback, code, requestId: id}
}

function acceptProgress(progress: DesktopUpdateProgress) {
  const nextState = normalizeState(progress.state)
  state.value = nextState
  message.value = progress.message || message.value
  downloadedBytes.value = Math.max(0, Number(progress.downloaded_bytes || 0))
  totalBytes.value = Math.max(0, Number(progress.total_bytes || 0))
  if (nextState !== 'Idle') hasChecked.value = true
  if (nextState === 'RolledBack') {
    errorCode.value = 'rolled_back'
    error.value = progress.message || '新版本启动失败，已恢复旧版本'
    if (!rolledBackNoticeShown) {
      rolledBackNoticeShown = true
      ElMessage.error({message: error.value, duration: 8000, showClose: true})
    }
  } else if (nextState === 'Updated') {
    error.value = ''
    errorCode.value = null
    requestId.value = ''
    if (!updatedNoticeShown) {
      updatedNoticeShown = true
      ElMessage.success(progress.message || '客户端已更新，当前正在使用新版')
    }
  } else if (nextState === 'Failed') {
    const details = updateError(progress, progress.message || '客户端更新失败，请重试')
    errorCode.value = details.code
    error.value = details.message
    requestId.value = details.requestId
  } else {
    error.value = ''
    errorCode.value = null
    requestId.value = ''
  }
}

async function initialize() {
  if (initialized) return
  if (initializePromise) return initializePromise
  initializePromise = (async () => {
    initialized = true
    const bridge = desktopBridge()
    if (!bridge) return

    try {
      await bridge.onClientUpdateProgress(acceptProgress)
    } catch {
      // Event subscription is best effort. Explicit check/apply still returns
      // the native error and the status command can restore the last snapshot.
    }
    window.addEventListener('bb-erp-server-changed', resetForServerChange)

    try {
      currentVersion.value = await bridge.appVersion()
      const current = await bridge.clientUpdateStatus()
      if (current?.state && current.state !== 'Idle') acceptProgress(current)
    } catch {
      // Status synchronisation must not block the first explicit check.
    }
  })()
  await initializePromise
}

async function check(): Promise<void> {
  const bridge = desktopBridge()
  if (!bridge) return
  await initialize()
  if (operation) return operation

  const requestGeneration = ++generation
  const origin = bridge.baseUrl()
  operationPending.value = true
  error.value = ''
  errorCode.value = null
  requestId.value = ''
  message.value = '正在检查客户端更新'
  state.value = 'Checking'
  resetProgress()

  const running = (async () => {
    try {
      currentVersion.value = await bridge.appVersion()
      const nextPlan = await bridge.checkClientUpdate()
      if (requestGeneration !== generation) return
      plan.value = nextPlan
      serverOrigin.value = origin
      hasChecked.value = true
      lastCheckedAt.value = new Date().toISOString()
      if (nextPlan) {
        state.value = 'Ready'
        message.value = '发现可用客户端更新'
      } else {
        state.value = 'Idle'
        message.value = '当前客户端已是最新版本'
      }
    } catch (reason) {
      if (requestGeneration !== generation) return
      const details = updateError(reason, '客户端更新检查失败')
      plan.value = null
      state.value = 'Failed'
      error.value = details.message
      errorCode.value = details.code
      requestId.value = details.requestId
      message.value = details.message
      hasChecked.value = true
      lastCheckedAt.value = new Date().toISOString()
      serverOrigin.value = origin
    } finally {
      if (requestGeneration === generation) operationPending.value = false
    }
  })()
  operation = running
  try {
    await running
  } finally {
    if (operation === running) operation = null
  }
}

/** Check once after the startup connection is verified, without logging in. */
async function checkOnConnection(): Promise<void> {
  const bridge = desktopBridge()
  if (!bridge) return
  await initialize()
  const origin = bridge.baseUrl()
  if (!origin || autoCheckedOrigin === origin) return
  autoCheckedOrigin = origin
  await check()
}

async function apply(): Promise<void> {
  const bridge = desktopBridge()
  if (!bridge || !plan.value) return
  if (!(await dirtyGuardRegistry.confirmLeave('client-update'))) return
  await initialize()
  if (operation) return operation

  const requestGeneration = generation
  const selectedPlan = plan.value
  operationPending.value = true
  error.value = ''
  errorCode.value = null
  requestId.value = ''
  message.value = '正在准备客户端更新'
  resetProgress()

  const running = (async () => {
    try {
      const result: DesktopUpdateApplyResult = await bridge.applyClientUpdate(selectedPlan)
      if (requestGeneration !== generation) return
      message.value = result.message || message.value
      if (result.success === false) throw Object.assign(new Error(result.message || '客户端更新失败'), result)
      if (result.state) state.value = normalizeState(result.state)
      else if (result.rolled_back) state.value = 'RolledBack'
      else if (result.restart_required && state.value !== 'Restarting') state.value = 'Restarting'
      if (state.value === 'RolledBack') {
        errorCode.value = 'rolled_back'
        error.value = result.message || '新版本启动失败，已恢复旧版本'
      }
    } catch (reason) {
      if (requestGeneration !== generation) return
      const details = updateError(reason, '客户端更新失败，请重试')
      const rolledBack = details.code === 'rolled_back' || details.message.includes('恢复旧版本') || details.message.includes('回滚')
      state.value = rolledBack ? 'RolledBack' : 'Failed'
      error.value = details.message
      errorCode.value = rolledBack ? 'rolled_back' : details.code
      requestId.value = details.requestId
      message.value = details.message
    } finally {
      if (requestGeneration === generation) operationPending.value = false
    }
  })()
  operation = running
  try {
    await running
  } finally {
    if (operation === running) operation = null
  }
}

async function retry(): Promise<void> {
  if (plan.value && state.value !== 'RolledBack') await apply()
  else await check()
}

const desktopAvailable = computed(() => Boolean(desktopBridge()))
const taskInProgress = computed(() => operationPending.value || ['Checking', 'Downloading', 'Verifying', 'Applying', 'Restarting'].includes(state.value))
const closeLocked = computed(() => state.value === 'Applying' || state.value === 'Restarting')
const downloadPercent = computed(() => {
  if (state.value !== 'Downloading' || totalBytes.value <= 0) return null
  return Math.min(100, Math.max(0, Math.round(downloadedBytes.value / totalBytes.value * 100)))
})

// Updating the executable is a hard lock: it cannot be bypassed by confirming a
// normal dirty-form prompt, and it protects module switching, logout, server
// changes, browser refresh and the Tauri title-bar close through one registry.
dirtyGuardRegistry.register({
  id: 'desktop-update-hard-lock',
  blocksUnload: () => ['Downloading', 'Verifying', 'Applying', 'Restarting'].includes(state.value),
  async confirmLeave() {
    if (!['Downloading', 'Verifying', 'Applying', 'Restarting'].includes(state.value)) return true
    const lockMessages: Partial<Record<DesktopUpdateState, string>> = {
      Downloading: '客户端更新正在下载，完成前不能离开或关闭窗口',
      Verifying: '客户端更新正在校验，完成前不能离开或关闭窗口',
      Applying: '客户端正在替换文件，完成前不能离开或关闭窗口',
      Restarting: '客户端正在重启，完成前不能离开或关闭窗口',
    }
    ElMessage.warning(lockMessages[state.value] || '客户端更新正在进行，完成前不能离开或关闭窗口')
    return false
  },
})

export function useDesktopUpdate() {
  return {
    desktopAvailable,
    state: readonly(state),
    plan: readonly(plan),
    currentVersion: readonly(currentVersion),
    message: readonly(message),
    error: readonly(error),
    errorCode: readonly(errorCode),
    requestId: readonly(requestId),
    downloadedBytes: readonly(downloadedBytes),
    totalBytes: readonly(totalBytes),
    hasChecked: readonly(hasChecked),
    lastCheckedAt: readonly(lastCheckedAt),
    serverOrigin: readonly(serverOrigin),
    taskInProgress,
    closeLocked,
    downloadPercent,
    initialize,
    check,
    checkOnConnection,
    apply,
    retry,
  }
}
