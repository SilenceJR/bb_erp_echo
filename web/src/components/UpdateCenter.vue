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
            <h2>{{ status?.enabled === false ? '更新检查已停用' : connectivityText }}</h2>
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
          <el-button :loading="loading" @click="loadStatus">刷新状态</el-button>
          <el-button v-if="canCheck" type="primary" :loading="checking" @click="checkNow">立即检查</el-button>
          <small v-else>你的账号只有查看权限</small>
        </div>
      </article>
    </div>

    <div class="update-package-grid">
      <PackageCard
        title="Go 服务端"
        description="只报告和下载新版本，不会自动替换正在运行的服务。"
        :item="serverStatus"
        @download="openDownload"
      />
      <PackageCard
        title="桌面客户端"
        description="安装包由服务端校验并缓存；Web 用户无需安装客户端升级包。"
        :item="clientStatus"
        @download="openDownload"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import {computed, defineComponent, h, onMounted, ref} from 'vue'
import {ElButton, ElMessage, ElTag} from 'element-plus'
import {downloadApiFile, request} from '../api/http'
import type {SystemUpdateStatus, UpdatePackageStatus} from '../types'

const props = defineProps<{
  token: string
  canCheck: boolean
}>()

const status = ref<SystemUpdateStatus | null>(null)
const loading = ref(false)
const checking = ref(false)
const loadError = ref('')
const manifestUrl = computed(() => String(status.value?.manifest_url || status.value?.source || ''))
const statusError = computed(() => String(status.value?.last_error || status.value?.error || ''))
const serverStatus = computed(() => status.value?.server || status.value?.server_update || {})
const clientStatus = computed(() => status.value?.client || status.value?.client_update || {})
const connectivityText = computed(() => {
  if (status.value?.checking) return '正在检查更新'
  if (statusError.value) return '更新源暂时不可用'
  if (status.value?.last_success_at || status.value?.reachable === true) return '更新源连接正常'
  return '等待首次检查'
})
const connectivityTag = computed(() => {
  if (status.value?.enabled === false) return '已停用'
  if (status.value?.checking) return '检查中'
  if (statusError.value || status.value?.reachable === false) return '连接异常'
  if (status.value?.last_success_at || status.value?.reachable === true) return '正常'
  return '待检查'
})
const connectivityType = computed<'success' | 'warning' | 'info'>(() => {
  if (statusError.value || status.value?.reachable === false) return 'warning'
  if (status.value?.last_success_at || status.value?.reachable === true) return 'success'
  return 'info'
})

function formatTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', {hour12: false})
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
  loading.value = true
  loadError.value = ''
  try {
    status.value = await request<SystemUpdateStatus>('/api/v1/system/updates/status', {}, props.token)
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '更新状态加载失败'
  } finally {
    loading.value = false
  }
}

async function checkNow() {
  checking.value = true
  loadError.value = ''
  try {
    status.value = await request<SystemUpdateStatus>('/api/v1/system/updates/check', {method: 'POST'}, props.token)
    if (statusError.value) ElMessage.warning('检查已完成，但更新源返回了错误')
    else ElMessage.success('更新检查完成')
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '立即检查失败'
  } finally {
    checking.value = false
  }
}

const PackageCard = defineComponent({
  props: {
    title: {type: String, required: true},
    description: {type: String, required: true},
    item: {type: Object as () => UpdatePackageStatus, required: true},
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
    const canDownload = () => Boolean(cardProps.item.download_url || cardProps.item.download_path)
    return () => h('article', {class: 'update-package-card'}, [
      h('div', {class: 'update-card-heading'}, [
        h('div', [h('span', {class: 'update-kicker'}, '安装包'), h('h2', cardProps.title)]),
        h(ElTag, {type: cardProps.item.available ? 'warning' : 'success', effect: 'light'}, () => cardProps.item.available ? '发现新版本' : '已是最新'),
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
        canDownload() ? h(ElButton, {type: 'primary', plain: true, onClick: () => emit('download', cardProps.item)}, () => cardProps.title === 'Go 服务端' ? '下载升级包' : '下载客户端') : h('small', '当前没有可下载的安装包'),
      ]),
    ])
  },
})

onMounted(loadStatus)

defineExpose({reload: loadStatus})
</script>
