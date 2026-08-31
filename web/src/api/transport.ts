import type {DesktopUpdateApplyResult, DesktopUpdatePlan, DesktopUpdateProgress} from '../types'
import type {FileSaveResult, ServerIdentity} from '../platform/types'

// HttpTransport 隔离业务请求与运行平台的网络实现。
// Web 使用浏览器同源 fetch；Tauri 在启动时注入 Rust HTTP 插件实现。
export interface HttpTransport {
  fetch(path: string, init?: RequestInit): Promise<Response>
  baseUrl(): string
}

export interface DesktopHttpBridge extends HttpTransport {
  setServerUrl(value: string): string
  testServerUrl(value: string): Promise<ServerIdentity>
  discoverServers(): Promise<ServerIdentity[]>
  saveApiFile(path: string, fileName: string, token?: string): Promise<FileSaveResult>
  onWindowCloseRequested(handler: () => Promise<boolean>): Promise<() => void>
  appVersion(): Promise<string>
  checkClientUpdate(): Promise<DesktopUpdatePlan | null>
  applyClientUpdate(plan: DesktopUpdatePlan): Promise<DesktopUpdateApplyResult>
  clientUpdateStatus(): Promise<DesktopUpdateProgress>
  onClientUpdateProgress(handler: (progress: DesktopUpdateProgress) => void): Promise<() => void>
}

declare global {
  interface Window {
    __BB_ERP_DESKTOP__?: DesktopHttpBridge
  }
}

const browserBaseUrl = import.meta.env.VITE_API_BASE_URL || ''

const browserTransport: HttpTransport = {
  fetch(path, init) {
    return window.fetch(`${browserBaseUrl}${path}`, init)
  },
  baseUrl() {
    return browserBaseUrl
  },
}

export function activeTransport(): HttpTransport {
  return window.__BB_ERP_DESKTOP__ || browserTransport
}

export function desktopBridge(): DesktopHttpBridge | undefined {
  return window.__BB_ERP_DESKTOP__
}
