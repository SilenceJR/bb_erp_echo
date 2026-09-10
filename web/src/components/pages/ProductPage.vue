<template>
  <div class="data-page product-page">
    <PageHeader title="产品资料" description="产品型号是产品与模具的统一业务标识。" :readonly="!canWrite" @back="switchModule('dashboard')">
      <template #actions>
        <el-button v-if="canImport" :loading="templateLoading" @click="downloadTemplate">下载模板</el-button>
        <el-button v-if="canRead" :loading="exporting" @click="exportProducts">导出 ZIP</el-button>
        <el-button v-if="canImport" @click="importVisible = true">导入 ZIP</el-button>
        <el-button v-if="canWrite" type="primary" @click="openCreate">＋ 新增产品资料</el-button>
      </template>
    </PageHeader>

    <template v-if="!detailVisible">
      <form class="product-filter-bar" role="search" aria-label="产品资料筛选" @submit.prevent="applyFilters">
        <el-input v-model.trim="filters.q" clearable placeholder="搜索产品型号、客户型号或材料" aria-label="产品型号关键词" />
        <el-select v-model="filters.material" clearable filterable allow-create default-first-option placeholder="产品材料" aria-label="产品材料筛选">
          <el-option v-for="item in materialOptions" :key="item" :label="item" :value="item" />
        </el-select>
        <el-select v-model="filters.ink" clearable placeholder="是否刷墨" aria-label="是否刷墨筛选">
          <el-option label="需刷墨" value="true" />
          <el-option label="不刷墨" value="false" />
        </el-select>
        <el-select v-model="filters.status" clearable placeholder="状态" aria-label="产品状态筛选">
          <el-option label="启用" value="active" />
          <el-option label="停用" value="disabled" />
        </el-select>
        <el-button type="primary" native-type="submit">查询</el-button>
        <el-button @click="resetFilters">重置</el-button>
      </form>

      <DataTableShell :loading="loading" :error="error" :rows-count="rows.length" :total="total" :page="page" :page-size="pageSize" empty-title="暂无产品资料" :empty-description="canWrite ? '请新增产品资料或导入产品资料包。' : '暂无可查看的产品资料。'" aria-label="产品资料列表" @retry="load" @update:page="changePage" @update:page-size="changePageSize">
        <el-table :data="rows" row-key="id" stripe class="data-table">
          <el-table-column label="产品型号" min-width="190"><template #default="{row}"><strong>{{ row.product_model }}</strong><small class="item-code">记录 {{ row.id }}</small></template></el-table-column>
          <el-table-column prop="customer_model" label="客户型号" min-width="140"><template #default="{row}">{{ row.customer_model || '—' }}</template></el-table-column>
          <el-table-column prop="material" label="产品材料" min-width="130"><template #default="{row}">{{ row.material || '—' }}</template></el-table-column>
          <el-table-column label="是否刷墨" width="100" align="center"><template #default="{row}"><StatusTag :label="row.ink_required ? '刷墨' : '白壳'" :tone="row.ink_required ? 'warning' : 'success'" /></template></el-table-column>
          <el-table-column label="关联模具" width="130"><template #default="{row}"><el-button v-if="canReadMolds" link type="primary" @click="openMoldDrawer(row as Product, $event)">{{ row.mold_count }} 条</el-button><span v-else>{{ row.mold_count }}</span></template></el-table-column>
          <el-table-column label="状态" width="90"><template #default="{row}"><StatusTag :label="row.status === 'active' ? '启用' : '停用'" :tone="row.status === 'active' ? 'success' : 'info'" /></template></el-table-column>
          <el-table-column label="查看" width="110" fixed="right"><template #default="{row}"><el-button link type="primary" @click="openDetail(Number(row.id), $event)">详情</el-button></template></el-table-column>
        </el-table>
      </DataTableShell>
    </template>

    <template v-else>
      <header class="product-detail-header">
        <el-button link @click="closeDetail">← 返回产品资料</el-button>
        <div class="product-detail-heading"><h1>{{ draft.product_model || '新增产品资料' }}</h1><span>{{ editing ? '正在编辑' : draft.customer_model || '未填写客户型号' }}</span></div>
        <div class="product-detail-actions"><el-button v-if="!editing && canWrite" type="primary" plain @click="startEditing">编辑</el-button><el-button v-if="editing" @click="cancelEditing">取消</el-button><el-button v-if="editing" type="primary" :loading="saving" @click="save">保存</el-button><el-button v-else-if="detailID && canWrite" type="danger" plain :loading="deleting" @click="deleteProduct">删除</el-button></div>
      </header>

      <el-form v-if="editing" class="product-form" label-position="top" @submit.prevent="save">
        <div class="form-grid">
          <el-form-item label="产品型号" required><el-input v-model.trim="draft.product_model" maxlength="160" autofocus /></el-form-item>
          <el-form-item label="客户型号"><el-input v-model.trim="draft.customer_model" maxlength="160" /></el-form-item>
          <el-form-item label="产品材料"><el-select v-model="draft.material" filterable allow-create default-first-option clearable placeholder="选择或输入材料"><el-option v-for="item in materialOptions" :key="item" :label="item" :value="item" /></el-select></el-form-item>
          <el-form-item label="是否刷墨"><el-switch v-model="draft.ink_required" inline-prompt active-text="是" inactive-text="否" /></el-form-item>
          <el-form-item label="状态"><el-radio-group v-model="draft.status"><el-radio-button value="active">启用</el-radio-button><el-radio-button value="disabled">停用</el-radio-button></el-radio-group></el-form-item>
        </div>
      </el-form>

      <template v-else>
        <PropertyList class="product-properties">
          <PropertyItem label="产品型号"><span class="detail-value">{{ draft.product_model }}</span></PropertyItem>
          <PropertyItem label="客户型号"><span class="detail-value">{{ draft.customer_model || '未填写' }}</span></PropertyItem>
          <PropertyItem label="产品材料"><span class="detail-value">{{ draft.material || '未填写' }}</span></PropertyItem>
          <PropertyItem label="是否刷墨"><span class="detail-value">{{ draft.ink_required ? '是' : '否' }}</span></PropertyItem>
          <PropertyItem label="状态"><span class="detail-value">{{ draft.status === 'active' ? '启用' : '停用' }}</span></PropertyItem>
        </PropertyList>

        <section v-if="detailID" class="product-assets">
          <ImageGallery owner-type="product" :owner-id="detailID" :token="token" :can-write="canWrite" title="产品图片" @busy-change="galleryBusy = $event" />
        </section>

        <section v-if="detailID" class="linked-molds" aria-label="关联模具">
          <div class="section-heading"><div><h2>关联模具</h2><small>同一产品型号下可维护多条模具档案。</small></div><el-button v-if="canWrite && draft.status === 'active'" @click="openCreateMold">新增模具</el-button></div>
          <el-table v-if="canReadMolds" :data="detailMolds" row-key="id" stripe class="data-table">
            <el-table-column prop="product_model" label="产品型号" min-width="180" />
            <el-table-column label="模具类型" width="100"><template #default="{row}">{{ row.mold_type === 'common' ? '共模' : '单模' }}</template></el-table-column>
            <el-table-column prop="cavity_count" label="模穴数" width="110" />
            <el-table-column label="模具位置" min-width="130"><template #default="{row}">{{ row.location?.code || '—' }}</template></el-table-column>
            <el-table-column prop="common_group_no" label="共模组号" min-width="120"><template #default="{row}">{{ row.common_group_no || '—' }}</template></el-table-column>
            <el-table-column label="查看" width="100"><template #default="{row}"><el-button link type="primary" @click="openFullMold(row.id)">模具详情</el-button></template></el-table-column>
          </el-table>
          <PageState v-else kind="readonly" title="无权限查看关联模具" description="联系管理员授予“模具查看”权限后可查看。" />
        </section>
      </template>
    </template>

    <el-drawer v-model="moldDrawerVisible" title="关联模具" size="min(760px, 92vw)" destroy-on-close>
      <div class="linked-mold-drawer" v-loading="moldDrawerLoading">
        <p class="linked-mold-drawer__model">{{ selectedProductModel }}</p>
        <el-table :data="drawerMolds" row-key="id" stripe>
          <el-table-column label="模具类型" width="100"><template #default="{row}">{{ row.mold_type === 'common' ? '共模' : '单模' }}</template></el-table-column>
          <el-table-column prop="cavity_count" label="模穴数" width="110" />
          <el-table-column label="位置" min-width="120"><template #default="{row}">{{ row.location?.code || '—' }}</template></el-table-column>
          <el-table-column prop="common_group_no" label="共模组号" min-width="120"><template #default="{row}">{{ row.common_group_no || '—' }}</template></el-table-column>
          <el-table-column label="操作" width="110"><template #default="{row}"><el-button link type="primary" @click="openFullMold(row.id)">查看模具</el-button></template></el-table-column>
        </el-table>
        <el-empty v-if="!moldDrawerLoading && !drawerMolds.length" description="暂无关联模具" />
      </div>
    </el-drawer>

    <el-dialog v-model="importVisible" title="导入产品资料包" width="min(640px, 92vw)">
      <el-alert type="info" :closable="false" title="products.xlsx 负责产品字段；zip 根目录下以产品型号命名的目录存放产品图片。" />
      <input ref="importInput" class="sr-only" type="file" accept=".zip,application/zip" @change="previewImport" />
      <div class="import-dropzone" tabindex="0" role="button" @click="importInput?.click()" @keydown.enter.prevent="importInput?.click()" @keydown.space.prevent="importInput?.click()"><strong>{{ importFile?.name || '选择产品资料 ZIP' }}</strong><span>支持产品图片批量导入</span></div>
      <pre v-if="importPreview" class="import-preview">{{ JSON.stringify(importPreview, null, 2) }}</pre>
      <el-alert v-if="importError" type="error" :title="importError" :closable="false" show-icon />
      <template #footer><el-button @click="importVisible = false">取消</el-button><el-button type="primary" :disabled="!importPreview?.token || importSubmitting" :loading="importSubmitting" @click="commitImport">提交导入</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import {computed, nextTick, onBeforeUnmount, onMounted, reactive, ref} from 'vue'
import {ElMessage} from 'element-plus'
import {appMessageBox} from '../../composables/useAppMessageBox'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import {downloadApiFile, request} from '../../api/http'
import {useDirtyGuard} from '../../composables/useDirtyGuard'
import DataTableShell from '../ui/DataTableShell.vue'
import ImageGallery from '../ImageGallery.vue'
import PageHeader from '../ui/PageHeader.vue'
import PageState from '../ui/PageState.vue'
import PropertyItem from '../ui/PropertyItem.vue'
import PropertyList from '../ui/PropertyList.vue'
import StatusTag from '../ui/StatusTag.vue'

type Product = {id: number; product_model: string; customer_model: string; material: string; ink_required: boolean; status: 'active' | 'disabled'; mold_count: number}
type Mold = {id: number; product_model: string; mold_type: 'single' | 'common'; cavity_count: string; common_group_no?: string; remark?: string; location?: {code?: string}; image_count?: number; drawing_count?: number}
type ImportPreview = {token?: string; summary?: {products?: number; created?: number; updated?: number}; errors?: Array<{row: number; column: string; reason: string}>}

const {token, hasPermission, switchModule, setPageDetailPanelVisible} = useWorkspaceContext()
const canRead = computed(() => hasPermission('product:read'))
const canWrite = computed(() => hasPermission('product:write'))
const canImport = computed(() => hasPermission('product:import'))
const canReadMolds = computed(() => hasPermission('mold:read'))
const materialOptions = ['DB', 'DB-2', '标白', 'ABS 白', 'ABS 黑', 'PPO 黑']
const rows = ref<Product[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const error = ref('')
const filters = reactive({q: '', material: '', ink: '', status: ''})
const detailVisible = ref(false)
const detailID = ref<number | null>(null)
const editing = ref(false)
const saving = ref(false)
const deleting = ref(false)
const draft = reactive({product_model: '', customer_model: '', material: '', ink_required: false, status: 'active' as 'active' | 'disabled'})
const savedDraft = ref('')
const detailMolds = ref<Mold[]>([])
const galleryBusy = ref(false)
const moldDrawerVisible = ref(false)
const moldDrawerLoading = ref(false)
const drawerMolds = ref<Mold[]>([])
const selectedProductModel = ref('')
const importVisible = ref(false)
const importInput = ref<HTMLInputElement | null>(null)
const importFile = ref<File | null>(null)
const importPreview = ref<ImportPreview | null>(null)
const importError = ref('')
const importSubmitting = ref(false)
const exporting = ref(false)
const templateLoading = ref(false)
const dirty = computed(() => detailVisible.value && editing.value && savedDraft.value !== JSON.stringify(draft))
useDirtyGuard('product-form', {busy: () => saving.value || deleting.value || galleryBusy.value, dirty: () => dirty.value, busyMessage: '产品资料正在保存，请稍候', dirtyMessage: '产品资料有未保存的修改，确认离开？'})

onMounted(() => { void load(); window.addEventListener('bb-product-refresh', handleExternalRefresh) })
onBeforeUnmount(() => { window.removeEventListener('bb-product-refresh', handleExternalRefresh); setPageDetailPanelVisible(false) })
function handleExternalRefresh() { void load(); if (detailID.value && !editing.value) void loadDetail(detailID.value) }

async function load() { loading.value = true; error.value = ''; try { const params = new URLSearchParams({page: String(page.value), page_size: String(pageSize.value)}); if (filters.q) params.set('q', filters.q); if (filters.material) params.set('material', filters.material); if (filters.ink) params.set('ink_required', filters.ink); if (filters.status) params.set('status', filters.status); const result = await request<{items: Product[]; total: number}>('/api/v1/products?' + params, {}, token.value); rows.value = result.items || []; total.value = result.total || 0 } catch (cause) { error.value = cause instanceof Error ? cause.message : '产品资料加载失败' } finally { loading.value = false } }
function applyFilters() { page.value = 1; void load() }
function resetFilters() { Object.assign(filters, {q: '', material: '', ink: '', status: ''}); applyFilters() }
function changePage(value: number) { page.value = value; void load() }
function changePageSize(value: number) { pageSize.value = value; page.value = 1; void load() }
function resetDraft() { Object.assign(draft, {product_model: '', customer_model: '', material: '', ink_required: false, status: 'active'}) }
function openCreate() { resetDraft(); detailID.value = null; detailMolds.value = []; editing.value = true; detailVisible.value = true; savedDraft.value = JSON.stringify(draft); setPageDetailPanelVisible(true); void nextTick(() => document.querySelector<HTMLElement>('.product-form input')?.focus()) }
async function openDetail(id: number, _event?: Event) { detailID.value = id; editing.value = false; detailVisible.value = true; setPageDetailPanelVisible(true); await loadDetail(id) }
async function loadDetail(id: number) { try { const item = await request<Product>(`/api/v1/products/${id}`, {}, token.value); Object.assign(draft, {product_model: item.product_model, customer_model: item.customer_model || '', material: item.material || '', ink_required: Boolean(item.ink_required), status: item.status}); savedDraft.value = JSON.stringify(draft); if (canReadMolds.value) detailMolds.value = await request<Mold[]>(`/api/v1/products/${id}/molds`, {}, token.value) } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '产品详情加载失败'); closeDetail() } }
function startEditing() { editing.value = true; void nextTick(() => document.querySelector<HTMLElement>('.product-form input')?.focus()) }
function cancelEditing() { if (detailID.value) { void loadDetail(detailID.value); editing.value = false } else closeDetail() }
async function save() { if (!draft.product_model.trim()) { ElMessage.warning('请填写产品型号'); return } saving.value = true; try { const body = {product_model: draft.product_model.trim(), customer_model: draft.customer_model.trim(), material: draft.material.trim(), ink_required: draft.ink_required, status: draft.status}; if (detailID.value) await request(`/api/v1/products/${detailID.value}`, {method: 'PATCH', body}, token.value); else { const created = await request<Product>('/api/v1/products', {method: 'POST', body}, token.value); detailID.value = created.id } savedDraft.value = JSON.stringify(draft); editing.value = false; await Promise.all([load(), detailID.value ? loadDetail(detailID.value) : Promise.resolve()]); ElMessage.success('产品资料已保存') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '产品资料保存失败') } finally { saving.value = false } }
async function deleteProduct() { if (!detailID.value) return; try { await appMessageBox.confirm(`确认删除产品“${draft.product_model}”吗？`, '删除产品资料', {type: 'warning', confirmButtonText: '确认删除', confirmButtonType: 'danger'}); await request(`/api/v1/products/${detailID.value}`, {method: 'DELETE'}, token.value); closeDetail(); await load(); ElMessage.success('产品资料已删除') } catch (cause) { if (cause instanceof Error && cause.message) ElMessage.error(cause.message) } }
function closeDetail() { detailVisible.value = false; detailID.value = null; detailMolds.value = []; setPageDetailPanelVisible(false) }
async function openMoldDrawer(product: Product, _event?: Event) { selectedProductModel.value = product.product_model; moldDrawerVisible.value = true; moldDrawerLoading.value = true; try { drawerMolds.value = await request<Mold[]>(`/api/v1/products/${product.id}/molds`, {}, token.value) } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '关联模具加载失败') } finally { moldDrawerLoading.value = false } }
function openFullMold(id: number) { sessionStorage.setItem('bb:pending-mold-open', String(id)); moldDrawerVisible.value = false; void switchModule('molds') }
function openCreateMold() { if (!detailID.value) return; sessionStorage.setItem('bb:new-mold-product-id', String(detailID.value)); void switchModule('molds') }
async function exportProducts() { if (!canRead.value) return; exporting.value = true; try { await downloadApiFile('/api/v1/products/export', '博邦产品资料包.zip', token.value) } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '产品资料导出失败') } finally { exporting.value = false } }
async function downloadTemplate() { templateLoading.value = true; try { await downloadApiFile('/api/v1/products/import-template', '博邦产品资料导入模板.zip', token.value) } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '产品资料模板下载失败') } finally { templateLoading.value = false } }
function previewImport(event: Event) { const input = event.target as HTMLInputElement; const file = input.files?.[0]; input.value = ''; if (!file) return; importFile.value = file; importPreview.value = null; importError.value = ''; const body = new FormData(); body.append('file', file); void request<ImportPreview>('/api/v1/products/import/preview', {method: 'POST', body}, token.value).then((result) => { importPreview.value = result }).catch((cause) => { importError.value = cause instanceof Error ? cause.message : '产品资料包预览失败'; importFile.value = null }) }
async function commitImport() { if (!importFile.value || !importPreview.value?.token) return; importSubmitting.value = true; try { const body = new FormData(); body.append('file', importFile.value); body.append('token', importPreview.value.token); await request('/api/v1/products/import/commit', {method: 'POST', body}, token.value); importVisible.value = false; importFile.value = null; importPreview.value = null; await load(); ElMessage.success('产品资料包已导入') } catch (cause) { importError.value = cause instanceof Error ? cause.message : '产品资料包导入失败' } finally { importSubmitting.value = false } }
</script>

<style scoped>
.product-page { display: grid; gap: var(--bb-space-4); }
.product-filter-bar { display: grid; grid-template-columns: minmax(240px, 1fr) repeat(3, minmax(130px, 180px)) auto auto; gap: var(--bb-space-2); }
.product-detail-header { display: flex; min-width: 0; align-items: center; gap: var(--bb-space-4); border-bottom: 1px solid var(--bb-border-subtle); padding-bottom: var(--bb-space-3); }
.product-detail-heading { min-width: 0; flex: 1; }
.product-detail-heading h1 { margin: 0; font-size: var(--bb-font-size-20); overflow-wrap: anywhere; }
.product-detail-heading span { color: var(--bb-text-secondary); font-size: var(--bb-font-size-13); }
.product-detail-actions { display: flex; gap: var(--bb-space-2); }
.product-form { max-width: 980px; }
.product-form .el-select { width: 100%; }
.product-assets { margin-top: var(--bb-space-4); }
.linked-molds { display: grid; gap: var(--bb-space-3); margin-top: var(--bb-space-5); }
.section-heading, .linked-molds .section-heading { display: flex; align-items: center; justify-content: space-between; gap: var(--bb-space-3); }
.section-heading h2 { margin: 0; }
.section-heading small, .linked-mold-drawer__model { color: var(--bb-text-secondary); }
.import-dropzone { display: grid; min-height: 110px; place-items: center; align-content: center; margin: var(--bb-space-4) 0; border: 1px dashed var(--bb-border-strong); border-radius: var(--bb-radius-lg); background: var(--bb-bg-subtle); cursor: pointer; }
.import-preview { max-height: 220px; overflow: auto; border-radius: var(--bb-radius-md); background: var(--bb-bg-subtle); padding: var(--bb-space-3); font-size: var(--bb-font-size-12); }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0, 0, 0, 0); }
@media (max-width: 1200px) { .product-filter-bar { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 760px) { .product-filter-bar, .product-detail-header { grid-template-columns: 1fr; } .product-detail-header { align-items: flex-start; flex-direction: column; } .product-detail-actions { width: 100%; } .product-detail-actions .el-button { flex: 1; } }
</style>
