import {fetch as tauriFetch} from '@tauri-apps/plugin-http'
import {getVersion} from '@tauri-apps/api/app'
import {invoke} from '@tauri-apps/api/core'
import {listen} from '@tauri-apps/api/event'
import {getCurrentWindow} from '@tauri-apps/api/window'
import type {DesktopFileUploadResult, DesktopHttpBridge} from '../../web/src/api/transport'
import type {DesktopUpdateApplyResult, DesktopUpdatePlan, DesktopUpdateProgress} from '../../web/src/types'
import type {FileSaveResult, ServerIdentity} from '../../web/src/platform/types'

const serverUrlKey = 'bb_erp_server_url'
const defaultServerUrl = import.meta.env.VITE_API_BASE_URL || 'http://127.0.0.1:8080'
const defaultRequestTimeoutMs = 8000
// 高清图转换可能需要较长时间；与原生拖放上传保持一致，避免服务端已入库而桌面端在 60 秒先报失败。
const fileRequestTimeoutMs = 2 * 60 * 60 * 1000
// 比服务端默认 10 分钟下载超时多留 2 分钟，确保桌面端能收到服务端的具体校验错误。
const serverUpdateDownloadTimeoutMs = 12 * 60 * 1000

function isPrivateIPv4(hostname: string): boolean {
  const octets = hostname.split('.').map(Number)
  if (octets.length !== 4 || octets.some((item) => !Number.isInteger(item) || item < 0 || item > 255)) return false
  const [first, second] = octets
  return first === 127 || first === 10 || first === 192 && second === 168 || first === 172 && second >= 16 && second <= 31
}

// 仅保存内网 IPv4 HTTP 源地址；服务身份验证由 Rust 命令完成。
function normalizeServerUrl(value: string): string {
  const input = value.trim()
  if (!input) throw new Error('请输入 Go 服务地址')

  let url: URL
  try {
    url = new URL(input)
  } catch {
    throw new Error('服务地址格式不正确，例如 http://192.168.1.20:8080')
  }
  if (url.protocol !== 'http:') {
    throw new Error('内网服务器仅支持 http:// 地址')
  }
  if (!isPrivateIPv4(url.hostname)) {
    throw new Error('请输入本机或局域网 IPv4 地址')
  }
  if (url.username || url.password) throw new Error('服务地址不能包含账号或密码')
  if ((url.pathname && url.pathname !== '/') || url.search || url.hash) {
    throw new Error('服务地址只能填写主机和端口，不能包含路径或参数')
  }
  if (!url.port) url.port = '8080'
  return url.origin
}

function storedServerUrl(): string {
  const saved = localStorage.getItem(serverUrlKey)
  try {
    return normalizeServerUrl(saved || defaultServerUrl)
  } catch {
    return normalizeServerUrl(defaultServerUrl)
  }
}

let currentServerUrl = storedServerUrl()

function apiUrl(path: string, serverUrl = currentServerUrl): string {
  if (!path.startsWith('/') || path.startsWith('//')) {
    throw new Error('桌面端请求必须使用站内 API 路径')
  }
  return `${serverUrl}${path}`
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
      || error instanceof Error && error.message === 'Request cancelled'
}

function connectionError(error: unknown, serverUrl = currentServerUrl): Error {
  if (isAbortError(error)) {
    return new Error(`连接 ${serverUrl} 超时，请检查 Go 服务是否启动、地址端口是否正确，以及系统防火墙是否放行`)
  }
  const detail = error instanceof Error ? error.message : String(error)
  return new Error(`无法连接 ${serverUrl}，请先点击“测试连接”确认服务器地址。${detail}`)
}

function isFileTransferPath(path: string): boolean {
  return path.startsWith('/api/v1/files')
    || path.startsWith('/api/v1/customers/import')
    || path.startsWith('/api/v1/customers/export')
}

async function desktopFetch(path: string, init: RequestInit = {}, serverUrl = currentServerUrl): Promise<Response> {
  const controller = new AbortController()
  const requestTimeoutMs = path.startsWith('/api/v1/system/updates/server/download')
      ? serverUpdateDownloadTimeoutMs
      : isFileTransferPath(path)
        ? fileRequestTimeoutMs
        : defaultRequestTimeoutMs
  // 合并调用方取消信号与桌面端请求超时，避免上传下载请求过早中断。
  const abortFromCaller = () => controller.abort()
  if (init.signal) {
    if (init.signal.aborted) controller.abort()
    else init.signal.addEventListener('abort', abortFromCaller, {once: true})
  }
  const timeout = window.setTimeout(() => controller.abort(), requestTimeoutMs)
  try {
    return await tauriFetch(apiUrl(path, serverUrl), {
      ...init,
      signal: controller.signal,
    })
  } catch (error) {
    throw connectionError(error, serverUrl)
  } finally {
    window.clearTimeout(timeout)
    init.signal?.removeEventListener('abort', abortFromCaller)
  }
}

const desktopHttpBridge: DesktopHttpBridge = {
  fetch(path, init) {
    return desktopFetch(path, init)
  },
  baseUrl() {
    return currentServerUrl
  },
  setServerUrl(value) {
    currentServerUrl = normalizeServerUrl(value)
    localStorage.setItem(serverUrlKey, currentServerUrl)
    window.dispatchEvent(new CustomEvent('bb-erp-server-changed'))
    return currentServerUrl
  },
  async testServerUrl(value) {
    const candidate = normalizeServerUrl(value)
    return await invoke<ServerIdentity>('test_server_connection', {serverUrl: candidate})
  },
  async discoverServers() {
    return await invoke<ServerIdentity[]>('discover_servers')
  },
  async saveApiFile(path, fileName, token = '') {
    return await invoke<FileSaveResult>('save_api_file', {
      serverUrl: currentServerUrl,
      apiPath: path,
      fileName,
      token,
    })
  },
  async uploadFiles(paths, endpoint, fields, token) {
    return await invoke<DesktopFileUploadResult>('upload_dropped_files', {
      request: {serverUrl: currentServerUrl, endpoint, paths, fields, token},
    })
  },
  async onWindowCloseRequested(handler) {
    const appWindow = getCurrentWindow()
    return await appWindow.onCloseRequested(async (event) => {
      event.preventDefault()
      if (await handler()) await appWindow.destroy()
    })
  },
  appVersion() {
    return getVersion()
  },
  checkClientUpdate() {
    return invoke<DesktopUpdatePlan | null>('client_update_check', {serverUrl: currentServerUrl})
  },
  applyClientUpdate(plan) {
    return invoke<DesktopUpdateApplyResult>('client_update_apply', {plan})
  },
  clientUpdateStatus() {
    return invoke<DesktopUpdateProgress>('client_update_status')
  },
  async onClientUpdateProgress(handler) {
    return await listen<DesktopUpdateProgress>('client-update-progress', (event) => handler(event.payload))
  },
}

window.__BB_ERP_DESKTOP__ = desktopHttpBridge
