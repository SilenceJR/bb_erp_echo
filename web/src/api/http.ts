import type { ApiErrorBody } from '../types'
import {activeTransport, desktopBridge} from './transport'
import type {DesktopFileUploadResult} from './transport'
import type {FileSaveResult} from '../platform/types'
import {normalizeApiErrorBody} from '../platform/apiError'
import {moduleUnavailableEvent} from '../platform/moduleAvailability'

export interface AuthSessionHooks {
  getToken: () => string
  refresh: () => Promise<string>
  onFailure: () => void
}

let authSessionHooks: AuthSessionHooks | null = null

// configureAuthSession 注册 Web 与 Tauri 共用的认证续期回调。
// 请求层只负责一次 401 重试，具体令牌状态仍由工作台控制器管理。
export function configureAuthSession(hooks: AuthSessionHooks | null): void {
  authSessionHooks = hooks
}

// ApiRequestOptions 扩展 fetch 配置，允许调用方直接传普通对象作为 JSON body。
type ApiRequestOptions = Omit<RequestInit, 'body'> & {
  body?: BodyInit | Record<string, unknown>
}

// ApiError 表示已经解析过的 HTTP 错误，便于页面直接展示 message 和 request_id。
export class ApiError extends Error {
  code: string
  requestId: string
  status: number

  constructor(status: number, body: ApiErrorBody) {
    super(body.message)
    this.name = 'ApiError'
    this.status = status
    this.code = body.code
    this.requestId = body.request_id
  }
}

// RequestTransportError 表示没有收到可解析 HTTP 响应的传输失败。
// resultMayBeUnknown=true 时，请求可能已到达服务器，页面应刷新核对后再允许用户重试。
export class RequestTransportError extends Error {
  resultMayBeUnknown: boolean

  constructor(message: string, resultMayBeUnknown: boolean) {
    super(message)
    this.name = 'RequestTransportError'
    this.resultMayBeUnknown = resultMayBeUnknown
  }
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message.trim() ? error.message : fallback
}

// request 统一封装 JSON 请求、Bearer Token 和错误解析。
//
// 参数说明：
// - path：后端接口路径，例如 /api/v1/auth/me。
// - options：fetch 配置，body 传对象时会自动序列化为 JSON。
// - token：当前登录令牌，存在时写入 Authorization 请求头。
export async function request<T>(
  path: string,
  options: ApiRequestOptions = {},
  token = '',
): Promise<T> {
  const headers = new Headers(options.headers)
  headers.set('Accept', 'application/json')

  let body = options.body
  if (body && typeof body === 'object' && !(body instanceof FormData) && !(body instanceof Blob)) {
    headers.set('Content-Type', 'application/json')
    body = JSON.stringify(body)
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  let response: Response
  try {
    response = await fetchWithAuthRetry(path, {
      ...options,
      headers,
      body,
    }, token)
  } catch (error) {
    throw new RequestTransportError(errorMessage(error, '网络请求失败'), true)
  }

  if (response.status === 204) {
    return undefined as T
  }

  const contentType = response.headers.get('content-type') || ''
  let data: any
  try {
    data = contentType.includes('application/json') ? await response.json() : await response.text()
  } catch (error) {
    throw new RequestTransportError(
      errorMessage(error, '读取服务器响应失败'),
      response.ok,
    )
  }

  if (!response.ok) {
    const error = new ApiError(response.status, normalizeApiErrorBody(response.status, data))
    notifyModuleUnavailable(path, error)
    throw error
  }

  return data as T
}

// uploadNativeFiles 让 Tauri 直接从本地路径流式上传文件，避免大资料包进入 WebView 内存。
export async function uploadNativeFiles<T>(
  path: string,
  paths: string[],
  fields: Record<string, string> = {},
  token = '',
): Promise<T> {
  const desktop = desktopBridge()
  if (!desktop) throw new Error('当前环境不支持原生文件拖放上传')
  let response: DesktopFileUploadResult
  try {
    response = await desktop.uploadFiles(paths, path, fields, token)
  } catch (error) {
    const message = errorMessage(error, '原生文件上传失败')
    const mayBeUnknown = message.includes('拖放上传失败') || message.includes('读取上传响应失败')
    throw new RequestTransportError(message, mayBeUnknown)
  }
  let data: unknown = null
  try { data = response.body ? JSON.parse(response.body) : null } catch { data = response.body }
  if (response.status < 200 || response.status >= 300) {
    const body = data && typeof data === 'object' ? data as Partial<ApiErrorBody> : null
    const error = new ApiError(response.status, {
      code: body?.code || `HTTP_${response.status}`,
      message: typeof data === 'string' ? data : body?.message || '请求失败',
      request_id: body?.request_id || '',
    })
    notifyModuleUnavailable(path, error)
    throw error
  }
  return data as T
}

// requestBlob 用于受保护的二进制资源，沿用 request 的 Bearer 认证和 ApiError 体验。
export async function requestBlob(
  path: string,
  options: Omit<RequestInit, 'body'> & {body?: BodyInit} = {},
  token = '',
): Promise<Blob> {
  const headers = new Headers(options.headers)
  headers.set('Accept', 'image/*, application/octet-stream')
  if (token) headers.set('Authorization', `Bearer ${token}`)
  const response = await fetchWithAuthRetry(path, {...options, headers}, token)
  if (!response.ok) {
    const contentType = response.headers.get('content-type') || ''
    const data = contentType.includes('application/json') ? await response.json() : await response.text()
    const error = new ApiError(response.status, normalizeApiErrorBody(response.status, data))
    notifyModuleUnavailable(path, error)
    throw error
  }
  return response.blob()
}

function notifyModuleUnavailable(path: string, error: ApiError): void {
  if (error.code !== 'module_not_initialized' || typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(moduleUnavailableEvent, {detail: {path, message: error.message}}))
}

async function fetchWithAuthRetry(path: string, init: RequestInit, token: string): Promise<Response> {
	let response = await activeTransport().fetch(path, init)
	if (!shouldRefreshAfterUnauthorized(path, token, response)) return response

	let refreshedToken: string
	try {
		const currentToken = authSessionHooks!.getToken()
		refreshedToken = currentToken && currentToken !== token
			? currentToken
			: await authSessionHooks!.refresh()
	} catch {
		authSessionHooks!.onFailure()
		return response
	}
	const retryHeaders = new Headers(init.headers)
	retryHeaders.set('Authorization', `Bearer ${refreshedToken}`)
	return await activeTransport().fetch(path, {...init, headers: retryHeaders})
}

function shouldRefreshAfterUnauthorized(path: string, token: string, response: Response): boolean {
  if (response.status !== 401 || !token || !authSessionHooks) return false
  return !path.startsWith('/api/v1/auth/login')
    && !path.startsWith('/api/v1/auth/refresh')
    && !path.startsWith('/api/v1/auth/logout')
}

// downloadApiFile 将桌面端文件写入交给原生保存能力。Web 则保留 Blob 下载，
// 其结果只代表浏览器已接管下载，不能证明磁盘写入已完成。
export async function downloadApiFile(path: string, fileName: string, token = ''): Promise<FileSaveResult> {
  const desktop = desktopBridge()
  if (desktop) {
    const result = await desktop.saveApiFile(path, fileName, token)
    if (result.status === 'error') throw new Error(result.message)
    return result
  }
  const blob = await requestBlob(path, {}, token)
  const objectUrl = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = objectUrl
  anchor.download = fileName || 'download'
  anchor.style.display = 'none'
  document.body.appendChild(anchor)
  try {
    anchor.click()
    return {status: 'saved', path: ''}
  } finally {
    anchor.remove()
    // 给浏览器和 Tauri WebView 留出接管下载的时间，再释放临时 URL。
    window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1000)
  }
}

// apiBaseUrl 暴露当前后端地址，用于页面状态展示和问题定位。
export function apiBaseUrl(): string {
  return activeTransport().baseUrl()
}

export function isDesktopClient(): boolean {
  return !!desktopBridge()
}

// desktopAppVersion 只在 Tauri 桥接存在时读取真实安装版本。Web 端返回空值，
// 避免用前端 package.json 版本冒充桌面客户端版本。
export async function desktopAppVersion(): Promise<string> {
  return await desktopBridge()?.appVersion() || ''
}

export function saveDesktopServerUrl(value: string): string {
  const bridge = desktopBridge()
  if (!bridge) throw new Error('Web 版使用当前页面同源服务，无需设置服务器地址')
  return bridge.setServerUrl(value)
}

export async function testDesktopServerUrl(value: string): Promise<void> {
  const bridge = desktopBridge()
  if (!bridge) throw new Error('Web 版使用当前页面同源服务，无需测试服务器地址')
  await bridge.testServerUrl(value)
}
