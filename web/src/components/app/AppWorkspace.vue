<template>
  <section class="workspace">
    <header class="topbar">
      <div class="brand">
        <img src="/bobang-logo-hd.png" alt="博邦光电" />
        <span class="brand-mark" aria-label="博邦光电">BB</span>
        <div><strong>博邦光电</strong><span>业务工作台</span></div>
      </div>
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

    <aside class="sidebar" aria-label="系统导航">
      <AppNavigation :active-key="activeKey" :business-items="businessItems" :system-items="systemItems" @select="switchModule" />
    </aside>

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
