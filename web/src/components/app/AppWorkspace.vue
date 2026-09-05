<template>
  <section
    class="workspace"
    :class="[`sidebar-${sidebarMode}`, {'has-docked-detail': detailPanelDocked, 'compact-dock-navigation': compactDockNavigationHidden}]"
    :style="{'--bb-detail-panel-width': `${detailPanelWidth}px`}"
  >
    <aside
      ref="sidebarElement"
      id="app-navigation"
      class="sidebar"
      :class="{'is-mobile-open': mobileNavOpen}"
      aria-label="系统导航"
      :aria-hidden="navigationInactive"
      :inert="navigationInactive"
      :role="usesOverlayNavigation ? 'dialog' : undefined"
      :aria-modal="usesOverlayNavigation && mobileNavOpen ? 'true' : undefined"
      :tabindex="usesOverlayNavigation ? -1 : undefined"
    >
      <div class="sidebar-brand">
        <img src="/bobang-logo-hd.png" alt="" />
        <div v-if="usesOverlayNavigation || sidebarMode !== 'icon'"><strong>博邦光电</strong><span>ERP 业务工作台</span></div>

      </div>
      <AppNavigation
        :active-key="activeKey"
        :business-items="businessItems"
        :system-items="systemItems"
        :mode="usesOverlayNavigation ? 'full' : sidebarMode"
        @select="handleModuleSelect"
        @settings="openSettings"
      />
    </aside>
    <button v-if="mobileNavOpen" class="mobile-nav-backdrop" type="button" tabindex="-1" aria-hidden="true" @click="closeMobileNavigation(true)"></button>

    <header class="topbar">
      <div class="topbar-leading">
        <el-button
          ref="sidebarToggle"
          class="sidebar-mode-toggle"
          :aria-label="sidebarToggleLabel"
          :title="sidebarToggleLabel"
          aria-controls="app-navigation"
          :aria-expanded="usesOverlayNavigation ? mobileNavOpen : undefined"
          :tabindex="mobileNavOpen ? -1 : 0"
          @click="toggleSidebar"
        >
          <el-icon aria-hidden="true"><Menu /></el-icon>
        </el-button>
        <div class="topbar-title" :inert="mobileNavOpen"><strong>{{ activeModule?.title || '首页' }}</strong></div>
      </div>

      <button
        v-if="healthStatus !== 'healthy'"
        class="service-notice"
        :class="`is-${healthStatus}`"
        type="button"
        :inert="mobileNavOpen"
        aria-live="polite"
        @click="openSettings"
      >
        <span aria-hidden="true"></span>
        {{ healthStatusLabel }}
      </button>

      <div class="user-chip" :inert="mobileNavOpen">
        <div class="user-copy"><span>{{ currentUser?.name || currentUser?.username }}</span><small>{{ accountTypeText }}</small></div>
        <el-dropdown trigger="click" @command="handleUserCommand">
          <el-button class="user-avatar" :aria-label="`${currentUser?.name || currentUser?.username || '用户'}菜单`">{{ userInitial }}</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="settings">设置</el-dropdown-item>
              <el-dropdown-item command="change-password">修改密码</el-dropdown-item>
              <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </header>

    <ChangePasswordDialog v-model="passwordDialogVisible" :token="token" @changed="handlePasswordChanged" />
    <SettingsPanel v-model="settingsVisible" />
    <section class="content" :inert="mobileNavOpen"><slot name="page" /></section>
    <div
      id="workspace-detail-host"
      class="workspace-detail-host"
      :aria-hidden="detailPanelDocked ? undefined : 'true'"
      :inert="!detailPanelDocked || mobileNavOpen"
    ></div>
    <slot name="overlays" />
  </section>
</template>

<script setup lang="ts">
import {computed, nextTick, onBeforeUnmount, onMounted, ref, watch} from 'vue'
import {ElMessage} from 'element-plus'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import {nextSidebarMode, normalizeSidebarMode, sidebarStorageKey, type SidebarMode} from '../../platform/appearance'
import {resolveFocusLoopTarget} from '../../platform/focusLoop'
import AppNavigation from '../ui/AppNavigation.vue'
import ChangePasswordDialog from './ChangePasswordDialog.vue'
import SettingsPanel from './SettingsPanel.vue'
import {Menu} from '@element-plus/icons-vue'
import {clientSidebarMode, clientViewport, activeDetailWidth, activeDetailDocked, requestActiveDetailClose} from '../../composables/detailLayout'

const {
  token, currentUser, activeKey, activeModule, businessItems, systemItems, userInitial,
  accountTypeText, healthStatus, healthStatusLabel, switchModule, logout, loginForm,
  warehouseDrawerVisible, workorderDrawerVisible, pageDetailPanelVisible,
} = useWorkspaceContext()
const passwordDialogVisible = ref(false)
const settingsVisible = ref(false)
const mobileNavOpen = ref(false)
const restoreFocusAfterMobileClose = ref(true)
const sidebarElement = ref<HTMLElement | null>(null)
const sidebarToggle = ref<{ref?: HTMLElement; $el?: HTMLElement} | null>(null)
const viewportWidth = clientViewport
const sidebarMode = clientSidebarMode
const isNarrow = computed(() => viewportWidth.value <= 1023)
const configuredSidebarWidth = computed(() => sidebarMode.value === 'hidden' ? 0 : sidebarMode.value === 'icon' ? 64 : 224)
const compactDockNavigationHidden = computed(() => detailPanelDocked.value && viewportWidth.value >= 1024 && viewportWidth.value - activeDetailWidth.value - configuredSidebarWidth.value < 720)
const usesOverlayNavigation = computed(() => isNarrow.value || compactDockNavigationHidden.value)
const navigationInactive = computed(() => usesOverlayNavigation.value ? !mobileNavOpen.value : sidebarMode.value === 'hidden')
const effectiveSidebarWidth = computed(() => isNarrow.value || compactDockNavigationHidden.value ? 0 : configuredSidebarWidth.value)
const detailPanelVisible = computed(() => warehouseDrawerVisible.value || workorderDrawerVisible.value || pageDetailPanelVisible.value)
const imageDetailVisible = computed(() => warehouseDrawerVisible.value || workorderDrawerVisible.value)
const detailPanelWidth = computed(() => Math.min(activeDetailWidth.value, viewportWidth.value))
const detailPanelDocked = activeDetailDocked
const sidebarToggleLabel = computed(() => isNarrow.value
  ? mobileNavOpen.value ? '关闭业务导航' : '打开业务导航'
  : compactDockNavigationHidden.value ? mobileNavOpen.value ? '关闭业务导航' : '打开业务导航'
  : sidebarMode.value === 'full' ? '切换为图标导航' : sidebarMode.value === 'icon' ? '隐藏业务导航' : '展开完整业务导航')

function syncViewport() {
  viewportWidth.value = window.innerWidth
  if (!isNarrow.value) mobileNavOpen.value = false
}

function toggleSidebar() {
  if (usesOverlayNavigation.value) {
    if (mobileNavOpen.value) closeMobileNavigation(true)
    else {
      restoreFocusAfterMobileClose.value = true
      mobileNavOpen.value = true
    }
    return
  }
  sidebarMode.value = nextSidebarMode(sidebarMode.value)
  localStorage.setItem(sidebarStorageKey, sidebarMode.value)
}

function handleModuleSelect(key: string) {
  closeMobileNavigation(true)
  switchModule(key)
}

async function openSettings() {
  closeMobileNavigation(false)
  if (settingsVisible.value || !(await requestActiveDetailClose())) return
  settingsVisible.value = true
}

function handleUserCommand(command: string) {
  if (command === 'settings') openSettings()
  if (command === 'change-password') passwordDialogVisible.value = true
  if (command === 'logout') void logout()
}

function handlePasswordChanged() {
  loginForm.password = ''
  void logout()
  ElMessage.success('密码修改成功，请使用新密码重新登录')
}

function closeMobileNavigation(restoreFocus = true) {
  restoreFocusAfterMobileClose.value = restoreFocus
  mobileNavOpen.value = false
}

function focusSidebarToggle() {
  const candidate = sidebarToggle.value?.ref || sidebarToggle.value?.$el
  candidate?.focus?.()
}

function handleNavigationKeydown(event: KeyboardEvent) {
  if (!mobileNavOpen.value || event.defaultPrevented || eventBelongsToElementLayer(event)) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopImmediatePropagation()
    closeMobileNavigation(true)
    return
  }
  if (event.key !== 'Tab') return
  const focusable = navigationFocusableElements()
  if (!focusable.length) {
    event.preventDefault()
    sidebarElement.value?.focus()
    return
  }
  const currentIndex = focusable.indexOf(document.activeElement as HTMLElement)
  const targetIndex = resolveFocusLoopTarget(focusable.length, currentIndex, event.shiftKey)
  if (targetIndex === null) return
  event.preventDefault()
  focusable[targetIndex]?.focus()
}

const navigationFocusableSelector = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[contenteditable="true"]',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

function navigationFocusableElements() {
  if (!sidebarElement.value) return []
  return [...sidebarElement.value.querySelectorAll<HTMLElement>(navigationFocusableSelector)].filter((element) => {
    if (element.closest('[inert]') || element.getAttribute('aria-hidden') === 'true') return false
    const style = window.getComputedStyle(element)
    return style.display !== 'none' && style.visibility !== 'hidden' && element.getClientRects().length > 0
  })
}

function eventBelongsToElementLayer(event: KeyboardEvent) {
  return event.target instanceof Element
    && Boolean(event.target.closest('.el-overlay:not([inert]), .el-popper:not([inert])'))
}

watch(mobileNavOpen, async (open, wasOpen) => {
  if (open) {
    await nextTick()
    document.querySelectorAll<HTMLElement>('.el-overlay').forEach((element) => {
      element.dataset.bbNavigationInert = 'true'
      element.inert = true
    })
    const firstFocusable = navigationFocusableElements()[0]
    if (firstFocusable) firstFocusable.focus()
    else sidebarElement.value?.focus()
  } else if (wasOpen) {
    document.querySelectorAll<HTMLElement>('[data-bb-navigation-inert="true"]').forEach((element) => {
      element.inert = false
      delete element.dataset.bbNavigationInert
    })
  }
  if (!open && wasOpen && usesOverlayNavigation.value && restoreFocusAfterMobileClose.value) {
    await nextTick()
    focusSidebarToggle()
  }
})

watch(compactDockNavigationHidden, (hidden) => {
  if (!hidden) mobileNavOpen.value = false
})

onMounted(() => {
  window.addEventListener('resize', syncViewport)
  document.addEventListener('keydown', handleNavigationKeydown)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', syncViewport)
  document.removeEventListener('keydown', handleNavigationKeydown)
})
</script>
