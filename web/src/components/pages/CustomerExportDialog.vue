<template>
  <el-dialog
    v-model="visible"
    class="customer-export-dialog"
    title="导出客户资料"
    width="min(1120px, 96vw)"
    :close-on-click-modal="!downloading"
    :close-on-press-escape="!downloading"
    destroy-on-close
    @closed="reset"
  >
    <AnimatePresence mode="wait" initial>
      <motion.div
        v-if="visible"
        :key="previewReady ? 'export-preview' : 'export-scope'"
        class="customer-dialog-motion"
        :initial="{opacity: 0, y: 10}"
        :animate="{opacity: 1, y: 0}"
        :exit="{opacity: 0, y: -8}"
        :transition="{duration: 0.18, ease: [0.2, 0, 0, 1]}"
      >
    <section v-if="!previewReady" class="export-scope-step">
      <div class="scope-heading"><h3>选择导出范围</h3><p>确认后先查看与最终 XLSX 一致的九列工作表预览。</p></div>
      <el-radio-group v-model="scope" class="export-scope-options">
        <el-radio value="current" border>
          <span><strong>导出当前筛选结果</strong><small>{{ currentScopeText }}</small></span>
        </el-radio>
        <el-radio value="all" border>
          <span><strong>导出全部客户资料</strong><small>忽略页面当前的关键词和分组筛选。</small></span>
        </el-radio>
      </el-radio-group>
      <el-alert title="预览后若数据发生变化，下载文件以下载时数据为准。" type="info" :closable="false" show-icon />
    </section>

    <section v-else class="export-preview-step">
      <div class="export-preview-toolbar">
        <div>
          <span class="worksheet-chip">{{ document?.sheet_name || '客户资料' }}</span>
          <strong>共 {{ document?.total_rows || 0 }} 条客户资料</strong>
          <small>第 {{ document?.page || 1 }} 页 · 每页 {{ document?.page_size || pageSize }} 条</small>
        </div>
        <div class="export-preview-actions">
          <el-button :disabled="loading || downloading" @click="backToScope">修改范围</el-button>
          <el-button :loading="loading" :disabled="downloading" @click="refreshPreview">刷新预览</el-button>
        </div>
      </div>
      <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
      <div v-if="loading && !document" class="preview-loading"><el-skeleton :rows="8" animated /></div>
      <template v-else-if="document">
        <PageState v-if="document.empty" kind="empty" title="当前范围没有可导出的客户资料" description="可返回修改导出范围，或关闭后调整客户列表筛选。" />
        <div v-else class="worksheet-shell" :aria-busy="loading" tabindex="0" aria-label="客户资料 Excel 导出预览，可横向滚动">
          <table class="worksheet-table">
            <caption>{{ document.title }}</caption>
            <colgroup><col v-for="column in document.columns" :key="column.key" :style="columnStyle(column.width)" /></colgroup>
            <thead><tr><th v-for="column in document.columns" :key="column.key" :class="alignmentClass(column.alignment)">{{ column.title }}</th></tr></thead>
            <tbody>
              <tr v-for="(row, rowIndex) in document.rows" :key="`${document.page}-${rowIndex}`">
                <td v-for="(value, columnIndex) in row" :key="columnIndex" :class="[alignmentClass(document.columns[columnIndex]?.alignment), {'is-text': document.columns[columnIndex]?.type === 'text'}]">{{ value || '' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="!document.empty" class="export-pagination">
          <el-pagination
            background
            layout="prev, pager, next, sizes, total"
            :total="document.total_rows"
            :current-page="document.page || 1"
            :page-size="document.page_size || pageSize"
            :page-sizes="[50, 100]"
            :disabled="loading || downloading"
            @update:current-page="changePage"
            @update:page-size="changePageSize"
          />
        </div>
      </template>
      <p class="export-freshness-note">数据如在预览后发生变化，下载文件以下载时数据为准。</p>
    </section>
      </motion.div>
    </AnimatePresence>

    <template #footer>
      <div class="dialog-actions">
        <el-button :disabled="downloading" @click="visible = false">取消</el-button>
        <el-button v-if="!previewReady" type="primary" :loading="loading" @click="createPreview">查看导出预览</el-button>
        <el-button v-else type="primary" :loading="downloading" :disabled="loading || !document || document.empty" @click="download">确认导出 XLSX</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import {computed, ref} from 'vue'
import {ElMessage} from 'element-plus'
import {AnimatePresence, motion} from 'motion-v'
import {downloadApiFile, request} from '../../api/http'
import {useDirtyGuard} from '../../composables/useDirtyGuard'
import PageState from '../ui/PageState.vue'
import type {SpreadsheetDocument} from '../../types'

const props = defineProps<{modelValue: boolean; token: string; keyword: string; filter: 'all' | 'multiple' | 'empty'}>()
const emit = defineEmits<{(event: 'update:modelValue', value: boolean): void}>()
const visible = computed({get: () => props.modelValue, set: (value) => emit('update:modelValue', value)})
const scope = ref<'current' | 'all'>('current')
const previewReady = ref(false)
const document = ref<SpreadsheetDocument | null>(null)
const loading = ref(false)
const downloading = ref(false)
const error = ref('')
const pageSize = ref(50)
const frozen = ref({scope: 'current' as 'current' | 'all', q: '', filter: 'all' as 'all' | 'multiple' | 'empty'})
const generation = ref(0)
const currentScopeText = computed(() => {
  const parts = []
  if (props.keyword) parts.push(`关键词“${props.keyword}”`)
  if (props.filter === 'multiple') parts.push('仅多资料编码')
  if (props.filter === 'empty') parts.push('仅无资料编码（预览将为空）')
  return parts.length ? parts.join(' · ') : '当前为全部编码，无其他筛选。'
})

useDirtyGuard('customer-export', {
  busy: () => props.modelValue && (loading.value || downloading.value),
  dirty: () => props.modelValue && (previewReady.value || scope.value !== 'current'),
  busyMessage: '客户资料导出正在处理，请等待完成后再离开',
  dirtyMessage: '当前导出范围或预览尚未完成，离开后不会保留。',
})

function query(page = 1) {
  const params = new URLSearchParams({scope: frozen.value.scope, page: String(page), page_size: String(pageSize.value)})
  if (frozen.value.scope === 'current') {
    if (frozen.value.q) params.set('q', frozen.value.q)
    if (frozen.value.filter !== 'all') params.set('filter', frozen.value.filter)
  }
  return params
}
async function createPreview() {
  frozen.value = {scope: scope.value, q: scope.value === 'current' ? props.keyword : '', filter: scope.value === 'current' ? props.filter : 'all'}
  previewReady.value = true
  await loadPreview(1)
}
async function loadPreview(page: number) {
  const requestGeneration = ++generation.value
  loading.value = true
  error.value = ''
  try {
    const data = await request<SpreadsheetDocument>(`/api/v1/customers/export/preview?${query(page)}`, {}, props.token)
    if (requestGeneration === generation.value) document.value = data
  } catch (cause) {
    if (requestGeneration === generation.value) error.value = cause instanceof Error ? cause.message : '导出预览加载失败'
  } finally { if (requestGeneration === generation.value) loading.value = false }
}
function changePage(page: number) { void loadPreview(page) }
function changePageSize(size: number) { pageSize.value = size; void loadPreview(1) }
function refreshPreview() {
  void loadPreview(document.value?.page || 1)
}
function backToScope() { generation.value++; loading.value = false; error.value = ''; document.value = null; previewReady.value = false }
async function download() {
  downloading.value = true
  error.value = ''
  try {
    const params = query(1); params.delete('page'); params.delete('page_size')
    const result = await downloadApiFile(`/api/v1/customers/export?${params}`, '客户资料.xlsx', props.token)
    if (result.status === 'error') throw new Error(result.message)
    if (result.status === 'saved') ElMessage.success(result.path ? `客户资料 XLSX 已保存到：${result.path}` : '浏览器已开始下载客户资料 XLSX')
    if (result.status === 'cancelled') ElMessage.info('已取消保存客户资料 XLSX')
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '导出失败'; ElMessage.error(error.value) }
  finally { downloading.value = false }
}
function columnStyle(width: number) { return {width: `${Math.max(width * 8, 72)}px`} }
function alignmentClass(value?: string) { return value === 'center' ? 'is-center' : value === 'right' ? 'is-right' : 'is-left' }
function reset() { generation.value++; scope.value = 'current'; previewReady.value = false; document.value = null; loading.value = false; downloading.value = false; error.value = ''; pageSize.value = 50 }
</script>

<style scoped>
.export-scope-step { display: grid; gap: var(--bb-space-5); min-height: 360px; align-content: start; }
.customer-dialog-motion { min-height: 360px; }
.scope-heading h3 { margin: 0; font-size: var(--bb-font-size-20); }
.scope-heading p { margin: var(--bb-space-2) 0 0; color: var(--bb-text-secondary); }
.export-scope-options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--bb-space-4); }
.export-scope-options :deep(.el-radio) { width: 100%; height: auto; min-height: 116px; align-items: flex-start; margin: 0; padding: var(--bb-space-5); }
.export-scope-options :deep(.el-radio__label) { width: 100%; white-space: normal; }
.export-scope-options span { display: grid; gap: var(--bb-space-2); }
.export-scope-options strong { color: var(--bb-text-primary); font-size: var(--bb-font-size-16); }
.export-scope-options small { color: var(--bb-text-secondary); line-height: var(--bb-line-height-base); }
.export-preview-step { display: grid; gap: var(--bb-space-3); min-width: 0; }
.export-preview-toolbar { display: flex; align-items: center; justify-content: space-between; gap: var(--bb-space-4); }
.export-preview-toolbar > div { display: flex; align-items: center; gap: var(--bb-space-2); }
.export-preview-toolbar small { color: var(--bb-text-secondary); }
.worksheet-chip { border-radius: var(--bb-radius-pill); background: var(--bb-brand-50); padding: var(--bb-space-1) var(--bb-space-2); color: var(--bb-brand-700); font-weight: var(--bb-font-weight-bold); }
.worksheet-shell { max-width: 100%; max-height: min(56vh, 560px); overflow: auto; border: 1px solid var(--bb-border-strong); border-radius: var(--bb-radius-lg); background: var(--bb-bg-surface); outline: none; }
.worksheet-shell:focus-visible { box-shadow: var(--bb-focus-ring); }
.worksheet-table { width: max-content; min-width: 100%; table-layout: fixed; border-collapse: separate; border-spacing: 0; color: var(--bb-text-primary); font-size: var(--bb-font-size-13); }
.worksheet-table caption { border-bottom: 1px solid var(--bb-border-strong); background: var(--bb-brand-50); padding: var(--bb-space-3); color: var(--bb-brand-800); font-size: var(--bb-font-size-16); font-weight: var(--bb-font-weight-bold); text-align: center; }
.worksheet-table thead { position: sticky; top: 0; z-index: var(--bb-z-sticky); }
.worksheet-table th { border-right: 1px solid var(--bb-border-strong); border-bottom: 1px solid var(--bb-border-strong); background: var(--bb-bg-sunken); padding: var(--bb-space-2); white-space: nowrap; }
.worksheet-table td { height: 38px; overflow: hidden; border-right: 1px solid var(--bb-border-default); border-bottom: 1px solid var(--bb-border-default); padding: var(--bb-space-2); text-overflow: ellipsis; white-space: nowrap; }
.worksheet-table td.is-text { mso-number-format: "\@"; }
.worksheet-table .is-center { text-align: center; }
.worksheet-table .is-right { text-align: right; }
.worksheet-table .is-left { text-align: left; }
.preview-loading { min-height: 360px; border: 1px solid var(--bb-border-default); border-radius: var(--bb-radius-lg); padding: var(--bb-space-5); }
.export-pagination { display: flex; justify-content: flex-end; overflow-x: auto; padding-bottom: var(--bb-space-1); }
.export-freshness-note { margin: 0; color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); text-align: right; }
.dialog-actions,
.export-preview-actions { display: flex; justify-content: flex-end; gap: var(--bb-space-2); }
</style>
