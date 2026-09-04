import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'
const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8')
const styles = read('../src/styles/workspace-components.css')
const customer = read('../src/components/pages/CustomerProfileDrawer.vue')
test('设置采用520宽度且移除旧零内边距覆盖', () => {
  assert.match(read('../src/components/app/SettingsPanel.vue'), /min\(520px, 100%\)/)
  assert.doesNotMatch(read('../src/styles/feature-patterns.css'), /\.el-drawer__body\s*\{\s*padding:\s*0\s*!important/)
  assert.match(styles, /\.el-drawer__body[^}]*padding: 24px/)
})
test('客户表单使用固定footer，取消旧正文底部占位', () => {
  assert.match(customer, /<template #footer>/)
  assert.match(customer, /form="customer-profile-editor"/)
  assert.doesNotMatch(customer, /padding-bottom: calc\(76px/)
  assert.match(styles, /\.customer-form-grid[^}]*grid-template-columns: minmax\(0, 1fr\)/)
})
test('客户查看态采用语义化属性列表，长值可换行', () => {
  assert.match(customer, /<PropertyList>/)
  assert.doesNotMatch(customer, /<el-descriptions/)
  assert.match(read('../src/components/ui/PropertyItem.vue'), /96px minmax\(0, 1fr\)/)
  assert.match(read('../src/components/ui/PropertyItem.vue'), /overflow-wrap: anywhere/)
})
