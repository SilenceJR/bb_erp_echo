import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'
import ts from 'typescript'

// Execute the actual controller functions in a small ref-backed harness.
// This covers state transitions; it is not browser or native acceptance.
const source = readFileSync(new URL('../src/composables/useWorkspaceController.ts', import.meta.url), 'utf8')
const parsed = ts.createSourceFile('controller.ts', source, ts.ScriptTarget.Latest, true)
function controllerFunction(name, dependencies) {
  let found
  function visit(node) {
    if (ts.isFunctionDeclaration(node) && node.name?.text === name) found = node
    ts.forEachChild(node, visit)
  }
  visit(parsed)
  assert.ok(found, `Missing controller function ${name}`)
  const js = ts.transpileModule(found.getText(parsed), {compilerOptions: {target: ts.ScriptTarget.ES2022}}).outputText
  return new Function(...Object.keys(dependencies), `${js}; return ${name}`)(...Object.values(dependencies))
}
const ref = value => ({value})

test('账号归属确认关闭同时清空目标并完成载体关闭', async () => {
  const target = ref({id: 1})
  let done = 0
  const close = controllerFunction('closeUserAffiliation', {
    affiliationSaving: ref(false), affiliationTarget: target,
    affiliationDepartmentID: ref(1), affiliationTerminalID: ref(undefined),
    affiliationInitial: ref({departmentID: 1, terminalID: undefined}),
    affiliationError: ref(''), appMessageBox: {confirm: async () => {}},
  })
  await close(() => done++)
  assert.equal(target.value, null)
  assert.equal(done, 1)
})

test('账号归属取消放弃后保持目标和编辑值', async () => {
  const target = ref({id: 1})
  const department = ref(2)
  let done = 0
  const close = controllerFunction('closeUserAffiliation', {
    affiliationSaving: ref(false), affiliationTarget: target,
    affiliationDepartmentID: department, affiliationTerminalID: ref(undefined),
    affiliationInitial: ref({departmentID: 1, terminalID: undefined}),
    affiliationError: ref(''), appMessageBox: {confirm: async () => {throw new Error('cancel')}},
  })
  await close(() => done++)
  assert.equal(target.value.id, 1)
  assert.equal(department.value, 2)
  assert.equal(done, 0)
})

test('模块503事件只标记不可用，不清空列表或关闭草稿', () => {
  const unavailable = ref(null)
  const rows = ref([{id: 1}])
  const open = ref(true)
  const target = ref({id: 1})
  const handle = controllerFunction('handleModuleUnavailableEvent', {
    deferredModuleForPath: () => 'suppliers', activeKey: ref('suppliers'),
    moduleUnavailable: unavailable, panelMessage: ref(''),
    rows, columns: ref(['name']), pageTotal: ref(1),
    showCreateForm: open, editingSupplier: target,
    performWarehouseClose: () => assert.fail('must not close warehouse'),
    closeWorkOrder: () => assert.fail('must not close workorder'),
  })
  handle({detail: {path: '/api/v1/suppliers', message: 'unavailable'}})
  assert.equal(unavailable.value.module, 'suppliers')
  assert.equal(open.value, true)
  assert.equal(target.value.id, 1)
  assert.equal(rows.value.length, 1)
})
