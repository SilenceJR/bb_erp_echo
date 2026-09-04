import type {ApiErrorBody} from '../types'

/** Keeps stable backend business codes while still tolerating text/non-standard errors. */
export function normalizeApiErrorBody(status: number, data: unknown): ApiErrorBody {
  const record = data && typeof data === 'object' ? data as Record<string, unknown> : null
  return {
    code: typeof record?.code === 'string' && record.code.trim()
      ? record.code
      : `HTTP_${status}`,
    message: typeof data === 'string'
      ? data
      : typeof record?.message === 'string' && record.message.trim()
        ? record.message
        : '请求失败',
    request_id: typeof record?.request_id === 'string' ? record.request_id : '',
  }
}
