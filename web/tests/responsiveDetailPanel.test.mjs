import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'
import {shouldRequestDockedDetailClose} from '../src/platform/detailPanel.ts'

const activeDockedDetail = {
  key: 'Escape',
  defaultPrevented: false,
  visible: true,
  docked: true,
  escapeEnabled: true,
  blockedByFloatingLayer: false,
}

test('停靠详情只在自身活跃且没有上层浮层时响应 Escape', () => {
  assert.equal(shouldRequestDockedDetailClose(activeDockedDetail), true)
  assert.equal(shouldRequestDockedDetailClose({...activeDockedDetail, key: 'Enter'}), false)
  assert.equal(shouldRequestDockedDetailClose({...activeDockedDetail, defaultPrevented: true}), false)
  assert.equal(shouldRequestDockedDetailClose({...activeDockedDetail, visible: false}), false)
  assert.equal(shouldRequestDockedDetailClose({...activeDockedDetail, docked: false}), false)
  assert.equal(shouldRequestDockedDetailClose({...activeDockedDetail, escapeEnabled: false}), false)
  assert.equal(shouldRequestDockedDetailClose({...activeDockedDetail, blockedByFloatingLayer: true}), false)
})

test('覆盖详情保留 Element Drawer 的遮罩和 Escape 关闭契约', () => {
  const component = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  assert.match(component, /:close-on-click-modal="closeOnClickModal"/)
  assert.match(component, /:close-on-press-escape="closeOnPressEscape"/)
  assert.match(component, /closeOnClickModal: true/)
  assert.match(component, /closeOnPressEscape: true/)
})

test('停靠面板从同轨道零宽平滑展开且首帧保持可读', () => {
  const shell = readFileSync(new URL('../src/styles/shell.css', import.meta.url), 'utf8')
  const component = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  assert.match(shell, /grid-template-columns: var\(--bb-shell-sidebar-width\) minmax\(0, 1fr\) 0/)
  assert.match(shell, /transition: grid-template-columns var\(--bb-duration-base\)/)
  assert.match(component, /workspace-detail-panel-enter-from \{ opacity: \.92; transform: translateX\(8px\); \}/)
  assert.doesNotMatch(component, /workspace-detail-panel-enter-from \{ opacity: 0/)
})

test('停靠编辑表单聚焦首个可编辑控件，查看态策略可保留上下文', () => {
  const carrier = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  const customer = readFileSync(new URL('../src/components/pages/CustomerProfileDrawer.vue', import.meta.url), 'utf8')
  const warehouse = readFileSync(new URL('../src/components/pages/WarehouseDrawer.vue', import.meta.url), 'utf8')
  const workorder = readFileSync(new URL('../src/components/pages/WorkorderDrawer.vue', import.meta.url), 'utf8')
  assert.match(carrier, /dockedAutoFocus\?: 'preserve' \| 'panel' \| 'first-editable'/)
  assert.match(carrier, /input:not\(\[disabled\]\):not\(\[readonly\]\):not\(\[type="hidden"\]\)/)
  assert.match(carrier, /target\.focus\(\{preventScroll: true\}\)/)
  assert.match(customer, /:docked-auto-focus="mode === 'view' \? 'preserve' : 'first-editable'"/)
  assert.match(warehouse, /docked-auto-focus="panel"/)
  assert.match(workorder, /docked-auto-focus="panel"/)
})

test('已打开的停靠详情从查看切换编辑时重新进入首个字段', () => {
  const carrier = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  assert.match(carrier, /watch\(\(\) => props\.dockedAutoFocus, async \(policy, previousPolicy\) =>/)
  assert.match(carrier, /policy !== 'first-editable'[^\n]+!props\.modelValue \|\| !props\.docked/)
  assert.match(carrier, /await nextTick\(\)\s+focusDockedEntry\(\)/)
  assert.match(carrier, /\}, \{flush: 'post'\}\)/)
})
