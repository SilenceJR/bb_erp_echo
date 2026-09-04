<template>
  <section class="mold-page" aria-label="模具">
    <div class="mold-toolbar">
      <div class="mold-filters">
        <el-input v-model.trim="filters.q" clearable aria-label="搜索模具编号、型号或备注" placeholder="搜索模具编号、型号或备注" @keyup.enter="applyFilters" />
        <el-select v-model="filters.type" clearable aria-label="模具类型" placeholder="模具类型" @change="applyFilters"><el-option label="单模" value="single" /><el-option label="共模" value="common" /></el-select>
        <el-select v-model="filters.locationID" clearable aria-label="模具位置" placeholder="模具位置" @change="applyFilters"><el-option v-for="item in locations" :key="item.id" :label="item.code" :value="item.id" /></el-select>
        <el-input v-model.trim="filters.groupNo" clearable aria-label="共模组号" placeholder="共模组号" @keyup.enter="applyFilters" />
        <el-button @click="applyFilters">查询</el-button><el-button @click="resetFilters">重置</el-button>
      </div>
      <div class="mold-actions">
        <el-button v-if="canWrite" @click="locationDialog = true">位置管理</el-button>
        <el-button v-if="canRead" :loading="exporting" @click="exportPackage">导出资料包</el-button>
        <el-button v-if="canImport" @click="openImport">导入资料包</el-button>
        <el-button v-if="canWrite" type="primary" @click="openCreate">＋ 新增模具</el-button>
      </div>
    </div>

    <div class="mold-summary" aria-label="模具摘要">
      <div><span>筛选结果</span><strong>{{ total }}</strong></div>
      <div><span>当前页图片</span><strong>{{ pageImages }}</strong></div>
      <div><span>已选择</span><strong>{{ selectedIDs.size }}</strong></div>
      <div><span>批量操作</span><el-button link type="primary" :disabled="!selectedIDs.size || !canWrite" @click="bulkDialog = true">统一移动位置</el-button></div>
    </div>

    <div class="selection-bar" v-if="selectedIDs.size || canWrite">
      <span v-if="selectedIDs.size">已选择 {{ selectedIDs.size }} 个模具，可跨页保留选择。</span>
      <span v-else>可按筛选条件全选结果后批量移动。</span>
      <div><el-button link :disabled="!total || !canWrite" @click="selectAllFiltered">全选筛选结果</el-button><el-button link :disabled="!selectedIDs.size" @click="clearSelection">清空选择</el-button></div>
    </div>

    <DataTableShell :loading="loading" :error="error" :rows-count="rows.length" :total="total" :page="page" :page-size="pageSize" empty-title="暂无模具档案" :empty-description="canWrite ? '请新增模具或导入 ZIP 资料包。' : '暂无可查看的模具档案，请联系管理员维护。'" aria-label="模具列表" @retry="load" @update:page="changePage" @update:page-size="changePageSize">
      <el-table :data="rows" row-key="id" stripe class="data-table">
        <el-table-column width="52" fixed="left"><template #header><span class="sr-only">选择</span></template><template #default="{row}"><el-checkbox :model-value="selectedIDs.has(row.id)" :aria-label="`选择模具 ${row.mold_number}`" @change="toggleSelection(row.id)" /></template></el-table-column>
        <el-table-column label="模具编号" min-width="180"><template #default="{row}"><strong>{{ row.mold_number }}</strong><small class="item-code">序号 {{ row.id }}</small></template></el-table-column>
        <el-table-column prop="model" label="模具型号" min-width="180" />
        <el-table-column label="类型" width="90"><template #default="{row}">{{ typeLabel(row.mold_type) }}</template></el-table-column>
        <el-table-column label="位置" width="120"><template #default="{row}">{{ row.location?.code || '—' }}</template></el-table-column>
        <el-table-column prop="common_group_no" label="共模组号" width="130" />
        <el-table-column label="图片" width="80"><template #default="{row}">{{ row.image_count || 0 }}</template></el-table-column>
        <el-table-column label="操作" width="120" fixed="right"><template #default="{row}"><el-button link type="primary" @click="openDetail(row.id)">详情</el-button></template></el-table-column>
      </el-table>
    </DataTableShell>

    <el-drawer v-model="detailVisible" class="mold-detail-drawer" :title="editing ? '编辑模具' : '模具详情'" size="min(720px, 100%)" destroy-on-close :close-on-click-modal="!saving && !deleting && !drawingSaving" :close-on-press-escape="!saving && !deleting && !drawingSaving" :before-close="beforeDetailClose">
      <PageState v-if="detailLoading" kind="loading" title="正在加载模具详情" />
      <PageState v-else-if="detailLoadError" kind="error" title="模具详情加载失败" :description="detailLoadError" action-label="重新加载" @action="retryDetail" />
      <template v-else>
        <el-form label-position="top" @submit.prevent="save">
          <div class="drawer-section-heading"><div><h3>基础信息</h3><small>每条模具档案对应一个产品型号。</small></div><el-button v-if="!editing && canWrite" type="primary" plain @click="editing = true">编辑资料</el-button></div>
          <div class="form-grid">
            <el-form-item label="模具编号" required><el-input v-model.trim="draft.mold_number" :disabled="!editing" /></el-form-item>
            <el-form-item label="模具型号" required><el-input v-model.trim="draft.model" :disabled="!editing" /></el-form-item>
            <el-form-item label="模具类型" required><el-select v-model="draft.mold_type" :disabled="!editing"><el-option label="单模" value="single" /><el-option label="共模" value="common" /></el-select></el-form-item>
            <el-form-item label="模具位置" required><el-select v-model="draft.location_id" :disabled="!editing"><el-option v-for="item in assignableLocations" :key="item.id" :label="item.code" :value="item.id" /></el-select></el-form-item>
            <el-form-item v-if="draft.mold_type === 'common'" label="共模组号" required><el-input v-model.trim="draft.common_group_no" :disabled="!editing" /></el-form-item>
            <el-form-item label="备注"><el-input v-model.trim="draft.remark" type="textarea" :rows="3" :disabled="!editing" /></el-form-item>
          </div>
          <el-alert v-if="detailError" :title="detailError" type="error" :closable="false" show-icon />
          <div v-if="editing" class="form-actions drawer-sticky-actions"><el-button :disabled="saving" @click="cancelEditing">取消编辑</el-button><el-button type="primary" native-type="submit" :loading="saving" :disabled="saving">保存</el-button></div>
        </el-form>
        <template v-if="detailID">
          <ImageGallery owner-type="mold" :owner-id="detailID" :token="token" :can-write="canWrite" category="product_material" title="产品材料图片" />
          <ImageGallery owner-type="mold" :owner-id="detailID" :token="token" :can-write="canWrite" category="supplement" title="补充图片" />
          <section class="drawing-panel" data-file-drop-target :class="{'is-dragging': drawingDragging}" aria-label="DWG 图纸" @dragenter.prevent="handleDrawingDragEnter" @dragover.prevent="handleDrawingDragOver" @dragleave.prevent="handleDrawingDragLeave" @drop.prevent="handleDrawingDrop" @bb-native-file-drag="handleNativeDrawingDrag">
            <div class="section-heading"><div><h3>DWG 图纸</h3><small>支持 .dwg、.fdwg；本期提供上传和下载。</small></div><el-button v-if="canWrite" :loading="drawingSaving" @click="drawingInput?.click()">上传图纸</el-button></div>
            <input ref="drawingInput" class="sr-only" type="file" accept=".dwg,.fdwg" @change="uploadDrawing" />
            <div v-if="drawings.length" class="drawing-list"><div v-for="item in drawings" :key="item.id"><span>{{ item.original_name }}</span><small>{{ formatSize(item.size) }}</small><div><el-button link type="primary" @click="downloadDrawing(item)">下载</el-button><el-button v-if="canWrite" link type="danger" @click="deleteDrawing(item)">删除</el-button></div></div></div><p v-else class="drawer-empty">暂无 DWG 图纸</p><p v-if="canWrite" class="drawing-drop-hint">{{ drawingDragging ? '松开以上传图纸' : '也可以将 .dwg 或 .fdwg 文件拖到此区域' }}</p>
          </section>
          <div v-if="canWrite" class="danger-zone"><el-button type="danger" plain :loading="deleting" :disabled="deleting" @click="deleteMold">删除此模具</el-button><small>将同时删除该模具的图片和 DWG 文件。</small></div>
        </template>
      </template>
    </el-drawer>

    <el-dialog v-model="bulkDialog" title="批量移动模具" width="420px"><p>将已选择的 {{ selectedIDs.size }} 个模具统一移动到：</p><el-select v-model="bulkLocationID" placeholder="选择目标位置" style="width: 100%"><el-option v-for="item in assignableLocations" :key="item.id" :label="item.code" :value="item.id" /></el-select><template #footer><el-button @click="bulkDialog = false">取消</el-button><el-button type="primary" :loading="bulkSaving" :disabled="!bulkLocationID || bulkSaving" @click="bulkMove">确认移动</el-button></template></el-dialog>

    <el-dialog v-model="locationDialog" title="模具位置管理" width="480px"><el-form @submit.prevent="createLocation"><el-form-item label="新增位置" required><el-input v-model.trim="newLocation" placeholder="例如 A2-1"><template #append><el-button :loading="locationSaving" @click="createLocation">新增</el-button></template></el-input></el-form-item></el-form><div class="location-list"><div v-for="item in locations" :key="item.id"><span>{{ item.code }}</span><el-tag :type="item.status === 'active' ? 'success' : 'info'">{{ item.status === 'active' ? '启用' : '停用' }}</el-tag><el-button v-if="canWrite" link :type="item.status === 'active' ? 'danger' : 'primary'" @click="toggleLocation(item)">{{ item.status === 'active' ? '停用' : '启用' }}</el-button></div></div></el-dialog>

    <input ref="importInput" class="sr-only" type="file" accept=".zip,application/zip" @change="previewImport" />
    <el-dialog v-model="importDialog" class="mold-import-dialog" title="导入模具资料包" width="min(620px, calc(100vw - 32px))"><p>导入会替换模具、模具图片、DWG 和位置字典，不影响其他业务模块。</p><div v-if="!importFile" class="import-dropzone" data-file-drop-target :class="{'is-dragging': importDragging}" role="button" tabindex="0" @click="chooseImportFile" @keydown.enter.prevent="chooseImportFile" @keydown.space.prevent="chooseImportFile" @dragenter.prevent="handleImportDragEnter" @dragover.prevent="handleImportDragOver" @dragleave.prevent="handleImportDragLeave" @drop.prevent="handleImportDrop" @bb-native-file-drag="handleNativeImportDrag"><strong>{{ importDragging ? '松开以导入 ZIP' : '将 ZIP 资料包拖到此处' }}</strong><span>或点击选择文件</span></div><div v-if="importFile" class="import-file"><strong>{{ importFile.name }}</strong><span>{{ importResult ? '资料包预览完成' : '正在检查资料包…' }}</span></div><el-alert v-if="importError" :title="importError" type="error" :closable="false" show-icon /><el-alert v-if="importResult" :title="`模具 ${importResult.summary.molds} 条，图片 ${importResult.summary.images} 张，图纸 ${importResult.summary.drawings} 个`" type="info" :closable="false" /><div v-if="importResult?.errors?.length" class="import-errors"><p v-for="item in importResult.errors" :key="`${item.row}-${item.column}-${item.value}`">{{ item.value || item.column }}：{{ item.reason }}</p></div><div v-if="importResult?.unresolved?.length" class="import-corrections"><p>以下图片无法从文件名识别模具，请人工指定：</p><div v-for="item in importResult.unresolved" :key="item.path"><span>{{ item.name }}</span><el-select v-model="corrections[item.path].category" :aria-label="`${item.name}图片分组`"><el-option label="产品材料" value="product_material" /><el-option label="补充图" value="supplement" /></el-select><el-input v-model="corrections[item.path].codes" :aria-label="`${item.name}对应模具编号`" placeholder="模具编号，多个用 + 或逗号分隔" /></div></div><template #footer><el-button @click="importDialog = false">取消</el-button><el-button v-if="importFile && !importResult" @click="chooseImportFile">重新选择</el-button><el-button type="primary" :loading="importing" :disabled="!canCommitImport" @click="commitImport">确认替换导入</el-button></template></el-dialog>
  </section>
</template>

<script setup lang="ts">
import {computed, onMounted, reactive, ref} from 'vue'
import {ElMessage} from 'element-plus'
import {appMessageBox} from '../../../composables/useAppMessageBox'
import {useDirtyGuard} from '../../../composables/useDirtyGuard'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import {downloadApiFile, request, uploadNativeFiles} from '../../../api/http'
import type {ImageFile, NativeFileDragDetail} from '../../../types'
import DataTableShell from '../../ui/DataTableShell.vue'
import ImageGallery from '../../ImageGallery.vue'
import PageState from '../../ui/PageState.vue'

type Location = {id: number; code: string; status: 'active' | 'disabled'}
type Mold = {id: number; mold_number: string; model: string; mold_type: 'single' | 'common'; location_id: number; location?: Location; common_group_no?: string; remark?: string; image_count?: number}
type Drawing = {id: number; original_name: string; size: number}
type Preview = {token?: string; summary: {molds: number; images: number; drawings: number; unresolved: number}; errors: Array<{row: number; column: string; value?: string; reason: string}>; unresolved: Array<{path: string; name: string}>}
const {token, hasPermission} = useWorkspaceContext()
const canRead = computed(() => hasPermission('mold:read'))
const canWrite = computed(() => hasPermission('mold:write'))
const canImport = computed(() => hasPermission('mold:import'))
const rows = ref<Mold[]>([]), total = ref(0), page = ref(1), pageSize = ref(20), loading = ref(false), error = ref('')
const locations = ref<Location[]>([]), selectedIDs = ref(new Set<number>())
const filters = reactive({q: '', type: '', locationID: undefined as number | undefined, groupNo: ''})
const detailVisible = ref(false), detailID = ref<number | null>(null), detailLoading = ref(false), detailLoadError = ref(''), detailError = ref(''), editing = ref(false), saving = ref(false), deleting = ref(false)
const draft = reactive({mold_number: '', model: '', mold_type: 'single' as 'single' | 'common', location_id: undefined as number | undefined, common_group_no: '', remark: ''})
const drawings = ref<Drawing[]>([]), drawingSaving = ref(false), drawingInput = ref<HTMLInputElement | null>(null), importInput = ref<HTMLInputElement | null>(null)
const drawingDragging = ref(false), importDragging = ref(false)
let drawingDragDepth = 0
let importDragDepth = 0
const bulkDialog = ref(false), bulkLocationID = ref<number>(), bulkSaving = ref(false), locationDialog = ref(false), newLocation = ref(''), locationSaving = ref(false), exporting = ref(false)
const importDialog = ref(false), importFile = ref<File | null>(null), importPath = ref<string | null>(null), importResult = ref<Preview | null>(null), importError = ref(''), importing = ref(false), corrections = reactive<Record<string, {category: string; codes: string}>>({})
const assignableLocations = computed(() => locations.value.filter((item) => item.status === 'active' || item.id === draft.location_id))
const pageImages = computed(() => rows.value.reduce((sum, item) => sum + Number(item.image_count || 0), 0))
const canCommitImport = computed(() => Boolean(importFile.value && importResult.value?.token && !importing.value && !(importResult.value?.unresolved || []).some((item) => !corrections[item.path]?.category || !corrections[item.path]?.codes.trim())))
const savedDraft = ref('')
const draftDirty = computed(() => editing.value && savedDraft.value !== JSON.stringify(draft))
const {confirmLeave} = useDirtyGuard('mold-form', {busy: () => saving.value || deleting.value || drawingSaving.value, dirty: () => draftDirty.value, busyMessage: '模具资料正在提交，请稍候', dirtyMessage: '模具资料尚未保存，关闭后修改将丢失。'})

async function load() { loading.value = true; error.value = ''; try { const params = new URLSearchParams({page: String(page.value), page_size: String(pageSize.value)}); if (filters.q) params.set('q', filters.q); if (filters.type) params.set('mold_type', filters.type); if (filters.locationID) params.set('location_id', String(filters.locationID)); if (filters.groupNo) params.set('common_group_no', filters.groupNo); const result = await request<{items: Mold[]; total: number}>(`/api/v1/molds?${params}`, {}, token.value); rows.value = result.items || []; total.value = result.total || 0 } catch (cause) { error.value = cause instanceof Error ? cause.message : '模具列表加载失败' } finally { loading.value = false } }
async function loadLocations() { locations.value = await request<Location[]>('/api/v1/mold-locations?include_disabled=true', {}, token.value) }
function applyFilters() { page.value = 1; void load() }
function resetFilters() { Object.assign(filters, {q: '', type: '', locationID: undefined, groupNo: ''}); applyFilters() }
function changePage(value: number) { page.value = value; void load() }
function changePageSize(value: number) { pageSize.value = value; page.value = 1; void load() }
function typeLabel(value: string) { return value === 'common' ? '共模' : '单模' }
function toggleSelection(id: number) { const next = new Set(selectedIDs.value); next.has(id) ? next.delete(id) : next.add(id); selectedIDs.value = next }
function clearSelection() { selectedIDs.value = new Set() }
async function selectAllFiltered() { const ids = new Set<number>(); let current = 1; do { const params = new URLSearchParams({page: String(current), page_size: '200'}); if (filters.q) params.set('q', filters.q); if (filters.type) params.set('mold_type', filters.type); if (filters.locationID) params.set('location_id', String(filters.locationID)); if (filters.groupNo) params.set('common_group_no', filters.groupNo); const result = await request<{items: Mold[]; total: number}>(`/api/v1/molds?${params}`, {}, token.value); for (const item of result.items || []) ids.add(item.id); current++; if (!result.items?.length) break; if (ids.size >= result.total) break } while (current < 1000); selectedIDs.value = ids; ElMessage.success(`已选择 ${ids.size} 个筛选结果`) }
function resetDraft() { Object.assign(draft, {mold_number: '', model: '', mold_type: 'single', location_id: locations.value.find((item) => item.status === 'active')?.id, common_group_no: '', remark: ''}) }
function openCreate() { resetDraft(); savedDraft.value = JSON.stringify(draft); detailID.value = null; detailLoadError.value = ''; detailError.value = ''; editing.value = true; detailVisible.value = true }
async function openDetail(id: number) { detailID.value = id; detailVisible.value = true; editing.value = false; detailLoading.value = true; detailLoadError.value = ''; detailError.value = ''; try { const item = await request<Mold>(`/api/v1/molds/${id}`, {}, token.value); Object.assign(draft, item, {location_id: item.location_id, common_group_no: item.common_group_no || '', remark: item.remark || ''}); savedDraft.value = JSON.stringify(draft); drawings.value = await request<Drawing[]>(`/api/v1/molds/${id}/drawings`, {}, token.value) } catch (cause) { detailLoadError.value = cause instanceof Error ? cause.message : '模具详情加载失败' } finally { detailLoading.value = false } }
async function save() { if (!draft.mold_number.trim() || !draft.model.trim() || !draft.location_id || (draft.mold_type === 'common' && !draft.common_group_no.trim())) { detailError.value = '请填写编号、型号、类型、位置及共模组号等必填项'; return }; saving.value = true; detailError.value = ''; try { const body = {...draft, location_id: Number(draft.location_id), ...(draft.mold_type === 'single' ? {common_group_no: ''} : {})}; const item = detailID.value ? await request<Mold>(`/api/v1/molds/${detailID.value}`, {method: 'PATCH', body}, token.value) : await request<Mold>('/api/v1/molds', {method: 'POST', body}, token.value); detailID.value = item.id; Object.assign(draft, item, {location_id: item.location_id, common_group_no: item.common_group_no || '', remark: item.remark || ''}); savedDraft.value = JSON.stringify(draft); editing.value = false; await Promise.all([load(), loadLocations()]); ElMessage.success('模具档案已保存') } catch (cause) { detailError.value = cause instanceof Error ? cause.message : '模具保存失败' } finally { saving.value = false } }
function retryDetail() { if (detailID.value) void openDetail(detailID.value) }
async function beforeDetailClose(done: () => void) { if (saving.value || deleting.value || drawingSaving.value) { ElMessage.warning('操作正在提交，请稍候'); return }; if (await confirmLeave()) done() }
async function cancelEditing() { if (!(await confirmLeave())) return; if (detailID.value) editing.value = false; else detailVisible.value = false }
async function deleteMold() { if (!detailID.value) return; try { await appMessageBox.confirm(`确认删除“${draft.mold_number}”吗？图片和 DWG 文件也会删除。`, '删除模具', {type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消'}) } catch { return }; deleting.value = true; try { await request(`/api/v1/molds/${detailID.value}`, {method: 'DELETE'}, token.value); detailVisible.value = false; await load(); ElMessage.success('模具已删除') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '模具删除失败') } finally { deleting.value = false } }
async function bulkMove() { if (!bulkLocationID.value || !selectedIDs.value.size) return; bulkSaving.value = true; try { await request('/api/v1/molds/bulk-location', {method: 'POST', body: {mold_ids: [...selectedIDs.value], location_id: bulkLocationID.value}}, token.value); bulkDialog.value = false; selectedIDs.value = new Set(); await load(); ElMessage.success('模具位置已批量更新') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '批量移动失败') } finally { bulkSaving.value = false } }
async function createLocation() { if (!newLocation.value.trim()) return; locationSaving.value = true; try { await request('/api/v1/mold-locations', {method: 'POST', body: {code: newLocation.value}}, token.value); newLocation.value = ''; await loadLocations(); ElMessage.success('位置已新增') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '位置新增失败') } finally { locationSaving.value = false } }
async function toggleLocation(item: Location) { const status = item.status === 'active' ? 'disabled' : 'active'; try { await appMessageBox.confirm(`确认${status === 'disabled' ? '停用' : '启用'}位置“${item.code}”吗？`, `${status === 'disabled' ? '停用' : '启用'}位置`, {type: status === 'disabled' ? 'warning' : 'info', confirmButtonText: `确认${status === 'disabled' ? '停用' : '启用'}`}) } catch { return }; try { await request(`/api/v1/mold-locations/${item.id}`, {method: 'PATCH', body: {status}}, token.value); await loadLocations(); ElMessage.success(`位置已${status === 'disabled' ? '停用' : '启用'}`) } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '位置状态更新失败') } }
function hasDraggedFiles(event: DragEvent): boolean { return Array.from(event.dataTransfer?.types || []).includes('Files') }
function handleDrawingDragEnter(event: DragEvent) { if (!canWrite.value || !hasDraggedFiles(event)) return; drawingDragDepth++; drawingDragging.value = true }
function handleDrawingDragOver(event: DragEvent) { if (canWrite.value && hasDraggedFiles(event)) event.dataTransfer!.dropEffect = 'copy' }
function handleDrawingDragLeave() { drawingDragDepth = Math.max(0, drawingDragDepth - 1); if (!drawingDragDepth) drawingDragging.value = false }
function handleDrawingDrop(event: DragEvent) { drawingDragDepth = 0; drawingDragging.value = false; if (!canWrite.value || drawingSaving.value) return; const file = event.dataTransfer?.files?.[0]; if (file) void uploadDrawingFile(file) }
function handleNativeDrawingDrag(event: Event) { const detail = (event as CustomEvent<NativeFileDragDetail>).detail; if (!detail) return; if (detail.phase === 'enter' || detail.phase === 'over') { if (canWrite.value) drawingDragging.value = true; return }; drawingDragging.value = false; if (detail.phase !== 'drop' || !canWrite.value || drawingSaving.value) return; if (detail.error) { ElMessage.error(detail.error); return }; if (detail.paths.length) void uploadNativeDrawingFile(detail.paths[0]); else if (detail.files[0]) void uploadDrawingFile(detail.files[0]) }
function uploadDrawing(event: Event) { const input = event.target as HTMLInputElement; const file = input.files?.[0]; input.value = ''; if (file) void uploadDrawingFile(file) }
async function uploadDrawingFile(file: File) { if (!detailID.value || !canWrite.value || drawingSaving.value) return; if (!/\.(dwg|fdwg)$/i.test(file.name)) { ElMessage.warning('仅支持 .dwg、.fdwg 图纸'); return }; drawingSaving.value = true; try { const body = new FormData(); body.append('file', file); await request(`/api/v1/molds/${detailID.value}/drawings`, {method: 'POST', body}, token.value); drawings.value = await request<Drawing[]>(`/api/v1/molds/${detailID.value}/drawings`, {}, token.value); ElMessage.success('图纸已上传') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '图纸上传失败') } finally { drawingSaving.value = false } }
async function uploadNativeDrawingFile(path: string) { if (!detailID.value || !canWrite.value || drawingSaving.value) return; if (!/\.(dwg|fdwg)$/i.test(path)) { ElMessage.warning('仅支持 .dwg、.fdwg 图纸'); return }; drawingSaving.value = true; try { await uploadNativeFiles(`/api/v1/molds/${detailID.value}/drawings`, [path], {}, token.value); drawings.value = await request<Drawing[]>(`/api/v1/molds/${detailID.value}/drawings`, {}, token.value); ElMessage.success('图纸已上传') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '图纸上传失败') } finally { drawingSaving.value = false } }
async function downloadDrawing(item: Drawing) { await downloadApiFile(`/api/v1/molds/${detailID.value}/drawings/${item.id}/content`, item.original_name, token.value) }
async function deleteDrawing(item: Drawing) { try { await appMessageBox.confirm(`确认删除“${item.original_name}”吗？`, '删除图纸', {type: 'warning', confirmButtonText: '确认删除'}) } catch { return }; await request(`/api/v1/molds/${detailID.value}/drawings/${item.id}`, {method: 'DELETE'}, token.value); drawings.value = drawings.value.filter((candidate) => candidate.id !== item.id) }
function chooseImportFile() { if (canImport.value && !importing.value) { importInput.value?.click() } }
function openImport() { importFile.value = null; importPath.value = null; importResult.value = null; importError.value = ''; importDragging.value = false; Object.keys(corrections).forEach((key) => delete corrections[key]); importDialog.value = true }
function handleImportDragEnter(event: DragEvent) { if (!canImport.value || !hasDraggedFiles(event)) return; importDragDepth++; importDragging.value = true }
function handleImportDragOver(event: DragEvent) { if (canImport.value && hasDraggedFiles(event)) event.dataTransfer!.dropEffect = 'copy' }
function handleImportDragLeave() { importDragDepth = Math.max(0, importDragDepth - 1); if (!importDragDepth) importDragging.value = false }
function handleImportDrop(event: DragEvent) { importDragDepth = 0; importDragging.value = false; if (!canImport.value || importing.value) return; const files = Array.from(event.dataTransfer?.files || []); if (files.length !== 1) { ElMessage.warning('请一次拖入一个 ZIP 资料包'); return }; void previewImportFile(files[0]) }
function handleNativeImportDrag(event: Event) { const detail = (event as CustomEvent<NativeFileDragDetail>).detail; if (!detail) return; if (detail.phase === 'enter' || detail.phase === 'over') { if (canImport.value) importDragging.value = true; return }; importDragging.value = false; if (detail.phase !== 'drop' || !canImport.value || importing.value) return; if (detail.error) { importError.value = detail.error; ElMessage.error(detail.error); return }; if (detail.paths.length !== 1 && detail.files.length !== 1) { ElMessage.warning('请一次拖入一个 ZIP 资料包'); return }; if (detail.paths.length) void previewImportPath(detail.paths[0]); else void previewImportFile(detail.files[0]) }
function previewImport(event: Event) { const input = event.target as HTMLInputElement; const file = input.files?.[0]; input.value = ''; if (file) void previewImportFile(file) }
async function applyImportPreview(result: Preview) { importResult.value = result; Object.keys(corrections).forEach((key) => delete corrections[key]); for (const item of result.unresolved || []) corrections[item.path] = {category: 'product_material', codes: ''} }
async function previewImportFile(file: File) { importPath.value = null; if (!/\.zip$/i.test(file.name)) { importError.value = '仅支持 ZIP 资料包'; ElMessage.warning(importError.value); return }; importFile.value = file; importResult.value = null; importError.value = ''; const body = new FormData(); body.append('file', file); try { await applyImportPreview(await request<Preview>('/api/v1/molds/import/preview', {method: 'POST', body}, token.value)) } catch (cause) { importError.value = cause instanceof Error ? cause.message : '资料包预览失败'; importFile.value = null; ElMessage.error(importError.value) } }
async function previewImportPath(path: string) { if (!/\.zip$/i.test(path)) { importError.value = '仅支持 ZIP 资料包'; ElMessage.warning(importError.value); return }; importPath.value = path; importFile.value = new File([], path.split(/[\\/]/).pop() || '拖入资料包'); importResult.value = null; importError.value = ''; try { await applyImportPreview(await uploadNativeFiles<Preview>('/api/v1/molds/import/preview', [path], {}, token.value)) } catch (cause) { importError.value = cause instanceof Error ? cause.message : '资料包预览失败'; importFile.value = null; importPath.value = null; ElMessage.error(importError.value) } }
async function commitImport() { if (!canCommitImport.value || !importFile.value || !importResult.value?.token) return; importing.value = true; try { const normalized = Object.fromEntries(Object.entries(corrections).map(([path, item]) => [path, {category: item.category, codes: item.codes.split(/[+,，、\s]+/).map((code) => code.trim()).filter(Boolean)}])); if (importPath.value) await uploadNativeFiles('/api/v1/molds/import/commit', [importPath.value], {token: importResult.value.token, corrections: JSON.stringify(normalized)}, token.value); else { const body = new FormData(); body.append('file', importFile.value); body.append('token', importResult.value.token); body.append('corrections', JSON.stringify(normalized)); await request('/api/v1/molds/import/commit', {method: 'POST', body}, token.value) } importDialog.value = false; await Promise.all([load(), loadLocations()]); ElMessage.success('模具资料包已导入') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '资料包导入失败') } finally { importing.value = false } }
async function exportPackage() { exporting.value = true; try { await downloadApiFile('/api/v1/molds/export', '博邦模具资料包.zip', token.value) } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '资料包导出失败') } finally { exporting.value = false } }
function formatSize(size: number) { if (size < 1024) return `${size} B`; if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`; return `${(size / 1024 / 1024).toFixed(1)} MB` }
onMounted(() => { void Promise.all([load(), loadLocations()]) })
</script>

<style scoped>
.mold-page { display: grid; min-width: 0; gap: var(--bb-space-4); }
.mold-toolbar { display: grid; grid-template-columns: minmax(0, 1fr); align-items: start; gap: var(--bb-space-3); }
.mold-filters, .mold-actions, .selection-bar, .section-heading, .drawer-section-heading { display: flex; align-items: center; gap: var(--bb-space-2); }
.mold-filters { display: grid; grid-template-columns: minmax(210px, 1fr) repeat(3, minmax(120px, 150px)) auto auto; min-width: 0; }
.mold-filters :deep(.el-input), .mold-filters :deep(.el-select) { width: 100%; min-width: 0; }
.mold-actions { flex-wrap: wrap; justify-content: flex-end; }
.mold-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--bb-space-3); }
.mold-summary > div { min-width: 0; padding: var(--bb-space-3); border: 1px solid var(--bb-border-default); border-radius: var(--bb-radius-lg); background: var(--bb-bg-surface); }
.mold-summary span, .mold-summary strong { display: block; }
.mold-summary span { color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); }
.mold-summary strong { margin-top: 4px; font-size: var(--bb-font-size-24); }
.selection-bar { justify-content: space-between; min-height: 32px; color: var(--bb-text-secondary); font-size: var(--bb-font-size-13); }
.selection-bar > span { min-width: 0; }
.item-code { display: block; color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); }
.drawer-section-heading { justify-content: space-between; margin-bottom: var(--bb-space-3); }
.drawer-section-heading h3 { margin: 0; font-size: var(--bb-font-size-16); }
.drawer-section-heading small { color: var(--bb-text-secondary); }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: var(--bb-space-3); }
.form-actions { display: flex; justify-content: flex-end; gap: var(--bb-space-2); margin-bottom: var(--bb-space-4); }
.drawing-panel { margin-top: var(--bb-space-5); padding-top: var(--bb-space-4); border-top: 1px solid var(--bb-border-default); }
.drawing-panel.is-dragging { border-radius: var(--bb-radius-md); border-color: var(--bb-brand-400); background: var(--bb-brand-50); padding-right: var(--bb-space-3); padding-left: var(--bb-space-3); }
.section-heading { justify-content: space-between; }
.section-heading > div { min-width: 0; }
.section-heading h3 { margin: 0; }
.section-heading small { color: var(--bb-text-secondary); }
.drawing-list, .location-list { display: grid; gap: 8px; margin-top: 12px; }
.drawing-list > div, .location-list > div { display: flex; align-items: center; gap: 8px; padding: 10px 0; border-bottom: 1px solid var(--bb-border-default); }
.drawing-list span, .location-list span { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.drawing-list small { color: var(--bb-text-secondary); }
.drawing-drop-hint { margin: var(--bb-space-2) 0 0; color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); }
.danger-zone { display: flex; align-items: center; gap: 12px; margin-top: var(--bb-space-5); padding-top: var(--bb-space-4); border-top: 1px solid var(--bb-border-default); }
.danger-zone small { color: var(--bb-text-secondary); }
.import-dropzone { display: grid; min-height: 112px; place-items: center; align-content: center; gap: var(--bb-space-1); margin: var(--bb-space-4) 0; border: 1px dashed var(--bb-border-strong); border-radius: var(--bb-radius-lg); background: var(--bb-bg-subtle); color: var(--bb-text-secondary); cursor: pointer; }
.import-dropzone:hover, .import-dropzone:focus-visible, .import-dropzone.is-dragging { border-color: var(--bb-brand-500); background: var(--bb-brand-50); color: var(--bb-brand-700); outline: none; }
.import-dropzone strong { color: inherit; font-size: var(--bb-font-size-14); }
.import-dropzone span, .import-file span { color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); }
.import-file { display: flex; min-width: 0; align-items: center; justify-content: space-between; gap: var(--bb-space-3); margin: var(--bb-space-3) 0; border: 1px solid var(--bb-info-border); border-radius: var(--bb-radius-md); background: var(--bb-info-bg); padding: var(--bb-space-3); }
.import-file strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.import-errors { max-height: 220px; overflow: auto; margin-top: 12px; border-radius: var(--bb-radius-sm); background: var(--bb-bg-subtle); padding: 10px; color: var(--bb-danger); font-size: var(--bb-font-size-13); }
.import-errors p { margin: 4px 0; }
.import-corrections { display: grid; gap: 8px; margin-top: 12px; }
.import-corrections > div { display: grid; grid-template-columns: minmax(0, 1fr) 120px minmax(0, 1.2fr); gap: 8px; align-items: center; }
.mold-page :deep(.ui-page-state) { min-height: 160px; padding: var(--bb-space-5); }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (max-width: 1279px) {
  .mold-toolbar { grid-template-columns: 1fr; }
  .mold-actions { justify-content: flex-start; }
  .mold-filters { grid-template-columns: minmax(180px, 1fr) repeat(2, minmax(120px, 1fr)) auto auto; }
}
@media (max-width: 760px) {
  .mold-filters { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .mold-filters > :first-child { grid-column: 1 / -1; }
  .mold-filters > .el-button { width: 100%; }
  .mold-actions { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .mold-actions .el-button:last-child { grid-column: 1 / -1; }
  .mold-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .selection-bar { align-items: flex-start; flex-direction: column; }
  .form-grid { grid-template-columns: 1fr; }
  .import-corrections > div { grid-template-columns: 1fr; }
  .import-file { align-items: flex-start; flex-direction: column; }
}
</style>

<style>
.mold-detail-drawer.el-drawer .el-drawer__header { margin-bottom: 0; border-bottom: 1px solid var(--bb-border-subtle); padding: var(--bb-space-5) var(--bb-space-6) var(--bb-space-4); }
.mold-detail-drawer.el-drawer .el-drawer__body { min-width: 0; overflow: auto; padding: var(--bb-space-4) var(--bb-space-6) calc(var(--bb-space-6) + env(safe-area-inset-bottom, 0px)) !important; }
.mold-detail-drawer.el-drawer .el-form { min-width: 0; }
.mold-import-dialog .el-dialog__body { max-height: min(62vh, 560px); overflow: auto; }
@media (max-width: 760px) {
  .mold-detail-drawer.el-drawer .el-drawer__header, .mold-detail-drawer.el-drawer .el-drawer__body { padding-right: var(--bb-space-4); padding-left: var(--bb-space-4); }
}
</style>
