<template>
  <section
    v-if="desktopAvailable && (!compact || compactVisible)"
    class="desktop-update-panel"
    :class="{'is-compact': compact}"
    :aria-label="compact ? '桌面客户端更新提示' : '客户端更新'"
  >
    <template v-if="compact">
      <div class="desktop-update-compact__copy" role="status" aria-live="polite">
        <strong>{{ compactTitle }}</strong>
        <span>{{ compactDescription }}</span>
      </div>
      <el-tag v-if="state === 'Ready'" type="warning" effect="plain" round>可更新</el-tag>
      <el-tag v-else-if="state === 'RolledBack'" type="danger" effect="plain" round>已回滚</el-tag>
      <el-progress
        v-if="downloadPercent !== null"
        class="desktop-update-compact__progress"
        :percentage="downloadPercent"
        :stroke-width="6"
        :show-text="false"
        :aria-label="`客户端更新已下载 ${downloadPercent}%`"
      />
      <el-button
        v-if="compactAction"
        type="primary"
        link
        :disabled="closeLocked"
        @click="handleCompactAction"
      >
        {{ compactAction }}
      </el-button>
    </template>

    <template v-else>
      <div class="desktop-update-heading">
        <div>
          <span class="update-kicker">桌面客户端</span>
          <h2>客户端更新</h2>
          <p>从已验证的内网服务器下载单 EXE 完整更新，校验通过后自动重启。</p>
        </div>
        <el-tag :type="statusTone" effect="light">{{ statusLabel }}</el-tag>
      </div>

      <el-alert
        v-if="error"
        :title="errorTitle"
        :description="errorDescription"
        :type="errorTone"
        :closable="false"
        show-icon
      />

      <dl class="desktop-update-facts">
        <div><dt>当前版本</dt><dd>{{ currentVersion || '—' }}</dd></div>
        <div><dt>目标版本</dt><dd>{{ targetVersion }}</dd></div>
        <div><dt>更新方式</dt><dd>Windows x64 · 单 EXE</dd></div>
        <div><dt>下载大小</dt><dd>{{ formatBytes(downloadSize) }}</dd></div>
        <div><dt>来源服务器</dt><dd>{{ serverOrigin || '已验证服务器' }}</dd></div>
        <div><dt>最近检查</dt><dd>{{ lastCheckedText }}</dd></div>
      </dl>

      <div class="desktop-update-status" role="status" aria-live="polite" aria-atomic="true">
        <strong>{{ message || idleMessage }}</strong>
      </div>
      <span v-if="state === 'Downloading' && totalBytes > 0" class="desktop-update-byte-summary" aria-hidden="true">
        {{ formatBytes(downloadedBytes) }} / {{ formatBytes(totalBytes) }}
      </span>
      <el-progress
        v-if="downloadPercent !== null"
        :percentage="downloadPercent"
        :stroke-width="10"
        :aria-label="`客户端下载进度 ${downloadPercent}%`"
      />

      <div class="update-actions desktop-update-actions">
        <el-button v-if="state === 'Ready' && plan" @click="check">重新检查</el-button>
        <el-button v-if="state === 'Ready' && plan" type="primary" @click="openConfirmation">立即更新</el-button>
        <el-button v-else-if="state === 'Failed' || state === 'RolledBack'" type="primary" @click="retryUpdate">重新检查</el-button>
        <el-button v-else-if="taskInProgress" type="primary" plain @click="progressDialogVisible = true">查看进度</el-button>
        <el-button v-else :loading="state === 'Checking'" :disabled="taskInProgress" @click="check">检查更新</el-button>
      </div>
    </template>

    <el-dialog
      v-model="progressDialogVisible"
      class="desktop-update-dialog"
      width="min(560px, calc(100vw - 24px))"
      :title="confirmationVisible ? '确认客户端更新' : '客户端更新进度'"
      :show-close="!closeLocked"
      :close-on-click-modal="!closeLocked"
      :close-on-press-escape="!closeLocked"
      :before-close="beforeDialogClose"
      append-to-body
      @opened="focusLaterAction"
    >
      <template v-if="confirmationVisible">
        <div class="desktop-update-confirmation">
          <p>将客户端从 <strong>{{ currentVersion || '当前版本' }}</strong> 更新到 <strong>{{ targetVersion }}</strong>。</p>
          <dl>
            <div><dt>更新方式</dt><dd>Windows x64 单 EXE</dd></div>
            <div><dt>下载大小</dt><dd>{{ formatBytes(downloadSize) }}</dd></div>
          </dl>
          <el-alert
            title="更新过程中应用会关闭，并在校验完成后自动重新启动。请先保存正在编辑的内容。"
            type="warning"
            :closable="false"
            show-icon
          />
        </div>
      </template>
      <template v-else>
        <el-alert v-if="error" :title="errorTitle" :description="errorDescription" :type="errorTone" :closable="false" show-icon />
        <el-steps class="desktop-update-steps" :active="activeStep" finish-status="success" align-center>
          <el-step title="检查" />
          <el-step title="下载" />
          <el-step title="校验" />
          <el-step title="替换" />
          <el-step title="重启" />
        </el-steps>
        <div class="desktop-update-dialog__status" role="status" aria-live="polite" aria-atomic="true">
          <strong>{{ stageLabel }}</strong>
          <span>{{ message }}</span>
        </div>
        <template v-if="state === 'Downloading'">
          <el-progress
            v-if="downloadPercent !== null"
            :percentage="downloadPercent"
            :stroke-width="12"
            :aria-label="`客户端下载进度 ${downloadPercent}%`"
          />
          <p v-if="totalBytes > 0" class="desktop-update-byte-progress">{{ formatBytes(downloadedBytes) }} / {{ formatBytes(totalBytes) }}</p>
          <p v-else class="desktop-update-byte-progress">正在等待服务器返回下载进度…</p>
        </template>
        <p v-else class="desktop-update-stage-note">{{ stageNote }}</p>
      </template>

      <template #footer>
        <div class="desktop-update-dialog__footer">
          <template v-if="confirmationVisible">
            <el-button ref="laterButton" class="desktop-update-later" @click="progressDialogVisible = false">稍后处理</el-button>
            <el-button type="primary" @click="startUpdate">开始更新并重启</el-button>
          </template>
          <template v-else>
            <el-button v-if="!closeLocked" @click="progressDialogVisible = false">关闭</el-button>
            <el-button v-if="state === 'Failed' || state === 'RolledBack'" type="primary" @click="retryUpdate">重新检查</el-button>
            <small v-if="closeLocked">正在替换并重启客户端，请勿关闭此窗口</small>
          </template>
        </div>
      </template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import {computed, nextTick, onMounted, ref, watch} from 'vue'
import {useDesktopUpdate} from '../composables/useDesktopUpdate'

const props = withDefaults(defineProps<{compact?: boolean}>(), {compact: false})
const updater = useDesktopUpdate()
const {
  desktopAvailable, state, plan, currentVersion, message, error, errorCode, requestId,
  downloadedBytes, totalBytes, lastCheckedAt, serverOrigin, taskInProgress,
  closeLocked, downloadPercent, initialize, check, apply, retry,
} = updater

const progressDialogVisible = ref(false)
const updateStarted = ref(false)
const laterButton = ref<{ $el: HTMLButtonElement } | null>(null)

const targetVersion = computed(() => String(plan.value?.latest_version || '—'))
const downloadSize = computed(() => Number(plan.value?.download_size || 0))
const compactVisible = computed(() => Boolean((state.value === 'Ready' && plan.value) || taskInProgress.value || state.value === 'RolledBack'))
const confirmationVisible = computed(() => state.value === 'Ready' && Boolean(plan.value) && !updateStarted.value)
const idleMessage = computed(() => updater.hasChecked.value ? '当前客户端已是最新版本' : '尚未检查客户端更新')
const statusLabel = computed(() => {
  if (state.value === 'Ready') return '发现新版本'
  if (state.value === 'Updated') return '更新完成'
  if (state.value === 'RolledBack') return '已回滚'
  if (state.value === 'Failed') return '检查失败'
  if (taskInProgress.value) return '更新进行中'
  return updater.hasChecked.value ? '已是最新' : '未检查'
})
const statusTone = computed<'success' | 'warning' | 'danger' | 'info'>(() => {
  if (state.value === 'Failed' || state.value === 'RolledBack') return 'danger'
  if (state.value === 'Updated') return 'success'
  if (state.value === 'Ready') return 'warning'
  if (taskInProgress.value) return 'info'
  return updater.hasChecked.value ? 'success' : 'info'
})
const errorTone = computed<'error' | 'warning'>(() => errorCode.value === 'rolled_back' ? 'warning' : 'error')
const errorTitle = computed(() => ({
  server_unreachable: '服务器不可达',
  server_unavailable: '更新服务暂不可用',
  not_published: '服务器尚未发布客户端更新',
  integrity_failure: '更新完整性校验失败',
  directory_not_writable: '客户端目录不可写',
  unsupported_platform: '当前环境不支持单 EXE 更新',
  plan_changed: '更新计划已变化',
  rolled_back: '新版启动失败，已恢复旧版本',
} as Record<string, string>)[errorCode.value || ''] || '客户端更新失败')
const errorDescription = computed(() => {
  const suffix = requestId.value ? ` 请求 ID：${requestId.value}` : ''
  if (errorCode.value === 'directory_not_writable') return `${error.value || '请将客户端复制到本机可写目录后重试。'}${suffix}`
  if (errorCode.value === 'not_published') return `${error.value || '请联系管理员把新版 EXE 和清单放入服务器 client 目录。'}${suffix}`
  return `${error.value || '请稍后重试。'}${suffix}`
})
const compactTitle = computed(() => {
  if (state.value === 'RolledBack') return '客户端更新已回滚'
  if (taskInProgress.value) return stageLabel.value
  return `客户端 ${targetVersion.value} 可更新`
})
const compactDescription = computed(() => {
  if (state.value === 'Downloading') return '正在下载安装包'
  if (state.value === 'RolledBack') return '已恢复旧版本，请重新检查'
  return `单 EXE 完整更新 · ${formatBytes(downloadSize.value)}`
})
const compactAction = computed(() => {
  if (state.value === 'Ready' && plan.value) return '立即更新'
  if (state.value === 'RolledBack') return '重新检查'
  if (taskInProgress.value) return '查看进度'
  return ''
})
const activeStep = computed(() => ({
  Idle: 0, Checking: 0, Ready: 0, Downloading: 1, Verifying: 2, Applying: 3, Restarting: 4, Updated: 4, RolledBack: 0, Failed: 0,
})[state.value])
const stageLabel = computed(() => ({
  Idle: '等待更新', Checking: '正在检查更新', Ready: '更新已准备就绪', Downloading: '正在下载更新',
  Verifying: '正在校验更新', Applying: '正在替换客户端', Restarting: '正在重启客户端', Updated: '客户端已更新',
  RolledBack: '已回滚旧版本', Failed: '更新失败',
})[state.value])
const stageNote = computed(() => ({
  Idle: '尚未开始更新。', Checking: '正在获取并验证服务器提供的更新计划。', Ready: '确认后将开始下载。',
  Verifying: '正在校验签名和文件完整性，此阶段不显示虚拟百分比。',
  Applying: '正在安全替换客户端文件，此阶段不可关闭窗口。',
  Restarting: '新版本正在启动；若启动失败，客户端会自动恢复旧版本。',
  Updated: '客户端已成功更新，当前正在使用新版。',
  RolledBack: '旧版本已恢复，可以重新检查服务器上的客户端更新。',
  Failed: '请查看错误信息并重新检查。', Downloading: '',
})[state.value])
const lastCheckedText = computed(() => lastCheckedAt.value ? formatDate(lastCheckedAt.value) : '尚未检查')

function formatBytes(value?: number): string {
  const amount = Number(value || 0)
  if (amount <= 0) return '—'
  const units = ['B', 'KiB', 'MiB', 'GiB']
  let size = amount
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit += 1 }
  return `${size.toFixed(unit ? 1 : 0)} ${units[unit]}`
}

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', {hour12: false})
}

function openConfirmation() {
  updateStarted.value = false
  progressDialogVisible.value = true
}

async function startUpdate() {
  updateStarted.value = true
  await apply()
}

async function retryUpdate() {
  updateStarted.value = Boolean(plan.value && state.value === 'Ready')
  progressDialogVisible.value = true
  await retry()
}

function handleCompactAction() {
  if (state.value === 'Ready' && plan.value) openConfirmation()
  else if (state.value === 'RolledBack') void retryUpdate()
  else if (taskInProgress.value) progressDialogVisible.value = true
}

function beforeDialogClose(done: () => void) {
  if (!closeLocked.value) done()
}

async function focusLaterAction() {
  if (!confirmationVisible.value) return
  await nextTick()
  laterButton.value?.$el?.focus()
}

watch(taskInProgress, (running) => {
  if (running && updateStarted.value) progressDialogVisible.value = true
})

onMounted(() => {
  void initialize()
})
</script>
