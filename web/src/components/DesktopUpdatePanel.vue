<template>
  <section
    v-if="!compact || compactVisible"
    class="desktop-update-panel"
    :class="{'is-compact': compact}"
    :aria-label="compact ? '桌面客户端更新' : undefined"
  >
    <template v-if="compact">
      <div class="desktop-update-compact__copy" role="status" aria-live="polite">
        <strong>{{ compactTitle }}</strong>
        <span>{{ compactDescription }}</span>
      </div>
      <el-tag v-if="displayStrategy" type="info" effect="light" round>
        {{ strategyLabel }}
      </el-tag>
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
        :loading="recoveryDownloading && compatibilityMode"
        :disabled="closeLocked || recoveryDownloading"
        @click="handleCompactAction"
      >
        {{ compactAction }}
      </el-button>
    </template>

    <template v-else>
      <div class="desktop-update-heading">
        <div>
          <span class="update-kicker">桌面客户端</span>
          <h2>客户端自动更新</h2>
          <p>由桌面端完成下载、签名校验、安装与重启，更新失败时会保留当前版本。</p>
        </div>
        <el-tag :type="statusTone" effect="light">{{ statusLabel }}</el-tag>
      </div>

      <el-alert
        v-if="compatibilityMode"
        title="服务器尚未提供客户端完整更新包"
        description="请检查 ERP 服务器上的发布计划任务是否成功，再重新检查更新；必要时可下载故障恢复 ZIP。"
        type="info"
        :closable="false"
        show-icon
      />
      <el-alert v-if="error" :title="error" :description="errorGuidance" type="error" :closable="false" show-icon />
      <el-alert v-if="recoveryDownloadError && canDownloadRecovery" :title="recoveryDownloadError" description="请确认局域网服务可用且发布状态正常后重试下载。" type="error" :closable="false" show-icon />

      <dl class="desktop-update-facts">
        <div><dt>当前版本</dt><dd>{{ currentVersion || legacyStatus?.current_version || '—' }}</dd></div>
        <div><dt>目标版本</dt><dd>{{ targetVersion }}</dd></div>
        <div><dt>更新方式</dt><dd>{{ strategyLabel }}</dd></div>
        <div><dt>下载大小</dt><dd>{{ formatBytes(downloadSize) }}</dd></div>
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
        <small v-if="recoveryAvailable && !canDownloadRecovery">故障恢复包仅限更新管理员下载，请联系管理员处理。</small>
        <small v-else-if="compatibilityMode">自动安装暂不可用，可下载完整 ZIP 进行故障恢复。</small>
        <el-button
          v-if="recoveryActionAvailable"
          plain
          :loading="recoveryDownloading"
          :disabled="recoveryDownloading"
          @click="$emit('download-recovery', legacyStatus)"
        >
          {{ recoveryDownloadError ? '重试下载完整 ZIP（故障恢复）' : '下载完整 ZIP（故障恢复）' }}
        </el-button>
        <el-button v-if="state === 'Ready' && plan" type="primary" @click="openConfirmation">立即更新</el-button>
        <el-button v-else-if="state === 'Failed'" type="primary" @click="retryUpdate">重试</el-button>
        <el-button v-else-if="taskInProgress" type="primary" plain @click="progressDialogVisible = true">查看进度</el-button>
        <el-button v-else :loading="state === 'Checking'" :disabled="taskInProgress" @click="check">检查客户端更新</el-button>
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
            <div><dt>更新方式</dt><dd>{{ strategyLabel }}</dd></div>
            <div><dt>下载大小</dt><dd>{{ formatBytes(downloadSize) }}</dd></div>
          </dl>
          <el-alert
            title="更新过程中应用会关闭，并在安装完成后自动重新启动。请先保存正在编辑的内容。"
            type="warning"
            :closable="false"
            show-icon
          />
        </div>
      </template>
      <template v-else>
        <el-alert v-if="error" :title="error" :description="errorGuidance" type="error" :closable="false" show-icon />
        <el-steps class="desktop-update-steps" :active="activeStep" finish-status="success" align-center>
          <el-step title="检查" />
          <el-step title="下载" />
          <el-step title="校验" />
          <el-step title="安装" />
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
            <el-button v-if="state === 'Failed'" type="primary" @click="retryUpdate">重试</el-button>
            <small v-if="closeLocked">正在替换并重启客户端，请勿关闭此窗口</small>
          </template>
        </div>
      </template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import {computed, nextTick, onMounted, ref, watch} from 'vue'
import type {UpdatePackageStatus} from '../types'
import {useDesktopUpdate} from '../composables/useDesktopUpdate'

const props = withDefaults(defineProps<{
  compact?: boolean
  legacyStatus?: UpdatePackageStatus
  recoveryDownloading?: boolean
  recoveryDownloadError?: string
  canDownloadRecovery?: boolean
}>(), {
  compact: false,
  legacyStatus: () => ({}),
  recoveryDownloading: false,
  recoveryDownloadError: '',
  canDownloadRecovery: false,
})

const emit = defineEmits<{
  (event: 'download-recovery', item: UpdatePackageStatus): void
}>()

const updater = useDesktopUpdate()
const {
  state, plan, currentVersion, message, error, downloadedBytes, totalBytes,
  activeStrategy, failedStage, compatibilityMode, taskInProgress,
  closeLocked, downloadPercent, initialize, check, apply, retry,
} = updater
const progressDialogVisible = ref(false)
const updateStarted = ref(false)
const laterButton = ref<{ $el: HTMLButtonElement } | null>(null)

const targetVersion = computed(() => String(plan.value?.latest_version || plan.value?.version || props.legacyStatus?.latest_version || '—'))
const displayStrategy = computed(() => activeStrategy.value || plan.value?.strategy || null)
const strategyLabel = computed(() => displayStrategy.value ? '完整更新' : '—')
const downloadSize = computed(() => Number(
  plan.value?.download_size
  || plan.value?.full_size
  || props.legacyStatus?.size
  || 0,
))
const recoveryAvailable = computed(() => Boolean(props.legacyStatus?.download_url || props.legacyStatus?.download_path))
const recoveryActionAvailable = computed(() => props.canDownloadRecovery && recoveryAvailable.value)
const compactVisible = computed(() => Boolean(plan.value || taskInProgress.value || state.value === 'Failed' || (compatibilityMode.value && recoveryAvailable.value)))
const confirmationVisible = computed(() => state.value === 'Ready' && Boolean(plan.value) && !updateStarted.value)
const idleMessage = computed(() => compatibilityMode.value ? '服务器尚未准备客户端更新' : '当前客户端已是最新版本')
const errorGuidance = computed(() => {
  if (error.value.includes('恢复旧版本失败')) return props.canDownloadRecovery
    ? '请停止继续更新并下载故障恢复 ZIP；如客户端无法正常启动，请由管理员人工恢复或重新安装。'
    : '请停止继续更新，并联系更新管理员下载故障恢复包或协助重新安装。'
  if (error.value.includes('已恢复旧版本')) return '当前客户端已恢复到旧版本。请确认服务器发布包和局域网状态后，再重新检查并确认更新。'
  if (error.value.includes('安装目录不可写')) return props.canDownloadRecovery
    ? '请下载故障恢复 ZIP，或将客户端重新安装到当前账号可写目录后再检查更新。'
    : '请联系更新管理员下载故障恢复包，或协助将客户端重新安装到当前账号可写目录。'
  if (plan.value) return '完整包下载或安装未完成，请确认局域网连接稳定、安装目录可写后重试。'
  return '请核对 ERP 服务器 IP 和端口，确认客户端与服务器在同一局域网、防火墙已放行且 Go 服务正在运行；若更新包尚未就绪，请检查服务器发布计划任务。'
})
const statusLabel = computed(() => {
  if (state.value === 'Ready') return '发现新版本'
  if (state.value === 'Failed') return '更新失败'
  if (taskInProgress.value) return '更新进行中'
  if (compatibilityMode.value) return '更新包未就绪'
  return '已是最新'
})
const statusTone = computed<'success' | 'warning' | 'danger' | 'info'>(() => {
  if (state.value === 'Failed') return 'danger'
  if (state.value === 'Ready') return 'warning'
  if (taskInProgress.value) return 'info'
  return compatibilityMode.value ? 'info' : 'success'
})
const compactTitle = computed(() => {
  if (state.value === 'Failed') return '客户端更新未完成'
  if (taskInProgress.value) return stageLabel.value
  if (compatibilityMode.value) return `客户端 ${targetVersion.value} 可下载`
  return `客户端 ${targetVersion.value} 可更新`
})
const compactDescription = computed(() => {
  if (state.value === 'Downloading') return '正在下载安装包'
  if (state.value === 'Failed') return error.value
  if (props.recoveryDownloadError) return props.recoveryDownloadError
  if (compatibilityMode.value && recoveryAvailable.value && !props.canDownloadRecovery) return '故障恢复包仅限更新管理员下载，请联系管理员处理'
  if (compatibilityMode.value) return '服务器尚未准备完整更新包'
  return `${strategyLabel.value} · ${formatBytes(downloadSize.value)}`
})
const compactAction = computed(() => {
  if (state.value === 'Ready' && plan.value) return '立即更新'
  if (state.value === 'Failed') return '重试'
  if (taskInProgress.value) return '查看进度'
  if (compatibilityMode.value && recoveryActionAvailable.value) return props.recoveryDownloadError ? '重试下载完整包' : '下载完整包'
  return ''
})
const phaseStep = {Idle: 0, Checking: 0, Ready: 0, Downloading: 1, Verifying: 2, Applying: 3, Restarting: 4} as const
const activeStep = computed(() => state.value === 'Failed' ? phaseStep[failedStage.value || 'Checking'] : phaseStep[state.value])
const failedStageLabel = computed(() => ({
  Idle: '准备', Checking: '检查', Ready: '确认', Downloading: '下载', Verifying: '校验', Applying: '安装', Restarting: '重启',
})[failedStage.value || 'Checking'])
const stageLabel = computed(() => ({
  Idle: '等待更新', Checking: '正在检查更新', Ready: '更新已准备就绪', Downloading: '正在下载更新',
  Verifying: '正在校验更新', Applying: '正在安装更新', Restarting: '正在重启客户端', Failed: `${failedStageLabel.value}失败`,
})[state.value])
const stageNote = computed(() => ({
  Idle: '尚未开始更新。', Checking: '正在获取并验证服务器提供的更新计划。', Ready: '确认后将开始下载。',
  Verifying: '正在校验签名和文件完整性，此阶段不显示虚拟百分比。',
  Applying: '正在安全替换客户端文件，此阶段不可关闭窗口。',
  Restarting: '新版本正在启动；若启动失败，客户端会自动恢复旧版本。',
  Failed: '请查看错误信息并重试。', Downloading: '',
})[state.value])

function formatBytes(value?: number): string {
  const amount = Number(value || 0)
  if (amount <= 0) return '—'
  const units = ['B', 'KiB', 'MiB', 'GiB']
  let size = amount
  let unit = 0
  while (size >= 1024 && unit < units.length - 1) { size /= 1024; unit += 1 }
  return `${size.toFixed(unit ? 1 : 0)} ${units[unit]}`
}

function openConfirmation() {
  updateStarted.value = false
  progressDialogVisible.value = true
}

async function startUpdate() {
  updateStarted.value = true
  await apply()
  if (state.value === 'Ready' && plan.value) updateStarted.value = false
}

async function retryUpdate() {
  updateStarted.value = Boolean(plan.value)
  progressDialogVisible.value = true
  await retry()
  if (state.value === 'Ready' && plan.value) updateStarted.value = false
}

function handleCompactAction() {
  if (state.value === 'Ready' && plan.value) openConfirmation()
  else if (state.value === 'Failed') void retryUpdate()
  else if (taskInProgress.value) progressDialogVisible.value = true
  else if (compatibilityMode.value && recoveryActionAvailable.value) {
    emit('download-recovery', props.legacyStatus)
  }
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

watch(confirmationVisible, (visible) => {
  if (visible) void focusLaterAction()
})

onMounted(async () => {
  await initialize()
  if ((state.value === 'Idle' && !message.value) || (state.value === 'Ready' && !plan.value)) await check()
})
</script>
