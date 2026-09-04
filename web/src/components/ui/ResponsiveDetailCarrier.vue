<template>
  <Teleport v-if="renderDocked" defer to="#workspace-detail-host">
    <Transition name="workspace-detail-panel" appear @after-leave="handleDockedAfterLeave">
      <aside ref="dockedAside" v-if="dockedContentVisible" :class="['workspace-detail-aside', drawerClass]" :aria-label="title" tabindex="-1">
        <header v-if="withHeader" class="workspace-detail-aside__header">
          <h2>{{ title }}</h2>
          <el-button circle :aria-label="`关闭${title}`" @click="requestClose"><el-icon><Close /></el-icon></el-button>
        </header>
        <slot />
      </aside>
    </Transition>
  </Teleport>
  <el-drawer
    v-else
    v-model="visible"
    :class="drawerClass"
    :size="size"
    :title="title"
    :with-header="withHeader"
    :close-on-click-modal="closeOnClickModal"
    :close-on-press-escape="closeOnPressEscape"
    :before-close="beforeClose"
    :destroy-on-close="destroyOnClose"
    @closed="emit('closed')"
  >
    <slot />
  </el-drawer>
</template>

<script setup lang="ts">
import {computed, nextTick, onBeforeUnmount, onMounted, ref, watch} from 'vue'
import {Close} from '@element-plus/icons-vue'
import {shouldRequestDockedDetailClose} from '../../platform/detailPanel'

const props = withDefaults(defineProps<{
  modelValue: boolean
  docked: boolean
  size: string
  title: string
  drawerClass?: string
  withHeader?: boolean
  closeOnClickModal?: boolean
  closeOnPressEscape?: boolean
  destroyOnClose?: boolean
  beforeClose?: (done: () => void) => void
  dockedAutoFocus?: 'preserve' | 'panel' | 'first-editable'
}>(), {
  drawerClass: 'workspace-detail-drawer',
  withHeader: true,
  closeOnClickModal: true,
  closeOnPressEscape: true,
  destroyOnClose: true,
  beforeClose: undefined,
  dockedAutoFocus: 'preserve',
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: boolean): void
  (event: 'closed'): void
}>()
const visible = computed({get: () => props.modelValue, set: (value) => emit('update:modelValue', value)})
const renderDocked = ref(props.modelValue && props.docked)
const dockedContentVisible = ref(props.modelValue && props.docked)
const dockedClosePending = ref(false)
const dockedAside = ref<HTMLElement | null>(null)
const returnFocus = ref<HTMLElement | null>(null)

watch(() => [props.modelValue, props.docked] as const, ([open, docked], [wasOpen, wasDocked]) => {
  if (open && !wasOpen && document.activeElement instanceof HTMLElement) returnFocus.value = document.activeElement
  if (open && docked) {
    const enteringDock = !wasOpen || !wasDocked
    dockedClosePending.value = false
    renderDocked.value = true
    dockedContentVisible.value = true
    if (enteringDock) void nextTick(focusDockedEntry)
    return
  }
  if (!open && wasOpen && wasDocked) {
    dockedClosePending.value = true
    dockedContentVisible.value = false
    return
  }
  dockedClosePending.value = false
  dockedContentVisible.value = false
  renderDocked.value = false
})

watch(() => props.dockedAutoFocus, async (policy, previousPolicy) => {
  if (policy !== 'first-editable' || policy === previousPolicy || !props.modelValue || !props.docked) return
  await nextTick()
  focusDockedEntry()
}, {flush: 'post'})

function requestClose() {
  const done = () => { visible.value = false }
  if (props.beforeClose) props.beforeClose(done)
  else done()
}

function handleDockedAfterLeave() {
  renderDocked.value = false
  if (!dockedClosePending.value) return
  dockedClosePending.value = false
  emit('closed')
  const target = returnFocus.value
  returnFocus.value = null
  if (target?.isConnected) void nextTick(() => target.focus({preventScroll: true}))
}

function focusDockedEntry() {
  if (props.dockedAutoFocus === 'preserve') return
  const panel = dockedAside.value
  if (!panel) return
  if (props.dockedAutoFocus === 'first-editable') {
    const editable = panel.querySelector<HTMLElement>([
      'input:not([disabled]):not([readonly]):not([type="hidden"])',
      'textarea:not([disabled]):not([readonly])',
      'select:not([disabled])',
      '[contenteditable="true"]',
    ].join(','))
    if (editable) {
      editable.focus({preventScroll: true})
      return
    }
  }
  panel.focus({preventScroll: true})
}

function hasVisibleFloatingLayer() {
  return [...document.querySelectorAll<HTMLElement>('.el-overlay, .el-popper')].some((element) => {
    const style = window.getComputedStyle(element)
    return style.display !== 'none' && style.visibility !== 'hidden' && style.pointerEvents !== 'none'
  })
}

function handleDockedEscape(event: KeyboardEvent) {
  if (!shouldRequestDockedDetailClose({
    key: event.key,
    defaultPrevented: event.defaultPrevented,
    visible: props.modelValue,
    docked: props.docked,
    escapeEnabled: props.closeOnPressEscape,
    blockedByFloatingLayer: hasVisibleFloatingLayer(),
  })) return
  event.preventDefault()
  requestClose()
}

onMounted(() => {
  document.addEventListener('keydown', handleDockedEscape)
  if (props.modelValue && props.docked) void nextTick(focusDockedEntry)
})
onBeforeUnmount(() => document.removeEventListener('keydown', handleDockedEscape))
</script>

<style scoped>
.workspace-detail-aside {
  width: var(--bb-detail-panel-width);
  min-width: var(--bb-detail-panel-width);
  height: 100%;
  overflow: auto;
  background: var(--bb-bg-elevated);
  padding: var(--bb-space-6);
  scrollbar-gutter: stable;
}
.workspace-detail-aside__header { display: flex; min-height: 44px; align-items: center; justify-content: space-between; gap: var(--bb-space-3); margin-bottom: var(--bb-space-5); border-bottom: 1px solid var(--bb-border-subtle); padding-bottom: var(--bb-space-4); }
.workspace-detail-aside__header h2 { margin: 0; color: var(--bb-text-primary); font-size: var(--bb-font-size-16); font-weight: var(--bb-font-weight-bold); }
.workspace-detail-aside__header :deep(.el-button) { flex: 0 0 auto; }
.workspace-detail-panel-enter-active,
.workspace-detail-panel-leave-active { transition: opacity var(--bb-duration-base) var(--bb-ease-standard), transform var(--bb-duration-base) var(--bb-ease-standard); }
.workspace-detail-panel-enter-from { opacity: .92; transform: translateX(8px); }
.workspace-detail-panel-leave-to { opacity: .94; transform: translateX(6px); }
</style>
