import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'
import ts from 'typescript'

const source = readFileSync(new URL('../../client/src/native-file-drop.ts', import.meta.url), 'utf8')
const javascript = ts.transpileModule(source.replace(/^import .*$/gm, '').replace('void installNativeFileDrop()', ''), {
  compilerOptions: {target: ts.ScriptTarget.ES2022},
}).outputText

function harness(ratio = 1) {
  const notifications = [], points = [], events = []
  let element = null
  const target = name => ({isConnected: true, dispatchEvent: event => events.push([name, event.detail.phase, event.detail.paths])})
  const document = {elementFromPoint(x, y) { points.push([x, y]); return element }}
  const window = {devicePixelRatio: ratio, addEventListener() {}}
  const api = new Function('window', 'document', 'CustomEvent', 'ElMessage', 'getCurrentWebview', 'console', `${javascript}; return {handleNativeDrag, installNativeFileDrop}`)(
    window, document, class {constructor(name, options) {Object.assign(this, options)}},
    {warning: value => notifications.push(value), error: value => notifications.push(value)},
    () => ({onDragDropEvent: async () => {throw new Error('registration denied')}}), {error() {}},
  )
  return {...api, events, notifications, points, target, hit(value) {element = value ? {closest: () => value} : null}, drag(type, paths = [], position = {x: 150, y: 75}) { api.handleNativeDrag({payload: {type, paths, position}}) }}
}

test('native drop targets SVG descendants at 100%, 125%, 150% scale', () => {
  for (const scale of [1, 1.25, 1.5]) {
    const h = harness(scale), gallery = h.target('images')
    h.hit(gallery)
    h.drag('enter')
    h.drag('drop', ['C:\\模具 图片\\零件.png'])
    assert.deepEqual(h.points[0], [150 / scale, 75 / scale])
    assert.deepEqual(h.events.at(-1), ['images', 'drop', ['C:\\模具 图片\\零件.png']])
    assert.equal(h.events.filter(item => item[1] === 'drop').length, 1)
  }
})

test('crossing targets and leaving the dropzone clears the prior highlight', () => {
  const h = harness(), a = h.target('images'), b = h.target('drawings')
  h.hit(a); h.drag('enter')
  h.hit(b); h.drag('over')
  h.hit(null); h.drag('over'); h.drag('drop', ['C:\\test.dwg'])
  assert.deepEqual(h.events.map(item => item.slice(0, 2)), [['images', 'enter'], ['images', 'leave'], ['drawings', 'over'], ['drawings', 'leave']])
  assert.equal(h.notifications.length, 1)
})

test('removed targets never receive a positionless drop', () => {
  const h = harness(), a = h.target('closed dialog')
  h.hit(a); h.drag('enter')
  a.isConnected = false
  h.handleNativeDrag({payload: {type: 'drop', paths: ['C:\\test.zip']}})
  assert.equal(h.events.some(item => item[1] === 'drop'), false)
})

test('listener registration failure has a visible file-picker fallback', async () => {
  const h = harness()
  await h.installNativeFileDrop()
  assert.match(h.notifications[0].message, /点击选择文件/)
})
