<template>
  <section class="startup-screen" aria-labelledby="startup-title">
    <div class="startup-card">
      <header class="startup-brand">
        <img src="/bobang-logo-hd.png" alt="博邦光电"/>
        <div>
          <span>博邦 ERP · {{ platformKind === 'tauri' ? '桌面客户端' : 'Web 工作台' }}</span>
          <h1 id="startup-title">连接工作台</h1>
          <small v-if="version">客户端版本 {{ version }}</small>
        </div>
      </header>


        <div
          v-if="isBusy"
          :key="phase"
          class="startup-progress"
          aria-live="polite"
          aria-busy="true"
        >
          <el-progress :percentage="100" :show-text="false" :indeterminate="true" :duration="1.6"/>
          <strong>{{ message }}</strong>
          <p>请稍候。</p>
        </div>

        <section
          v-else-if="phase === 'AutoConnected'"
          :key="phase"
          class="startup-result startup-connected"
          aria-live="polite"
        >
          <StatusTag label="连接成功" tone="success"/>
          <strong>{{ message }}</strong>
        </section>

        <div
          v-else-if="phase === 'SelectServer'"
          :key="phase"
        >
          <section ref="resultHeading" class="startup-result" tabindex="-1" aria-labelledby="server-result-title">
            <div class="startup-result__heading">
              <div>
                <span>发现多个服务器</span>
                <h2 id="server-result-title">选择服务器</h2>
              </div>
              <StatusTag :label="`${candidates.length} 个服务`" tone="warning"/>
            </div>
            <el-alert title="请选择要使用的服务器；不确定时请联系管理员确认。" type="warning" :closable="false" show-icon/>
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
        </div>

        <div
          v-else
          :key="phase"
        >
          <section ref="resultHeading" class="startup-result" tabindex="-1" aria-labelledby="manual-result-title">
            <div class="startup-result__heading">
              <div>
                <span>{{ failureLabel }}</span>
                <h2 id="manual-result-title">{{ message }}</h2>
              </div>
              <StatusTag label="未连接" tone="danger"/>
            </div>
            <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon role="alert"/>

            <template v-if="platformKind === 'tauri'">
              <p class="startup-help">请确认电脑与服务器连接同一网络，也可以输入服务器地址连接。</p>
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

async function focusResult() {
  if (isBusy.value) return
  await nextTick()
  resultHeading.value?.focus()
}

watch(focusRevision, focusResult)
watch(phase, focusResult, {flush: 'post'})

</script>

<style scoped>
.startup-screen {
  display: grid;
  min-height: 100vh;
  place-items: center;
  background: var(--bb-bg-page);
  padding: var(--bb-space-8);
}
.startup-card {
  display: grid;
  width: min(560px, 100%);
  gap: var(--bb-space-8);
  border: 1px solid var(--bb-border-default);
  border-radius: 10px;
  background: var(--bb-bg-surface);
  padding: 32px;
}
.startup-brand { display: flex; align-items: center; gap: var(--bb-space-5); }
.startup-brand img {
  width: 40px;
  height: 40px;
  object-fit: contain;
  border: 1px solid var(--bb-border-subtle);
  border-radius: var(--bb-radius-md);
}
.startup-brand div, .startup-progress, .startup-result, .server-list { display: grid; gap: var(--bb-space-3); }
.startup-brand span, .startup-result__heading span { color: var(--bb-accent-text); font-size: var(--bb-font-size-12); font-weight: var(--bb-font-weight-bold); letter-spacing: .06em; }
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
</style>
