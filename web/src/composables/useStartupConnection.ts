import {computed, readonly, ref} from 'vue'
import {platformCapabilities} from '../platform/connection'
import type {ServerIdentity, StartupFailure, StartupPhase} from '../platform/types'
import {dirtyGuardRegistry} from '../platform/dirtyGuard'
import {
  canonicalServerOrigin,
  clearStoredWorkspaceSession,
  automaticServerCandidate,
  isSameServerIdentity,
  serverIdentityKey,
  shouldClearWorkspaceSession,
  uniqueServerIdentities,
} from '../platform/connectionPolicy'

function clearPersistedSession() {
  clearStoredWorkspaceSession(localStorage)
}

function errorMessage(value: unknown, fallback: string): string {
  return value instanceof Error && value.message ? value.message : fallback
}

export function useStartupConnection() {
  const platform = platformCapabilities()
  const phase = ref<StartupPhase>('Booting')
  const failure = ref<StartupFailure>('none')
  const message = ref('正在启动客户端')
  const error = ref('')
  const version = ref('')
  const candidates = ref<ServerIdentity[]>([])
  const currentServer = ref<ServerIdentity | null>(null)
  const manualAddress = ref('')
  const manualTesting = ref(false)
  const focusRevision = ref(0)
  let generation = 0

  const isReady = computed(() => phase.value === 'LoginReady')
  const isBusy = computed(() => phase.value === 'Booting' || phase.value === 'Discovering' || phase.value === 'Validating')
  const savedServerKey = computed(() => {
    const saved = platform.connection.savedIdentity()
    return saved ? serverIdentityKey(saved) : ''
  })

  function transition(next: StartupPhase, status: string) {
    phase.value = next
    message.value = status
    error.value = ''
    focusRevision.value += 1
  }

  function invalidateSessionWhenInstanceChanges(next: ServerIdentity) {
    const previous = platform.connection.savedIdentity()
    if (shouldClearWorkspaceSession(previous, next)) clearPersistedSession()
  }

  async function finishConnection(identity: ServerIdentity, expectedGeneration: number, note = '') {
    if (expectedGeneration !== generation) return
    invalidateSessionWhenInstanceChanges(identity)
    currentServer.value = platform.connection.connect(identity)
    failure.value = 'none'
    transition('AutoConnected', note || `已连接 ${identity.server_name}`)
    await new Promise((resolve) => window.setTimeout(resolve, 650))
    if (expectedGeneration !== generation) return
    transition('LoginReady', note || `已连接 ${identity.server_name}`)
  }

  async function useSavedServer(expectedGeneration: number, reason: StartupFailure): Promise<boolean> {
    const saved = platform.connection.savedIdentity()
    if (!saved) return false
    const savedOrigin = canonicalServerOrigin(saved.origin)
    if (!savedOrigin) return false
    if (canonicalServerOrigin(platform.connection.currentOrigin()) !== savedOrigin) clearPersistedSession()
    transition('Validating', '未收到自动发现响应，正在验证上次使用的服务器')
    try {
      const identity = await platform.connection.validate(savedOrigin)
      if (expectedGeneration !== generation) return true
      if (!isSameServerIdentity(identity, saved)) {
        throw new Error('上次地址返回了不同的服务器身份，已阻止自动连接')
      }
      await finishConnection(identity, expectedGeneration, '自动发现未收到响应，已连接上次使用的服务器')
      return true
    } catch (reasonValue) {
      if (expectedGeneration !== generation) return true
      failure.value = reason === 'discovery-failed' ? reason : 'validation-failed'
      error.value = errorMessage(reasonValue, '上次使用的服务器未通过就绪与身份验证')
      return false
    }
  }

  async function startWeb(expectedGeneration: number) {
    transition('Validating', '正在验证当前站点服务')
    try {
      const identity = await platform.connection.validate()
      await finishConnection(identity, expectedGeneration)
    } catch (reason) {
      if (expectedGeneration !== generation) return
      failure.value = 'validation-failed'
      error.value = errorMessage(reason, '当前站点服务不可用')
      transition('ManualSetup', '无法连接当前站点服务')
      error.value = errorMessage(reason, '当前站点服务不可用，请联系系统管理员')
    }
  }

  async function startDesktop(expectedGeneration: number) {
    transition('Discovering', '正在发现局域网内的博邦 ERP 服务')
    try {
      const discovered = uniqueServerIdentities(await platform.discovery.discover())
      if (expectedGeneration !== generation) return
      transition('Validating', '正在核对服务就绪状态与身份')
      candidates.value = discovered
      const automaticCandidate = automaticServerCandidate(discovered)
      if (automaticCandidate) {
        await finishConnection(automaticCandidate, expectedGeneration)
        return
      }
      if (discovered.length > 1) {
        failure.value = 'none'
        transition('SelectServer', `发现 ${discovered.length} 个博邦 ERP 服务候选，请明确选择`)
        return
      }
      if (await useSavedServer(expectedGeneration, 'no-server')) return
      if (expectedGeneration !== generation) return
      if (failure.value === 'validation-failed') {
        const validationError = error.value
        transition('ManualSetup', '上次使用的服务器验证失败')
        error.value = validationError
        return
      }
      failure.value = 'no-server'
      transition('ManualSetup', '未发现可连接的博邦 ERP 服务')
    } catch (reason) {
      if (expectedGeneration !== generation) return
      const savedConnected = await useSavedServer(expectedGeneration, 'discovery-failed')
      if (savedConnected || expectedGeneration !== generation) return
      failure.value = 'discovery-failed'
      transition('ManualSetup', '自动发现失败')
      error.value ||= errorMessage(reason, '无法完成局域网自动发现')
    }
  }

  async function start() {
    const expectedGeneration = ++generation
    failure.value = 'none'
    candidates.value = []
    currentServer.value = null
    transition('Booting', platform.kind === 'tauri' ? '正在启动客户端' : '正在打开业务工作台')
    try {
      version.value = await platform.appVersion()
    } catch {
      version.value = ''
    }
    if (expectedGeneration !== generation) return
    if (platform.kind === 'web') await startWeb(expectedGeneration)
    else await startDesktop(expectedGeneration)
  }

  async function rediscover() {
    if (platform.kind !== 'tauri') return start()
    const expectedGeneration = ++generation
    failure.value = 'none'
    candidates.value = []
    currentServer.value = null
    await startDesktop(expectedGeneration)
  }

  async function changeServer() {
    if (!(await dirtyGuardRegistry.confirmLeave('change-server'))) return
    clearPersistedSession()
    await rediscover()
  }

  async function selectServer(identity: ServerIdentity) {
    const selectedKey = serverIdentityKey(identity)
    const selected = candidates.value.find(
      (candidate) => serverIdentityKey(candidate) === selectedKey,
    )
    if (!selected) return
    const expectedGeneration = generation
    await finishConnection(selected, expectedGeneration)
  }

  async function connectManually() {
    if (manualTesting.value) return
    const expectedGeneration = generation
    manualTesting.value = true
    failure.value = 'none'
    error.value = ''
    transition('Validating', '正在验证手动填写的服务器')
    try {
      const identity = await platform.connection.validate(manualAddress.value)
      await finishConnection(identity, expectedGeneration)
    } catch (reason) {
      if (expectedGeneration !== generation) return
      failure.value = 'validation-failed'
      transition('ManualSetup', '服务器验证失败')
      error.value = errorMessage(reason, '该地址未通过就绪与身份验证')
    } finally {
      manualTesting.value = false
    }
  }

  return {
    platformKind: platform.kind,
    canChangeServer: platform.connection.canChangeServer,
    phase: readonly(phase),
    failure: readonly(failure),
    message: readonly(message),
    error: readonly(error),
    version: readonly(version),
    candidates: readonly(candidates),
    currentServer: readonly(currentServer),
    manualAddress,
    manualTesting: readonly(manualTesting),
    focusRevision: readonly(focusRevision),
    isReady,
    isBusy,
    savedServerKey,
    start,
    rediscover,
    changeServer,
    selectServer,
    connectManually,
  }
}

export type StartupConnection = ReturnType<typeof useStartupConnection>
