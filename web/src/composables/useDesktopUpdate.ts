import {computed, readonly, ref} from 'vue'
import {ElMessage} from 'element-plus'
import {desktopBridge} from '../api/transport'
import {dirtyGuardRegistry} from '../platform/dirtyGuard'
import type {
  DesktopUpdatePlan,
  DesktopUpdateProgress,
  DesktopUpdateState,
} from '../types'

const state = ref<DesktopUpdateState>('Idle')
const plan = ref<DesktopUpdatePlan | null>(null)
const currentVersion = ref('')
const message = ref('')
const error = ref('')
const downloadedBytes = ref(0)
const totalBytes = ref(0)
const operationPending = ref(false)

let initialized = false
let generation = 0
let operation: Promise<void> | null = null

function errorMessage(value: unknown, fallback: string): string {
  if (value instanceof Error) return value.message || fallback
  if (typeof value === 'string') return value || fallback
  return fallback
}

function resetProgress() {
  downloadedBytes.value = 0
  totalBytes.value = 0
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
        state.value = 'Ready'
        message.value = nextPlan.message || '客户端更新已准备就绪'
      } else {
        state.value = 'Idle'
        message.value = '当前客户端已是最新版本'
      }
    } catch (reason) {
      if (requestGeneration !== generation) return
      plan.value = null
      state.value = 'Failed'
      error.value = errorMessage(reason, '客户端更新检查失败')
      message.value = error.value
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
  if (!(await dirtyGuardRegistry.confirmLeave('client-update'))) return
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
  if (plan.value) await apply()
  else await check()
}

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
      Applying: '客户端正在安装更新，完成前不能离开或关闭窗口',
      Restarting: '客户端正在重启，完成前不能离开或关闭窗口',
    }
    ElMessage.warning(lockMessages[state.value] || '客户端更新正在进行，完成前不能离开或关闭窗口')
    return false
  },
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
    taskInProgress,
    closeLocked,
    downloadPercent,
    initialize,
    check,
    apply,
    retry,
  }
}
