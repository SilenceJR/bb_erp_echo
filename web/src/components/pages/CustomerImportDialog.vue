<template>
  <el-dialog
    v-model="visible"
    class="customer-excel-dialog"
    title="导入客户资料"
    width="min(760px, 94vw)"
    :close-on-click-modal="!busy"
    :close-on-press-escape="!busy"
    :before-close="beforeClose"
    destroy-on-close
    @closed="reset"
  >
    <div v-if="visible" :key="`import-step-${step}`" class="customer-dialog-motion">
        <el-steps :active="step" finish-status="success" simple class="excel-steps">
          <el-step title="准备文件" />
          <el-step title="校验预览" />
          <el-step title="导入完成" />
        </el-steps>

    <section v-if="step === 0" class="import-step">
      <div class="import-guide">
        <span class="step-number">1</span>
        <div><h3>下载标准模板</h3><p>支持 .xls 和 .xlsx，首个工作表、最多 10,000 条、文件不超过 10 MiB。</p></div>
        <el-button :loading="templateLoading" @click="downloadTemplate">下载导入模板</el-button>
      </div>
      <div class="import-guide import-file-guide">
        <span class="step-number">2</span>
        <div><h3>拖放或选择 Excel 文件</h3><p>重复编码会形成多条客户资料；任一行错误都不会写入数据库。</p></div>
        <div class="customer-import-dropzone" data-file-drop-target :class="{'is-dragging': dragging}" role="button" tabindex="0" @click="openFilePicker" @keydown.enter.prevent="openFilePicker" @keydown.space.prevent="openFilePicker" @dragenter.prevent="handleDragEnter" @dragover.prevent="handleDragOver" @dragleave.prevent="handleDragLeave" @drop.prevent="handleDrop" @bb-native-file-drag="handleNativeFileDrag"><strong>{{ dragging ? '松开以选择文件' : '拖入 Excel 文件' }}</strong><el-button type="primary" plain @click.stop="openFilePicker">选择文件</el-button></div>
        <input ref="fileInput" class="bb-sr-only" type="file" accept=".xls,.xlsx" @change="selectFile" />
      </div>
      <div v-if="file" class="selected-file">
        <div><strong>{{ file.name }}</strong><small>{{ formatBytes(file.size) }}</small></div>
        <el-button link type="danger" @click="clearFile">移除</el-button>
      </div>
      <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
        </section>

    <section v-else-if="step === 1" class="import-step" aria-live="polite">
      <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
      <template v-if="preview">
        <div class="import-summary-grid">
          <article><span>数据行</span><strong>{{ preview.summary.total_rows }}</strong></article>
          <article><span>新编码</span><strong>{{ preview.summary.new_codes }}</strong></article>
          <article><span>新资料</span><strong>{{ preview.summary.new_profiles }}</strong></article>
          <article><span>多资料编码组</span><strong>{{ preview.summary.multiple_code_groups }}</strong></article>
        </div>
        <el-alert
          v-if="!preview.errors.length"
          title="校验通过，本次导入将整批写入"
          :description="expiryText"
          type="success"
          :closable="false"
          show-icon
        />
        <section v-else class="import-errors" aria-label="导入错误">
          <div class="section-heading"><div><h3>发现 {{ preview.errors.length }} 个错误</h3><p>请修正原文件后重新选择，当前数据未写入。</p></div></div>
          <div class="import-error-table">
            <div v-for="(item, index) in preview.errors" :key="`${item.row}-${item.column}-${index}`">
              <strong>{{ item.row ? `第 ${item.row} 行` : '文件' }} · {{ item.column }}</strong>
              <span>{{ item.reason }}</span>
              <small v-if="item.value">原值：{{ item.value }}</small>
            </div>
          </div>
        </section>
      </template>
    </section>

    <section v-else class="import-result" role="status">
      <span aria-hidden="true">✓</span>
      <h3>客户资料导入完成</h3>
      <p>已新增 {{ result?.imported_codes || 0 }} 个客户编码、{{ result?.imported_profiles || 0 }} 条客户资料。</p>
    </section>
    </div>

    <template #footer>
      <div class="dialog-actions">
        <el-button v-if="step === 0" :disabled="busy" @click="visible = false">取消</el-button>
        <el-button v-if="step === 0" type="primary" :loading="previewLoading" :disabled="!file" @click="loadPreview">校验并预览</el-button>
        <el-button v-if="step === 1" :disabled="busy" @click="backToFile">重新选择</el-button>
        <el-button v-if="step === 1" type="primary" :loading="commitLoading" :disabled="!preview?.token || Boolean(preview?.errors.length)" @click="commit">确认导入</el-button>
        <el-button v-if="step === 2" type="primary" @click="visible = false">完成</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import {computed, ref} from 'vue'
import {ElMessage} from 'element-plus'
import {downloadApiFile, request, uploadNativeFiles} from '../../api/http'
import {appMessageBox} from '../../composables/useAppMessageBox'
import {useDirtyGuard} from '../../composables/useDirtyGuard'
import type {CustomerImportPreview, NativeFileDragDetail} from '../../types'

const props = defineProps<{modelValue: boolean; token: string}>()
const emit = defineEmits<{(event: 'update:modelValue', value: boolean): void; (event: 'completed'): void}>()
const visible = computed({get: () => props.modelValue, set: (value) => emit('update:modelValue', value)})
const step = ref(0)
const file = ref<File | null>(null)
const nativeFilePath = ref<string | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const preview = ref<CustomerImportPreview | null>(null)
const result = ref<{imported_codes: number; imported_profiles: number; completed_at: string} | null>(null)
const previewLoading = ref(false)
const commitLoading = ref(false)
const templateLoading = ref(false)
const error = ref('')
const dragging = ref(false)
let dragDepth = 0
const busy = computed(() => previewLoading.value || commitLoading.value || templateLoading.value)
const expiryText = computed(() => preview.value?.expires_at ? `预览令牌将于 ${new Date(preview.value.expires_at).toLocaleString('zh-CN', {hour12: false})} 过期，提交时会重新校验同一文件。` : '')

useDirtyGuard('customer-import', {
  busy: () => props.modelValue && busy.value,
  dirty: () => props.modelValue && (file.value !== null || step.value > 0),
  busyMessage: '客户资料导入正在处理，请等待完成后再离开',
  dirtyMessage: '当前客户导入文件或校验结果尚未完成，离开后不会保留。',
})

function openFilePicker() { if (!busy.value) fileInput.value?.click() }
function hasDraggedFiles(event: DragEvent): boolean { return Array.from(event.dataTransfer?.types || []).includes('Files') }
function handleDragEnter(event: DragEvent) { if (!hasDraggedFiles(event)) return; dragDepth++; dragging.value = true }
function handleDragOver(event: DragEvent) { if (hasDraggedFiles(event)) event.dataTransfer!.dropEffect = 'copy' }
function handleDragLeave() { dragDepth = Math.max(0, dragDepth - 1); if (!dragDepth) dragging.value = false }
function handleDrop(event: DragEvent) { dragDepth = 0; dragging.value = false; const files = Array.from(event.dataTransfer?.files || []); if (files.length !== 1) { error.value = '请一次拖入一个 Excel 文件'; return }; acceptFile(files[0]) }
function handleNativeFileDrag(event: Event) { const detail = (event as CustomEvent<NativeFileDragDetail>).detail; if (!detail) return; if (detail.phase === 'enter' || detail.phase === 'over') { if (!busy.value) dragging.value = true; return }; dragging.value = false; if (detail.phase !== 'drop' || busy.value) return; if (detail.error) { error.value = detail.error; return }; if (detail.paths.length !== 1 && detail.files.length !== 1) { error.value = '请一次拖入一个 Excel 文件'; return }; if (detail.paths.length === 1) { const path = detail.paths[0]; acceptFile(new File([], path.split(/[\\/]/).pop() || '拖入文件')); nativeFilePath.value = path; return }; acceptFile(detail.files[0]) }
function selectFile(event: Event) {
  const input = event.target as HTMLInputElement
  const selected = input.files?.[0] || null
  input.value = ''
  if (selected) acceptFile(selected)
}
function acceptFile(selected: File) {
  nativeFilePath.value = null
  error.value = ''
  if (!/\.(xls|xlsx)$/i.test(selected.name)) { error.value = '仅支持 .xls 或 .xlsx 文件'; clearFile(); return }
  if (selected.size > 10 * 1024 * 1024) { error.value = '文件不能超过 10 MiB'; clearFile(); return }
  file.value = selected
}
function clearFile() { file.value = null; nativeFilePath.value = null; if (fileInput.value) fileInput.value.value = '' }
function formatBytes(size: number) { return size < 1024 * 1024 ? `${(size / 1024).toFixed(1)} KiB` : `${(size / 1024 / 1024).toFixed(1)} MiB` }

async function downloadTemplate() {
  templateLoading.value = true
  try {
    const result = await downloadApiFile('/api/v1/customers/import-template', '客户资料导入模板.xlsx', props.token)
    if (result.status === 'error') throw new Error(result.message)
    if (result.status === 'saved') ElMessage.success(result.path ? `模板已保存到：${result.path}` : '浏览器已开始下载模板')
    if (result.status === 'cancelled') ElMessage.info('已取消保存导入模板')
  }
  catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '模板下载失败') }
  finally { templateLoading.value = false }
}
async function loadPreview() {
  if (!file.value) return
  previewLoading.value = true
  error.value = ''
  try {
    if (nativeFilePath.value) preview.value = await uploadNativeFiles<CustomerImportPreview>('/api/v1/customers/import/preview', [nativeFilePath.value], {}, props.token)
    else { const form = new FormData(); form.append('file', file.value); preview.value = await request<CustomerImportPreview>('/api/v1/customers/import/preview', {method: 'POST', body: form}, props.token) }
    step.value = 1
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '导入预览失败' }
  finally { previewLoading.value = false }
}
async function commit() {
  if (!file.value || !preview.value?.token) return
  commitLoading.value = true
  error.value = ''
  try {
    if (nativeFilePath.value) result.value = await uploadNativeFiles('/api/v1/customers/import/commit', [nativeFilePath.value], {token: preview.value.token}, props.token)
    else { const form = new FormData(); form.append('file', file.value); form.append('token', preview.value.token); result.value = await request('/api/v1/customers/import/commit', {method: 'POST', body: form}, props.token) }
    step.value = 2
    emit('completed')
    ElMessage.success('客户资料导入成功')
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '导入失败' }
  finally { commitLoading.value = false }
}
function backToFile() { preview.value = null; error.value = ''; step.value = 0 }
function reset() { step.value = 0; preview.value = null; result.value = null; error.value = ''; dragging.value = false; dragDepth = 0; clearFile() }
async function beforeClose(done: () => void) {
  if (busy.value) return
  if (step.value === 1 && preview.value?.token) {
    try { await appMessageBox.confirm('关闭后当前校验结果不再保留，确认关闭？', '放弃本次导入', {type: 'warning'}) } catch { return }
  }
  done()
}
</script>

<style scoped>
.excel-steps { margin-bottom: var(--bb-space-6); }
.customer-dialog-motion { min-height: 320px; }
.import-step { display: grid; gap: var(--bb-space-4); min-height: 300px; }
.import-guide { display: grid; grid-template-columns: 36px minmax(0, 1fr) auto; align-items: center; gap: var(--bb-space-3); border: 1px solid var(--bb-border-default); border-radius: var(--bb-radius-lg); padding: var(--bb-space-4); }
.import-guide h3,
.import-result h3 { margin: 0; }
.import-guide p,
.import-result p { margin: var(--bb-space-1) 0 0; color: var(--bb-text-secondary); line-height: var(--bb-line-height-base); }
.step-number { display: grid; width: 32px; height: 32px; place-items: center; border-radius: 50%; background: var(--bb-brand-50); color: var(--bb-brand-700); font-weight: var(--bb-font-weight-bold); }
.customer-import-dropzone { display: flex; min-height: 72px; align-items: center; justify-content: space-between; gap: var(--bb-space-3); border: 1px dashed var(--bb-border-strong); border-radius: var(--bb-radius-lg); background: var(--bb-bg-subtle); padding: var(--bb-space-3) var(--bb-space-4); color: var(--bb-text-secondary); cursor: pointer; }
.customer-import-dropzone:hover,
.customer-import-dropzone:focus-visible,
.customer-import-dropzone.is-dragging { border-color: var(--bb-brand-500); background: var(--bb-brand-50); color: var(--bb-brand-700); outline: none; }
.customer-import-dropzone strong { color: inherit; }
.selected-file { display: flex; min-height: 56px; align-items: center; justify-content: space-between; gap: var(--bb-space-3); border-radius: var(--bb-radius-lg); background: var(--bb-bg-subtle); padding: var(--bb-space-3) var(--bb-space-4); }
.selected-file div { display: grid; gap: var(--bb-space-1); }
.selected-file small { color: var(--bb-text-secondary); }
.import-summary-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--bb-space-3); }
.import-summary-grid article { display: grid; gap: var(--bb-space-1); border: 1px solid var(--bb-border-default); border-radius: var(--bb-radius-lg); padding: var(--bb-space-3); }
.import-summary-grid span { color: var(--bb-text-secondary); font-size: var(--bb-font-size-13); }
.import-summary-grid strong { font-size: var(--bb-font-size-24); }
.import-errors { display: grid; gap: var(--bb-space-3); }
.section-heading h3 { margin: 0; }
.section-heading p { margin: var(--bb-space-1) 0 0; color: var(--bb-text-secondary); }
.import-error-table { display: grid; max-height: 280px; overflow: auto; border: 1px solid var(--bb-danger-border); border-radius: var(--bb-radius-lg); }
.import-error-table > div { display: grid; grid-template-columns: 130px minmax(0, 1fr); gap: var(--bb-space-1) var(--bb-space-3); border-bottom: 1px solid var(--bb-border-subtle); padding: var(--bb-space-3); }
.import-error-table > div:last-child { border-bottom: 0; }
.import-error-table small { grid-column: 2; color: var(--bb-text-secondary); }
.import-result { display: grid; min-height: 320px; place-items: center; align-content: center; text-align: center; }
.import-result > span { display: grid; width: 64px; height: 64px; place-items: center; border-radius: 50%; background: var(--bb-success-bg); color: var(--bb-success); font-size: var(--bb-font-size-30); }
.dialog-actions { display: flex; justify-content: flex-end; gap: var(--bb-space-2); }
@media (max-width: 760px) {
  .import-file-guide { grid-template-columns: 36px minmax(0, 1fr); }
  .import-file-guide .customer-import-dropzone { grid-column: 2; }
  .customer-import-dropzone { align-items: stretch; flex-direction: column; }
}
</style>
