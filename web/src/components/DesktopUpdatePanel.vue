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
      <el-tag v-if="displayStrategy" :type="displayStrategy === 'delta' ? 'success' : 'info'" effect="light" round>
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
          <h2>客户端自动更新</h2>
          <p>由桌面端完成下载、签名校验、安装与重启，更新失败时会保留当前版本。</p>
        </div>
        <el-tag :type="statusTone" effect="light">{{ statusLabel }}</el-tag>
      </div>

      <el-alert
        v-if="compatibilityMode"
        title="当前服务端不支持自动更新协议"
        description="可继续使用服务端提供的完整客户端 ZIP；升级服务端后即可使用自动更新。"
        type="info"
        :closable="false"
        show-icon
      />
      <el-alert
        v-if="fallbackReason"
        :title="`增量更新已自动切换为完整更新：${fallbackReason}`"
        type="warning"
        :closable="false"
        show-icon
      />
      <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />

      <dl class="desktop-update-facts">
        <div><dt>当前版本</dt><dd>{{ currentVersion || legacyStatus?.current_version || '—' }}</dd></div>
        <div><dt>目标版本</dt><dd>{{ targetVersion }}</dd></div>
        <div><dt>更新方式</dt><dd>{{ strategyLabel }}</dd></div>
        <div><dt>下载大小</dt><dd>{{ formatBytes(downloadSize) }}</dd></div>
      </dl>

      <div v-if="displayStrategy === 'delta'" class="desktop-update-saving">
        <div>
          <strong>预计节省 {{ savingPercent }}%</strong>
          <span>少下载 {{ formatBytes(savedBytes) }}</span>
        </div>
        <el-progress :percentage="savingPercent" :stroke-width="8" status="success" />
        <small>若增量包校验或应用失败，将自动切换为签名完整安装包，无需再次确认。</small>
      </div>

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
        <small v-if="compatibilityMode">兼容模式只提供完整安装包，不执行自动安装。</small>
        <el-button
          v-if="recoveryAvailable"
          plain
          @click="$emit('download-recovery', legacyStatus)"
        >
          下载完整 ZIP（故障恢复）
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
        <el-alert
          v-if="fallbackReason"
          :title="`增量更新未能继续，已自动改用完整更新：${fallbackReason}`"
          type="warning"
          :closable="false"
          show-icon
        />
        <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
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
}>(), {
  compact: false,
  legacyStatus: () => ({}),
})

const emit = defineEmits<{
  (event: 'download-recovery', item: UpdatePackageStatus): void
}>()

const updater = useDesktopUpdate()
const {
  state, plan, currentVersion, message, error, downloadedBytes, totalBytes,
  activeStrategy, fallbackReason, compatibilityMode, taskInProgress,
  closeLocked, downloadPercent, initialize, check, apply, retry,
} = updater
const progressDialogVisible = ref(false)
const updateStarted = ref(false)
const laterButton = ref<{ $el: HTMLButtonElement } | null>(null)

const targetVersion = computed(() => String(plan.value?.latest_version || plan.value?.version || props.legacyStatus?.latest_version || '—'))
const displayStrategy = computed(() => activeStrategy.value || plan.value?.strategy || null)
const strategyLabel = computed(() => displayStrategy.value === 'delta' ? '增量更新' : displayStrategy.value === 'full' ? '完整更新' : '—')
const downloadSize = computed(() => Number(
  (displayStrategy.value === 'full' ? plan.value?.full_size : plan.value?.download_size)
  || props.legacyStatus?.size
  || 0,
))
const savedBytes = computed(() => Math.max(0, Number(plan.value?.saved_bytes || 0)))
const savingPercent = computed(() => {
  if (plan.value?.saved_percent !== undefined) return Math.min(100, Math.max(0, Math.round(Number(plan.value.saved_percent))))
  const full = Number(plan.value?.full_size || 0)
  return full > 0 ? Math.min(100, Math.max(0, Math.round(savedBytes.value / full * 100))) : 0
})
const recoveryAvailable = computed(() => Boolean(props.legacyStatus?.download_url || props.legacyStatus?.download_path))
const compactVisible = computed(() => Boolean(plan.value || taskInProgress.value || state.value === 'Failed' || fallbackReason.value || (compatibilityMode.value && recoveryAvailable.value)))
const confirmationVisible = computed(() => state.value === 'Ready' && Boolean(plan.value) && !updateStarted.value)
const idleMessage = computed(() => compatibilityMode.value ? '当前服务端仅支持完整包更新' : '当前客户端已是最新版本')
const statusLabel = computed(() => {
  if (state.value === 'Ready') return '发现新版本'
  if (state.value === 'Failed') return '更新失败'
  if (taskInProgress.value) return '更新进行中'
  if (compatibilityMode.value) return '兼容模式'
  return '已是最新'
})
const statusTone = computed<'success' | 'warning' | 'danger' | 'info'>(() => {
  if (state.value === 'Failed') return 'danger'
  if (state.value === 'Ready' || fallbackReason.value) return 'warning'
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
  if (fallbackReason.value) return '增量失败，正在自动改用完整更新'
  if (state.value === 'Failed') return error.value
  if (compatibilityMode.value) return '旧服务端兼容模式'
  return `${strategyLabel.value} · ${formatBytes(downloadSize.value)}`
})
const compactAction = computed(() => {
  if (state.value === 'Ready' && plan.value) return '立即更新'
  if (state.value === 'Failed') return '重试'
  if (taskInProgress.value) return '查看进度'
  if (compatibilityMode.value && recoveryAvailable.value) return '下载完整包'
  return ''
})
const activeStep = computed(() => ({
  Idle: 0, Checking: 0, Ready: 0, Downloading: 1, Verifying: 2, Applying: 3, Restarting: 4, Failed: 0,
})[state.value])
const stageLabel = computed(() => ({
  Idle: '等待更新', Checking: '正在检查更新', Ready: '更新已准备就绪', Downloading: '正在下载更新',
  Verifying: '正在校验更新', Applying: '正在安装更新', Restarting: '正在重启客户端', Failed: '更新失败',
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
}

async function retryUpdate() {
  updateStarted.value = Boolean(plan.value)
  progressDialogVisible.value = true
  await retry()
}

function handleCompactAction() {
  if (state.value === 'Ready' && plan.value) openConfirmation()
  else if (state.value === 'Failed') void retryUpdate()
  else if (taskInProgress.value) progressDialogVisible.value = true
  else if (compatibilityMode.value && recoveryAvailable.value) {
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

onMounted(async () => {
  await initialize()
  if ((state.value === 'Idle' && !message.value) || (state.value === 'Ready' && !plan.value)) await check()
})
</script>
