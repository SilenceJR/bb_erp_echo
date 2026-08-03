import {fetch as tauriFetch} from '@tauri-apps/plugin-http'
import type {DesktopHttpBridge} from '../../web/src/api/transport'

const serverUrlKey = 'bb_erp_server_url'
const defaultServerUrl = import.meta.env.VITE_API_BASE_URL || 'http://127.0.0.1:8080'

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

const desktopHttpBridge: DesktopHttpBridge = {
  fetch(path, init) {
    return tauriFetch(apiUrl(path), init)
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
    const controller = new AbortController()
    const timeout = window.setTimeout(() => controller.abort(), 5000)
    try {
      const response = await tauriFetch(apiUrl('/health', candidate), {
        method: 'GET',
        headers: {Accept: 'application/json'},
        signal: controller.signal,
      })
      if (!response.ok) throw new Error(`服务返回 HTTP ${response.status}`)
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        throw new Error('连接超时，请检查地址、端口和防火墙')
      }
      throw error
    } finally {
      window.clearTimeout(timeout)
    }
  },
}

window.__BB_ERP_DESKTOP__ = desktopHttpBridge

