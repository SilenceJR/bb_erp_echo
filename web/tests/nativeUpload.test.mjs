import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'
import ts from 'typescript'

const source = readFileSync(new URL('../src/api/http.ts', import.meta.url), 'utf8')
const desktopSource = readFileSync(new URL('../../client/src/desktop-http.ts', import.meta.url), 'utf8')
const js = ts.transpileModule(source.replace(/^import .*$/gm, '').replace(/^export /gm, ''), {compilerOptions: {target: ts.ScriptTarget.ES2022}}).outputText
function harness(statuses, refresh = async () => 'fresh') {
  const calls = [], failures = []
  const api = new Function('desktopBridge', 'window', 'moduleUnavailableEvent', `${js}; return {uploadNativeFiles, configureAuthSession}`)(
    () => ({uploadFiles: async (...args) => {
      calls.push(args)
      const status = statuses.shift()
      if (status instanceof Error || typeof status === 'string') throw status
      return {status, body: status === 200 ? '[{"id":1}]' : '{"message":"身份失效"}', content_type: 'application/json'}
    }}), {dispatchEvent() {}}, 'unavailable',
  )
  api.configureAuthSession({getToken: () => 'expired', refresh, onFailure: () => failures.push(true)})
  return {...api, calls, failures}
}

test('desktop bridge sends the nested Rust request with snake_case server_url', () => {
  assert.match(desktopSource, /request:\s*\{server_url:\s*currentServerUrl,\s*endpoint,\s*paths,\s*fields,\s*token\}/)
  assert.doesNotMatch(desktopSource, /request:\s*\{serverUrl:/)
})

test('native file upload refreshes a rejected token once and preserves the batch', async () => {
  const h = harness([401, 200])
  const paths = ['C:\\图片\\模具 1.png', 'C:\\图片\\模具 2.png']
  assert.deepEqual(await h.uploadNativeFiles('/api/v1/files/images', paths, {owner_id: '12'}, 'expired'), [{id: 1}])
  assert.equal(h.calls.length, 2)
  assert.deepEqual(h.calls[1], [paths, '/api/v1/files/images', {owner_id: '12'}, 'fresh'])
})

test('native auth failure stops, while transport uncertainty never retries a write', async () => {
  const h = harness([401], async () => {throw new Error('expired refresh')})
  await assert.rejects(h.uploadNativeFiles('/api/v1/files/images', ['file'], {}, 'expired'), /身份失效/)
  assert.equal(h.calls.length, 1)
  assert.equal(h.failures.length, 1)
  const network = harness(['拖放上传失败：连接中断'])
  await assert.rejects(network.uploadNativeFiles('/api/v1/files/images', ['file'], {}, 'expired'), error => error.resultMayBeUnknown === true)
  assert.equal(network.calls.length, 1)
  const denied = harness([403])
  await assert.rejects(denied.uploadNativeFiles('/api/v1/files/images', ['file'], {}, 'expired'))
  assert.equal(denied.calls.length, 1)
})
