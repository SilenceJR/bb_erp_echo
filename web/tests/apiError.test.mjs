import assert from 'node:assert/strict'
import test from 'node:test'
import {normalizeApiErrorBody} from '../src/platform/apiError.ts'

test('错误响应保留后端稳定业务码', () => {
  assert.deepEqual(normalizeApiErrorBody(503, {
    code: 'module_not_initialized',
    message: '仓库数据结构待重构',
    request_id: 'req-1',
  }), {
    code: 'module_not_initialized',
    message: '仓库数据结构待重构',
    request_id: 'req-1',
  })
})

test('非标准错误响应安全回退到 HTTP 状态码', () => {
  assert.deepEqual(normalizeApiErrorBody(502, 'upstream failed'), {
    code: 'HTTP_502',
    message: 'upstream failed',
    request_id: '',
  })
})
