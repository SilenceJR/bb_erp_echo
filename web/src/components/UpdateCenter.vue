<template>
  <section v-loading="loading" class="update-center" aria-label="版本与更新">
    <el-alert
      v-if="loadError"
      :title="loadError"
      type="error"
      :closable="false"
      show-icon
    />

    <div class="update-overview">
      <article class="update-source-card">
        <div class="update-card-heading">
          <div>
            <span class="update-kicker">更新源</span>
            <h2>{{ connectivityText }}</h2>
          </div>
          <el-tag :type="connectivityType" effect="light" round>{{ connectivityTag }}</el-tag>
        </div>
        <p class="update-manifest">{{ manifestUrl || '尚未配置升级 manifest 地址' }}</p>
        <dl class="update-meta-grid">
          <div><dt>最后尝试</dt><dd>{{ formatTime(status?.last_attempt_at) }}</dd></div>
          <div><dt>最后成功</dt><dd>{{ formatTime(status?.last_success_at) }}</dd></div>
          <div><dt>下次检查</dt><dd>{{ formatTime(status?.next_check_at) }}</dd></div>
          <div><dt>检查周期</dt><dd>{{ status?.check_interval || '由服务端配置' }}</dd></div>
        </dl>
        <el-alert
          v-if="statusError"
          :title="statusError"
          type="warning"
          :closable="false"
          show-icon
        />
        <div class="update-actions">
          <el-button :loading="loading" :disabled="requestBusy" @click="loadStatus">刷新状态</el-button>
          <el-button v-if="canCheck" type="primary" :loading="checking" :disabled="requestBusy || status?.enabled === false" @click="checkNow">立即检查</el-button>
          <small v-if="status?.enabled === false">需由服务端开启更新检查后才能手动检查</small>
          <small v-else-if="!canCheck">你的账号只有查看权限</small>
        </div>
      </article>
    </div>

    <div class="update-package-grid">
      <PackageCard
        title="Go 服务端"
        description="只报告和下载新版本，不会自动替换正在运行的服务。"
        :item="serverStatus"
        :known="statusKnown"
        @download="openDownload"
      />
      <DesktopUpdatePanel
        v-if="desktopClient"
        :legacy-status="clientStatus"
        @download-recovery="openDownload"
      />
      <PackageCard
        v-else
        title="桌面客户端"
        description="Web 端不执行桌面客户端安装；完整 ZIP 仅供管理员故障恢复使用。"
        download-label="下载完整 ZIP（故障恢复）"
        :allow-download="canCheck"
        :item="clientStatus"
        :known="statusKnown"
        @download="openDownload"
      />
    </div>

    <article v-if="clientProtocolVersion > 0" class="update-package-card" aria-labelledby="client-v2-cache-title">
      <div class="update-card-heading">
        <div>
          <span class="update-kicker">客户端更新缓存</span>
          <h2 id="client-v2-cache-title">增量更新资源</h2>
        </div>
        <el-tag type="info" effect="light">协议 v{{ clientProtocolVersion }}</el-tag>
      </div>
      <p class="update-package-description">服务端已验签并缓存的 Windows 客户端更新资源摘要。</p>
      <dl class="update-version-list">
        <div><dt>完整包</dt><dd>{{ cacheStateLabel(status?.client_full_cached) }}</dd></div>
        <div><dt>差分来源版本</dt><dd>{{ status?.client_delta_from_version || '暂无可用差分' }}</dd></div>
        <div><dt>差分包</dt><dd>{{ cacheStateLabel(status?.client_delta_cached) }}</dd></div>
        <div><dt>缓存总量</dt><dd>{{ formatBytes(status?.client_cache_bytes) }}</dd></div>
      </dl>
      <el-alert
        v-if="clientDeltaDegraded"
        :title="`差分更新已降级：${clientDeltaDegraded}`"
        type="warning"
        :closable="false"
        show-icon
      />
    </article>
  </section>
</template>

<script setup lang="ts">
import {computed, defineComponent, h, onMounted, ref} from 'vue'
import {ElButton, ElMessage, ElTag} from 'element-plus'
import DesktopUpdatePanel from './DesktopUpdatePanel.vue'
import {downloadApiFile, isDesktopClient, request} from '../api/http'
import type {SystemUpdateStatus, UpdatePackageStatus} from '../types'

const props = defineProps<{
  token: string
  canCheck: boolean
}>()

const status = ref<SystemUpdateStatus | null>(null)
const desktopClient = isDesktopClient()
const loading = ref(false)
const checking = ref(false)
const loadError = ref('')
let statusRequestGeneration = 0
const requestBusy = computed(() => loading.value || checking.value)
const manifestUrl = computed(() => String(status.value?.manifest_url || status.value?.source || ''))
const statusError = computed(() => String(status.value?.last_error || status.value?.error || ''))
const clientProtocolVersion = computed(() => Math.max(0, Number(status.value?.client_protocol_version || 0)))
const clientDeltaDegraded = computed(() => String(status.value?.client_delta_degraded || '').trim())
const statusKnown = computed(() => Boolean(
  status.value
  && !loadError.value
  && !statusError.value
  && !requestBusy.value
  && status.value.enabled !== false
  && status.value.reachable !== false
  && !status.value.checking,
))
const serverStatus = computed(() => status.value?.server || status.value?.server_update || {})
const clientStatus = computed(() => status.value?.client || status.value?.client_update || {})
const connectivityText = computed(() => {
  if (!status.value || loadError.value) return '更新状态未知'
  if (requestBusy.value) return '正在获取更新状态'
  if (status.value.enabled === false) return '更新检查已停用'
  if (statusError.value || status.value.reachable === false) return '更新状态未知'
  if (status.value?.checking) return '正在检查更新'
  if (status.value?.last_success_at || status.value?.reachable === true) return '更新源连接正常'
  return '等待首次检查'
})
const connectivityTag = computed(() => {
  if (!status.value || loadError.value) return '状态未知'
  if (requestBusy.value) return '状态未知'
  if (status.value?.enabled === false) return '已停用'
  if (statusError.value || status.value.reachable === false) return '状态未知'
  if (status.value?.checking) return '检查中'
  if (status.value?.last_success_at || status.value?.reachable === true) return '正常'
  return '待检查'
})
const connectivityType = computed<'success' | 'warning' | 'info'>(() => {
  if (!status.value || loadError.value || statusError.value || status.value.reachable === false || requestBusy.value || status.value.enabled === false || status.value.checking) return 'info'
  if (status.value?.last_success_at || status.value?.reachable === true) return 'success'
  return 'info'
})

function formatTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', {hour12: false})
}

function formatBytes(value?: number): string {
  const amount = Number(value || 0)
  if (amount <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB']
  let size = amount
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit += 1 }
  return `${size.toFixed(unit ? 1 : 0)} ${units[unit]}`
}

function cacheStateLabel(value?: boolean): string {
  if (value === true) return '已校验并缓存'
  if (value === false) return '尚未缓存'
  return '状态未知'
}

async function openDownload(item: UpdatePackageStatus) {
  const target = item.download_url || item.download_path || ''
  if (!target) return
  try {
    if (/^https?:\/\//i.test(target)) {
      window.open(target, '_blank', 'noopener,noreferrer')
      return
    }
    const apiPath = target.startsWith('/') ? target : `/${target}`
    await downloadApiFile(apiPath, item.file_name || 'bb-erp-update.zip', props.token)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '升级包下载失败')
  }
}

async function loadStatus() {
  if (requestBusy.value) return
  const requestGeneration = ++statusRequestGeneration
  loading.value = true
  loadError.value = ''
  try {
    const data = await request<SystemUpdateStatus>('/api/v1/system/updates/status', {}, props.token)
    if (requestGeneration === statusRequestGeneration) status.value = data
  } catch (error) {
    if (requestGeneration === statusRequestGeneration) loadError.value = error instanceof Error ? error.message : '更新状态加载失败'
  } finally {
    if (requestGeneration === statusRequestGeneration) loading.value = false
  }
}

async function checkNow() {
  if (requestBusy.value || status.value?.enabled === false) return
  const requestGeneration = ++statusRequestGeneration
  checking.value = true
  loadError.value = ''
  try {
    const data = await request<SystemUpdateStatus>('/api/v1/system/updates/check', {method: 'POST'}, props.token)
    if (requestGeneration !== statusRequestGeneration) return
    status.value = data
    if (statusError.value) ElMessage.warning('检查已完成，但更新源返回了错误')
    else ElMessage.success('更新检查完成')
  } catch (error) {
    if (requestGeneration === statusRequestGeneration) loadError.value = error instanceof Error ? error.message : '立即检查失败'
  } finally {
    if (requestGeneration === statusRequestGeneration) checking.value = false
  }
}

const PackageCard = defineComponent({
  props: {
    title: {type: String, required: true},
    description: {type: String, required: true},
    item: {type: Object as () => UpdatePackageStatus, required: true},
    known: {type: Boolean, required: true},
    downloadLabel: {type: String, default: ''},
    allowDownload: {type: Boolean, default: true},
  },
  emits: ['download'],
  setup(cardProps, {emit}) {
    const version = (value?: string) => value || '—'
    const size = (value?: number) => {
      if (!value) return '—'
      const units = ['B', 'KiB', 'MiB', 'GiB']
      let amount = value
      let unit = 0
      while (amount >= 1024 && unit < units.length - 1) { amount /= 1024; unit += 1 }
      return `${amount.toFixed(unit ? 1 : 0)} ${units[unit]}`
    }
    const canDownload = () => cardProps.allowDownload && Boolean(cardProps.item.download_url || cardProps.item.download_path)
    const downloadUnavailableText = () => !cardProps.allowDownload ? '需要更新管理权限才能下载故障恢复包' : '当前没有可下载的安装包'
    const hasVersionData = () => Boolean(cardProps.item.current_version && cardProps.item.latest_version)
    const packageState = () => {
      if (!cardProps.known) return {type: 'info' as const, label: '状态未知'}
      if (cardProps.item.available === true) return {type: 'warning' as const, label: '发现新版本'}
      if (cardProps.item.available === false && hasVersionData()) return {type: 'success' as const, label: '已是最新'}
      return {type: 'info' as const, label: '状态未知'}
    }
    return () => h('article', {class: 'update-package-card'}, [
      h('div', {class: 'update-card-heading'}, [
        h('div', [h('span', {class: 'update-kicker'}, '安装包'), h('h2', cardProps.title)]),
        h(ElTag, {type: packageState().type, effect: 'light'}, () => packageState().label),
      ]),
      h('p', {class: 'update-package-description'}, cardProps.description),
      h('dl', {class: 'update-version-list'}, [
        h('div', [h('dt', '当前版本'), h('dd', version(cardProps.item.current_version))]),
        h('div', [h('dt', '最新版本'), h('dd', version(cardProps.item.latest_version))]),
        h('div', [h('dt', '文件'), h('dd', cardProps.item.file_name || '—')]),
        h('div', [h('dt', '大小'), h('dd', size(cardProps.item.size))]),
        h('div', [h('dt', '缓存状态'), h('dd', cardProps.item.cached === undefined ? '不适用' : (cardProps.item.cached ? '已校验并缓存' : '尚未缓存'))]),
      ]),
      cardProps.item.message ? h('p', {class: 'update-package-message'}, cardProps.item.message) : null,
      h('div', {class: 'update-actions'}, [
        canDownload() ? h(ElButton, {type: 'primary', plain: true, onClick: () => emit('download', cardProps.item)}, () => cardProps.downloadLabel || (cardProps.title === 'Go 服务端' ? '下载升级包' : '下载客户端')) : h('small', downloadUnavailableText()),
      ]),
    ])
  },
})

onMounted(loadStatus)

defineExpose({reload: loadStatus})
</script>
