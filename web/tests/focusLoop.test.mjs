import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'
import {resolveFocusLoopTarget} from '../src/platform/focusLoop.ts'

test('移动导航在首末可聚焦项之间循环 Tab', () => {
  assert.equal(resolveFocusLoopTarget(4, 3, false), 0)
  assert.equal(resolveFocusLoopTarget(4, 0, true), 3)
  assert.equal(resolveFocusLoopTarget(4, 1, false), null)
  assert.equal(resolveFocusLoopTarget(4, 2, true), null)
})

test('焦点逃出导航时拉回正确方向，无元素时安全结束', () => {
  assert.equal(resolveFocusLoopTarget(3, -1, false), 0)
  assert.equal(resolveFocusLoopTarget(3, -1, true), 2)
  assert.equal(resolveFocusLoopTarget(0, -1, false), null)
})

test('导航实现保留 Escape、焦点回归并避让 Element 浮层', () => {
  const workspace = readFileSync(new URL('../src/components/app/AppWorkspace.vue', import.meta.url), 'utf8')
  assert.match(workspace, /event\.key === 'Escape'/)
  assert.match(workspace, /resolveFocusLoopTarget\(focusable\.length, currentIndex, event\.shiftKey\)/)
  assert.match(workspace, /sidebarElement\.value\?\.focus\(\)/)
  assert.match(workspace, /\.el-overlay:not\(\[inert\]\), \.el-popper:not\(\[inert\]\)/)
  assert.match(workspace, /focusSidebarToggle\(\)/)
})
