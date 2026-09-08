import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'
import ts from 'typescript'

function method(file, name, dependencies) {
  const vue = readFileSync(new URL(file, import.meta.url), 'utf8')
  const source = vue.match(/<script setup[^>]*>([\s\S]*?)<\/script>/)[1]
  const ast = ts.createSourceFile('component.ts', source, ts.ScriptTarget.Latest, true)
  const fn = ast.statements.find(node => ts.isFunctionDeclaration(node) && node.name?.text === name)
  assert.ok(fn)
  const js = ts.transpileModule(fn.getText(ast), {compilerOptions: {target: ts.ScriptTarget.ES2022}}).outputText
  return new Function(...Object.keys(dependencies), `${js}; return ${name}`)(...Object.values(dependencies))
}
const ref = value => ({value})

test('image drag refuses read-only/busy uploads and uses only one payload source', () => {
  const calls = [], props = {canWrite: false}, saving = ref(false), loading = ref(false), errorMessage = ref('')
  const handle = method('../src/components/ImageGallery.vue', 'handleNativeFileDrag', {
    props, saving, loading, errorMessage, statusMessage: ref(''), isDragging: ref(false),
    uploadNativePaths: paths => calls.push(paths), uploadSelectedFiles: files => calls.push(files),
  })
  const event = {detail: {phase: 'drop', paths: ['image.png'], files: ['duplicate']}}
  handle(event); assert.match(errorMessage.value, /权限/)
  props.canWrite = true; saving.value = true
  handle(event); assert.match(errorMessage.value, /正在处理/)
  saving.value = false; handle(event)
  assert.deepEqual(calls, [['image.png']])
})

test('native image batches reject unsupported extensions before invoking upload', async () => {
  const errorMessage = ref('')
  const upload = method('../src/components/ImageGallery.vue', 'uploadNativePaths', {
    props: {canWrite: true}, saving: ref(false), maxImageBatch: 100,
    errorMessage, statusMessage: ref(''), allowedExtensions: new Set(['png']),
    imageFileName: path => path.split(/[\\/]/).pop(), allowedFormatMessage: '仅支持图片',
  })
  await upload(['C:\\照片.png', 'C:\\文档.exe'])
  assert.match(errorMessage.value, /本次未发起上传/)
})

test('customer native drop does not keep the path of a rejected file', () => {
  const file = ref(null), nativeFilePath = ref(null), error = ref('')
  const handle = method('../src/components/pages/CustomerImportDialog.vue', 'handleNativeFileDrag', {
    busy: ref(false), dragging: ref(false), error, file, nativeFilePath, File,
    acceptFile: () => {file.value = null; error.value = '仅支持 Excel'},
  })
  handle({detail: {phase: 'drop', paths: ['C:\\bad.exe'], files: []}})
  assert.equal(nativeFilePath.value, null)
  handle({detail: {phase: 'drop', paths: ['a.xlsx', 'b.xlsx'], files: [new File([], 'other.xlsx')]}})
  assert.match(error.value, /一次拖入一个/)
})

test('mold featured image survives refresh and falls back after deletion with valid thumbnail page', () => {
  const mainImageID = ref(50), thumbnailPage = ref(32), images = ref(Array.from({length: 100}, (_,i) => ({id: i + 1})))
  const preserve = method('../src/components/ImageGallery.vue', 'preserveMainImage', {mainImageID, thumbnailPage})
  const choose = method('../src/components/ImageGallery.vue', 'chooseMainImage', {mainImageID, images})
  preserve(images.value)
  assert.equal(mainImageID.value, 50)
  assert.equal(thumbnailPage.value, 32)
  choose(100); assert.equal(mainImageID.value, 100)
  choose(999); assert.equal(mainImageID.value, 100)
  preserve([{id: 2}, {id: 3}, {id: 4}, {id: 5}])
  assert.equal(mainImageID.value, 2)
  assert.equal(thumbnailPage.value, 0)
  preserve([])
  assert.equal(mainImageID.value, null)
  assert.equal(thumbnailPage.value, 0)
})
