<template>
  <section class="startup-screen" aria-labelledby="startup-title">
    <div class="startup-card">
      <header class="startup-brand">
        <img src="/bobang-logo-hd.png" alt="博邦光电"/>
        <div>
          <span>博邦 ERP · {{ platformKind === 'tauri' ? 'Windows 客户端' : 'Web 工作台' }}</span>
          <h1 id="startup-title">正在准备业务工作台</h1>
          <small v-if="version">客户端版本 {{ version }}</small>
        </div>
      </header>

      <div v-if="isBusy" class="startup-progress" aria-live="polite" aria-busy="true">
        <el-progress :percentage="100" :show-text="false" :indeterminate="true" :duration="1.6"/>
        <strong>{{ message }}</strong>
        <p>通常只需几秒，请保持服务器和局域网连接正常。</p>
      </div>

      <section v-else-if="phase === 'AutoConnected'" class="startup-result startup-connected" aria-live="polite">
        <StatusTag label="连接成功" tone="success"/>
        <strong>{{ message }}</strong>
      </section>

      <section v-else-if="phase === 'SelectServer'" ref="resultHeading" class="startup-result" tabindex="-1" aria-labelledby="server-result-title">
        <div class="startup-result__heading">
          <div>
            <span>发现服务冲突</span>
            <h2 id="server-result-title">请选择本次连接的服务器</h2>
          </div>
          <StatusTag :label="`${candidates.length} 个服务`" tone="warning"/>
        </div>
        <el-alert title="局域网内存在多个已验证候选（可能包含克隆身份），客户端不会静默选择或覆盖上次连接。" type="warning" :closable="false" show-icon/>
        <div class="server-list">
          <article v-for="server in candidates" :key="serverIdentityKey(server)" class="server-card">
            <div class="server-card__copy">
              <div>
                <strong>{{ server.server_name }}</strong>
                <StatusTag v-if="serverIdentityKey(server) === savedServerKey" label="上次使用" tone="info"/>
              </div>
              <span>{{ server.origin }}</span>
              <small>服务端版本 {{ server.server_version || '未提供' }}</small>
            </div>
            <el-button type="primary" @click="selectServer(server)">连接此服务器</el-button>
          </article>
        </div>
        <div class="startup-actions">
          <el-button @click="rediscover">重新发现</el-button>
        </div>
      </section>

      <section v-else ref="resultHeading" class="startup-result" tabindex="-1" aria-labelledby="manual-result-title">
        <div class="startup-result__heading">
          <div>
            <span>{{ failureLabel }}</span>
            <h2 id="manual-result-title">{{ message }}</h2>
          </div>
          <StatusTag label="未连接" tone="danger"/>
        </div>
        <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon role="alert"/>

        <template v-if="platformKind === 'tauri'">
          <p class="startup-help">确认服务端已启动，并允许 Windows 专用网络访问 ERP TCP 端口和 UDP 39080。UDP 不可用时仍可手动连接。</p>
          <el-form class="manual-server-form" label-position="top" @submit.prevent="connectManually">
            <el-form-item label="服务器 IPv4 地址">
              <el-input v-model.trim="manualAddress" :disabled="manualTesting" placeholder="例如 192.168.1.20:8080" clearable/>
            </el-form-item>
            <div class="startup-actions">
              <el-button :disabled="manualTesting" @click="rediscover">重新发现</el-button>
              <el-button type="primary" native-type="submit" :loading="manualTesting" :disabled="manualTesting || !manualAddress.trim()">验证并连接</el-button>
            </div>
          </el-form>
        </template>
        <template v-else>
          <p class="startup-help">Web 版只连接当前站点配置的服务。请重试；若仍失败，请联系系统管理员检查服务状态。</p>
          <div class="startup-actions">
            <el-button type="primary" @click="start">重试连接</el-button>
          </div>
        </template>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import {computed, nextTick, ref, watch} from 'vue'
import {useStartupConnectionContext} from '../../composables/workspaceContext'
import StatusTag from '../ui/StatusTag.vue'
import {serverIdentityKey} from '../../platform/connectionPolicy'

const {
  platformKind,
  phase,
  failure,
  message,
  error,
  version,
  candidates,
  manualAddress,
  manualTesting,
  focusRevision,
  isBusy,
  savedServerKey,
  start,
  rediscover,
  selectServer,
  connectManually,
} = useStartupConnectionContext()

const resultHeading = ref<HTMLElement>()
const failureLabel = computed(() => ({
  'no-server': '未发现服务',
  'discovery-failed': '自动发现失败',
  'validation-failed': '服务验证失败',
  none: '连接未完成',
})[failure.value])

watch(focusRevision, async () => {
  if (isBusy.value) return
  await nextTick()
  resultHeading.value?.focus()
})

</script>

<style scoped>
.startup-screen {
  display: grid;
  min-height: 100vh;
  place-items: center;
  background:
    radial-gradient(circle at 18% 15%, var(--bb-brand-100), transparent 34%),
    var(--bb-bg-page);
  padding: var(--bb-space-8);
}
.startup-card {
  display: grid;
  width: min(760px, 100%);
  gap: var(--bb-space-8);
  border: 1px solid var(--bb-border-default);
  border-radius: var(--bb-radius-xl);
  background: var(--bb-bg-surface);
  padding: var(--bb-space-10);
  box-shadow: var(--bb-shadow-md);
}
.startup-brand { display: flex; align-items: center; gap: var(--bb-space-5); }
.startup-brand img {
  width: 148px;
  border: 1px solid var(--bb-border-subtle);
  border-radius: var(--bb-radius-md);
  animation: startup-logo-in var(--bb-duration-slow) var(--bb-ease-standard) both;
}
.startup-brand div, .startup-progress, .startup-result, .server-list { display: grid; gap: var(--bb-space-3); }
.startup-brand span, .startup-result__heading span { color: var(--bb-brand-700); font-size: var(--bb-font-size-12); font-weight: var(--bb-font-weight-bold); letter-spacing: .06em; }
.startup-brand h1, .startup-result__heading h2 { margin: 0; color: var(--bb-text-primary); }
.startup-brand h1 { font-size: var(--bb-font-size-24); }
.startup-brand small, .startup-progress p, .startup-help { color: var(--bb-text-secondary); }
.startup-progress { padding: var(--bb-space-4) 0; }
.startup-progress strong { color: var(--bb-text-primary); }
.startup-progress p, .startup-help { margin: 0; line-height: var(--bb-line-height-relaxed); }
.startup-result { outline: none; }
.startup-connected { justify-items: start; color: var(--bb-text-primary); }
.startup-result:focus-visible { border-radius: var(--bb-radius-sm); box-shadow: var(--bb-focus-ring); }
.startup-result__heading { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--bb-space-4); }
.startup-result__heading > div { display: grid; gap: var(--bb-space-2); }
.startup-result__heading h2 { font-size: var(--bb-font-size-20); }
.server-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--bb-space-5);
  border: 1px solid var(--bb-border-default);
  border-radius: var(--bb-radius-lg);
  background: var(--bb-bg-subtle);
  padding: var(--bb-space-4);
}
.server-card__copy { display: grid; min-width: 0; gap: var(--bb-space-1); }
.server-card__copy > div { display: flex; flex-wrap: wrap; align-items: center; gap: var(--bb-space-2); }
.server-card__copy span { color: var(--bb-text-regular); font-family: var(--bb-font-mono); font-size: var(--bb-font-size-13); }
.server-card__copy small { color: var(--bb-text-secondary); }
.manual-server-form { display: grid; gap: var(--bb-space-4); }
.manual-server-form :deep(.el-form-item) { margin-bottom: 0; }
.startup-actions { display: flex; justify-content: flex-end; gap: var(--bb-space-2); }
@keyframes startup-logo-in {
  from { opacity: 0; transform: translateY(6px); }
  to { opacity: 1; transform: translateY(0); }
}
@media (prefers-reduced-motion: reduce) {
  .startup-brand img { animation: none; }
}
</style>
