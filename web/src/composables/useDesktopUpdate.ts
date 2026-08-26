import {computed, readonly, ref} from 'vue'
import {desktopBridge} from '../api/transport'
import type {
  DesktopUpdatePlan,
  DesktopUpdateProgress,
  DesktopUpdateState,
  DesktopUpdateStrategy,
} from '../types'

const state = ref<DesktopUpdateState>('Idle')
const plan = ref<DesktopUpdatePlan | null>(null)
const currentVersion = ref('')
const message = ref('')
const error = ref('')
const downloadedBytes = ref(0)
const totalBytes = ref(0)
const activeStrategy = ref<DesktopUpdateStrategy | null>(null)
const fallbackReason = ref('')
const compatibilityMode = ref(false)
const operationPending = ref(false)

let initialized = false
let generation = 0
let operation: Promise<void> | null = null

function errorMessage(value: unknown, fallback: string): string {
  if (value instanceof Error) return value.message || fallback
  if (typeof value === 'string') return value || fallback
  return fallback
}

function isUnsupportedPlanError(value: unknown): boolean {
  const detail = errorMessage(value, '').toLowerCase()
  return /(^|\D)404(\D|$)|not found|unsupported|unknown command/.test(detail)
}

function resetProgress() {
  downloadedBytes.value = 0
  totalBytes.value = 0
  fallbackReason.value = ''
}

function normalizeState(value: unknown): DesktopUpdateState {
  const normalized = String(value || '').trim().toLowerCase()
  return ({
    idle: 'Idle', checking: 'Checking', ready: 'Ready', downloading: 'Downloading',
    verifying: 'Verifying', applying: 'Applying', restarting: 'Restarting', failed: 'Failed',
  } as Record<string, DesktopUpdateState>)[normalized] || 'Idle'
}

function acceptProgress(progress: DesktopUpdateProgress) {
  state.value = normalizeState(progress.state)
  message.value = progress.message || message.value
  downloadedBytes.value = Math.max(0, Number(progress.downloaded_bytes || 0))
  totalBytes.value = Math.max(0, Number(progress.total_bytes || 0))
  if (progress.strategy) activeStrategy.value = progress.strategy
  if (progress.fallback_reason) {
    fallbackReason.value = progress.fallback_reason
    activeStrategy.value = 'full'
    if (plan.value) plan.value = {...plan.value, strategy: 'full'}
  }
  if (progress.state !== 'Failed') error.value = ''
  else error.value = progress.message || '客户端更新失败，请重试'
}

async function initialize() {
  if (initialized) return
  initialized = true
  const bridge = desktopBridge()
  if (!bridge) return

  await bridge.onClientUpdateProgress(acceptProgress)
  window.addEventListener('bb-erp-server-changed', () => {
    generation += 1
    operation = null
    operationPending.value = false
    plan.value = null
    state.value = 'Idle'
    compatibilityMode.value = false
    error.value = ''
    message.value = ''
    resetProgress()
  })

  try {
    currentVersion.value = await bridge.appVersion()
    const current = await bridge.clientUpdateStatus()
    if (current?.state && current.state !== 'Idle') acceptProgress(current)
  } catch {
    // 状态同步失败不阻止用户重新检查；check() 会提供明确错误。
  }
}

async function check(): Promise<void> {
  const bridge = desktopBridge()
  if (!bridge) return
  await initialize()
  if (operation) return operation

  const requestGeneration = ++generation
  operationPending.value = true
  compatibilityMode.value = false
  error.value = ''
  message.value = '正在检查客户端更新'
  state.value = 'Checking'
  resetProgress()

  operation = (async () => {
    try {
      currentVersion.value = await bridge.appVersion()
      const nextPlan = await bridge.checkClientUpdate()
      if (requestGeneration !== generation) return
      plan.value = nextPlan
      if (nextPlan) {
        activeStrategy.value = nextPlan.strategy
        state.value = 'Ready'
        message.value = nextPlan.message || '客户端更新已准备就绪'
      } else {
        activeStrategy.value = null
        state.value = 'Idle'
        message.value = '当前客户端已是最新版本'
      }
    } catch (reason) {
      if (requestGeneration !== generation) return
      plan.value = null
      if (isUnsupportedPlanError(reason)) {
        compatibilityMode.value = true
        state.value = 'Idle'
        error.value = ''
        fallbackReason.value = ''
        message.value = '当前服务端暂不支持自动升级，可使用完整安装包更新'
      } else {
        state.value = 'Failed'
        error.value = errorMessage(reason, '客户端更新检查失败')
        message.value = error.value
      }
    } finally {
      if (requestGeneration === generation) operationPending.value = false
      operation = null
    }
  })()
  return operation
}

async function apply(): Promise<void> {
  const bridge = desktopBridge()
  if (!bridge || !plan.value) return
  await initialize()
  if (operation) return operation

  const requestGeneration = generation
  const selectedPlan = plan.value
  operationPending.value = true
  error.value = ''
  message.value = '正在准备客户端更新'
  resetProgress()

  operation = (async () => {
    try {
      const result = await bridge.applyClientUpdate(selectedPlan)
      if (requestGeneration !== generation) return
      if (result.fallback_reason) {
        fallbackReason.value = result.fallback_reason
        activeStrategy.value = 'full'
        plan.value = {...selectedPlan, strategy: 'full'}
      }
      if (result.strategy) activeStrategy.value = result.strategy
      message.value = result.message || message.value
      if (result.success === false) throw new Error(result.message || '客户端更新失败')
      if (result.state) state.value = normalizeState(result.state)
      else if (result.restart_required && state.value !== 'Restarting') state.value = 'Restarting'
    } catch (reason) {
      if (requestGeneration !== generation) return
      state.value = 'Failed'
      error.value = errorMessage(reason, '客户端更新失败，请重试')
      message.value = error.value
    } finally {
      if (requestGeneration === generation) operationPending.value = false
      operation = null
    }
  })()
  return operation
}

async function retry(): Promise<void> {
  if (plan.value && !compatibilityMode.value) await apply()
  else await check()
}

const taskInProgress = computed(() => operationPending.value || ['Checking', 'Downloading', 'Verifying', 'Applying', 'Restarting'].includes(state.value))
const closeLocked = computed(() => state.value === 'Applying' || state.value === 'Restarting')
const downloadPercent = computed(() => {
  if (state.value !== 'Downloading' || totalBytes.value <= 0) return null
  return Math.min(100, Math.max(0, Math.round(downloadedBytes.value / totalBytes.value * 100)))
})

export function useDesktopUpdate() {
  return {
    state: readonly(state),
    plan: readonly(plan),
    currentVersion: readonly(currentVersion),
    message: readonly(message),
    error: readonly(error),
    downloadedBytes: readonly(downloadedBytes),
    totalBytes: readonly(totalBytes),
    activeStrategy: readonly(activeStrategy),
    fallbackReason: readonly(fallbackReason),
    compatibilityMode: readonly(compatibilityMode),
    taskInProgress,
    closeLocked,
    downloadPercent,
    initialize,
    check,
    apply,
    retry,
  }
}
