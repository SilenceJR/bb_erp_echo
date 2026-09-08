<template>
  <section class="mold-page" aria-label="模具">
    <template v-if="!detailVisible">
      <PageHeader title="模具">
        <template #actions>
          <el-dropdown trigger="click"><el-button ref="moreActionsButton">更多操作</el-button><template #dropdown><el-dropdown-menu>
            <el-dropdown-item v-if="canWrite" @click="openLocationDialog">位置管理</el-dropdown-item>
            <el-dropdown-item v-if="canRead" :disabled="exporting || templateLoading" @click="exportPackage">{{ exportDefinition.label }}</el-dropdown-item>
            <el-dropdown-item v-if="canImport" @click="openImport">{{ importDefinition.label }}</el-dropdown-item>
            <el-dropdown-item v-if="canWrite" :disabled="!total" @click="selectAllFiltered">全选筛选结果</el-dropdown-item>
          </el-dropdown-menu></template></el-dropdown>
          <el-button v-if="canWrite" class="mold-create-button" type="primary" @click="openCreate">新增模具</el-button>
        </template>
      </PageHeader>
      <div class="mold-toolbar">
        <div class="mold-filters">
          <el-input v-model.trim="filters.q" clearable aria-label="搜索模具编号、型号或备注" placeholder="搜索模具编号、型号或备注" @keyup.enter="applyFilters" />
          <el-select v-model="filters.type" clearable aria-label="模具类型" placeholder="模具类型" @change="applyFilters"><el-option label="单模" value="single" /><el-option label="共模" value="common" /></el-select>
          <MoldLocationPicker v-model="filters.locationID" :locations="locations" clearable compact label="筛选" @update:model-value="applyFilters" />
          <el-input v-model.trim="filters.groupNo" clearable aria-label="共模组号" placeholder="共模组号" @keyup.enter="applyFilters" />
          <el-button @click="applyFilters">查询</el-button><el-button @click="resetFilters">重置</el-button>
        </div>
      </div>

      <div class="selection-bar" v-if="selectedIDs.size">
        <span>已选择 {{ selectedIDs.size }} 个模具</span>
        <div><el-button v-if="canWrite" @click="openBulkDialog">移动位置</el-button><el-button @click="clearSelection">取消选择</el-button></div>
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
          <el-table-column label="操作" width="120" fixed="right"><template #default="{row}"><span class="mold-detail-trigger" :data-mold-id="row.id"><el-button link type="primary" @click="openDetail(row.id, $event)">详情</el-button></span></template></el-table-column>
        </el-table>
      </DataTableShell>
    </template>

    <section v-else class="mold-detail-page" aria-label="模具详情">
      <header class="mold-detail-page__header">
        <div class="mold-detail-page__identity">
          <el-button class="mold-detail-page__back" link @click="requestDetailClose">← 返回模具列表</el-button>
          <div class="mold-detail-page__title">
            <h1>{{ draft.mold_number || '未命名模具' }}</h1>
            <span class="mold-detail-page__model">{{ draft.model || '未填写型号' }}</span>
          </div>
        </div>
        <div class="mold-detail-page__actions">
          <el-tag v-if="!detailLoading && !detailLoadError" effect="plain" :type="draft.mold_type === 'common' ? 'info' : 'success'">{{ typeLabel(draft.mold_type) }}</el-tag>
          <template v-if="!detailLoading && !detailLoadError && !editing && detailID && canWrite">
            <el-button class="mold-detail-page__delete" type="danger" plain :loading="deleting" :disabled="detailDeleteBusy" @click="deleteMold">删除模具</el-button>
            <el-button type="primary" plain :disabled="detailDeleteBusy" @click="startEditing">编辑资料</el-button>
          </template>
        </div>
      </header>

      <PageState v-if="detailLoading" kind="loading" title="正在加载模具详情" />
      <PageState v-else-if="detailLoadError" kind="error" title="模具详情加载失败" :description="detailLoadError" action-label="重新加载" @action="retryDetail" />
      <template v-else>
        <el-form v-if="editing" id="mold-editor" class="mold-editor" label-position="top" @submit.prevent="save">
          <section class="drawer-section-heading"><div><h2>基础信息</h2><small>每条模具档案对应一个产品型号。</small></div></section>
          <div class="form-grid">
            <el-form-item label="模具编号" required><el-input v-model.trim="draft.mold_number" autofocus /></el-form-item>
            <el-form-item label="模具型号" required><el-input v-model.trim="draft.model" /></el-form-item>
            <el-form-item label="模具类型" required><el-select v-model="draft.mold_type"><el-option label="单模" value="single" /><el-option label="共模" value="common" /></el-select></el-form-item>
            <el-form-item class="mold-form-item--wide" label="模具位置" required><MoldLocationPicker v-model="draft.location_id" :locations="assignableLocations" label="模具" /></el-form-item>
            <el-form-item v-if="draft.mold_type === 'common'" label="共模组号" required><el-input v-model.trim="draft.common_group_no" /></el-form-item>
            <el-form-item class="mold-form-item--wide" label="备注"><el-input v-model.trim="draft.remark" type="textarea" :rows="3" /></el-form-item>
          </div>
          <el-alert v-if="detailError" :title="detailError" type="error" :closable="false" show-icon />
        </el-form>
        <div v-else class="mold-detail-view">
          <section class="mold-detail-info" aria-label="模具基础信息">
            <el-alert v-if="detailError" :title="detailError" type="error" :closable="false" show-icon />
            <PropertyList>
              <PropertyItem label="模具编号"><span class="detail-value text-cell">{{ display(draft.mold_number) }}</span></PropertyItem>
              <PropertyItem label="模具型号"><span class="detail-value">{{ display(draft.model) }}</span></PropertyItem>
              <PropertyItem label="模具类型"><span class="detail-value">{{ typeLabel(draft.mold_type) }}</span></PropertyItem>
              <PropertyItem label="模具位置"><span class="detail-value">{{ locationLabel(draft.location_id) }}</span></PropertyItem>
              <PropertyItem v-if="draft.mold_type === 'common'" label="共模组号"><span class="detail-value text-cell">{{ display(draft.common_group_no) }}</span></PropertyItem>
              <PropertyItem label="备注"><span class="detail-value">{{ display(draft.remark) }}</span></PropertyItem>
            </PropertyList>
          </section>
          <div v-if="detailID" class="mold-detail-assets">
            <div class="mold-gallery-grid">
              <ImageGallery variant="mold-detail" owner-type="mold" :owner-id="detailID" :token="token" :can-write="canWrite" category="product_material" title="产品图" @busy-change="productGalleryBusy = $event" />
              <ImageGallery variant="mold-detail" owner-type="mold" :owner-id="detailID" :token="token" :can-write="canWrite" category="supplement" title="模具图" @busy-change="supplementGalleryBusy = $event" />
            </div>
            <section class="drawing-panel" data-file-drop-target :class="{'is-dragging': drawingDragging}" :aria-busy="drawingSaving" aria-label="DWG 图纸" @dragenter.prevent="handleDrawingDragEnter" @dragover.prevent="handleDrawingDragOver" @dragleave.prevent="handleDrawingDragLeave" @drop.prevent="handleDrawingDrop" @bb-native-file-drag="handleNativeDrawingDrag">
              <div class="section-heading"><div><h2>DWG 图纸</h2><small>支持单个 .dwg、.fdwg 文件；可拖放到此区域。</small></div><el-button v-if="canWrite" :loading="drawingSaving" :disabled="drawingSaving" @click="drawingInput?.click()">上传图纸</el-button></div>
              <input ref="drawingInput" class="sr-only" type="file" accept=".dwg,.fdwg" @change="uploadDrawing" />
              <div v-if="drawings.length" class="drawing-list"><div v-for="item in drawings" :key="item.id"><span>{{ item.original_name }}</span><small>{{ formatSize(item.size) }}</small><div><el-button link type="primary" @click="downloadDrawing(item)">下载</el-button><el-button v-if="canWrite" link type="danger" :disabled="drawingSaving" @click="deleteDrawing(item)">删除</el-button></div></div></div><p v-else class="drawer-empty">暂无 DWG 图纸</p><p v-if="canWrite" class="drawing-drop-hint">{{ drawingSaving ? '图纸正在上传，请稍候' : drawingDragging ? '松开以上传图纸' : '也可以将单个 .dwg 或 .fdwg 文件拖到此区域' }}</p>
            </section>
          </div>
        </div>
      </template>
      <footer v-if="editing" class="mold-detail-page__footer">
        <el-button :disabled="saving || deleting || drawingSaving" @click="editing ? cancelEditing() : requestDetailClose()">{{ editing ? (detailID ? '取消编辑' : '取消') : '返回列表' }}</el-button>
        <el-button v-if="editing" type="primary" native-type="submit" form="mold-editor" :loading="saving" :disabled="saving">保存模具</el-button>
      </footer>
    </section>

    <el-dialog v-model="bulkDialog" title="批量移动模具" width="min(520px, calc(100vw - 32px))" :close-on-click-modal="!bulkSaving" :close-on-press-escape="!bulkSaving" :before-close="beforeBulkClose"><p>将已选择的 {{ selectedIDs.size }} 个模具统一移动到：</p><MoldLocationPicker v-model="bulkLocationID" :locations="assignableLocations" label="批量移动" /><template #footer><el-button :disabled="bulkSaving" @click="requestBulkClose">取消</el-button><el-button type="primary" :loading="bulkSaving" :disabled="!bulkLocationID || bulkSaving" @click="bulkMove">确认移动</el-button></template></el-dialog>

    <ResponsiveDetailCarrier v-model="locationDialog" drawer-class="business-form-drawer mold-location-carrier workspace-detail-drawer" title="模具位置管理" :docked="locationPanel.docked.value" :size="locationPanel.size.value" docked-auto-focus="first-editable" :close-on-click-modal="!locationSaving" :close-on-press-escape="!locationSaving" :before-close="beforeLocationClose" destroy-on-close>
      <section class="location-management-section">
        <div class="drawer-section-heading"><div><h3>批量创建 / 扩展货架</h3><small>输入区名和上限，只补充缺少的位置，不影响停用状态。</small></div></div>
        <el-form id="mold-location-bulk-editor" label-position="top" :disabled="locationSaving" @submit.prevent="createBulkLocations">
          <div class="location-bulk-form">
            <el-form-item label="区名" required><el-input v-model.trim="bulkLocation.zone" maxlength="8" placeholder="例如 E" @input="bulkLocation.zone = bulkLocation.zone.toUpperCase().replace(/[^A-Z]/g, '')" /></el-form-item>
            <el-form-item label="排数" required><el-input-number v-model="bulkLocation.rows" :min="1" :max="100" controls-position="right" /></el-form-item>
            <el-form-item label="列数" required><el-input-number v-model="bulkLocation.columns" :min="1" :max="100" controls-position="right" /></el-form-item>
          </div>
          <el-alert :title="bulkLocationPreviewTitle" :type="bulkLocationPreviewCount ? 'info' : 'success'" :closable="false" show-icon />
        </el-form>
      </section>
      <section class="location-management-section">
        <div class="drawer-section-heading"><div><h3>新增自定义位置</h3><small>可保留旧资料包中的位置编码。</small></div></div>
        <el-form id="mold-location-editor" label-position="top" :disabled="locationSaving" @submit.prevent="createLocation"><el-form-item label="自定义位置编码" required><el-input v-model.trim="newLocation" placeholder="例如 A2-1 或自定义编码" /></el-form-item></el-form>
      </section>
      <div class="location-list"><div v-for="item in locations" :key="item.id"><span>{{ item.code }}</span><el-tag :type="item.status === 'active' ? 'success' : 'info'">{{ item.status === 'active' ? '启用' : '停用' }}</el-tag><el-button v-if="canWrite" link :type="item.status === 'active' ? 'danger' : 'primary'" @click="toggleLocation(item)">{{ item.status === 'active' ? '停用' : '启用' }}</el-button></div></div>
      <template #footer><div class="form-actions"><el-button :disabled="locationSaving" @click="requestLocationClose">关闭</el-button><el-button type="primary" native-type="submit" form="mold-location-bulk-editor" :loading="locationSaving" :disabled="!bulkLocation.zone.trim() || !bulkLocationPreviewCount">扩展货架</el-button><el-button plain type="primary" native-type="submit" form="mold-location-editor" :loading="locationSaving" :disabled="!newLocation.trim()">新增单个</el-button></div></template>
    </ResponsiveDetailCarrier>

    <el-dialog v-model="importDialog" class="mold-import-dialog" title="导入模具资料包" width="min(720px, calc(100vw - 32px))" :close-on-click-modal="!importing && !importPreviewing && !templateLoading" :close-on-press-escape="!importing && !importPreviewing && !templateLoading" :before-close="beforeImportClose" destroy-on-close @opened="focusImportDialogEntry" @closed="restoreImportTriggerFocus">
      <p>导入会全量更新模具档案和位置字典；资料目录存在的模具会覆盖其产品图、模具图和 DWG，无资料目录的模具保留现有资料，Excel 中删除的模具及资料会一并删除。</p>
      <section v-if="canImport" class="import-template-guide"><div><h3>{{ templateDefinition.label }}</h3><p>ZIP 根目录放置 molds.xlsx、locations.json 和扁平模具目录；允许额外套一层统一包装目录。单模目录使用完整模具编号，共模目录使用同组全部编号以 + 连接。图片与 DWG/FDWG 直接放在对应模具目录中，不要建立 images、drawings 或图片分类子目录。文件名含“产品刷墨图”或以 -正整数结尾的图片归产品图，其他图片归模具图，DWG/FDWG 归图纸。</p></div><el-button :loading="templateLoading" :disabled="templateLoading || exporting || importing || importPreviewing" @click="downloadTemplate">{{ templateDefinition.label }}</el-button></section>
      <input v-if="canImport && importDialog" ref="importInput" class="sr-only" type="file" :accept="importDefinition.accept" tabindex="-1" aria-hidden="true" @change="previewImport" />
      <div v-if="!importFile" class="import-dropzone" data-file-drop-target :class="{'is-dragging': importDragging}" role="button" tabindex="0" @click="chooseImportFile" @keydown.enter.prevent="chooseImportFile" @keydown.space.prevent="chooseImportFile" @dragenter.prevent="handleImportDragEnter" @dragover.prevent="handleImportDragOver" @dragleave.prevent="handleImportDragLeave" @drop.prevent="handleImportDrop" @bb-native-file-drag="handleNativeImportDrag"><strong>{{ importDragging ? '松开以导入 ZIP' : '将 ZIP 资料包拖到此处' }}</strong><span>或点击选择文件</span></div>
      <div v-if="importFile" class="import-file"><strong>{{ importFile.name }}</strong><span>{{ importResult ? '资料包预览完成' : '正在检查资料包…' }}</span></div>
      <el-alert v-if="importError" :title="importError" type="error" :closable="false" show-icon />
      <el-alert v-if="importResult" :title="`模具 ${importResult.summary.molds} 条，图片 ${importResult.summary.images} 张，图纸 ${importResult.summary.drawings} 个`" type="info" :closable="false" />
      <div v-if="importResult?.errors?.length" class="import-errors"><p v-for="item in importResult.errors" :key="`${item.row}-${item.column}-${item.value}`">{{ item.value || item.column }}：{{ item.reason }}</p></div>
      <div v-if="importResult?.unresolved?.length" class="import-corrections">
        <p>以下资料无法从文件名确定归属，请人工选择完整模具编号；图片还需选择类型：</p>
        <div class="import-correction-header" aria-hidden="true"><span>文件</span><span>类型</span><span>归属模具</span></div>
        <div v-for="item in importResult.unresolved" :key="item.path">
          <span>{{ item.name }}</span>
          <el-select v-if="item.kind !== 'drawing'" v-model="corrections[item.path].category" :aria-label="`${item.name}图片类型`" placeholder="选择图片类型"><el-option label="产品图" value="product_material" /><el-option label="模具图" value="supplement" /></el-select>
          <span v-else class="import-correction-kind">图纸</span>
          <el-select v-model="corrections[item.path].codes" multiple collapse-tags :aria-label="`${item.name}对应模具编号`" placeholder="选择模具编号"><el-option v-for="code in item.allowed_codes" :key="code" :label="code" :value="code" /></el-select>
        </div>
      </div>
      <template #footer><el-button :disabled="importing || importPreviewing || templateLoading" @click="requestImportClose">取消</el-button><el-button v-if="importFile && !importResult" :disabled="importing || importPreviewing || templateLoading" @click="chooseImportFile">重新选择</el-button><el-button type="primary" :loading="importing" :disabled="!canCommitImport || importPreviewing || templateLoading" @click="commitImport">确认导入并更新资料</el-button></template>
    </el-dialog>
  </section>
</template>

<script setup lang="ts">
import {computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch, type ComponentPublicInstance} from 'vue'
import {ElMessage} from 'element-plus'
import {appMessageBox} from '../../../composables/useAppMessageBox'
import {useDirtyGuard} from '../../../composables/useDirtyGuard'
import {useResponsiveDetailPanel} from '../../../composables/useResponsiveDetailPanel'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import {getTransferDefinition, getTransferImportDefinition} from '../../../data/transferRegistry'
import {useTransferDownload} from '../../../composables/useTransferDownload'
import {downloadApiFile, request, uploadNativeFiles} from '../../../api/http'
import {dirtyGuardRegistry} from '../../../platform/dirtyGuard'
import type {NativeFileDragDetail} from '../../../types'
import DataTableShell from '../../ui/DataTableShell.vue'
import PageHeader from '../../ui/PageHeader.vue'
import ImageGallery from '../../ImageGallery.vue'
import PageState from '../../ui/PageState.vue'
import PropertyItem from '../../ui/PropertyItem.vue'
import PropertyList from '../../ui/PropertyList.vue'
import ResponsiveDetailCarrier from '../../ui/ResponsiveDetailCarrier.vue'
import MoldLocationPicker from './MoldLocationPicker.vue'
import {missingShelfCodes, type MoldLocationOption} from './moldLocation'

type Location = MoldLocationOption & {status: 'active' | 'disabled'}
type Mold = {id: number; mold_number: string; model: string; mold_type: 'single' | 'common'; location_id?: number; location?: Location; common_group_no?: string; remark?: string; image_count?: number}
type Drawing = {id: number; original_name: string; size: number}
type Preview = {token?: string; summary: {molds: number; images: number; drawings: number; unresolved: number}; errors: Array<{row: number; column: string; value?: string; reason: string}>; unresolved: Array<{path: string; name: string; kind?: 'image' | 'drawing'; allowed_codes: string[]}>}
const {token, hasPermission, setPageDetailPanelVisible} = useWorkspaceContext()
const importDefinition = getTransferImportDefinition('molds')
const templateDefinition = getTransferDefinition('molds', 'template')
const exportDefinition = getTransferDefinition('molds', 'export')
const canRead = computed(() => hasPermission(exportDefinition.permission))
const canWrite = computed(() => hasPermission('mold:write'))
const canImport = computed(() => hasPermission(importDefinition.permission))
const {download: downloadTransfer, isLoading: isTransferLoading} = useTransferDownload(token)
const moreActionsButton = ref<ComponentPublicInstance | null>(null)
const rows = ref<Mold[]>([]), total = ref(0), page = ref(1), pageSize = ref(20), loading = ref(false), error = ref('')
const locations = ref<Location[]>([]), selectedIDs = ref(new Set<number>())
const filters = reactive({q: '', type: '', locationID: undefined as number | undefined, groupNo: ''})
const detailVisible = ref(false), detailID = ref<number | null>(null), detailLoading = ref(false), detailLoadError = ref(''), detailError = ref(''), editing = ref(false), saving = ref(false), deleting = ref(false)
const detailGeneration = ref(0)
const draft = reactive({mold_number: '', model: '', mold_type: 'single' as 'single' | 'common', location_id: undefined as number | undefined, common_group_no: '', remark: ''})
const drawings = ref<Drawing[]>([]), drawingSaving = ref(false), drawingInput = ref<HTMLInputElement | null>(null), importInput = ref<HTMLInputElement | null>(null)
const productGalleryBusy = ref(false), supplementGalleryBusy = ref(false)
const drawingDragging = ref(false), importDragging = ref(false)
let drawingDragDepth = 0
let importDragDepth = 0
const bulkDialog = ref(false), bulkLocationID = ref<number>(), bulkSaving = ref(false), locationDialog = ref(false), newLocation = ref(''), locationSaving = ref(false)
const bulkLocation = reactive({zone: 'A', rows: 7, columns: 4})
const locationInitial = ref('')
const importDialog = ref(false), importFile = ref<File | null>(null), importPath = ref<string | null>(null), importResult = ref<Preview | null>(null), importError = ref(''), importing = ref(false), importPreviewing = ref(false), corrections = reactive<Record<string, {category: string; codes: string[]}>>({})
const exporting = computed(() => isTransferLoading('molds', 'export'))
const templateLoading = computed(() => isTransferLoading('molds', 'template'))
const assignableLocations = computed(() => locations.value.filter((item) => item.status === 'active' || item.id === draft.location_id))
const canCommitImport = computed(() => Boolean(importFile.value && importResult.value?.token && !importing.value && !(importResult.value?.unresolved || []).some((item) => (item.kind !== 'drawing' && !corrections[item.path]?.category) || !corrections[item.path]?.codes.length)))
const savedDraft = ref('')
const draftDirty = computed(() => detailVisible.value && editing.value && savedDraft.value !== JSON.stringify(draft))
const locationDirty = computed(() => locationDialog.value && locationInitial.value !== JSON.stringify({newLocation: newLocation.value, ...bulkLocation}))
const bulkDirty = computed(() => bulkDialog.value && Boolean(bulkLocationID.value))
const importDirty = computed(() => importDialog.value && Boolean(importFile.value || importResult.value || importError.value))
const moldBusy = computed(() => saving.value || deleting.value || drawingSaving.value || bulkSaving.value || locationSaving.value || importing.value || importPreviewing.value || templateLoading.value || exporting.value)
const detailDeleteBusy = computed(() => saving.value || deleting.value || drawingSaving.value || productGalleryBusy.value || supplementGalleryBusy.value || dirtyGuardRegistry.blocksUnload())
const moldDirty = computed(() => draftDirty.value || locationDirty.value || bulkDirty.value || importDirty.value)
useDirtyGuard('mold-form', {busy: () => moldBusy.value, dirty: () => moldDirty.value, busyMessage: '模具操作正在提交，请稍候', dirtyMessage: '模具页面有尚未保存的修改，确认离开？'})
const locationPanel = useResponsiveDetailPanel(locationDialog, {complexity: 'standard-form'})
const detailReturnFocus = ref<{kind: 'mold'; id: number} | {kind: 'create'} | null>(null)
const listScrollTop = ref(0)
const bulkLocationPreviewCodes = computed(() => missingShelfCodes(locations.value, bulkLocation.zone, bulkLocation.rows, bulkLocation.columns))
const bulkLocationPreviewCount = computed(() => bulkLocationPreviewCodes.value.length)
const bulkLocationPreviewTitle = computed(() => {
  const total = Math.max(0, bulkLocation.rows) * Math.max(0, bulkLocation.columns)
  return bulkLocationPreviewCount.value ? `将新增 ${bulkLocationPreviewCount.value} 个位置（本次范围共 ${total} 个）` : '当前范围内的位置已经存在，无需重复创建'
})

watch(locationDialog, (locationOpen) => setPageDetailPanelVisible(locationOpen), {immediate: true, flush: 'sync'})

async function load() { loading.value = true; error.value = ''; try { const params = new URLSearchParams({page: String(page.value), page_size: String(pageSize.value)}); if (filters.q) params.set('q', filters.q); if (filters.type) params.set('mold_type', filters.type); if (filters.locationID) params.set('location_id', String(filters.locationID)); if (filters.groupNo) params.set('common_group_no', filters.groupNo); const result = await request<{items: Mold[]; total: number}>(`/api/v1/molds?${params}`, {}, token.value); rows.value = result.items || []; total.value = result.total || 0 } catch (cause) { error.value = cause instanceof Error ? cause.message : '模具列表加载失败' } finally { loading.value = false } }
async function loadLocations() { locations.value = await request<Location[]>('/api/v1/mold-locations?include_disabled=true', {}, token.value) }
function applyFilters() { page.value = 1; void load() }
function resetFilters() { Object.assign(filters, {q: '', type: '', locationID: undefined, groupNo: ''}); applyFilters() }
function changePage(value: number) { page.value = value; void load() }
function changePageSize(value: number) { pageSize.value = value; page.value = 1; void load() }
function typeLabel(value: string) { return value === 'common' ? '共模' : '单模' }
function toggleSelection(id: number) { const next = new Set(selectedIDs.value); next.has(id) ? next.delete(id) : next.add(id); selectedIDs.value = next }
function clearSelection() { selectedIDs.value = new Set() }
function openBulkDialog() { bulkLocationID.value = undefined; bulkDialog.value = true }
async function selectAllFiltered() { const ids = new Set<number>(); let current = 1; do { const params = new URLSearchParams({page: String(current), page_size: '200'}); if (filters.q) params.set('q', filters.q); if (filters.type) params.set('mold_type', filters.type); if (filters.locationID) params.set('location_id', String(filters.locationID)); if (filters.groupNo) params.set('common_group_no', filters.groupNo); const result = await request<{items: Mold[]; total: number}>(`/api/v1/molds?${params}`, {}, token.value); for (const item of result.items || []) ids.add(item.id); current++; if (!result.items?.length) break; if (ids.size >= result.total) break } while (current < 1000); selectedIDs.value = ids; ElMessage.success(`已选择 ${ids.size} 个筛选结果`) }
function resetDraft() { Object.assign(draft, {mold_number: '', model: '', mold_type: 'single', location_id: locations.value.find((item) => item.status === 'active')?.id, common_group_no: '', remark: ''}) }
function clearDetailDraft() { Object.assign(draft, {mold_number: '', model: '', mold_type: 'single', location_id: undefined, common_group_no: '', remark: ''}) }
function rememberListView(event?: Event) {
  const currentTarget = event?.currentTarget instanceof HTMLElement ? event.currentTarget : null
  const eventTarget = currentTarget?.closest<HTMLElement>('[data-mold-id]') || (event?.target instanceof Element ? event.target.closest<HTMLElement>('[data-mold-id]') : null)
  const target = eventTarget || (document.activeElement instanceof HTMLElement ? document.activeElement.closest<HTMLElement>('[data-mold-id]') : null)
  const moldID = Number(target?.getAttribute('data-mold-id'))
  detailReturnFocus.value = Number.isInteger(moldID) && moldID > 0 ? {kind: 'mold', id: moldID} : {kind: 'create'}
  const content = document.querySelector<HTMLElement>('.content')
  listScrollTop.value = content?.scrollTop || 0
}
function findListFocusTarget(returnFocus: typeof detailReturnFocus.value) {
  if (returnFocus?.kind === 'mold') {
    const root = document.querySelector<HTMLElement>(`[data-mold-id="${returnFocus.id}"]`)
    return root?.matches('button') ? root : root?.querySelector<HTMLElement>('button, [role="button"]') || null
  }
  if (returnFocus?.kind === 'create') return document.querySelector<HTMLElement>('.mold-create-button')
  return null
}
async function restoreListView() {
  await nextTick()
  document.querySelector<HTMLElement>('.content')?.scrollTo({top: listScrollTop.value})
  const returnFocus = detailReturnFocus.value
  detailReturnFocus.value = null
  let target = findListFocusTarget(returnFocus)
  if (!target) {
    // The table component can finish rebuilding its body one tick after the
    // v-if branch is mounted. Give it that tick before falling back to body.
    await nextTick()
    target = findListFocusTarget(returnFocus)
  }
  if (!target) target = document.querySelector<HTMLElement>('.mold-create-button, [data-mold-id] button, .mold-page .ui-page-header__actions button')
  if (target?.isConnected) target.focus({preventScroll: true})
}
function scrollDetailToTop() { void nextTick(() => document.querySelector<HTMLElement>('.content')?.scrollTo({top: 0})) }
async function confirmDetailLeave() { return await dirtyGuardRegistry.confirmLeave('dialog-close') }
async function focusMoldEditor() { await nextTick(); document.querySelector<HTMLElement>('#mold-editor input:not([type="hidden"]), #mold-editor textarea, #mold-editor button, #mold-editor [tabindex]:not([tabindex="-1"])')?.focus({preventScroll: true}) }
function startEditing() { if (detailDeleteBusy.value) return; editing.value = true; void focusMoldEditor() }
async function openCreate(event?: Event) { if (detailVisible.value && !(await confirmDetailLeave())) return; rememberListView(event); detailGeneration.value += 1; resetDraft(); savedDraft.value = JSON.stringify(draft); detailID.value = null; detailLoadError.value = ''; detailError.value = ''; drawings.value = []; editing.value = true; detailVisible.value = true; scrollDetailToTop(); void focusMoldEditor() }
async function openLocationDialog() { if (detailVisible.value) { if (!(await requestDetailClose())) return }; newLocation.value = ''; bulkLocation.zone = 'A'; bulkLocation.rows = 7; bulkLocation.columns = 4; locationInitial.value = JSON.stringify({newLocation: newLocation.value, ...bulkLocation}); locationDialog.value = true }
async function openDetail(id: number, event?: Event) {
  if (detailVisible.value && !(await confirmDetailLeave())) return
  if (!detailVisible.value) { rememberListView(event); clearDetailDraft() }
  const generation = ++detailGeneration.value
  detailID.value = id
  detailVisible.value = true
  editing.value = false
  detailLoading.value = true
  detailLoadError.value = ''
  detailError.value = ''
  scrollDetailToTop()
  try {
    const item = await request<Mold>(`/api/v1/molds/${id}`, {}, token.value)
    if (generation !== detailGeneration.value || !detailVisible.value || detailID.value !== id) return
    Object.assign(draft, item, {location_id: item.location_id, common_group_no: item.common_group_no || '', remark: item.remark || ''})
    savedDraft.value = JSON.stringify(draft)
    const nextDrawings = await request<Drawing[]>(`/api/v1/molds/${id}/drawings`, {}, token.value)
    if (generation === detailGeneration.value && detailVisible.value && detailID.value === id) drawings.value = nextDrawings
  } catch (cause) {
    if (generation === detailGeneration.value && detailVisible.value && detailID.value === id) detailLoadError.value = cause instanceof Error ? cause.message : '模具详情加载失败'
  } finally {
    if (generation === detailGeneration.value) detailLoading.value = false
  }
}
async function save() { if (!draft.mold_number.trim() || !draft.model.trim() || !draft.location_id || (draft.mold_type === 'common' && !draft.common_group_no.trim())) { detailError.value = '请填写编号、型号、类型、位置及共模组号等必填项'; return }; saving.value = true; detailError.value = ''; try { const body = {...draft, location_id: Number(draft.location_id), ...(draft.mold_type === 'single' ? {common_group_no: ''} : {})}; const item = detailID.value ? await request<Mold>(`/api/v1/molds/${detailID.value}`, {method: 'PATCH', body}, token.value) : await request<Mold>('/api/v1/molds', {method: 'POST', body}, token.value); detailID.value = item.id; Object.assign(draft, item, {location_id: item.location_id, common_group_no: item.common_group_no || '', remark: item.remark || ''}); savedDraft.value = JSON.stringify(draft); editing.value = false; await Promise.all([load(), loadLocations()]); ElMessage.success('模具档案已保存') } catch (cause) { detailError.value = cause instanceof Error ? cause.message : '模具保存失败' } finally { saving.value = false } }
function retryDetail() { if (detailID.value) { clearDetailDraft(); void openDetail(detailID.value) } }
function resetDetail() { detailGeneration.value += 1; detailID.value = null; detailLoading.value = false; detailLoadError.value = ''; detailError.value = ''; editing.value = false; drawings.value = []; savedDraft.value = ''; drawingDragging.value = false; drawingDragDepth = 0; productGalleryBusy.value = false; supplementGalleryBusy.value = false; clearDetailDraft() }
async function beforeDetailClose() { return await confirmDetailLeave() }
async function requestDetailClose() { if (!(await beforeDetailClose())) return false; detailVisible.value = false; resetDetail(); await restoreListView(); return true }
function restoreDraft() { if (!savedDraft.value) return; try { Object.assign(draft, JSON.parse(savedDraft.value)) } catch { /* the current draft remains visible if a legacy response is malformed */ } }
async function cancelEditing() { if (detailID.value) { if (!(await confirmDetailLeave())) return; restoreDraft(); editing.value = false } else await requestDetailClose() }
function display(value?: string) { return value?.trim() || '未填写' }
function locationLabel(id?: number) { return locations.value.find((item) => item.id === Number(id))?.code || (id ? `位置 #${id}` : '—') }
async function beforeBulkClose(done: () => void) { if (bulkSaving.value) { ElMessage.warning('批量移动正在提交，请稍候'); return }; if (bulkDirty.value) { try { await appMessageBox.confirm('批量移动尚未提交，确认关闭？', '放弃批量移动', {type: 'warning'}); } catch { return } }; bulkLocationID.value = undefined; done() }
async function requestBulkClose() { await beforeBulkClose(() => { bulkDialog.value = false }) }
async function beforeLocationClose(done: () => void) { if (locationSaving.value) { ElMessage.warning('位置正在保存，请稍候'); return }; if (locationDirty.value) { try { await appMessageBox.confirm('位置改动尚未提交，确认关闭？', '放弃位置改动', {type: 'warning'}); } catch { return } }; newLocation.value = ''; bulkLocation.zone = 'A'; bulkLocation.rows = 7; bulkLocation.columns = 4; done() }
async function requestLocationClose() { await beforeLocationClose(() => { locationDialog.value = false }) }
async function beforeImportClose(done: () => void) { if (importing.value || importPreviewing.value || templateLoading.value) { ElMessage.warning(templateLoading.value ? '模板正在下载，请稍候' : importPreviewing.value ? '资料包正在检查，请稍候' : '资料包正在导入，请稍候'); return }; if (importDirty.value) { try { await appMessageBox.confirm('资料包预览或人工映射尚未完成，确认关闭？', '放弃资料包导入', {type: 'warning'}); } catch { return } }; done() }
async function requestImportClose() { await beforeImportClose(() => { importDialog.value = false }) }
async function deleteMold() {
  if (!detailID.value || detailDeleteBusy.value) return
  try {
    const confirmation = appMessageBox.confirm(`确认删除模具“${draft.mold_number}”吗？\n\n产品图、模具图和 DWG 图纸都会一并删除，且永久删除后无法恢复。`, '删除模具', {
      type: 'warning',
      confirmButtonText: '确认永久删除',
      cancelButtonText: '取消',
      confirmButtonType: 'danger',
      customClass: 'mold-delete-confirm',
      autofocus: false,
    })
    // Element Plus exposes only a boolean autofocus option; focus the safe
    // cancel button after mounting and the focus trap have both settled.
    await nextTick()
    requestAnimationFrame(() => document.querySelector<HTMLElement>('.mold-delete-confirm .el-message-box__btns .el-button:first-child')?.focus({preventScroll: true}))
    await confirmation
  } catch {
    return
  }
  if (detailDeleteBusy.value) { ElMessage.warning('模具仍有上传或保存操作，请完成后再删除'); return }
  deleting.value = true
  try {
    await request(`/api/v1/molds/${detailID.value}`, {method: 'DELETE'}, token.value)
    detailVisible.value = false
    resetDetail()
    await load()
    await restoreListView()
    ElMessage.success('模具已删除')
  } catch (cause) {
    ElMessage.error(cause instanceof Error ? cause.message : '模具删除失败')
  } finally {
    deleting.value = false
  }
}
async function bulkMove() { if (!bulkLocationID.value || !selectedIDs.value.size) return; bulkSaving.value = true; try { await request('/api/v1/molds/bulk-location', {method: 'POST', body: {mold_ids: [...selectedIDs.value], location_id: bulkLocationID.value}}, token.value); bulkDialog.value = false; selectedIDs.value = new Set(); await load(); ElMessage.success('模具位置已批量更新') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '批量移动失败') } finally { bulkSaving.value = false } }
async function createBulkLocations() {
  const zone = bulkLocation.zone.trim().toUpperCase()
  if (!/^[A-Z]{1,8}$/.test(zone)) { ElMessage.warning('区名只能使用 1-8 位英文字母'); return }
  if (!Number.isInteger(bulkLocation.rows) || !Number.isInteger(bulkLocation.columns) || bulkLocation.rows < 1 || bulkLocation.columns < 1) { ElMessage.warning('排数和列数必须为正整数'); return }
  if (!bulkLocationPreviewCount.value) { ElMessage.info('当前范围内的位置已经存在'); return }
  locationSaving.value = true
  try {
    const result = await request<{created: number}>('/api/v1/mold-locations/bulk', {method: 'POST', body: {zone, rows: bulkLocation.rows, columns: bulkLocation.columns}}, token.value)
    await loadLocations()
    locationInitial.value = JSON.stringify({newLocation: newLocation.value, ...bulkLocation})
    ElMessage.success(`已新增 ${Number(result?.created || 0)} 个位置`)
  } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '批量位置创建失败') } finally { locationSaving.value = false }
}
async function createLocation() { if (!newLocation.value.trim()) return; locationSaving.value = true; try { await request('/api/v1/mold-locations', {method: 'POST', body: {code: newLocation.value.trim()}}, token.value); newLocation.value = ''; locationInitial.value = JSON.stringify({newLocation: newLocation.value, ...bulkLocation}); await loadLocations(); ElMessage.success('位置已新增') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '位置新增失败') } finally { locationSaving.value = false } }
async function toggleLocation(item: Location) { const status = item.status === 'active' ? 'disabled' : 'active'; try { await appMessageBox.confirm(`确认${status === 'disabled' ? '停用' : '启用'}位置“${item.code}”吗？`, `${status === 'disabled' ? '停用' : '启用'}位置`, {type: status === 'disabled' ? 'warning' : 'info', confirmButtonText: `确认${status === 'disabled' ? '停用' : '启用'}`}) } catch { return }; try { await request(`/api/v1/mold-locations/${item.id}`, {method: 'PATCH', body: {status}}, token.value); await loadLocations(); ElMessage.success(`位置已${status === 'disabled' ? '停用' : '启用'}`) } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '位置状态更新失败') } }
function hasDraggedFiles(event: DragEvent): boolean { return Array.from(event.dataTransfer?.types || []).some((type) => type.toLowerCase() === 'files') }
function handleDrawingDragEnter(event: DragEvent) { if (!canWrite.value || drawingSaving.value || !hasDraggedFiles(event)) return; drawingDragDepth++; drawingDragging.value = true }
function handleDrawingDragOver(event: DragEvent) { if (canWrite.value && !drawingSaving.value && hasDraggedFiles(event) && event.dataTransfer) event.dataTransfer.dropEffect = 'copy' }
function handleDrawingDragLeave() { drawingDragDepth = Math.max(0, drawingDragDepth - 1); if (!drawingDragDepth) drawingDragging.value = false }
function handleDrawingDrop(event: DragEvent) {
  drawingDragDepth = 0
  drawingDragging.value = false
  if (!canWrite.value) { ElMessage.warning('当前账号没有图纸上传权限'); return }
  if (drawingSaving.value) { ElMessage.warning('图纸正在上传，请稍候'); return }
  const files = Array.from(event.dataTransfer?.files || [])
  if (files.length !== 1) { ElMessage.warning('请一次拖入一个 DWG 或 FDWG 文件'); return }
  void uploadDrawingFile(files[0])
}
function handleNativeDrawingDrag(event: Event) {
  const detail = (event as CustomEvent<NativeFileDragDetail>).detail
  if (!detail) return
  if (detail.phase === 'enter' || detail.phase === 'over') { if (canWrite.value && !drawingSaving.value) drawingDragging.value = true; return }
  drawingDragging.value = false
  if (detail.phase === 'leave') return
  if (detail.phase !== 'drop') return
  if (!canWrite.value) { ElMessage.warning('当前账号没有图纸上传权限'); return }
  if (drawingSaving.value) { ElMessage.warning('图纸正在上传，请稍候'); return }
  if (detail.error) { ElMessage.error(detail.error); return }
  const totalFiles = detail.paths.length + detail.files.length
  if (totalFiles !== 1) { ElMessage.warning('请一次拖入一个 DWG 或 FDWG 文件'); return }
  if (detail.paths.length === 1) void uploadNativeDrawingFile(detail.paths[0])
  else void uploadDrawingFile(detail.files[0])
}
function uploadDrawing(event: Event) { const input = event.target as HTMLInputElement; const file = input.files?.[0]; input.value = ''; if (file) void uploadDrawingFile(file) }
async function uploadDrawingFile(file: File) { if (!detailID.value || !canWrite.value || drawingSaving.value) return; if (!/\.(dwg|fdwg)$/i.test(file.name)) { ElMessage.warning('仅支持 .dwg、.fdwg 图纸'); return }; drawingSaving.value = true; try { const body = new FormData(); body.append('file', file); await request(`/api/v1/molds/${detailID.value}/drawings`, {method: 'POST', body}, token.value); drawings.value = await request<Drawing[]>(`/api/v1/molds/${detailID.value}/drawings`, {}, token.value); ElMessage.success('图纸已上传') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '图纸上传失败') } finally { drawingSaving.value = false } }
async function uploadNativeDrawingFile(path: string) { if (!detailID.value || !canWrite.value || drawingSaving.value) return; if (!/\.(dwg|fdwg)$/i.test(path)) { ElMessage.warning('仅支持 .dwg、.fdwg 图纸'); return }; drawingSaving.value = true; try { await uploadNativeFiles(`/api/v1/molds/${detailID.value}/drawings`, [path], {}, token.value); drawings.value = await request<Drawing[]>(`/api/v1/molds/${detailID.value}/drawings`, {}, token.value); ElMessage.success('图纸已上传') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '图纸上传失败') } finally { drawingSaving.value = false } }
async function downloadDrawing(item: Drawing) { await downloadApiFile(`/api/v1/molds/${detailID.value}/drawings/${item.id}/content`, item.original_name, token.value) }
async function deleteDrawing(item: Drawing) { if (!detailID.value || drawingSaving.value) return; try { await appMessageBox.confirm(`确认删除“${item.original_name}”吗？`, '删除图纸', {type: 'warning', confirmButtonText: '确认删除', confirmButtonType: 'danger'}) } catch { return }; drawingSaving.value = true; try { await request(`/api/v1/molds/${detailID.value}/drawings/${item.id}`, {method: 'DELETE'}, token.value); drawings.value = drawings.value.filter((candidate) => candidate.id !== item.id); ElMessage.success('图纸已删除') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '图纸删除失败') } finally { drawingSaving.value = false } }
function chooseImportFile() { if (canImport.value && !importing.value && !importPreviewing.value) { importInput.value?.click() } }
function openImport() { if (!canImport.value) return; importFile.value = null; importPath.value = null; importResult.value = null; importError.value = ''; importDragging.value = false; importPreviewing.value = false; Object.keys(corrections).forEach((key) => delete corrections[key]); importDialog.value = true }
function handleImportDragEnter(event: DragEvent) { if (!canImport.value || importing.value || importPreviewing.value || !hasDraggedFiles(event)) return; importDragDepth++; importDragging.value = true }
function handleImportDragOver(event: DragEvent) { if (canImport.value && !importing.value && !importPreviewing.value && hasDraggedFiles(event) && event.dataTransfer) event.dataTransfer.dropEffect = 'copy' }
function handleImportDragLeave() { importDragDepth = Math.max(0, importDragDepth - 1); if (!importDragDepth) importDragging.value = false }
function handleImportDrop(event: DragEvent) { importDragDepth = 0; importDragging.value = false; if (!canImport.value) { ElMessage.warning('当前账号没有资料包导入权限'); return }; if (importing.value || importPreviewing.value) { ElMessage.warning('资料包正在处理，请稍候'); return }; const files = Array.from(event.dataTransfer?.files || []); if (files.length !== 1) { ElMessage.warning('请一次拖入一个 ZIP 资料包'); return }; void previewImportFile(files[0]) }
function handleNativeImportDrag(event: Event) { const detail = (event as CustomEvent<NativeFileDragDetail>).detail; if (!detail) return; if (detail.phase === 'enter' || detail.phase === 'over') { if (canImport.value && !importing.value && !importPreviewing.value) importDragging.value = true; return }; importDragging.value = false; if (detail.phase === 'leave') return; if (detail.phase !== 'drop') return; if (!canImport.value) { importError.value = '当前账号没有资料包导入权限'; ElMessage.warning(importError.value); return }; if (importing.value || importPreviewing.value) { importError.value = '资料包正在处理，请稍候'; ElMessage.warning(importError.value); return }; if (detail.error) { importError.value = detail.error; ElMessage.error(detail.error); return }; const totalFiles = detail.paths.length + detail.files.length; if (totalFiles !== 1) { ElMessage.warning('请一次拖入一个 ZIP 资料包'); return }; if (detail.paths.length) void previewImportPath(detail.paths[0]); else void previewImportFile(detail.files[0]) }
function previewImport(event: Event) { const input = event.target as HTMLInputElement; const file = input.files?.[0]; input.value = ''; if (file) void previewImportFile(file) }
const importExtensions = importDefinition.accept.split(',').map((value) => value.trim().toLowerCase()).filter((value) => value.startsWith('.'))
function acceptsImportFileName(name: string) { const normalizedName = name.trim().toLowerCase(); return importExtensions.some((extension) => normalizedName.endsWith(extension)) }
async function applyImportPreview(result: Preview) { importResult.value = result; Object.keys(corrections).forEach((key) => delete corrections[key]); for (const item of result.unresolved || []) corrections[item.path] = {category: '', codes: []} }
async function previewImportFile(file: File) { importPath.value = null; if (!acceptsImportFileName(file.name)) { importError.value = '仅支持 ZIP 资料包'; return }; importFile.value = file; importResult.value = null; importError.value = ''; const body = new FormData(); body.append('file', file); importPreviewing.value = true; try { await applyImportPreview(await request<Preview>(importDefinition.previewPath, {method: 'POST', body}, token.value)) } catch (cause) { importError.value = cause instanceof Error ? cause.message : '资料包预览失败'; importFile.value = null } finally { importPreviewing.value = false } }
async function previewImportPath(path: string) { if (!acceptsImportFileName(path)) { importError.value = '仅支持 ZIP 资料包'; return }; importPath.value = path; importFile.value = new File([], path.split(/[\\/]/).pop() || '拖入资料包'); importResult.value = null; importError.value = ''; importPreviewing.value = true; try { await applyImportPreview(await uploadNativeFiles<Preview>(importDefinition.previewPath, [path], {}, token.value)) } catch (cause) { importError.value = cause instanceof Error ? cause.message : '资料包预览失败'; importFile.value = null; importPath.value = null } finally { importPreviewing.value = false } }
async function commitImport() { if (!canCommitImport.value || !importFile.value || !importResult.value?.token) return; importing.value = true; importError.value = ''; try { const normalized = Object.fromEntries(Object.entries(corrections).map(([path, item]) => [path, {category: item.category, codes: item.codes}])); if (importPath.value) await uploadNativeFiles(importDefinition.commitPath, [importPath.value], {token: importResult.value.token, corrections: JSON.stringify(normalized)}, token.value); else { const body = new FormData(); body.append('file', importFile.value); body.append('token', importResult.value.token); body.append('corrections', JSON.stringify(normalized)); await request(importDefinition.commitPath, {method: 'POST', body}, token.value) } importDialog.value = false; await Promise.all([load(), loadLocations()]); ElMessage.success('模具资料包已导入') } catch (cause) { importError.value = cause instanceof Error ? cause.message : '资料包导入失败' } finally { importing.value = false } }
async function downloadTemplate() { importError.value = ''; try { await downloadTransfer('molds', 'template') } catch { /* 下载反馈由共享传输 composable 统一负责 */ } }
async function exportPackage() { try { await downloadTransfer('molds', 'export') } catch { /* composable already reports the failure */ } }
async function focusImportDialogEntry() {
  await nextTick()
  const dialog = document.querySelector<HTMLElement>('.el-dialog.mold-import-dialog')
  const dropzone = dialog?.querySelector<HTMLElement>('.import-dropzone')
  const target = dropzone || dialog?.querySelector<HTMLElement>('.import-template-guide button, .el-dialog__headerbtn')
  target?.focus({preventScroll: true})
}
async function restoreImportTriggerFocus() {
  await nextTick()
  const root = moreActionsButton.value?.$el
  const target = root instanceof HTMLElement && root.matches('button') ? root : root?.querySelector?.('button')
  target?.focus({preventScroll: true})
}
function formatSize(size: number) { if (size < 1024) return `${size} B`; if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`; return `${(size / 1024 / 1024).toFixed(1)} MB` }
function handleDetailEscape(event: KeyboardEvent) {
  if (event.key !== 'Escape' || event.defaultPrevented || !detailVisible.value) return
  if ([...document.querySelectorAll<HTMLElement>('.el-overlay, .el-popper')].some((element) => {
    const style = window.getComputedStyle(element)
    return style.display !== 'none' && style.visibility !== 'hidden' && style.pointerEvents !== 'none'
  })) return
  event.preventDefault()
  void requestDetailClose()
}
onMounted(() => { void Promise.all([load(), loadLocations()]); document.addEventListener('keydown', handleDetailEscape) })
onBeforeUnmount(() => { document.removeEventListener('keydown', handleDetailEscape); setPageDetailPanelVisible(false) })
</script>

<style scoped>
.mold-page { display: grid; min-width: 0; gap: var(--bb-space-4); }
.mold-detail-page { container-name: mold-detail; container-type: inline-size; display: grid; min-width: 0; gap: var(--bb-space-5); padding-bottom: var(--bb-space-4); }
.mold-detail-page__header { display: flex; min-width: 0; min-height: 56px; align-items: center; justify-content: space-between; gap: var(--bb-space-4); border-bottom: 1px solid var(--bb-border-subtle); padding-bottom: var(--bb-space-3); }
.mold-detail-page__identity { display: flex; min-width: 0; align-items: center; gap: var(--bb-space-4); }
.mold-detail-page__back { justify-self: start; padding: 0; }
.mold-detail-page__title { display: flex; min-width: 0; align-items: baseline; flex-wrap: wrap; gap: var(--bb-space-2) var(--bb-space-3); }
.mold-detail-page__title h1 { margin: 0; color: var(--bb-text-primary); font-size: var(--bb-font-size-20); line-height: var(--bb-line-height-tight); overflow-wrap: anywhere; }
.mold-detail-page__model { min-width: 0; overflow: hidden; color: var(--bb-text-secondary); font-size: var(--bb-font-size-14); text-overflow: ellipsis; white-space: nowrap; }
.mold-detail-page__actions { display: flex; flex: 0 0 auto; align-items: center; gap: var(--bb-space-2); }
.mold-detail-page__delete { color: var(--bb-danger); }
.mold-detail-page__footer { position: sticky; z-index: var(--bb-z-sticky); bottom: 0; display: flex; justify-content: flex-end; gap: var(--bb-space-2); border-top: 1px solid var(--bb-border-subtle); background: var(--bb-bg-surface); padding: var(--bb-space-4) 0 calc(var(--bb-space-4) + env(safe-area-inset-bottom, 0px)); }
.mold-toolbar { display: grid; grid-template-columns: minmax(0, 1fr); align-items: start; gap: var(--bb-space-3); }
.mold-filters, .selection-bar, .section-heading, .drawer-section-heading { display: flex; align-items: center; gap: var(--bb-space-2); }
.mold-filters { display: grid !important; grid-template-columns: minmax(260px, 1fr) minmax(140px, 180px) minmax(340px, 360px) minmax(140px, 180px) auto auto; min-width: 0; }
.mold-filters > :deep(.el-input), .mold-filters > :deep(.el-select) { flex: none !important; width: 100% !important; max-width: none; min-width: 0; }
.selection-bar { justify-content: space-between; min-height: 32px; color: var(--bb-text-secondary); font-size: var(--bb-font-size-13); }
.selection-bar > span { min-width: 0; }
.item-code { display: block; color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); }
.drawer-section-heading { justify-content: space-between; margin-bottom: var(--bb-space-3); }
.drawer-section-heading h3 { margin: 0; font-size: var(--bb-font-size-16); }
.drawer-section-heading h2 { margin: 0; font-size: var(--bb-font-size-18); }
.drawer-section-heading small { color: var(--bb-text-secondary); }
.form-grid { display: grid; grid-template-columns: minmax(0, 1fr); }
.mold-editor { display: grid; min-width: 0; gap: var(--bb-space-4); max-width: 1120px; }
.mold-editor .form-grid { column-gap: var(--bb-space-6); row-gap: var(--bb-space-4); }
.mold-editor .mold-form-item--wide { grid-column: 1 / -1; }
.mold-editor .el-form-item { margin-bottom: 0; }
.form-actions { display: flex; justify-content: flex-end; gap: var(--bb-space-2); margin: 0; }
.mold-detail-view { display: grid; min-width: 0; gap: var(--bb-space-6); }
.mold-detail-info, .mold-detail-assets { min-width: 0; }
.mold-detail-info :deep(.property-item) { grid-template-columns: 96px minmax(0, 1fr); }
.detail-eyebrow { color: var(--bb-accent-text); font-family: var(--bb-font-mono); font-size: var(--bb-font-size-13); font-weight: var(--bb-font-weight-bold); overflow-wrap: anywhere; }
.detail-value { display: inline; overflow-wrap: anywhere; white-space: normal; }
.mold-gallery-grid { display: grid; min-width: 0; grid-template-columns: minmax(0, 1fr); gap: var(--bb-space-6); }
.drawing-panel { margin-top: var(--bb-space-6); padding-top: var(--bb-space-4); border-top: 1px solid var(--bb-border-default); }
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
.import-dropzone { display: grid; min-height: 112px; place-items: center; align-content: center; gap: var(--bb-space-1); margin: var(--bb-space-4) 0; border: 1px dashed var(--bb-border-strong); border-radius: var(--bb-radius-lg); background: var(--bb-bg-subtle); color: var(--bb-text-secondary); cursor: pointer; }
.import-dropzone:hover, .import-dropzone:focus-visible, .import-dropzone.is-dragging { border-color: var(--bb-accent-icon); background: var(--bb-accent-selected-bg); color: var(--bb-accent-selected-text); }
.import-dropzone:focus-visible { outline: 2px solid var(--bb-focus-color); outline-offset: 2px; }
.import-dropzone strong { color: inherit; font-size: var(--bb-font-size-14); }
.import-dropzone span, .import-file span { color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); }
.import-template-guide { display: flex; align-items: center; justify-content: space-between; gap: var(--bb-space-4); margin: var(--bb-space-4) 0; border: 1px solid var(--bb-border-subtle); border-radius: var(--bb-radius-md); background: var(--bb-bg-subtle); padding: var(--bb-space-4); }
.import-template-guide > div { min-width: 0; }
.import-template-guide h3 { margin: 0; color: var(--bb-text-primary); font-size: var(--bb-font-size-14); }
.import-template-guide p { margin: var(--bb-space-1) 0 0; color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); line-height: var(--bb-line-height-base); }
.import-file { display: flex; min-width: 0; align-items: center; justify-content: space-between; gap: var(--bb-space-3); margin: var(--bb-space-3) 0; border: 1px solid var(--bb-info-border); border-radius: var(--bb-radius-md); background: var(--bb-info-bg); padding: var(--bb-space-3); }
.import-file strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.import-errors { max-height: 220px; overflow: auto; margin-top: 12px; border-radius: var(--bb-radius-sm); background: var(--bb-bg-subtle); padding: 10px; color: var(--bb-danger); font-size: var(--bb-font-size-13); }
.import-errors p { margin: 4px 0; }
.import-corrections { display: grid; gap: 8px; margin-top: 12px; }
.import-corrections > div { display: grid; grid-template-columns: minmax(0, 1fr) 120px minmax(0, 1.2fr); gap: 8px; align-items: center; }
.import-corrections > .import-correction-header { color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); font-weight: 600; }
.import-correction-kind { color: var(--bb-text-secondary); font-size: var(--bb-font-size-13); }
.location-management-section { display: grid; gap: var(--bb-space-3); margin-bottom: var(--bb-space-5); }
.location-bulk-form { display: grid; grid-template-columns: minmax(0, 1fr) repeat(2, minmax(90px, 120px)); gap: var(--bb-space-2); }
.location-bulk-form .el-form-item { margin-bottom: 0; }
.mold-page :deep(.ui-page-state) { min-height: 160px; padding: var(--bb-space-5); }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (min-width: 960px) {
  .mold-editor .form-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 1279px) {
  .mold-toolbar { grid-template-columns: 1fr; }
}
@media (min-width: 761px) and (max-width: 1439px) {
  .mold-filters { grid-template-columns: minmax(0, 1fr) minmax(140px, 180px) minmax(150px, 200px) auto auto; }
  .mold-filters > :nth-child(3) { grid-column: 1 / -1; grid-row: 2; }
  .mold-filters > :nth-child(4) { grid-column: 3; grid-row: 1; }
  .mold-filters > :nth-child(5) { grid-column: 4; grid-row: 1; }
  .mold-filters > :nth-child(6) { grid-column: 5; grid-row: 1; }
}
@media (max-width: 760px) {
  .mold-detail-page__header { flex-direction: column; gap: var(--bb-space-4); }
  .mold-detail-page__actions { width: 100%; flex-wrap: wrap; }
  .mold-detail-page__footer { justify-content: stretch; }
  .mold-detail-page__footer :deep(.el-button) { flex: 1; }
  .mold-filters { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .mold-filters > :first-child { grid-column: 1 / -1; }
  .mold-filters > .el-button { width: 100%; }
  .selection-bar { align-items: flex-start; flex-direction: column; }
  .import-template-guide { align-items: flex-start; flex-direction: column; }
  .import-corrections > div { grid-template-columns: 1fr; }
  .import-corrections > .import-correction-header { display: none; }
  .import-file { align-items: flex-start; flex-direction: column; }
  .mold-gallery-grid { grid-template-columns: minmax(0, 1fr); }
  .mold-editor .form-grid { grid-template-columns: minmax(0, 1fr); }
  .mold-editor .mold-form-item--wide { grid-column: auto; }
  .location-bulk-form { grid-template-columns: minmax(0, 1fr) repeat(2, minmax(0, 1fr)); }
}
@container mold-detail (min-width: 720px) and (max-width: 959px) {
  .mold-detail-info :deep(.property-list) { grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: var(--bb-space-6); }
  .mold-detail-info :deep(.property-item:last-child) { grid-column: 1 / -1; }
}
@container mold-detail (min-width: 960px) {
  .mold-detail-view { grid-template-columns: 272px minmax(0, 1fr); column-gap: var(--bb-space-8); align-items: start; }
}
@container mold-detail (min-width: 1280px) {
  .mold-detail-view { grid-template-columns: 296px minmax(0, 1fr); }
  .mold-gallery-grid { grid-template-columns: minmax(0, 1.35fr) minmax(0, .85fr); gap: var(--bb-space-8); }
}
</style>

<style>
.mold-location-carrier.el-drawer .el-drawer__header { margin-bottom: 0; border-bottom: 1px solid var(--bb-border-subtle); padding: var(--bb-space-5) var(--bb-space-6) var(--bb-space-4); }
.mold-location-carrier.el-drawer .el-drawer__body { min-width: 0; overflow: auto; padding: var(--bb-space-4) var(--bb-space-6) calc(var(--bb-space-6) + env(safe-area-inset-bottom, 0px)) !important; }
.mold-location-carrier.el-drawer .el-form { min-width: 0; }
.mold-import-dialog .el-dialog__body { max-height: min(62vh, 560px); overflow: auto; }
@media (max-width: 760px) {
  .mold-location-carrier.el-drawer .el-drawer__header, .mold-location-carrier.el-drawer .el-drawer__body { padding-right: var(--bb-space-4); padding-left: var(--bb-space-4); }
}
</style>
