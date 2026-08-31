import assert from 'node:assert/strict'
import test from 'node:test'
import {
  automaticServerCandidate,
  clearStoredWorkspaceSession,
  isSameServerIdentity,
  serverIdentityKey,
  shouldClearWorkspaceSession,
  uniqueServerIdentities,
} from '../src/platform/connectionPolicy.ts'

const identity = (origin, instanceID = 'shared-instance') => ({
  product: 'bb-erp',
  discovery_protocol: 1,
  instance_id: instanceID,
  server_name: '博邦 ERP',
  server_version: '1.0.0',
  origin,
})

test('同 instance_id、不同 origin 保留为两个候选且不会自动连接', () => {
  const first = identity('http://192.168.1.10:8080')
  const clone = identity('http://192.168.1.11:8080')
  const duplicate = identity('http://192.168.1.10:8080/')
  const candidates = uniqueServerIdentities([first, clone, duplicate])

  assert.equal(candidates.length, 2)
  assert.notEqual(serverIdentityKey(first), serverIdentityKey(clone))
  assert.equal(isSameServerIdentity(first, clone), false)
  assert.equal(automaticServerCandidate(candidates), null)
})

test('origin 变化时旧认证会话被完整清除', () => {
  const values = new Map([
    ['bb_erp_access_token', 'old-access'],
    ['bb_erp_refresh_token', 'old-refresh'],
    ['bb_erp_access_token_expires_at', '2099-01-01T00:00:00Z'],
    ['unrelated', 'keep'],
  ])
  const storage = {removeItem: (key) => values.delete(key)}
  const saved = identity('http://192.168.1.10:8080')
  const clone = identity('http://192.168.1.11:8080')

  assert.equal(shouldClearWorkspaceSession(saved, clone), true)
  clearStoredWorkspaceSession(storage)
  assert.equal(values.has('bb_erp_access_token'), false)
  assert.equal(values.has('bb_erp_refresh_token'), false)
  assert.equal(values.has('bb_erp_access_token_expires_at'), false)
  assert.equal(values.get('unrelated'), 'keep')
})
