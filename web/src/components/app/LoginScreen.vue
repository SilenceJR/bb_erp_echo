<template>
  <section class="login-screen" aria-labelledby="login-title">
    <div class="login-card">
      <header class="login-brand">
        <img src="/bobang-logo-hd.png" alt="博邦光电" />
        <span>博邦 ERP</span>
      </header>
      <div class="login-heading">
        <h1 id="login-title">登录</h1>
        <p>使用工作账号进入业务工作台</p>
      </div>
      <el-form class="login-form" label-position="top" aria-label="账号登录表单" :aria-busy="loading" @submit.prevent="login">
        <el-form-item label="账号">
          <el-input ref="loginUsernameInput" v-model.trim="loginForm.username" autocomplete="username" autofocus clearable :disabled="loading" placeholder="请输入账号" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="loginForm.password" type="password" autocomplete="current-password" show-password :disabled="loading" placeholder="请输入密码" />
        </el-form-item>
        <el-alert v-if="errorMessage" :title="errorMessage" type="error" :closable="false" show-icon role="alert" />
        <el-button class="login-submit" type="primary" :loading="loading" :disabled="loading" native-type="submit">{{ loading ? '正在登录' : '登录' }}</el-button>
      </el-form>
      <section v-if="desktopClient" class="login-update" aria-labelledby="login-update-title">
        <div class="login-update__heading">
          <div>
            <h2 id="login-update-title">客户端更新</h2>
            <p>连接已验证，可在登录前检查服务器上的新版客户端。</p>
          </div>
          <el-button :loading="state === 'Checking'" :disabled="taskInProgress" @click="check">检查更新</el-button>
        </div>
        <p class="login-update__status" role="status" aria-live="polite">{{ loginUpdateStatus }}</p>
        <DesktopUpdatePanel compact />
      </section>
      <footer class="login-connection">
        <div><span>{{ currentServer?.server_name || '当前服务器' }}</span><small>{{ currentServer?.origin || '当前站点' }}</small></div>
        <el-button v-if="canChangeServer" link :disabled="loading" @click="changeServer">切换服务器</el-button>
      </footer>
    </div>
  </section>
</template>

<script setup lang="ts">
import {computed, onMounted} from 'vue'
import {useStartupConnectionContext, useWorkspaceContext} from '../../composables/workspaceContext'
import {desktopBridge} from '../../api/transport'
import {useDesktopUpdate} from '../../composables/useDesktopUpdate'
import DesktopUpdatePanel from '../DesktopUpdatePanel.vue'

const {login, loginForm, loginUsernameInput, loading, errorMessage} = useWorkspaceContext()
const {canChangeServer, currentServer, changeServer} = useStartupConnectionContext()
const desktopClient = Boolean(desktopBridge())
const updater = useDesktopUpdate()
const {state, taskInProgress, check, checkOnConnection} = updater
const loginUpdateStatus = computed(() => {
  if (state.value === 'Checking') return '正在检查服务器上的客户端更新…'
  if (state.value === 'Ready') return `发现客户端 ${updater.plan.value?.latest_version || ''}，可在登录前更新。`
  if (state.value === 'Updated') return '客户端已成功更新，当前正在使用新版。'
  if (state.value === 'RolledBack') return '上次更新已回滚旧版本，请重新检查。'
  if (state.value === 'Failed') return updater.error.value || '检查更新失败，可在设置中重试。'
  if (updater.hasChecked.value) return '当前客户端已是最新版本。'
  return '尚未检查客户端更新。'
})

onMounted(() => {
  if (desktopClient) void checkOnConnection()
})
</script>

<style scoped>
.login-card { width: min(400px, 100%); padding: 32px; border: 1px solid var(--bb-border-subtle); border-radius: 10px; background: var(--bb-bg-surface); }
.login-brand { display: flex; align-items: center; gap: 10px; font-size: 14px; font-weight: 600; }
.login-brand img { width: 32px; height: 32px; object-fit: contain; border-radius: 6px; }
.login-heading { margin: 32px 0 24px; }
.login-heading h1 { margin: 0; font-size: 24px; line-height: 32px; font-weight: 600; }
.login-heading p { margin: 8px 0 0; color: var(--bb-text-secondary); font-size: 13px; }
.login-form .login-submit { width: 100%; margin: 6px 0 0; }
.login-form .el-alert { margin-bottom: 12px; }
.login-update { display: grid; gap: 12px; margin-top: 24px; border-top: 1px solid var(--bb-border-subtle); padding-top: 20px; }
.login-update__heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.login-update__heading h2 { margin: 0; font-size: 16px; line-height: 24px; }
.login-update__heading p, .login-update__status { margin: 4px 0 0; color: var(--bb-text-secondary); font-size: 12px; line-height: 1.5; }
.login-update__status { margin: 0; }
.login-update :deep(.desktop-update-panel.is-compact) { margin-top: 0; }
.login-connection { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-top: 28px; padding-top: 20px; border-top: 1px solid var(--bb-border-subtle); font-size: 13px; }
.login-connection > div { min-width: 0; display: grid; gap: 4px; }
.login-connection small { color: var(--bb-text-secondary); overflow-wrap: anywhere; }
@media (max-width: 480px) { .login-card { padding: 24px; } }
</style>
