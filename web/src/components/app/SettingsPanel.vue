<template>
  <ResponsiveDetailCarrier
    v-model="visible"
    drawer-class="settings-drawer workspace-detail-drawer"
    title="设置"
    :docked="settingsPanel.docked.value"
    :size="settingsPanel.size.value"
    docked-auto-focus="panel"
    destroy-on-close
  >
    <FormPanelContent class="settings-content">
      <FormSection title="外观" description="仅保存在此设备。">
        <div class="setting-field">
          <div><strong>显示模式</strong></div>
          <el-radio-group v-model="theme" class="theme-mode-switch" aria-label="显示模式" @change="saveAppearance">
            <el-radio-button value="light">亮色</el-radio-button>
            <el-radio-button value="dark">暗色</el-radio-button>
          </el-radio-group>
        </div>

        <div class="setting-field is-stacked">
          <div><strong>主题颜色</strong></div>
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
              <small v-if="accent === option.value" aria-hidden="true">✓</small>
            </button>
          </div>
        </div>
      </FormSection>

      <FormSection title="连接与服务">
        <div class="settings-section__heading">
          <StatusTag :label="healthStatusLabel" :tone="connectionTone" />
        </div>
        <dl class="connection-details" aria-label="连接与服务事实">
          <div v-if="currentServer?.server_name"><dt>服务器</dt><dd>{{ currentServer.server_name }}</dd></div>
          <div v-if="currentServer?.origin"><dt>地址</dt><dd class="is-mono">{{ currentServer.origin }}</dd></div>
          <div v-if="currentServer?.server_version"><dt>服务端版本</dt><dd>{{ currentServer.server_version }}</dd></div>
          <div><dt>最近检查</dt><dd>{{ lastCheckText }}</dd></div>
        </dl>

        <el-alert
          v-if="healthStatus === 'error'"
          title="服务暂不可用"
          description="请检查网络后重试；仍无法连接时，请联系管理员。"
          type="error"
          :closable="false"
          show-icon
        />
        <div class="connection-actions" aria-label="连接操作">
          <el-button :loading="healthStatus === 'checking'" @click="loadHealth">重新检查</el-button>
          <el-button v-if="canChangeServer" type="primary" plain @click="requestServerChange">切换服务器</el-button>
        </div>
      </FormSection>
    </FormPanelContent>
  </ResponsiveDetailCarrier>
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
import ResponsiveDetailCarrier from '../ui/ResponsiveDetailCarrier.vue'
import {useResponsiveDetailPanel} from '../../composables/useResponsiveDetailPanel'
import FormPanelContent from '../ui/FormPanelContent.vue'
import FormSection from '../ui/FormSection.vue'

const props = defineProps<{modelValue: boolean}>()
const emit = defineEmits<{(event: 'update:modelValue', value: boolean): void}>()
const visible = computed({get: () => props.modelValue, set: (value) => emit('update:modelValue', value)})
const settingsPanel = useResponsiveDetailPanel(visible, {complexity: 'standard-form'})

const {healthStatus, healthStatusLabel, lastHealthCheckAt, loadHealth, formatDate} = useWorkspaceContext()
const {canChangeServer, changeServer, currentServer} = useStartupConnectionContext()
const theme = ref<ThemeMode>(normalizeThemeMode(localStorage.getItem(themeStorageKey)))
const accent = ref<AccentTheme>(normalizeAccentTheme(localStorage.getItem(accentStorageKey)))
const accentOptionRefs = new Map<AccentTheme, HTMLButtonElement>()
const accentOptions: {value: AccentTheme; label: string; description: string}[] = [
  {value: 'bobbang', label: '博邦蓝', description: '默认品牌色'},
  {value: 'teal', label: '青绿色', description: ''},
  {value: 'violet', label: '紫色', description: ''},
]
const connectionTone = computed(() => healthStatus.value === 'healthy' ? 'success' : healthStatus.value === 'checking' ? 'info' : 'danger')
const lastCheckText = computed(() => lastHealthCheckAt.value ? formatDate(lastHealthCheckAt.value) : '尚未检查')

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
.settings-content { gap: var(--bb-space-8); padding-bottom: var(--bb-space-6); }
.settings-section__heading { display: flex; justify-content: flex-end; }
.setting-field { display: flex; min-width: 0; align-items: center; justify-content: space-between; gap: var(--bb-space-4); }
.setting-field.is-stacked { display: grid; }
.setting-field > div:first-child { display: grid; gap: var(--bb-space-1); }
.setting-field small, .settings-note { color: var(--bb-text-secondary); line-height: var(--bb-line-height-base); }
.theme-mode-switch {
  display: flex;
  width: 160px;
  flex: 0 0 160px;
  --el-radio-button-checked-bg-color: var(--bb-bg-elevated);
  --el-radio-button-checked-border-color: var(--bb-border-strong);
  --el-radio-button-checked-text-color: var(--bb-accent-text);
}
.theme-mode-switch :deep(.el-radio-button) { flex: 1 1 50%; }
.theme-mode-switch :deep(.el-radio-button__inner) { width: 100%; padding-inline: 8px; }
.theme-mode-switch :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
  border-color: var(--bb-border-strong);
  background: var(--bb-bg-elevated);
  box-shadow: -1px 0 0 0 var(--bb-border-strong);
  color: var(--bb-accent-text);
}
.accent-options { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--bb-space-2); }
.accent-option { display: grid; min-height: 64px; grid-template-columns: 16px minmax(0, 1fr) auto; align-items: center; gap: 8px; border: 1px solid var(--bb-border-default); border-radius: 6px; background: var(--bb-bg-surface); padding: 12px; color: var(--bb-text-primary); text-align: left; cursor: pointer; transition: background-color 120ms, border-color 120ms; }
.accent-option:hover { border-color: var(--bb-border-strong); }
.accent-option.active { border-color: var(--bb-border-strong); background: var(--bb-bg-subtle); box-shadow: inset 2px 0 0 var(--bb-action-primary); }
.accent-option:focus-visible { outline: 2px solid var(--bb-focus-color); outline-offset: 2px; }
.accent-option > span { width: 16px; height: 16px; border-radius: 50%; background: var(--bb-accent-swatch-bobbang); }
.accent-option.is-teal > span { background: var(--bb-accent-swatch-teal); }
.accent-option.is-violet > span { background: var(--bb-accent-swatch-violet); }
.accent-option small { margin-left: auto; font-size: 12px; }
.connection-details { display: grid; gap: var(--bb-space-3); margin: 0; }
.connection-details > div { display: grid; grid-template-columns: 104px minmax(0, 1fr); gap: var(--bb-space-3); border-bottom: 1px solid var(--bb-border-subtle); padding-bottom: var(--bb-space-3); }
.connection-details dt { color: var(--bb-text-secondary); }
.connection-details dd { min-width: 0; margin: 0; overflow-wrap: anywhere; color: var(--bb-text-primary); }
.connection-details .is-mono { font-family: var(--bb-font-mono); font-size: var(--bb-font-size-13); }
.connection-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: var(--bb-space-2); }
.settings-note { margin: 0; font-size: var(--bb-font-size-12); }
@media (max-width: 520px) {
  .setting-field { display: grid; }
  .theme-mode-switch { width: 160px; }
  .accent-options { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}
</style>
