import type { ApiErrorBody } from '../types'

// API_BASE_URL 是后端服务地址。
//
// 参数说明：
// - VITE_API_BASE_URL：可选覆盖值，适合桌面端或跨域调试。
//
// 默认值使用空字符串，表示请求当前页面同源服务。
// 这样 Web 管理端被 Echo 静态托管时，登录请求会直接发送到同一个后端进程，
// 避免打到写死端口后出现浏览器 Load failed 且后端无请求日志的问题。
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || ''

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

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...options,
    headers,
    body,
  })

  if (response.status === 204) {
    return undefined as T
  }

  const contentType = response.headers.get('content-type') || ''
  const data = contentType.includes('application/json') ? await response.json() : await response.text()

  if (!response.ok) {
    const fallback: ApiErrorBody = {
      code: `HTTP_${response.status}`,
      message: typeof data === 'string' ? data : data?.message || '请求失败',
      request_id: data?.request_id || '',
    }
    throw new ApiError(response.status, fallback)
  }

  return data as T
}

// apiBaseUrl 暴露当前后端地址，用于页面状态展示和问题定位。
export function apiBaseUrl(): string {
  return API_BASE_URL
}
