import {activeTransport, desktopBridge} from '../api/transport'
import type {ConnectionPort, DiscoveryPort, FileSavePort, PlatformCapabilities, ServerIdentity} from './types'
import {discoveryProtocol, erpProduct} from './types'
import {uniqueServerIdentities} from './connectionPolicy'

const savedIdentityKey = 'bb_erp_server_identity'

function isPrivateIPv4(hostname: string): boolean {
  const octets = hostname.split('.').map(Number)
  if (octets.length !== 4 || octets.some((value) => !Number.isInteger(value) || value < 0 || value > 255)) return false
  const [first, second] = octets
  return first === 127
    || first === 10
    || first === 192 && second === 168
    || first === 172 && second >= 16 && second <= 31
}

export function normalizeInternalServerOrigin(value: string): string {
  const input = value.trim()
  if (!input) throw new Error('请输入服务器 IP 地址')
  let url: URL
  try {
    url = new URL(input.includes('://') ? input : `http://${input}`)
  } catch {
    throw new Error('服务器地址格式不正确，例如 192.168.1.20:8080')
  }
  if (url.protocol !== 'http:') throw new Error('内网服务器仅支持 HTTP 地址')
  if (!isPrivateIPv4(url.hostname)) throw new Error('请输入本机或局域网 IPv4 地址')
  if (url.username || url.password || url.search || url.hash || (url.pathname && url.pathname !== '/')) {
    throw new Error('服务器地址只能包含 IP 和端口')
  }
  if (!url.port) url.port = '8080'
  return url.origin
}

function readSavedIdentity(): ServerIdentity | null {
  try {
    return assertIdentity(JSON.parse(localStorage.getItem(savedIdentityKey) || 'null'))
  } catch {
    localStorage.removeItem(savedIdentityKey)
    return null
  }
}

function persistIdentity(identity: ServerIdentity): ServerIdentity {
  localStorage.setItem(savedIdentityKey, JSON.stringify(identity))
  return identity
}

function assertIdentity(value: unknown, origin?: string): ServerIdentity {
  const candidate = value as Partial<ServerIdentity> | null
  if (!candidate
    || candidate.product !== erpProduct
    || candidate.discovery_protocol !== discoveryProtocol
    || !candidate.instance_id?.trim()
    || !candidate.server_name?.trim()) {
    throw new Error('目标服务不是受支持的博邦 ERP 服务')
  }
  return {
    product: erpProduct,
    discovery_protocol: discoveryProtocol,
    instance_id: candidate.instance_id.trim(),
    server_name: candidate.server_name.trim(),
    server_version: candidate.server_version || '',
    origin: origin || candidate.origin || '',
  }
}

async function parseReady(response: Response): Promise<void> {
  if (!response.ok) throw new Error(`服务尚未就绪（HTTP ${response.status}）`)
  const body = await response.json().catch(() => null) as {status?: unknown} | null
  if (body?.status !== 'ready') throw new Error('服务尚未完成数据库初始化')
}

async function validateWebOrigin(): Promise<ServerIdentity> {
  await parseReady(await activeTransport().fetch('/ready', {headers: {Accept: 'application/json'}}))
  const response = await activeTransport().fetch('/api/v1/discovery/identity', {headers: {Accept: 'application/json'}})
  if (!response.ok) throw new Error(`服务身份验证失败（HTTP ${response.status}）`)
  const origin = activeTransport().baseUrl() || window.location.origin
  return assertIdentity(await response.json(), origin)
}

function tauriCapabilities(): PlatformCapabilities {
  const bridge = desktopBridge()!
  const connection: ConnectionPort = {
    canChangeServer: true,
    currentOrigin: () => bridge.baseUrl(),
    savedIdentity: readSavedIdentity,
    async validate(origin) {
      const normalized = normalizeInternalServerOrigin(origin || bridge.baseUrl())
      return assertIdentity(await bridge.testServerUrl(normalized), normalized)
    },
    connect(identity) {
      const normalized = normalizeInternalServerOrigin(identity.origin)
      bridge.setServerUrl(normalized)
      return persistIdentity({...assertIdentity(identity, normalized), origin: normalized})
    },
  }
  const discovery: DiscoveryPort = {
    supported: true,
    async discover() {
      const identities = await bridge.discoverServers()
      const verified: ServerIdentity[] = []
      for (const value of identities) {
        const identity = assertIdentity(value)
        identity.origin = normalizeInternalServerOrigin(identity.origin)
        verified.push(identity)
      }
      // 同一实例 ID 可能来自克隆服务。只有 origin 与 instance_id 都相同才是重复响应。
      return uniqueServerIdentities(verified)
    },
  }
  const fileSave: FileSavePort = {
    save: (path, fileName, token = '') => bridge.saveApiFile(path, fileName, token),
  }
  return {kind: 'tauri', connection, discovery, fileSave, appVersion: () => bridge.appVersion()}
}

function webCapabilities(): PlatformCapabilities {
  const connection: ConnectionPort = {
    canChangeServer: false,
    currentOrigin: () => activeTransport().baseUrl() || window.location.origin,
    savedIdentity: readSavedIdentity,
    validate: validateWebOrigin,
    connect: persistIdentity,
  }
  return {
    kind: 'web',
    connection,
    discovery: {supported: false, discover: async () => []},
    fileSave: {save: async () => ({status: 'error', message: 'Web 下载由请求层处理'})},
    appVersion: async () => '',
  }
}

export function platformCapabilities(): PlatformCapabilities {
  return desktopBridge() ? tauriCapabilities() : webCapabilities()
}
