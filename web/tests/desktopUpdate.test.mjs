import assert from 'node:assert/strict'
import test from 'node:test'
import {readFileSync} from 'node:fs'

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8')

test('客户端更新入口收敛到登录页、设置和低打扰 Dashboard 条', () => {
  const modules = read('../src/data/modules.ts')
  const login = read('../src/components/app/LoginScreen.vue')
  const settings = read('../src/components/app/SettingsPanel.vue')
  const dashboard = read('../src/components/pages/DashboardPage.vue')
  const frame = read('../src/components/pages/module/ModulePageFrame.vue')

  assert.doesNotMatch(modules, /key: 'updates'/)
  assert.match(login, /checkOnConnection/)
  assert.match(login, /检查更新/)
  assert.match(settings, /DesktopUpdatePanel/)
  assert.match(settings, /v-if="desktopClient"/)
  assert.match(dashboard, /DesktopUpdatePanel v-if="desktopClient" compact/)
  assert.doesNotMatch(frame, /UpdateCenter/)
  assert.doesNotMatch(frame, /activeKey === 'updates'/)
})

test('更新面板只显示单 EXE 状态，并保留真实进度和首帧焦点入口', () => {
  const panel = read('../src/components/DesktopUpdatePanel.vue')
  const composable = read('../src/composables/useDesktopUpdate.ts')
  const types = read('../src/types.ts')
  const styles = read('../src/styles/feature-patterns.css')

  assert.match(panel, /desktopAvailable &&/)
  assert.match(panel, /Windows x64 · 单 EXE/)
  assert.match(panel, /state\.value === 'RolledBack'/)
  assert.match(panel, /ref="laterButton"/)
  assert.match(panel, /downloadPercent !== null/)
  assert.doesNotMatch(panel, /initial:.*opacity/)
  assert.match(composable, /checkOnConnection/)
  assert.match(composable, /server_unreachable/)
  assert.match(composable, /directory_not_writable/)
  assert.match(types, /DesktopUpdateErrorCode/)
  assert.match(types, /Updated/)
  assert.match(types, /RolledBack/)
  assert.doesNotMatch(styles, /\.update-center/)
})
