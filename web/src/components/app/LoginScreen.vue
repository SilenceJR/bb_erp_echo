<template>
  <section class="login-screen" aria-label="博邦光电登录">
    <div class="login-layout">
      <aside class="login-welcome" aria-labelledby="login-welcome-title">
        <img class="login-logo" src="/bobang-logo-hd.png" alt="博邦光电"/>
        <div class="login-welcome__copy">
          <span class="login-eyebrow">博邦 ERP · 业务协同平台</span>
          <h1 id="login-welcome-title">让库存、任务与模具状态清晰可见</h1>
          <p>面向办公室与部门终端的统一工作台，帮助团队快速完成日常业务办理与进度核对。</p>
        </div>
        <ul class="login-capabilities" aria-label="平台能力">
          <li><strong>统一入口</strong><span>按账号权限呈现可用业务</span></li>
          <li><strong>状态明确</strong><span>集中查看库存预警与任务进度</span></li>
          <li><strong>内网友好</strong><span>桌面端自动发现企业内网服务</span></li>
        </ul>
      </aside>

      <div class="login-panel">
        <header class="login-panel__heading">
          <span class="login-eyebrow">账号登录</span>
          <h2>登录业务工作台</h2>
          <p>使用当前已验证服务器中的账号继续。</p>
        </header>

        <section class="verified-server" aria-labelledby="verified-server-title">
          <div class="verified-server__copy">
            <div>
              <span id="verified-server-title">{{ currentServer?.server_name }}</span>
              <StatusTag label="已验证" tone="success"/>
            </div>
            <small>{{ currentServer?.origin }}<template v-if="currentServer?.server_version"> · 服务端 {{ currentServer.server_version }}</template></small>
          </div>
          <el-button v-if="canChangeServer" plain :disabled="loading" @click="changeServer">更换服务器</el-button>
        </section>

        <el-form class="login-form" label-position="top" aria-label="账号登录表单" :aria-busy="loading" @submit.prevent="login">
          <el-form-item label="账号">
            <el-input ref="loginUsernameInput" v-model.trim="loginForm.username" autocomplete="username" autofocus clearable :disabled="loading" placeholder="请输入账号"/>
          </el-form-item>
          <el-form-item label="密码">
            <el-input v-model="loginForm.password" type="password" autocomplete="current-password" show-password :disabled="loading" placeholder="请输入密码"/>
          </el-form-item>
          <el-button class="login-submit" type="primary" :loading="loading" :disabled="loading" native-type="submit">
            {{ loading ? '正在登录' : '登录' }}
          </el-button>
          <el-alert v-if="errorMessage" :title="errorMessage" type="error" :closable="false" show-icon role="alert"/>
        </el-form>

        <p v-if="!canChangeServer" class="login-web-note">Web 版连接当前站点服务；如无法登录，请联系系统管理员。</p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import StatusTag from '../ui/StatusTag.vue'
import {useStartupConnectionContext, useWorkspaceContext} from '../../composables/workspaceContext'

const {login, loginForm, loginUsernameInput, loading, errorMessage} = useWorkspaceContext()
const {canChangeServer, currentServer, changeServer} = useStartupConnectionContext()
</script>

<style scoped>
.verified-server {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--bb-space-4);
  border: 1px solid var(--bb-success-border);
  border-radius: var(--bb-radius-lg);
  background: var(--bb-success-bg);
  padding: var(--bb-space-4);
}
.verified-server__copy { display: grid; min-width: 0; gap: var(--bb-space-1); }
.verified-server__copy > div { display: flex; flex-wrap: wrap; align-items: center; gap: var(--bb-space-2); }
.verified-server__copy span { color: var(--bb-text-primary); font-weight: var(--bb-font-weight-semibold); }
.verified-server__copy small { overflow-wrap: anywhere; color: var(--bb-text-regular); font-family: var(--bb-font-mono); }
</style>
