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

test('现有入口默认只使用右侧停靠，保留未启用 Drawer 回退代码', () => {
  const component = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  const panel = readFileSync(new URL('../src/composables/useResponsiveDetailPanel.ts', import.meta.url), 'utf8')
  assert.match(component, /<Teleport v-if="preferDocked"/)
  assert.match(component, /preferDocked: true/)
  assert.match(component, /<el-drawer\s+v-else/)
  assert.match(panel, /visible\.value && activeDetailKey\.value === key/)
  assert.doesNotMatch(panel, /canDockDetail/)
  assert.match(component, /:close-on-click-modal="closeOnClickModal"/)
  assert.match(component, /:close-on-press-escape="closeOnPressEscape"/)
  assert.match(component, /closeOnClickModal: true/)
  assert.match(component, /closeOnPressEscape: true/)
})

test('停靠面板从同轨道零宽平滑展开且首帧保持可读', () => {
  const shell = readFileSync(new URL('../src/styles/shell.css', import.meta.url), 'utf8')
  const component = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  assert.match(shell, /grid-template-columns: var\(--bb-shell-sidebar-width\) minmax\(0, 1fr\) 0/)
  assert.match(shell, /transition: grid-template-columns var\(--bb-duration-slow\)/)
  assert.match(component, /workspace-detail-panel-enter-from \{ transform: translateX\(6px\); \}/)
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
  assert.match(warehouse, /:docked-auto-focus="showQuickSupplier \? 'first-editable' : 'panel'"/)
  assert.match(workorder, /:docked-auto-focus="actionDialogVisible \? 'first-editable' : 'panel'"/)
})

test('已打开的停靠详情从查看切换编辑时重新进入首个字段', () => {
  const carrier = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  assert.match(carrier, /watch\(\(\) => props\.dockedAutoFocus, async \(policy, previousPolicy\) =>/)
  assert.match(carrier, /policy !== 'first-editable'[^\n]+!props\.modelValue/)
  assert.match(carrier, /if \(props\.docked\) focusDockedEntry\(\)\s+else focusOverlayEntry\(\)/)
  assert.match(carrier, /\}, \{flush: 'post'\}\)/)
})

test('新面板替换旧面板，右侧轨道不保留多层退出栈', () => {
  const layout = readFileSync(new URL('../src/composables/detailLayout.ts', import.meta.url), 'utf8')
  const panel = readFileSync(new URL('../src/composables/useResponsiveDetailPanel.ts', import.meta.url), 'utf8')
  const carrier = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  assert.match(layout, /activeDetailKey = ref<symbol>\(\)/)
  assert.match(layout, /activeDetailDocked = computed\(\(\) => activeDetailKey\.value !== undefined/)
  assert.match(panel, /activeDetailKey\.value = key/)
  assert.match(panel, /if \(activeDetailKey\.value === key\) activeDetailKey\.value = undefined/)
  assert.match(carrier, /open && !docked && wasOpen && wasDocked && props\.preferDocked/)
  assert.match(carrier, /requestClose\(\)/)
})

test('关闭事件前先恢复触发点，连续替换不丢失焦点', () => {
  const carrier = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  assert.match(carrier, /active\.closest\('\.el-popper, \.el-overlay'\)/)
  assert.match(carrier, /active\.closest\('\.sidebar'\)/)
  assert.match(carrier, /\[aria-haspopup\]\[aria-expanded="true"\], \.topbar \.user-avatar/)
  assert.match(carrier, /if \(target\?\.isConnected\) target\.focus\(\{preventScroll: true\}\)\s+emit\('closed'\)/)
})

test('客户表格重建操作列后仍按资料标识恢复详情触发点', () => {
  const page = readFileSync(new URL('../src/components/pages/CustomerPage.vue', import.meta.url), 'utf8')
  const drawer = readFileSync(new URL('../src/components/pages/CustomerProfileDrawer.vue', import.meta.url), 'utf8')
  assert.match(page, /data-customer-profile-trigger/)
  assert.match(drawer, /original\?\.isConnected/)
  assert.match(drawer, /data-customer-profile-trigger/)
})

test('设置在接管右轨前等待当前面板的未保存守卫', () => {
  const layout = readFileSync(new URL('../src/composables/detailLayout.ts', import.meta.url), 'utf8')
  const carrier = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  const workspace = readFileSync(new URL('../src/components/app/AppWorkspace.vue', import.meta.url), 'utf8')
  assert.match(layout, /bb:request-active-detail-close/)
  assert.match(carrier, /requestCloseWithResult\(\)\.then\(request\.resolve\)/)
  assert.match(carrier, /if \(result instanceof Promise\)/)
  assert.match(carrier, /lastPanelFocus\.value/)
  assert.match(carrier, /document\.addEventListener\('focusin', rememberPanelFocus\)/)
  assert.match(workspace, /!\(await requestActiveDetailClose\(\)\)/)
})

test('覆盖导航先消费 Escape，不与底层右轨同时关闭', () => {
  const carrier = readFileSync(new URL('../src/components/ui/ResponsiveDetailCarrier.vue', import.meta.url), 'utf8')
  const workspace = readFileSync(new URL('../src/components/app/AppWorkspace.vue', import.meta.url), 'utf8')
  assert.match(workspace, /event\.stopImmediatePropagation\(\)/)
  assert.match(carrier, /\.sidebar\.is-mobile-open, \.mobile-nav-backdrop/)
})

test('嵌套建档在同一载体内切换步骤，不创建第二条右轨', () => {
  const warehouse = readFileSync(new URL('../src/components/pages/WarehouseDrawer.vue', import.meta.url), 'utf8')
  const productField = readFileSync(new URL('../src/components/pages/WorkorderProductField.vue', import.meta.url), 'utf8')
  const actionStep = readFileSync(new URL('../src/components/pages/WorkorderActionDialog.vue', import.meta.url), 'utf8')
  const workorderDrawer = readFileSync(new URL('../src/components/pages/WorkorderDrawer.vue', import.meta.url), 'utf8')
  const moduleForm = readFileSync(new URL('../src/components/pages/module/ModuleCreateForm.vue', import.meta.url), 'utf8')
  assert.equal((warehouse.match(/<ResponsiveDetailCarrier/g) || []).length, 1)
  assert.match(warehouse, /v-if="showQuickSupplier"/)
  assert.match(warehouse, /返回入库办理/)
  assert.doesNotMatch(warehouse, /supplierPanel/)
  assert.doesNotMatch(productField, /ResponsiveDetailCarrier|useResponsiveDetailPanel/)
  assert.doesNotMatch(actionStep, /ResponsiveDetailCarrier|useResponsiveDetailPanel/)
  assert.match(moduleForm, /<WorkorderTemporaryProductStep v-if="temporaryProductDialogVisible"/)
  assert.match(moduleForm, /返回任务单/)
  assert.equal((workorderDrawer.match(/<ResponsiveDetailCarrier/g) || []).length, 1)
  assert.match(workorderDrawer, /<WorkorderActionDialog v-if="actionDialogVisible"/)
  assert.match(actionStep, /返回任务详情/)
})

test('同轨道切换对象先保护未保存内容', () => {
  const workspaceController = readFileSync(new URL('../src/composables/useWorkspaceController.ts', import.meta.url), 'utf8')
  const directory = readFileSync(new URL('../src/composables/useDirectoryOperations.ts', import.meta.url), 'utf8')
  const warehouseOperations = readFileSync(new URL('../src/composables/useWarehouseOperations.ts', import.meta.url), 'utf8')
  const warehouse = readFileSync(new URL('../src/components/pages/WarehouseDrawer.vue', import.meta.url), 'utf8')
  assert.match(workspaceController, /当前权限配置尚未保存，切换后修改将丢失/)
  assert.match(workspaceController, /当前账号归属尚未保存，切换后修改将丢失/)
  assert.match(directory, /当前供应商资料尚未保存，切换后修改将丢失/)
  assert.match(warehouseOperations, /replacingCurrent && !\(await requestWarehouseClose\(\)\)/)
  assert.match(warehouse, /if \(await requestWarehouseClose\(\)\) \{\s+invalidateWarehouseRequests\(\)\s+done\(\)/)
})
