import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'

const patterns = readFileSync(new URL('../src/styles/feature-patterns.css', import.meta.url), 'utf8')
const settings = readFileSync(new URL('../src/components/app/SettingsPanel.vue', import.meta.url), 'utf8')
const customer = readFileSync(new URL('../src/components/pages/CustomerProfileDrawer.vue', import.meta.url), 'utf8')

test('设置 Drawer 用实际根类覆盖全局零内边距并统一正文基线', () => {
  assert.match(patterns, /\.el-drawer\.settings-drawer \{ --bb-settings-inline-inset: var\(--bb-space-6\)/)
  assert.match(patterns, /> \.el-drawer__header \{[^}]*var\(--bb-settings-inline-inset\)/)
  assert.match(patterns, /> \.el-drawer__body \{[^}]*padding: 0 var\(--bb-settings-inline-inset\)[^}]*!important/)
  assert.match(patterns, /@media \(max-width: 520px\) \{[\s\S]*--bb-settings-inline-inset: var\(--bb-space-4\)/)
  assert.match(settings, /\.settings-content \{ display: grid; width: 100%; min-width: 0;/)
})

test('客户编码切换是中性分段控件且与字段保持同宽', () => {
  assert.match(customer, /\.code-mode-switch \{[^}]*width: 100%;[^}]*background: var\(--bb-bg-subtle\);[^}]*padding: 3px;/)
  assert.match(customer, /\.el-radio-button\.is-active \.el-radio-button__inner\) \{[^}]*background: var\(--bb-bg-surface\);/)
  const switchStyles = customer.slice(customer.indexOf('.code-mode-switch {'), customer.indexOf('.customer-form-grid {'))
  assert.doesNotMatch(switchStyles, /--bb-action-primary|--bb-brand-500/)
})

test('420px 停靠客户表单为单列，覆盖式 Drawer 保持两列且预留底部滚动空间', () => {
  assert.match(customer, /\.customer-form-grid \{[^}]*repeat\(2, minmax\(0, 1fr\)\)/)
  assert.match(patterns, /\.workspace-detail-aside\.customer-profile-drawer \.customer-form-grid \{ grid-template-columns: 1fr; \}/)
  assert.match(patterns, /\.el-drawer\.customer-profile-drawer > \.el-drawer__body \{ scroll-padding-bottom: calc\(96px/)
  assert.match(customer, /padding-bottom: calc\(76px \+ env\(safe-area-inset-bottom, 0px\)\)/)
})

test('客户查看态固定标签列并让长值换行，空值使用克制占位', () => {
  assert.match(customer, /<el-descriptions class="customer-profile-details" :column="1" border>/)
  assert.match(customer, /\.el-descriptions__label\.el-descriptions__cell\) \{ width: 116px; min-width: 116px;/)
  assert.match(customer, /\.el-descriptions__content\.el-descriptions__cell\) \{[^}]*overflow-wrap: anywhere; word-break: break-word;/)
  assert.match(customer, /\.customer-detail-value\.is-empty \{ color: var\(--bb-text-placeholder\); \}/)
  assert.match(customer, /return value \|\| '—'/)
})
