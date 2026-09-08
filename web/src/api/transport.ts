import type {DesktopUpdateApplyResult, DesktopUpdatePlan, DesktopUpdateProgress} from '../types'
import type {FileSaveResult, ServerIdentity} from '../platform/types'

export interface DesktopFileUploadResult {
  status: number
  contentType: string
  body: string
}

/** The updater is deliberately a single portable Windows executable. */
export interface DesktopUpdateCapabilities {
  supported: boolean
  target?: 'windows-x86_64' | string
  strategy?: 'full' | 'portable' | string
  reason?: string
}

// HttpTransport 隔离业务请求与运行平台的网络实现。
// Web 使用浏览器同源 fetch；Tauri 在启动时注入 Rust HTTP 插件实现。
export interface HttpTransport {
  fetch(path: string, init?: RequestInit): Promise<Response>
  /** Long-lived response transport used by authenticated SSE streams. */
  fetchStream?(path: string, init?: RequestInit): Promise<Response>
  baseUrl(): string
}

export interface DesktopHttpBridge extends HttpTransport {
  setServerUrl(value: string): string
  testServerUrl(value: string): Promise<ServerIdentity>
  discoverServers(): Promise<ServerIdentity[]>
  saveApiFile(path: string, fileName: string, token?: string): Promise<FileSaveResult>
  uploadFiles(paths: string[], endpoint: string, fields: Record<string, string>, token: string): Promise<DesktopFileUploadResult>
  onWindowCloseRequested(handler: () => Promise<boolean>): Promise<() => void>
  appVersion(): Promise<string>
  checkClientUpdate(): Promise<DesktopUpdatePlan | null>
  applyClientUpdate(plan: DesktopUpdatePlan): Promise<DesktopUpdateApplyResult>
  clientUpdateStatus(): Promise<DesktopUpdateProgress>
  onClientUpdateProgress(handler: (progress: DesktopUpdateProgress) => void): Promise<() => void>
  /** Optional so older embedded bridges can still load the shared Web shell. */
  clientUpdateCapabilities?: () => Promise<DesktopUpdateCapabilities>
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
  fetchStream(path, init) {
    return window.fetch(`${browserBaseUrl}${path}`, init)
  },
  baseUrl() {
    return browserBaseUrl
  },
}

/**
 * Use a platform-specific long-lived response when available. Older embedded
 * clients may not expose it yet, so the browser-compatible fallback keeps the
 * shared shell loadable while those clients are upgraded.
 */
export function fetchStream(path: string, init?: RequestInit): Promise<Response> {
  const transport = activeTransport()
  return transport.fetchStream ? transport.fetchStream(path, init) : transport.fetch(path, init)
}

export function activeTransport(): HttpTransport {
  return window.__BB_ERP_DESKTOP__ || browserTransport
}

export function desktopBridge(): DesktopHttpBridge | undefined {
  return window.__BB_ERP_DESKTOP__
}
