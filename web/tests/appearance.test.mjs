import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'
import {
  accentStorageKey,
  applyAppearance,
  nextSidebarMode,
  normalizeAccentTheme,
  normalizeSidebarMode,
  normalizeThemeMode,
  setAppearance,
  themeStorageKey,
} from '../src/platform/appearance.ts'

test('外观设置只接受受控主题值并默认使用博邦亮色', () => {
  assert.equal(normalizeThemeMode('dark'), 'dark')
  assert.equal(normalizeThemeMode('system'), 'light')
  assert.equal(normalizeAccentTheme('teal'), 'teal')
  assert.equal(normalizeAccentTheme('#ff0000'), 'bobbang')
  assert.equal(normalizeSidebarMode('icon'), 'icon')
  assert.equal(normalizeSidebarMode('wide'), 'full')
})

test('保存外观时同步更新根元素和本地存储', () => {
  const dataset = {}
  const values = new Map()
  setAppearance({dataset}, {setItem: (key, value) => values.set(key, value)}, 'dark', 'teal')
  assert.deepEqual(dataset, {theme: 'dark', accent: 'teal'})
  assert.equal(values.get(themeStorageKey), 'dark')
  assert.equal(values.get(accentStorageKey), 'teal')
})

test('应用启动前从本地存储恢复主题到根元素', () => {
  const dataset = {}
  const values = new Map([[themeStorageKey, 'dark'], [accentStorageKey, 'violet']])
  const result = applyAppearance({dataset}, {getItem: (key) => values.get(key) ?? null})

  assert.deepEqual(result, {theme: 'dark', accent: 'violet'})
  assert.deepEqual(dataset, {theme: 'dark', accent: 'violet'})
})

test('左侧导航按完整、图标、隐藏顺序循环', () => {
  assert.equal(nextSidebarMode('full'), 'icon')
  assert.equal(nextSidebarMode('icon'), 'hidden')
  assert.equal(nextSidebarMode('hidden'), 'full')
})

test('暗色三品牌的文字、选中态和焦点令牌达到对比度门槛', () => {
  const css = readFileSync(new URL('../src/design-system.css', import.meta.url), 'utf8')
  const selectors = [
    ':root[data-theme="dark"]',
    ':root[data-theme="dark"][data-accent="teal"]',
    ':root[data-theme="dark"][data-accent="violet"]',
  ]
  for (const selector of selectors) {
    const start = css.indexOf(`${selector} {`)
    assert.notEqual(start, -1, `missing ${selector}`)
    const block = css.slice(start, css.indexOf('\n}', start) + 2)
    const text = tokenHex(block, '--bb-accent-text')
    const selectedBackground = tokenHex(block, '--bb-accent-selected-bg')
    const selectedText = tokenHex(block, '--bb-accent-selected-text')
    const focus = tokenHex(block, '--bb-focus-color')
    assert.ok(contrast(text, '#272a2e') >= 4.5, `${selector} accent text contrast`)
    assert.ok(contrast(selectedText, selectedBackground) >= 4.5, `${selector} selected contrast`)
    assert.ok(contrast(focus, '#202225') >= 3, `${selector} focus contrast`)
  }
})

test('暗色三品牌覆盖 Element primary 层级且 plain 按钮保持清晰', () => {
  const css = readFileSync(new URL('../src/design-system.css', import.meta.url), 'utf8')
  const elementTheme = readFileSync(new URL('../src/styles/element-theme.css', import.meta.url), 'utf8')
  const cases = [
    [':root[data-theme="dark"]', '#7cc7ff'],
    [':root[data-theme="dark"][data-accent="teal"]', '#73e3c9'],
    [':root[data-theme="dark"][data-accent="violet"]', '#cbb4ff'],
  ]
  for (const [selector, plainText] of cases) {
    const start = css.indexOf(`${selector} {`)
    assert.notEqual(start, -1, `missing ${selector}`)
    const block = css.slice(start, css.indexOf('\n}', start) + 2)
    for (const suffix of ['3', '5', '7', '8', '9']) tokenHex(block, `--el-color-primary-light-${suffix}`)
    tokenHex(block, '--el-color-primary-dark-2')
    assert.ok(contrast(plainText, tokenHex(block, '--el-color-primary-light-9')) >= 4.5, `${selector} plain button contrast`)
    const darkStart = css.indexOf(':root[data-theme="dark"] {')
    const darkBlock = css.slice(darkStart, css.indexOf('\n}', darkStart) + 2)
    const onBrand = tokenHex(darkBlock, '--bb-text-on-brand')
    assert.ok(contrast(onBrand, tokenHex(block, '--el-color-primary-light-3')) >= 4.5, `${selector} primary hover contrast`)
  }
  assert.match(elementTheme, /\.el-button--primary\.is-plain/)
  assert.match(elementTheme, /--el-button-text-color: var\(--bb-accent-text\)/)
})

function tokenHex(block, name) {
  const match = block.match(new RegExp(`${name}:\\s*(#[0-9a-f]{6})`, 'i'))
  assert.ok(match, `missing ${name}`)
  return match[1]
}

function contrast(left, right) {
  const lighter = Math.max(luminance(left), luminance(right))
  const darker = Math.min(luminance(left), luminance(right))
  return (lighter + 0.05) / (darker + 0.05)
}

function luminance(value) {
  const channels = value.slice(1).match(/.{2}/g).map((part) => Number.parseInt(part, 16) / 255)
  const linear = channels.map((channel) => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4)
  return 0.2126 * linear[0] + 0.7152 * linear[1] + 0.0722 * linear[2]
}
