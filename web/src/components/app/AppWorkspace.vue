<template>
  <section class="workspace">
    <header class="topbar">
      <div class="brand">
        <img src="/bobang-logo-hd.png" alt="博邦光电" />
        <span class="brand-mark" aria-label="博邦光电">BB</span>
        <div><strong>博邦光电</strong><span>业务工作台</span></div>
      </div>
      <el-button
        class="mobile-nav-toggle"
        :aria-expanded="mobileNavOpen"
        aria-controls="app-navigation"
        @click="mobileNavOpen = !mobileNavOpen"
      >
        {{ mobileNavOpen ? '关闭菜单' : '打开菜单' }}
      </el-button>
      <div class="server-indicator" aria-live="polite">
        <span class="server-indicator__dot" :class="`is-${healthStatus}`" aria-hidden="true"></span>
        <div>
          <strong>{{ currentServer?.server_name || 'ERP 服务器' }}</strong>
          <small>{{ currentServer?.origin || '当前站点' }} · {{ healthStatusLabel }}</small>
        </div>
        <el-button v-if="canChangeServer" link type="primary" @click="changeServer">切换</el-button>
      </div>
      <div class="user-chip">
        <div class="user-copy"><span>{{ currentUser?.name || currentUser?.username }}</span><small>{{ accountTypeText }}</small></div>
        <el-dropdown trigger="click" @command="handleUserCommand">
          <el-button circle class="user-avatar" :aria-label="`${currentUser?.name || currentUser?.username || '用户'}菜单`">{{ userInitial }}</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-if="canChangeServer" command="change-server">更换服务器</el-dropdown-item>
              <el-dropdown-item command="change-password">修改密码</el-dropdown-item>
              <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </header>

    <aside id="app-navigation" class="sidebar" :class="{ 'is-mobile-open': mobileNavOpen }" aria-label="系统导航">
      <AppNavigation :active-key="activeKey" :business-items="businessItems" :system-items="systemItems" @select="handleModuleSelect" />
    </aside>
    <button v-if="mobileNavOpen" class="mobile-nav-backdrop" type="button" aria-label="关闭导航菜单" @click="mobileNavOpen = false"></button>

    <ChangePasswordDialog v-model="passwordDialogVisible" :token="token" @changed="handlePasswordChanged" />
    <section class="content"><slot name="page" /></section>
    <slot name="overlays" />
  </section>
</template>

<script setup lang="ts">
import {ref} from 'vue'
import {ElMessage} from 'element-plus'
import {useStartupConnectionContext, useWorkspaceContext} from '../../composables/workspaceContext'
import AppNavigation from '../ui/AppNavigation.vue'
import ChangePasswordDialog from './ChangePasswordDialog.vue'

const {token, currentUser, activeKey, businessItems, systemItems, userInitial, accountTypeText, healthStatus, healthStatusLabel, switchModule, logout, loginForm} = useWorkspaceContext()
const {canChangeServer, changeServer, currentServer} = useStartupConnectionContext()
const passwordDialogVisible = ref(false)
const mobileNavOpen = ref(false)

function handleModuleSelect(key: string) {
  mobileNavOpen.value = false
  switchModule(key)
}

function handleUserCommand(command: string) {
  if (command === 'change-server') return void changeServer()
  if (command === 'change-password') passwordDialogVisible.value = true
  if (command === 'logout') void logout()
}

function handlePasswordChanged() {
  loginForm.password = ''
  void logout()
  ElMessage.success('密码修改成功，请使用新密码重新登录')
}
</script>
