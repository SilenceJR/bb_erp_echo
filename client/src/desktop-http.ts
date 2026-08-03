import {fetch as tauriFetch} from '@tauri-apps/plugin-http'
import type {DesktopHttpBridge} from '../../web/src/api/transport'

const serverUrlKey = 'bb_erp_server_url'
const defaultServerUrl = import.meta.env.VITE_API_BASE_URL || 'http://127.0.0.1:8080'
const defaultRequestTimeoutMs = 8000

// normalizeServerUrl 只接受纯 HTTP(S) 源地址，避免把路径、凭据或查询参数
// 保存为服务地址。后续切换公网 HTTPS 时无需重新构建客户端。
function normalizeServerUrl(value: string): string {
  const input = value.trim()
  if (!input) throw new Error('请输入 Go 服务地址')

  let url: URL
  try {
    url = new URL(input)
  } catch {
    throw new Error('服务地址格式不正确，例如 http://192.168.1.20:8080')
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new Error('服务地址只支持 http:// 或 https://')
  }
  if (url.username || url.password) throw new Error('服务地址不能包含账号或密码')
  if ((url.pathname && url.pathname !== '/') || url.search || url.hash) {
    throw new Error('服务地址只能填写主机和端口，不能包含路径或参数')
  }
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
    return new Error(`连接 ${serverUrl} 超时，请检查 Go 服务是否启动、地址端口是否正确，以及 macOS 防火墙是否放行`)
  }
  const detail = error instanceof Error ? error.message : String(error)
  return new Error(`无法连接 ${serverUrl}，请先点击“测试连接”确认服务器地址。${detail}`)
}

async function desktopFetch(path: string, init: RequestInit = {}, serverUrl = currentServerUrl): Promise<Response> {
  const controller = init.signal ? undefined : new AbortController()
  const timeout = controller ? window.setTimeout(() => controller.abort(), defaultRequestTimeoutMs) : undefined
  try {
    return await tauriFetch(apiUrl(path, serverUrl), {
      ...init,
      signal: init.signal || controller?.signal,
    })
  } catch (error) {
    throw connectionError(error, serverUrl)
  } finally {
    if (timeout) window.clearTimeout(timeout)
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
    return currentServerUrl
  },
  async testServerUrl(value) {
    const candidate = normalizeServerUrl(value)
    const response = await desktopFetch('/health', {
      method: 'GET',
      headers: {Accept: 'application/json'},
    }, candidate)
    if (!response.ok) throw new Error(`服务返回 HTTP ${response.status}，请确认这是博邦 ERP Go 服务地址`)
  },
}

window.__BB_ERP_DESKTOP__ = desktopHttpBridge
