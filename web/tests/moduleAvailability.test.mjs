import assert from 'node:assert/strict'
import test from 'node:test'
import {deferredModuleForPath, isModuleNotInitialized, statisticsSourceIsUnavailable} from '../src/platform/moduleAvailability.ts'

test('只把明确的 module_not_initialized 错误识别为待重构模块', () => {
  const unavailable = Object.assign(new Error('数据结构待重构'), {code: 'module_not_initialized'})
  const forbidden = Object.assign(new Error('无权限'), {code: 'forbidden'})
  assert.equal(isModuleNotInitialized(unavailable), true)
  assert.equal(isModuleNotInitialized(forbidden), false)
  assert.equal(isModuleNotInitialized({code: 'module_not_initialized'}), false)
})

test('统计降级按数据源别名识别，不影响仍可用分区', () => {
  const sources = ['inventory_balances', 'department_tasks']
  assert.equal(statisticsSourceIsUnavailable('sources_unavailable', sources, 'inventory'), true)
  assert.equal(statisticsSourceIsUnavailable('sources_unavailable', sources, 'workorders'), true)
  assert.equal(statisticsSourceIsUnavailable('sources_unavailable', sources, 'suppliers'), false)
  assert.equal(statisticsSourceIsUnavailable('ready', sources, 'inventory'), false)
})

test('暂缓接口路径只映射到对应业务模块', () => {
  assert.equal(deferredModuleForPath('/api/v1/warehouse/items/product/1'), 'warehouses')
  assert.equal(deferredModuleForPath('/api/v1/inventory/documents'), 'warehouses')
  assert.equal(deferredModuleForPath('/api/v1/workorder/2/logs'), 'workorder')
  assert.equal(deferredModuleForPath('/api/v1/suppliers?page=1'), 'suppliers')
  assert.equal(deferredModuleForPath('/api/v1/customers'), '')
})
