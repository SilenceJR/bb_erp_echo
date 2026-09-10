<template>
  <div class="data-page warehouse-page">
    <PageHeader title="仓库" description="按产品型号统计数量和库位，不在此维护产品资料。" @back="switchModule('dashboard')">
      <template #actions><el-button @click="openLocationDialog">库位管理</el-button><el-button :loading="loading" @click="load">刷新</el-button></template>
    </PageHeader>

    <form class="warehouse-filter" role="search" @submit.prevent="applyFilters">
      <el-input v-model.trim="keyword" clearable placeholder="搜索产品型号" aria-label="搜索产品型号" />
      <el-button type="primary" native-type="submit">查询</el-button>
    </form>

    <DataTableShell :loading="loading" :error="error" :rows-count="rows.length" :total="total" :page="page" :page-size="pageSize" empty-title="暂无产品库存" empty-description="请先在产品资料中建立产品型号。" aria-label="产品库存列表" @retry="load" @update:page="changePage" @update:page-size="changePageSize">
      <el-table :data="rows" row-key="product_id" stripe class="data-table">
        <el-table-column prop="product_model" label="产品型号" min-width="220"><template #default="{row}"><strong>{{ row.product_model }}</strong></template></el-table-column>
        <el-table-column label="总数量" width="140"><template #default="{row}">{{ formatQuantity(row.quantity) }}</template></el-table-column>
        <el-table-column prop="location_count" label="库位数" width="100" />
        <el-table-column label="状态" width="100"><template #default="{row}"><StatusTag :label="row.status === 'active' ? '启用' : '停用'" :tone="row.status === 'active' ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="查看" width="120" fixed="right"><template #default="{row}"><el-button link type="primary" @click="openDetail(row as StockItem)">数量与位置</el-button></template></el-table-column>
      </el-table>
    </DataTableShell>

    <el-drawer v-model="drawerVisible" :title="selected?.product_model || '产品库存'" size="min(760px, 94vw)" destroy-on-close>
      <div v-loading="detailLoading" class="stock-detail">
        <div class="stock-detail__summary"><div><small>总数量</small><strong>{{ formatQuantity(detailTotal) }}</strong></div><el-button v-if="canWrite" type="primary" @click="openMovement">数量操作</el-button></div>
        <section><h3>库位数量</h3><el-table :data="balances" row-key="location_id" stripe><el-table-column prop="location_code" label="库位" min-width="120" /><el-table-column label="数量" width="140"><template #default="{row}">{{ formatQuantity(row.quantity) }}</template></el-table-column><el-table-column prop="updated_at" label="更新时间" min-width="180" /></el-table></section>
        <section><h3>库存流水</h3><el-table :data="movements" row-key="id" stripe><el-table-column label="时间" min-width="170"><template #default="{row}">{{ formatDate(row.created_at) }}</template></el-table-column><el-table-column label="操作" width="90"><template #default="{row}">{{ actionLabel(row.action) }}</template></el-table-column><el-table-column prop="location_code" label="库位" width="100" /><el-table-column label="数量" width="110"><template #default="{row}">{{ formatQuantity(row.quantity) }}</template></el-table-column><el-table-column prop="operator_name" label="操作人" width="110" /><el-table-column prop="reason" label="原因" min-width="160" show-overflow-tooltip /></el-table></section>
      </div>
    </el-drawer>

    <el-dialog v-model="movementVisible" title="产品数量操作" width="min(620px, 92vw)" destroy-on-close>
      <el-form label-position="top" @submit.prevent="submitMovement">
        <el-form-item label="操作类型" required><el-select v-model="movement.action"><el-option label="入库" value="inbound" /><el-option label="出库" value="outbound" /><el-option label="库位调整" value="transfer" /><el-option label="盘点修正" value="adjustment" /></el-select></el-form-item>
        <template v-if="movement.action === 'transfer'"><el-form-item label="来源库位" required><el-select v-model="movement.from_location_id"><el-option v-for="item in locations" :key="item.id" :label="item.code" :value="item.id" /></el-select></el-form-item><el-form-item label="目标库位" required><el-select v-model="movement.to_location_id"><el-option v-for="item in locations" :key="item.id" :label="item.code" :value="item.id" /></el-select></el-form-item></template>
        <el-form-item v-else label="库位" required><el-select v-model="movement.location_id"><el-option v-for="item in locations" :key="item.id" :label="item.code" :value="item.id" /></el-select></el-form-item>
        <el-form-item v-if="movement.action === 'adjustment'" label="目标数量" required><el-input-number v-model="movement.target_quantity_decimal" :min="0" :precision="4" :step="1" /></el-form-item>
        <el-form-item v-else label="数量" required><el-input-number v-model="movement.quantity_decimal" :min="0.0001" :precision="4" :step="1" /></el-form-item>
        <el-form-item label="原因" required><el-input v-model.trim="movement.reason" maxlength="255" type="textarea" :rows="3" /></el-form-item>
        <el-form-item label="本次操作人" required><el-select v-model="movement.operator_employee_id" :loading="operators.loading.value"><el-option v-for="item in operators.employees.value" :key="item.id" :label="item.name" :value="item.id" /></el-select><small v-if="operators.unavailableReason.value" class="form-error">{{ operators.unavailableReason.value }}</small></el-form-item>
        <el-alert v-if="movementError" type="error" :title="movementError" :closable="false" show-icon />
      </el-form>
      <template #footer><el-button @click="movementVisible = false">取消</el-button><el-button type="primary" :loading="movementSaving" @click="submitMovement">确认提交</el-button></template>
    </el-dialog>

    <el-dialog v-model="locationDialogVisible" title="库位管理" width="min(680px, 92vw)">
      <div class="location-create"><el-input v-model.trim="newLocationCode" placeholder="库位编码，例如 A1-1" /><el-input v-model.trim="newLocationName" placeholder="库位名称" /><el-select v-model="movement.operator_employee_id" placeholder="本次操作人" :loading="operators.loading.value"><el-option v-for="item in operators.employees.value" :key="item.id" :label="item.name" :value="item.id" /></el-select><el-button type="primary" :loading="locationSaving" @click="createLocation">新增库位</el-button></div>
      <el-table :data="locations" row-key="id" stripe><el-table-column prop="code" label="库位编码" min-width="130" /><el-table-column prop="name" label="库位名称" min-width="160" /><el-table-column prop="status" label="状态" width="90" /></el-table>
      <el-alert v-if="locationError" type="error" :title="locationError" :closable="false" show-icon />
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, reactive, ref} from 'vue'
import {ElMessage} from 'element-plus'
import {request} from '../../api/http'
import {useOperatorEmployees} from '../../composables/useOperatorEmployees'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import DataTableShell from '../ui/DataTableShell.vue'
import PageHeader from '../ui/PageHeader.vue'
import StatusTag from '../ui/StatusTag.vue'

type StockItem = {product_id: number; product_model: string; status: 'active' | 'disabled'; quantity: number; location_count: number}
type Balance = {location_id: number; location_code: string; quantity: number; updated_at: string}
type Movement = {id: number; document_code: string; action: string; quantity: number; location_code: string; balance_quantity: number; reason: string; operator_name: string; created_at: string}
type Location = {id: number; code: string; name: string; status: string}

const {token, hasPermission, switchModule} = useWorkspaceContext()
const canWrite = computed(() => hasPermission('warehouse:write'))
const rows = ref<StockItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const loading = ref(false)
const error = ref('')
const drawerVisible = ref(false)
const selected = ref<StockItem | null>(null)
const detailLoading = ref(false)
const balances = ref<Balance[]>([])
const movements = ref<Movement[]>([])
const movementVisible = ref(false)
const movementSaving = ref(false)
const movementError = ref('')
const locations = ref<Location[]>([])
const locationDialogVisible = ref(false)
const newLocationCode = ref('')
const newLocationName = ref('')
const locationSaving = ref(false)
const locationError = ref('')
const operators = useOperatorEmployees(token)
const movement = reactive({action: 'inbound', quantity_decimal: 1, target_quantity_decimal: 0, location_id: undefined as number | undefined, from_location_id: undefined as number | undefined, to_location_id: undefined as number | undefined, reason: '', operator_employee_id: undefined as number | undefined})
const detailTotal = computed(() => balances.value.reduce((sum, item) => sum + Number(item.quantity || 0), 0))

onMounted(() => { void Promise.all([load(), loadLocations(), operators.load()]); window.addEventListener('bb-warehouse-refresh', handleExternalRefresh) })
onBeforeUnmount(() => window.removeEventListener('bb-warehouse-refresh', handleExternalRefresh))
function handleExternalRefresh() { void load(); if (selected.value && drawerVisible.value) void openDetail(selected.value) }
async function load() { loading.value = true; error.value = ''; try { const params = new URLSearchParams({page: String(page.value), page_size: String(pageSize.value)}); if (keyword.value) params.set('q', keyword.value); const result = await request<{items: StockItem[]; total: number}>('/api/v1/warehouse/products?' + params, {}, token.value); rows.value = result.items || []; total.value = result.total || 0 } catch (cause) { error.value = cause instanceof Error ? cause.message : '产品库存加载失败' } finally { loading.value = false } }
function applyFilters() { page.value = 1; void load() }
function changePage(value: number) { page.value = value; void load() }
function changePageSize(value: number) { pageSize.value = value; page.value = 1; void load() }
async function openDetail(item: StockItem) { selected.value = item; drawerVisible.value = true; detailLoading.value = true; try { const [detail, history] = await Promise.all([request<{balances: Balance[]}>(`/api/v1/warehouse/products/${item.product_id}`, {}, token.value), request<Movement[]>(`/api/v1/warehouse/products/${item.product_id}/movements`, {}, token.value)]); balances.value = detail.balances || []; movements.value = history || [] } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '库存详情加载失败') } finally { detailLoading.value = false } }
async function loadLocations() { try { locations.value = (await request<Location[]>('/api/v1/locations', {}, token.value)).filter((item) => item.status === 'active') } catch (cause) { locationError.value = cause instanceof Error ? cause.message : '库位加载失败' } }
function formatQuantity(value: unknown) { const number = Number(value || 0) / 10000; return Number.isInteger(number) ? String(number) : number.toLocaleString('zh-CN', {maximumFractionDigits: 4}) }
function formatDate(value: unknown) { const date = new Date(String(value || '')); return Number.isNaN(date.getTime()) ? String(value || '') : date.toLocaleString('zh-CN', {hour12: false}) }
function actionLabel(value: string) { return ({inbound: '入库', outbound: '出库', transfer: '库位调整', adjustment: '盘点修正'} as Record<string, string>)[value] || value }
function openMovement() { Object.assign(movement, {action: 'inbound', quantity_decimal: 1, target_quantity_decimal: 0, location_id: undefined, from_location_id: undefined, to_location_id: undefined, reason: '', operator_employee_id: undefined}); movementError.value = ''; movementVisible.value = true; void operators.load() }
function openLocationDialog() { locationError.value = ''; locationDialogVisible.value = true; void operators.load() }
async function submitMovement() { if (!selected.value || !movement.reason.trim() || !movement.operator_employee_id) { movementError.value = '请填写操作原因和本次操作人'; return } movementSaving.value = true; movementError.value = ''; try { const body: Record<string, unknown> = {action: movement.action, reason: movement.reason.trim(), operator_employee_id: movement.operator_employee_id}; if (movement.action === 'transfer') { body.from_location_id = movement.from_location_id; body.to_location_id = movement.to_location_id; body.quantity = Math.round(movement.quantity_decimal * 10000) } else { body.location_id = movement.location_id; if (movement.action === 'adjustment') body.target_quantity = Math.round(movement.target_quantity_decimal * 10000); else body.quantity = Math.round(movement.quantity_decimal * 10000) } await request(`/api/v1/warehouse/products/${selected.value.product_id}/movements`, {method: 'POST', body}, token.value); movementVisible.value = false; await Promise.all([load(), openDetail(selected.value)]); ElMessage.success('库存数量已更新') } catch (cause) { movementError.value = cause instanceof Error ? cause.message : '库存操作失败' } finally { movementSaving.value = false } }
async function createLocation() { if (!newLocationCode.value.trim() || !newLocationName.value.trim() || !movement.operator_employee_id) { locationError.value = '请填写库位编码、名称和页面中的操作人'; return } locationSaving.value = true; locationError.value = ''; try { await request('/api/v1/locations', {method: 'POST', body: {code: newLocationCode.value.trim(), name: newLocationName.value.trim(), operator_employee_id: movement.operator_employee_id}}, token.value); newLocationCode.value = ''; newLocationName.value = ''; await loadLocations(); ElMessage.success('库位已新增') } catch (cause) { locationError.value = cause instanceof Error ? cause.message : '库位新增失败' } finally { locationSaving.value = false } }
</script>

<style scoped>
.warehouse-page { display: grid; gap: var(--bb-space-4); }
.warehouse-filter { display: grid; grid-template-columns: minmax(260px, 1fr) auto; gap: var(--bb-space-2); }
.stock-detail { display: grid; gap: var(--bb-space-5); }
.stock-detail__summary { display: flex; align-items: center; justify-content: space-between; border: 1px solid var(--bb-border-subtle); border-radius: var(--bb-radius-md); padding: var(--bb-space-4); }
.stock-detail__summary div { display: grid; gap: var(--bb-space-1); }
.stock-detail__summary strong { font-size: var(--bb-font-size-24); }
.stock-detail section { display: grid; gap: var(--bb-space-2); }
.stock-detail h3 { margin: 0; }
.location-create { display: grid; grid-template-columns: minmax(130px, 1fr) minmax(160px, 1fr) minmax(150px, 1fr) auto; gap: var(--bb-space-2); margin-bottom: var(--bb-space-3); }
.form-error { color: var(--bb-danger); }
@media (max-width: 760px) { .warehouse-filter, .location-create { grid-template-columns: 1fr; } }
</style>
