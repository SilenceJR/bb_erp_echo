<template>
  <div class="notification-center">
    <el-popover
      :visible="panelOpen"
      trigger="click"
      placement="bottom-end"
      :width="360"
      :show-arrow="false"
      :teleported="true"
      popper-class="bb-notification-popover"
      @update:visible="handlePopoverVisibility"
      @show="markAllRead"
    >
      <template #reference>
        <button
          ref="notificationTrigger"
          class="notification-trigger"
          type="button"
          :aria-label="triggerLabel"
          aria-haspopup="dialog"
          aria-controls="notification-panel"
          :aria-expanded="panelOpen"
        >
          <el-icon aria-hidden="true"><Bell /></el-icon>
          <span v-if="unreadCount" class="notification-badge" aria-live="polite">{{ badgeText }}</span>
        </button>
      </template>

      <section
        id="notification-panel"
        ref="notificationPanel"
        class="notification-panel"
        role="dialog"
        aria-label="实时通知"
        tabindex="-1"
        @keydown="handlePanelKeydown"
      >
        <header class="notification-panel__header">
          <div>
            <strong>实时通知</strong>
            <span v-if="notifications.length">最近 {{ notifications.length }} 条</span>
          </div>
          <el-button v-if="notifications.length" link type="primary" @click="clearAll">全部清除</el-button>
        </header>
        <div v-if="notifications.length" class="notification-list">
          <article v-for="item in notifications" :key="item.id" class="notification-list-item" :class="{'is-unread': !item.read}">
            <button class="notification-list-item__body" type="button" @click="void openNotification(item)">
              <span class="notification-list-item__title"><i aria-hidden="true"></i>{{ item.title }}</span>
              <span class="notification-list-item__summary">{{ item.summary }}</span>
              <span class="notification-list-item__meta">{{ formatTime(item.occurredAt) }} · {{ item.count }} 条</span>
            </button>
            <button class="notification-list-item__close" type="button" aria-label="关闭此通知" @click="dismiss(item.id)">
              <el-icon aria-hidden="true"><Close /></el-icon>
            </button>
          </article>
        </div>
        <div v-else class="notification-empty">
          <el-icon aria-hidden="true"><Bell /></el-icon>
          <strong>暂时没有新通知</strong>
          <span>其他用户的新增或修改会显示在这里。</span>
        </div>
      </section>
    </el-popover>

    <Transition name="notification-toast">
      <article
        v-if="visibleToast"
        class="notification-toast"
        role="status"
        tabindex="0"
        aria-live="polite"
        @mouseenter="pauseToast"
        @mouseleave="resumeToast"
        @focusin="pauseToast"
        @focusout="resumeToast"
        @keydown.esc.stop.prevent="dismiss(visibleToast.id)"
      >
        <div class="notification-toast__topline">
          <span class="notification-toast__label"><i aria-hidden="true"></i>数据更新</span>
          <button type="button" aria-label="关闭通知提醒" @click="dismiss(visibleToast.id)">
            <el-icon aria-hidden="true"><Close /></el-icon>
          </button>
        </div>
        <strong class="notification-toast__title">{{ visibleToast.title }}</strong>
        <span class="notification-toast__summary">{{ visibleToast.summary }}</span>
        <div class="notification-toast__modules" aria-label="更新范围">
          <span v-for="module in visibleToast.modules.slice(0, 3)" :key="module.module">{{ module.title }} {{ module.count }} 条</span>
          <span v-if="visibleToast.modules.length > 3">另有 {{ visibleToast.modules.length - 3 }} 类更新</span>
        </div>
        <button class="notification-toast__view" type="button" @click="void openNotification(visibleToast)">
          查看更新 <el-icon aria-hidden="true"><ArrowRight /></el-icon>
        </button>
      </article>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import {computed, nextTick, onBeforeUnmount, ref, watch} from 'vue'
import {ArrowRight, Bell, Close} from '@element-plus/icons-vue'
import {useNotificationCenter} from '../../composables/workspaceContext'

const {
  notifications, visibleToast, panelOpen, unreadCount,
  markAllRead, closePanel, dismiss, clearAll, openNotification,
  pauseToast, resumeToast,
} = useNotificationCenter()

const badgeText = computed(() => unreadCount.value > 99 ? '99+' : String(unreadCount.value))
const triggerLabel = computed(() => panelOpen.value
  ? '关闭通知面板'
  : unreadCount.value
    ? `打开通知面板，${badgeText.value}条未读`
    : '打开通知面板')
const notificationTrigger = ref<HTMLElement | null>(null)
const notificationPanel = ref<HTMLElement | null>(null)
const panelFocusableSelector = [
  'button:not([disabled])',
  'a[href]',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[contenteditable="true"]',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

function panelFocusableElements(): HTMLElement[] {
  const panel = notificationPanel.value || document.getElementById('notification-panel')
  if (!panel) return []
  return [...panel.querySelectorAll<HTMLElement>(panelFocusableSelector)].filter((element) => {
    if (element.closest('[inert]') || element.getAttribute('aria-hidden') === 'true') return false
    const style = window.getComputedStyle(element)
    return style.display !== 'none' && style.visibility !== 'hidden' && element.getClientRects().length > 0
  })
}

async function focusPanel() {
  await nextTick()
  const panel = notificationPanel.value || document.getElementById('notification-panel')
  if (!panel) {
    if (panelOpen.value) window.setTimeout(() => void focusPanel(), 0)
    return
  }
  const first = panelFocusableElements()[0]
  if (first) first.focus({preventScroll: true})
  else panel.focus({preventScroll: true})
}

function restoreTriggerFocus() {
  void nextTick(() => notificationTrigger.value?.focus({preventScroll: true}))
}

function handlePanelKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    closePanel()
    restoreTriggerFocus()
    return
  }
  if (event.key !== 'Tab') return
  const focusable = panelFocusableElements()
  if (!focusable.length) {
    event.preventDefault()
    notificationPanel.value?.focus({preventScroll: true})
    return
  }
  const currentIndex = focusable.indexOf(document.activeElement as HTMLElement)
  if (event.shiftKey && (currentIndex <= 0 || currentIndex === -1)) {
    event.preventDefault()
    focusable[focusable.length - 1]?.focus({preventScroll: true})
  } else if (!event.shiftKey && currentIndex === focusable.length - 1) {
    event.preventDefault()
    focusable[0]?.focus({preventScroll: true})
  }
}

function handlePopoverVisibility(visible: boolean) {
  panelOpen.value = visible
  if (visible) markAllRead()
}

watch(panelOpen, (visible, wasVisible) => {
  if (visible) void focusPanel()
  else if (wasVisible && notificationPanel.value?.contains(document.activeElement)) restoreTriggerFocus()
})

onBeforeUnmount(() => {
  notificationPanel.value = null
  notificationTrigger.value = null
})

function formatTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '刚刚'
  return date.toLocaleTimeString('zh-CN', {hour: '2-digit', minute: '2-digit'})
}
</script>

<style>
.notification-center { position: relative; display: flex; flex: 0 0 auto; align-items: center; }
.notification-trigger { position: relative; display: grid; width: 40px; height: 40px; place-items: center; border: 1px solid transparent; border-radius: var(--bb-radius-sm); background: transparent; color: var(--bb-text-regular); transition: background var(--bb-duration-fast) var(--bb-ease-standard), border-color var(--bb-duration-fast) var(--bb-ease-standard), color var(--bb-duration-fast) var(--bb-ease-standard); }
.notification-trigger:hover { border-color: var(--bb-border-default); background: var(--bb-bg-subtle); color: var(--bb-accent-text); }
.notification-trigger:focus-visible { outline: 2px solid var(--bb-focus-color); outline-offset: 2px; }
.notification-trigger .el-icon { font-size: 18px; }
.notification-badge { position: absolute; top: 1px; right: -2px; min-width: 17px; height: 17px; border: 2px solid var(--bb-bg-elevated); border-radius: var(--bb-radius-pill); background: var(--bb-danger); padding: 0 4px; color: #fff; font-size: 10px; font-weight: var(--bb-font-weight-bold); line-height: 13px; text-align: center; }
.notification-panel { display: grid; width: 100%; max-height: 440px; grid-template-rows: auto minmax(0, 1fr); overflow: hidden; color: var(--bb-text-primary); }
.notification-panel__header { display: flex; min-height: 44px; align-items: center; justify-content: space-between; gap: var(--bb-space-3); border-bottom: 1px solid var(--bb-border-subtle); padding: 0 var(--bb-space-1) var(--bb-space-3); }
.notification-panel__header .el-button { min-height: 40px; }
.notification-panel__header > div { display: flex; min-width: 0; align-items: baseline; gap: var(--bb-space-2); }
.notification-panel__header strong { font-size: var(--bb-font-size-16); }
.notification-panel__header span { color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); }
.notification-list { min-height: 0; overflow-y: auto; padding-top: var(--bb-space-2); }
.notification-list-item { display: flex; min-width: 0; align-items: stretch; border-bottom: 1px solid var(--bb-border-subtle); }
.notification-list-item:last-child { border-bottom: 0; }
.notification-list-item__body { display: grid; min-width: 0; min-height: 40px; flex: 1 1 auto; gap: 3px; border: 0; background: transparent; padding: 11px 4px 11px 8px; color: inherit; text-align: left; }
.notification-list-item__body:hover { background: var(--bb-bg-subtle); }
.notification-list-item__body:focus-visible, .notification-list-item__close:focus-visible { z-index: 1; outline: 2px solid var(--bb-focus-color); outline-offset: -2px; }
.notification-list-item__title { display: flex; min-width: 0; align-items: center; gap: 6px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--bb-font-size-14); font-weight: var(--bb-font-weight-semibold); }
.notification-list-item__title i, .notification-toast__label i { width: 6px; height: 6px; flex: 0 0 auto; border-radius: 50%; background: var(--bb-border-strong); }
.notification-list-item.is-unread .notification-list-item__title i { background: var(--bb-accent-icon); }
.notification-list-item__summary { overflow: hidden; color: var(--bb-text-regular); text-overflow: ellipsis; white-space: nowrap; font-size: var(--bb-font-size-13); }
.notification-list-item__meta { color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); }
.notification-list-item__close { display: grid; width: 40px; min-height: 40px; flex: 0 0 auto; place-items: center; border: 0; background: transparent; color: var(--bb-text-secondary); }
.notification-list-item__close:hover { background: var(--bb-bg-subtle); color: var(--bb-danger); }
.notification-empty { display: grid; min-height: 180px; place-items: center; align-content: center; gap: var(--bb-space-2); padding: var(--bb-space-6); color: var(--bb-text-secondary); text-align: center; }
.notification-empty .el-icon { color: var(--bb-text-placeholder); font-size: 24px; }
.notification-empty strong { color: var(--bb-text-primary); font-size: var(--bb-font-size-14); }
.notification-empty span { font-size: var(--bb-font-size-12); line-height: var(--bb-line-height-base); }
.bb-notification-popover { border: 1px solid var(--bb-border-default) !important; border-radius: var(--bb-radius-lg) !important; background: var(--bb-bg-elevated) !important; box-shadow: var(--bb-shadow-md) !important; padding: var(--bb-space-4) !important; }
.notification-toast { position: fixed; z-index: var(--bb-z-toast); top: calc(var(--bb-shell-header-height) + 12px); right: 16px; display: grid; width: min(380px, calc(100vw - 32px)); gap: var(--bb-space-2); border: 1px solid var(--bb-border-default); border-left: 3px solid var(--bb-accent-icon); border-radius: var(--bb-radius-lg); background: var(--bb-bg-elevated); box-shadow: var(--bb-shadow-lg); padding: var(--bb-space-4); color: var(--bb-text-primary); }
.notification-toast:focus-visible { outline: 2px solid var(--bb-focus-color); outline-offset: 2px; }
.notification-toast__topline { display: flex; align-items: center; justify-content: space-between; gap: var(--bb-space-3); }
.notification-toast__label { display: inline-flex; align-items: center; gap: 6px; color: var(--bb-accent-text); font-size: var(--bb-font-size-12); font-weight: var(--bb-font-weight-semibold); }
.notification-toast__topline > button { display: grid; width: 40px; height: 40px; place-items: center; border: 0; border-radius: var(--bb-radius-sm); background: transparent; color: var(--bb-text-secondary); }
.notification-toast__topline > button:hover { background: var(--bb-bg-subtle); color: var(--bb-danger); }
.notification-toast__topline > button:focus-visible { outline: 2px solid var(--bb-focus-color); outline-offset: 1px; }
.notification-toast__title { overflow-wrap: anywhere; font-size: var(--bb-font-size-16); line-height: var(--bb-line-height-tight); }
.notification-toast__summary { color: var(--bb-text-regular); font-size: var(--bb-font-size-14); line-height: var(--bb-line-height-base); overflow-wrap: anywhere; }
.notification-toast__modules { display: flex; flex-wrap: wrap; gap: 6px; color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); }
.notification-toast__modules span { border-radius: var(--bb-radius-pill); background: var(--bb-bg-subtle); padding: 3px 8px; }
.notification-toast__view { display: inline-flex; min-height: 40px; align-items: center; justify-content: center; justify-self: start; gap: 4px; border: 0; border-radius: var(--bb-radius-sm); background: var(--bb-accent-selected-bg); padding: 0 12px; color: var(--bb-accent-text); font-size: var(--bb-font-size-13); font-weight: var(--bb-font-weight-semibold); }
.notification-toast__view:hover { filter: brightness(.98); }
.notification-toast__view:focus-visible { outline: 2px solid var(--bb-focus-color); outline-offset: 2px; }
.notification-toast-enter-active, .notification-toast-leave-active { transition: opacity var(--bb-duration-base) var(--bb-ease-standard), transform var(--bb-duration-base) var(--bb-ease-standard); }
.notification-toast-enter-from, .notification-toast-leave-to { opacity: 0; transform: translateY(-6px); }
@media (max-width: 520px) {
  .notification-toast { right: 16px; width: calc(100vw - 32px); }
  .bb-notification-popover { max-width: calc(100vw - 24px) !important; }
}
</style>
