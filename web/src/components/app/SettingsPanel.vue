<template>
  <el-drawer
    v-model="visible"
    class="settings-drawer"
    title="设置"
    size="min(500px, 100%)"
    destroy-on-close
  >
    <div class="settings-content">
      <section class="settings-section" aria-labelledby="appearance-title">
        <div class="settings-section__heading">
          <div>
            <span>个性化</span>
            <h2 id="appearance-title">外观</h2>
          </div>
          <small>只保存在当前设备</small>
        </div>

        <div class="setting-field">
          <div><strong>显示模式</strong><small>暗色使用低对比深灰表面，适合长时间查看。</small></div>
          <el-radio-group v-model="theme" aria-label="显示模式" @change="saveAppearance">
            <el-radio-button value="light">亮色</el-radio-button>
            <el-radio-button value="dark">暗色</el-radio-button>
          </el-radio-group>
        </div>

        <div class="setting-field is-stacked">
          <div><strong>主题颜色</strong><small>业务状态色保持固定，仅切换品牌强调色。</small></div>
          <div class="accent-options" role="radiogroup" aria-label="主题颜色">
            <button
              v-for="option in accentOptions"
              :key="option.value"
              :ref="(element) => setAccentOptionRef(option.value, element)"
              type="button"
              role="radio"
              :aria-checked="accent === option.value"
              :tabindex="accent === option.value ? 0 : -1"
              :class="['accent-option', `is-${option.value}`, {active: accent === option.value}]"
              @click="selectAccent(option.value)"
              @keydown.left.prevent="moveAccentFocus(option.value, -1)"
              @keydown.up.prevent="moveAccentFocus(option.value, -1)"
              @keydown.right.prevent="moveAccentFocus(option.value, 1)"
              @keydown.down.prevent="moveAccentFocus(option.value, 1)"
              @keydown.home.prevent="focusAccentAt(0)"
              @keydown.end.prevent="focusAccentAt(accentOptions.length - 1)"
            >
              <span aria-hidden="true"></span>
              <strong>{{ option.label }}</strong>
              <small>{{ option.description }}</small>
            </button>
          </div>
        </div>
      </section>

      <section class="settings-section" aria-labelledby="connection-title">
        <div class="settings-section__heading">
          <div>
            <span>客户端</span>
            <h2 id="connection-title">连接与服务</h2>
          </div>
          <StatusTag :label="healthStatusLabel" :tone="connectionTone" />
        </div>

        <dl class="connection-details">
          <div><dt>服务器</dt><dd>{{ currentServer?.server_name || 'ERP 服务器' }}</dd></div>
          <div><dt>地址</dt><dd class="is-mono">{{ currentServer?.origin || '当前站点' }}</dd></div>
          <div><dt>服务端版本</dt><dd>{{ currentServer?.server_version || '未提供' }}</dd></div>
          <div><dt>最近检查</dt><dd>{{ lastCheckText }}</dd></div>
        </dl>

        <el-alert
          v-if="healthStatus === 'error'"
          title="服务暂不可用"
          description="请先检查网络和服务器状态；切换服务器会退出当前登录。"
          type="error"
          :closable="false"
          show-icon
        />
        <div class="connection-actions">
          <el-button :loading="healthStatus === 'checking'" @click="loadHealth">重新检查</el-button>
          <el-button v-if="canChangeServer" type="primary" plain @click="requestServerChange">切换服务器</el-button>
        </div>
        <p v-if="!canChangeServer" class="settings-note">Web 版使用当前站点配置的服务，无需在客户端切换地址。</p>
      </section>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import {computed, nextTick, ref, watch, type ComponentPublicInstance} from 'vue'
import {
  accentStorageKey,
  normalizeAccentTheme,
  normalizeThemeMode,
  setAppearance,
  themeStorageKey,
  type AccentTheme,
  type ThemeMode,
} from '../../platform/appearance'
import {useStartupConnectionContext, useWorkspaceContext} from '../../composables/workspaceContext'
import StatusTag from '../ui/StatusTag.vue'

const props = defineProps<{modelValue: boolean}>()
const emit = defineEmits<{(event: 'update:modelValue', value: boolean): void}>()
const visible = computed({get: () => props.modelValue, set: (value) => emit('update:modelValue', value)})

const {healthStatus, healthStatusLabel, lastHealthCheckAt, loadHealth, formatDate} = useWorkspaceContext()
const {canChangeServer, changeServer, currentServer} = useStartupConnectionContext()
const theme = ref<ThemeMode>(normalizeThemeMode(localStorage.getItem(themeStorageKey)))
const accent = ref<AccentTheme>(normalizeAccentTheme(localStorage.getItem(accentStorageKey)))
const accentOptionRefs = new Map<AccentTheme, HTMLButtonElement>()
const accentOptions: {value: AccentTheme; label: string; description: string}[] = [
  {value: 'bobbang', label: '博邦蓝', description: '默认品牌色'},
  {value: 'teal', label: '青绿色', description: '柔和清晰'},
  {value: 'violet', label: '紫色', description: '稳重醒目'},
]
const connectionTone = computed(() => healthStatus.value === 'healthy' ? 'success' : healthStatus.value === 'checking' ? 'info' : 'danger')
const lastCheckText = computed(() => lastHealthCheckAt.value ? formatDate(lastHealthCheckAt.value) : '本次会话尚未检查')

watch(() => props.modelValue, (open) => {
  if (!open) return
  theme.value = normalizeThemeMode(localStorage.getItem(themeStorageKey))
  accent.value = normalizeAccentTheme(localStorage.getItem(accentStorageKey))
})

function saveAppearance() {
  setAppearance(document.documentElement, localStorage, theme.value, accent.value)
}

function selectAccent(value: AccentTheme) {
  accent.value = value
  saveAppearance()
}

function setAccentOptionRef(value: AccentTheme, element: Element | ComponentPublicInstance | null) {
  if (element instanceof HTMLButtonElement) accentOptionRefs.set(value, element)
  else accentOptionRefs.delete(value)
}

function focusAccentAt(index: number) {
  const option = accentOptions[index]
  if (!option) return
  selectAccent(option.value)
  void nextTick(() => accentOptionRefs.get(option.value)?.focus())
}

function moveAccentFocus(value: AccentTheme, direction: -1 | 1) {
  const current = accentOptions.findIndex((option) => option.value === value)
  focusAccentAt((current + direction + accentOptions.length) % accentOptions.length)
}

async function requestServerChange() {
  visible.value = false
  await changeServer()
}
</script>

<style scoped>
.settings-content { display: grid; width: 100%; min-width: 0; gap: 0; padding-bottom: var(--bb-space-8); }
.settings-section { display: grid; gap: var(--bb-space-5); padding: 0 0 var(--bb-space-5); }
.settings-section + .settings-section { border-top: 1px solid var(--bb-border-default); padding-top: var(--bb-space-5); }
.settings-section__heading { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--bb-space-3); }
.settings-section__heading > div { display: grid; min-width: 0; gap: var(--bb-space-1); }
.settings-section__heading span { color: var(--bb-accent-text); font-size: var(--bb-font-size-12); font-weight: var(--bb-font-weight-bold); letter-spacing: .08em; }
.settings-section__heading h2 { margin: 0; font-size: var(--bb-font-size-20); }
.settings-section__heading > small { color: var(--bb-text-secondary); }
.setting-field { display: flex; min-width: 0; align-items: center; justify-content: space-between; gap: var(--bb-space-4); }
.setting-field.is-stacked { display: grid; }
.setting-field > div:first-child { display: grid; gap: var(--bb-space-1); }
.setting-field small, .settings-note { color: var(--bb-text-secondary); line-height: var(--bb-line-height-base); }
.accent-options { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--bb-space-2); }
.accent-option { display: grid; min-height: 76px; gap: var(--bb-space-1); border: 1px solid var(--bb-border-default); border-radius: var(--bb-radius-sm); background: var(--bb-bg-surface); padding: var(--bb-space-3); color: var(--bb-text-primary); text-align: left; }
.accent-option:hover { border-color: var(--bb-border-strong); }
.accent-option.active { border-color: var(--bb-border-strong); background: var(--bb-bg-subtle); box-shadow: inset 2px 0 0 var(--bb-action-primary); }
.accent-option:focus-visible { outline: 2px solid var(--bb-focus-color); outline-offset: 2px; }
.accent-option > span { width: 24px; height: 24px; border-radius: 50%; background: #0758ad; }
.accent-option.is-teal > span { background: #0e675b; }
.accent-option.is-violet > span { background: #552d98; }
.accent-option small { font-size: var(--bb-font-size-12); }
.connection-details { display: grid; gap: var(--bb-space-3); margin: 0; }
.connection-details > div { display: grid; grid-template-columns: 104px minmax(0, 1fr); gap: var(--bb-space-3); border-bottom: 1px solid var(--bb-border-subtle); padding-bottom: var(--bb-space-3); }
.connection-details dt { color: var(--bb-text-secondary); }
.connection-details dd { min-width: 0; margin: 0; overflow-wrap: anywhere; color: var(--bb-text-primary); }
.connection-details .is-mono { font-family: var(--bb-font-mono); font-size: var(--bb-font-size-13); }
.connection-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: var(--bb-space-2); }
.settings-note { margin: 0; font-size: var(--bb-font-size-12); }
@media (max-width: 520px) {
  .setting-field { display: grid; }
  .accent-options { grid-template-columns: 1fr; }
}
</style>
