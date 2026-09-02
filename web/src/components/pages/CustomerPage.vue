<template>
  <div class="data-page customer-page">
    <LayoutGroup id="customer-profile-transition">
    <PageHeader
      title="客户资料"
      description="客户编码是后续业务的稳定关联；每个编码可维护一条或多条具体资料。"
      :readonly="!canWrite"
      @back="switchModule('dashboard')"
    >
      <template #actions>
        <el-button @click="exportVisible = true">导出 Excel</el-button>
        <el-button v-if="canImport" @click="importVisible = true">导入 Excel</el-button>
        <el-button v-if="canWrite" type="primary" @click="openCreateProfile(null, $event)">＋ 新增客户资料</el-button>
      </template>
    </PageHeader>

    <el-tabs v-model="activeTab" class="customer-tabs" @tab-change="handleTabChange">
      <el-tab-pane label="客户资料" name="profiles" />
      <el-tab-pane label="客户编码" name="codes" />
    </el-tabs>

    <form class="customer-filter-bar" role="search" aria-label="客户资料筛选" @submit.prevent>
      <div class="customer-filter-controls">
        <el-input
          v-model.trim="keyword"
          clearable
          :placeholder="activeTab === 'profiles' ? '搜索编码、名称、联系人、电话、地址或业务员' : '搜索客户编码或关联资料'"
          aria-label="客户关键词"
        />
        <el-select v-model="filter" aria-label="客户编码类型">
          <el-option label="全部客户编码" value="all" />
          <el-option label="多资料编码" value="multiple" />
          <el-option label="无资料编码" value="empty" />
        </el-select>
        <el-button v-if="hasFilters" @click="resetFilters">重置</el-button>
      </div>
      <div class="customer-filter-actions">
        <span aria-live="polite">{{ syncMessage }}</span>
        <el-button :loading="loading" @click="loadCodes">刷新数据</el-button>
      </div>
    </form>

    <section v-if="filteredCodes.length" class="customer-summary" aria-label="当前筛选客户摘要" aria-live="polite">
      <span v-if="activeTab === 'profiles'"><strong>{{ filteredCodes.length }}</strong> 个编码 · <strong>{{ filteredProfileCount }}</strong> 条资料 · <strong>{{ filteredMultipleCount }}</strong> 个多资料编码</span>
      <span v-else><strong>{{ filteredCodes.length }}</strong> 个编码 · <strong>{{ filteredEmptyCount }}</strong> 个未关联资料</span>
    </section>

    <el-alert v-if="listError && sourceCodes.length" :title="listError" type="error" :closable="false" show-icon />
    <PageState v-if="loading && !sourceCodes.length" kind="loading" title="正在加载客户资料" />
    <PageState v-else-if="listError && !sourceCodes.length" kind="error" title="客户资料加载失败" :description="listError" action-label="重新加载" @action="loadCodes" />
    <PageState
      v-else-if="!filteredCodes.length"
      kind="empty"
      :title="hasFilters ? '没有符合当前条件的客户' : '还没有客户编码'"
      :description="hasFilters ? '请调整关键词或分组筛选。' : canWrite ? '可新增客户资料，系统会同时维护客户编码。' : '当前账号仅可查看，暂无可显示数据。'"
    />

    <template v-else-if="activeTab === 'profiles'">
      <section class="customer-desktop-table" aria-label="按客户编码分组的客户资料">
        <el-table :data="codes" row-key="id" stripe>
          <el-table-column label="客户编码" width="132"><template #default="{row}"><span class="customer-code">{{ row.code }}</span></template></el-table-column>
          <el-table-column label="客户资料" min-width="210"><template #default="{row}"><div class="primary-profile"><motion.strong v-if="row.default_profile && !(drawerVisible && selectedProfile?.id === row.default_profile.id)" :layout-id="`customer-profile-title-${row.default_profile.id}`" :transition="{duration: 0.28, ease: [0.2, 0, 0, 1]}">{{ profileTitle(row.default_profile) }}</motion.strong><strong v-else>{{ profileTitle(row.default_profile) }}</strong><small>{{ row.default_profile?.name || (row.profile_count ? '未填写客户名称' : '尚未建立资料') }}</small></div></template></el-table-column>
          <el-table-column label="联系人" min-width="140"><template #default="{row}">{{ row.default_profile?.contact_name || '—' }}</template></el-table-column>
          <el-table-column label="联系电话" min-width="150"><template #default="{row}"><span class="text-cell">{{ row.default_profile?.contact_phone || '—' }}</span></template></el-table-column>
          <el-table-column label="业务员" min-width="120"><template #default="{row}">{{ row.default_profile?.salesperson || '—' }}</template></el-table-column>
          <el-table-column label="操作" width="100" align="center" fixed="right"><template #default="{row}"><el-button v-if="row.default_profile" link type="primary" @click="openProfile(row.default_profile, asCustomerCode(row), $event)">详情</el-button><span v-else>—</span></template></el-table-column>
        </el-table>
      </section>

    </template>

    <template v-else>
      <div v-if="canWrite" class="code-toolbar"><div><strong>客户编码单独维护</strong><span>只有没有关联资料的编码才能删除。</span></div><el-button type="primary" @click="openCodeDialog('create')">＋ 新增客户编码</el-button></div>
      <section class="customer-desktop-table code-table" aria-label="客户编码列表">
        <el-table :data="codes" row-key="id" stripe>
          <el-table-column label="客户编码" min-width="180"><template #default="{row}"><span class="customer-code">{{ row.code }}</span></template></el-table-column>
          <el-table-column label="关联资料摘要" min-width="320"><template #default="{row}">{{ codeProfileSummary(asCustomerCode(row)) }}</template></el-table-column>
          <el-table-column prop="updated_at" label="更新时间" min-width="180"><template #default="{row}">{{ formatDate(row.updated_at) }}</template></el-table-column>
          <el-table-column v-if="canWrite" label="操作" width="160" align="center" fixed="right"><template #default="{row}"><el-button link type="primary" :disabled="deletingCodeID !== null" @click="openCodeDialog('edit', asCustomerCode(row))">修改编码</el-button><el-button v-if="row.profile_count === 0" link type="danger" :loading="deletingCodeID === row.id" :disabled="deletingCodeID !== null && deletingCodeID !== row.id" @click="deleteCode(asCustomerCode(row))">删除</el-button></template></el-table-column>
        </el-table>
      </section>
    </template>

    <div v-if="filteredCodes.length" class="customer-pagination">
      <el-pagination background layout="prev, pager, next, sizes, total" :total="total" :current-page="page" :page-size="pageSize" :page-sizes="[10, 20, 50, 100]" @update:current-page="changePage" @update:page-size="changePageSize" />
    </div>

    <CustomerProfileDrawer
      ref="profileDrawer"
      v-model="drawerVisible"
      :mode="drawerMode"
      :profile="selectedProfile"
      :code="selectedCode"
      :codes="allCodeOptions"
      :suggested-code="suggestedCode"
      :saving="drawerSaving"
      :error="drawerError"
      :can-write="canWrite"
      :return-focus="returnFocus"
      @save="saveProfile"
      @edit="editProfile"
      @delete="requestDeleteProfile"
      @set-default="setDefaultProfile"
      @add-same="openCreateProfile"
      @select-profile="openProfile"
    />

    <el-dialog v-model="codeDialogVisible" :title="codeDialogMode === 'create' ? '新增客户编码' : '修改客户编码'" width="min(460px, 92vw)" :close-on-click-modal="!codeSaving" :close-on-press-escape="!codeSaving" :before-close="beforeCloseCodeDialog" @closed="codeError = ''">
      <div v-if="codeDialogVisible" key="customer-code-dialog-content" class="customer-dialog-motion">
          <el-form label-position="top" :disabled="codeSaving" @submit.prevent="saveCode">
            <el-alert v-if="codeError" :title="codeError" type="error" :closable="false" show-icon />
            <el-form-item label="客户编码" required :error="codeFieldError"><el-input v-model.trim="codeInput" maxlength="40" autofocus placeholder="BB-001" @blur="normalizeCodeInput" /><small class="field-help">1、BB-1、bb-001 均会规范为 BB-001。数字必须大于 0。</small></el-form-item>
            <el-alert v-if="codeDialogMode === 'edit'" title="修改后所有关联资料和选择项将显示新编码。" type="warning" :closable="false" show-icon />
          </el-form>
      </div>
      <template #footer><div class="dialog-actions"><el-button :disabled="codeSaving" @click="closeCodeDialog">取消</el-button><el-button type="primary" :loading="codeSaving" @click="saveCode">保存编码</el-button></div></template>
    </el-dialog>

    <el-dialog v-model="replacementVisible" title="选择替代默认资料" width="min(520px, 92vw)" :close-on-click-modal="!deleting" :close-on-press-escape="!deleting">
      <div v-if="replacementVisible" key="replacement-dialog-content" class="customer-dialog-motion">
          <p class="replacement-tip">当前删除的是默认资料。请先从同一客户编码中选择新默认，切换与删除会在同一事务中完成。</p>
          <el-alert v-if="deleteError" :title="deleteError" type="error" :closable="false" show-icon />
          <el-radio-group v-model="replacementID" class="replacement-list">
            <el-radio v-for="profile in replacementOptions" :key="profile.id" :value="profile.id" border><span><strong>{{ profile.short_name || profile.name || `资料 #${profile.id}` }}</strong><small>{{ profile.name || '未填写客户名称' }} · {{ profile.contact_name || '未填写联系人' }}</small></span></el-radio>
          </el-radio-group>
      </div>
      <template #footer><div class="dialog-actions"><el-button :disabled="deleting" @click="replacementVisible = false">取消</el-button><el-button type="danger" :loading="deleting" :disabled="!replacementID" @click="confirmDeleteWithReplacement">切换默认并删除</el-button></div></template>
    </el-dialog>

    <CustomerImportDialog v-if="canImport" v-model="importVisible" :token="token" @completed="handleImportCompleted" />
    <CustomerExportDialog v-model="exportVisible" :token="token" :keyword="keyword" :filter="filter" />
    </LayoutGroup>
  </div>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref, watch} from 'vue'
import {LayoutGroup, motion} from 'motion-v'
import {ElMessage} from 'element-plus'
import {request} from '../../api/http'
import {appMessageBox} from '../../composables/useAppMessageBox'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import type {CustomerCodeItem, CustomerOption, CustomerProfile, PaginatedResponse} from '../../types'
import PageHeader from '../ui/PageHeader.vue'
import PageState from '../ui/PageState.vue'
import CustomerProfileDrawer, {type CustomerProfileFormValue} from './CustomerProfileDrawer.vue'
import CustomerImportDialog from './CustomerImportDialog.vue'
import CustomerExportDialog from './CustomerExportDialog.vue'

const {token, hasPermission, switchModule, registerModuleLeaveGuard, cache} = useWorkspaceContext()
const canWrite = computed(() => hasPermission('customers:write'))
const canImport = computed(() => hasPermission('customers:import'))
const activeTab = ref<'profiles' | 'codes'>('profiles')
const sourceCodes = ref<CustomerCodeItem[]>([])
const allCodeOptions = computed(() => sourceCodes.value)
const loading = ref(false)
const listError = ref('')
const syncMessage = ref('')
const keyword = ref('')
const filter = ref<'all' | 'multiple' | 'empty'>('all')
const page = ref(1)
const pageSize = ref(20)
const listGeneration = ref(0)
const profileDetailGeneration = ref(0)
const normalizedKeyword = computed(() => keyword.value.trim().toLocaleLowerCase('zh-CN'))
const filteredCodes = computed(() => sourceCodes.value.filter((item) => {
  if (filter.value === 'multiple' && item.profile_count <= 1) return false
  if (filter.value === 'empty' && item.profile_count !== 0) return false
  if (!normalizedKeyword.value) return true
  const searchable = [
    item.code,
    ...item.profiles.flatMap((profile) => [
      profile.short_name,
      profile.name,
      profile.address,
      profile.phone,
      profile.contact_name,
      profile.contact_phone,
      profile.salesperson,
    ]),
  ]
  return searchable.some((value) => String(value || '').toLocaleLowerCase('zh-CN').includes(normalizedKeyword.value))
}))
const total = computed(() => filteredCodes.value.length)
const codes = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredCodes.value.slice(start, start + pageSize.value)
})
const filteredProfileCount = computed(() => filteredCodes.value.reduce((sum, item) => sum + item.profile_count, 0))
const filteredMultipleCount = computed(() => filteredCodes.value.filter((item) => item.profile_count > 1).length)
const filteredEmptyCount = computed(() => filteredCodes.value.filter((item) => item.profile_count === 0).length)
const hasFilters = computed(() => Boolean(normalizedKeyword.value || filter.value !== 'all'))

const drawerVisible = ref(false)
const drawerMode = ref<'view' | 'create' | 'edit'>('view')
const selectedProfile = ref<CustomerProfile | null>(null)
const selectedCode = ref<CustomerCodeItem | null>(null)
const suggestedCode = ref('')
const drawerSaving = ref(false)
const drawerError = ref('')
const profileDrawer = ref<InstanceType<typeof CustomerProfileDrawer> | null>(null)
const returnFocus = ref<HTMLElement | null>(null)
const importVisible = ref(false)
const exportVisible = ref(false)

const codeDialogVisible = ref(false)
const codeDialogMode = ref<'create' | 'edit'>('create')
const editingCode = ref<CustomerCodeItem | null>(null)
const codeInput = ref('')
const codeInitial = ref('')
const codeFieldError = ref('')
const codeError = ref('')
const codeSaving = ref(false)
const codeDirty = computed(() => codeInput.value !== codeInitial.value)

const replacementVisible = ref(false)
const pendingDeleteProfile = ref<CustomerProfile | null>(null)
const pendingDeleteCode = ref<CustomerCodeItem | null>(null)
const replacementID = ref<number | undefined>()
const deleting = ref(false)
const deletingCodeID = ref<number | null>(null)
const deleteError = ref('')
const replacementOptions = computed(() => (pendingDeleteCode.value?.profiles || []).filter((item) => item.id !== pendingDeleteProfile.value?.id))

function normalizeCodeRecord(item: CustomerCodeItem): CustomerCodeItem {
  const profiles = (item.profiles || []).map((profile) => ({...profile, code: item.code}))
  const defaultProfile = item.default_profile ? {...item.default_profile, code: item.code} : profiles.find((profile) => profile.is_default)
  return {...item, profiles, profile_count: Number(item.profile_count ?? profiles.length), default_profile: defaultProfile}
}
function asCustomerCode(value: unknown) { return value as CustomerCodeItem }
async function loadCodes() {
  const generation = ++listGeneration.value
  loading.value = true
  listError.value = ''
  syncMessage.value = sourceCodes.value.length ? '正在同步…' : ''
  try {
    const nextCodes: CustomerCodeItem[] = []
    let remotePage = 1
    let remoteTotal = 0
    do {
      const result = await request<PaginatedResponse<CustomerCodeItem>>(`/api/v1/customer-codes?page=${remotePage}&page_size=200`, {}, token.value)
      if (generation !== listGeneration.value) return
      nextCodes.push(...result.items.map(normalizeCodeRecord))
      remoteTotal = result.total
      remotePage += 1
      if (!result.items.length) break
    } while (nextCodes.length < remoteTotal)
    if (generation !== listGeneration.value) return
    sourceCodes.value = nextCodes
    syncMessage.value = '已同步'
    await loadCodeOptions(generation)
  } catch (cause) {
    if (generation === listGeneration.value) { listError.value = cause instanceof Error ? cause.message : '加载失败'; syncMessage.value = listError.value }
  } finally { if (generation === listGeneration.value) loading.value = false }
}
async function loadCodeOptions(generation: number) {
  try {
    const options = await request<CustomerOption[]>('/api/v1/customers/options', {}, token.value)
    if (generation !== listGeneration.value) return
    cache.customers = options.map((item) => ({...item, name: item.short_name || item.name || item.code}))
  } catch { /* The visible grouped list remains usable if background options refresh fails. */ }
}
function resetFilters() { keyword.value = ''; filter.value = 'all'; page.value = 1 }
function changePage(value: number) { page.value = value }
function changePageSize(value: number) { pageSize.value = value; page.value = 1 }
function handleTabChange() { page.value = 1 }
function profileTitle(profile?: CustomerProfile | null) { return profile?.short_name || profile?.name || (profile ? `资料 #${profile.id}` : '无默认资料') }
function codeProfileSummary(code: CustomerCodeItem) {
  if (!code.profile_count) return '尚未关联客户资料'
  const title = profileTitle(code.default_profile)
  return code.profile_count > 1 ? `默认：${title}；另有关联资料` : title
}
function formatDate(value?: string) { if (!value) return '—'; const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', {hour12: false}) }
function eventTarget(event?: Event) { return event?.currentTarget instanceof HTMLElement ? event.currentTarget : document.activeElement instanceof HTMLElement ? document.activeElement : null }

async function openCreateProfile(code?: CustomerCodeItem | null, event?: Event) {
  profileDetailGeneration.value += 1
  if (event || !drawerVisible.value) returnFocus.value = eventTarget(event)
  drawerMode.value = 'create'
  selectedProfile.value = null
  selectedCode.value = code || null
  drawerError.value = ''
  suggestedCode.value = ''
  if (!code) {
    try { suggestedCode.value = (await request<{code: string}>('/api/v1/customer-codes/next', {}, token.value)).code } catch { suggestedCode.value = 'BB-001' }
  }
  drawerVisible.value = true
}
async function openProfile(profile: CustomerProfile, code: CustomerCodeItem, event?: Event) {
  const generation = ++profileDetailGeneration.value
  const targetProfileID = profile.id
  if (event || !drawerVisible.value) returnFocus.value = eventTarget(event)
  drawerMode.value = 'view'
  selectedCode.value = code
  selectedProfile.value = profile
  drawerError.value = ''
  drawerVisible.value = true
  try {
    const detail = await request<CustomerProfile>(`/api/v1/customers/${targetProfileID}`, {}, token.value)
    if (generation !== profileDetailGeneration.value || !drawerVisible.value || drawerMode.value !== 'view' || selectedProfile.value?.id !== targetProfileID || detail.id !== targetProfileID) return
    selectedProfile.value = detail
  } catch (cause) {
    if (generation === profileDetailGeneration.value && drawerVisible.value && drawerMode.value === 'view' && selectedProfile.value?.id === targetProfileID) {
      drawerError.value = cause instanceof Error ? cause.message : '资料详情加载失败'
    }
  }
}
function editProfile(profile: CustomerProfile, code: CustomerCodeItem | null) { profileDetailGeneration.value += 1; selectedProfile.value = profile; selectedCode.value = code; drawerMode.value = 'edit'; drawerError.value = '' }

async function saveProfile(form: CustomerProfileFormValue) {
  drawerSaving.value = true
  drawerError.value = ''
  try {
    if (drawerMode.value === 'edit' && selectedProfile.value) {
      const {customer_code_id: _customerCodeID, new_code: _newCode, ...body} = form
      await request(`/api/v1/customers/${selectedProfile.value.id}`, {method: 'PATCH', body}, token.value)
      ElMessage.success('客户资料已保存')
    } else {
      let customerCodeID = form.customer_code_id
      if (!customerCodeID) {
        const created = await request<CustomerCodeItem>('/api/v1/customer-codes', {method: 'POST', body: {code: form.new_code}}, token.value)
        customerCodeID = created.id
      }
      const {new_code: _newCode, ...rest} = form
      await request('/api/v1/customers', {method: 'POST', body: {...rest, customer_code_id: customerCodeID}}, token.value)
      ElMessage.success('客户资料已新增')
    }
    drawerVisible.value = false
    await loadCodes()
  } catch (cause) { drawerError.value = cause instanceof Error ? cause.message : '客户资料保存失败' }
  finally { drawerSaving.value = false }
}
async function setDefaultProfile(profile: CustomerProfile) {
  try { await appMessageBox.confirm(`确认将“${profileTitle(profile)}”设为该客户编码的默认资料？`, '切换默认资料', {type: 'warning'}) } catch { return }
  drawerSaving.value = true; drawerError.value = ''
  try {
    const updated = await request<CustomerProfile>(`/api/v1/customers/${profile.id}/default`, {method: 'PUT'}, token.value)
    selectedProfile.value = updated
    await loadCodes()
    selectedCode.value = sourceCodes.value.find((item) => item.id === updated.customer_code_id) || selectedCode.value
    ElMessage.success('默认资料已切换')
  } catch (cause) { drawerError.value = cause instanceof Error ? cause.message : '默认资料切换失败' }
  finally { drawerSaving.value = false }
}
async function requestDeleteProfile(profile: CustomerProfile, code: CustomerCodeItem | null) {
  if (!code) return
  if (profile.is_default && code.profiles.length > 1) {
    pendingDeleteProfile.value = profile; pendingDeleteCode.value = code; replacementID.value = undefined; deleteError.value = ''; replacementVisible.value = true; return
  }
  try { await appMessageBox.confirm(`删除后无法恢复，确认物理删除“${profileTitle(profile)}”？`, '删除客户资料', {type: 'warning', confirmButtonText: '确认删除'}) } catch { return }
  await deleteProfile(profile.id)
}
async function confirmDeleteWithReplacement() { if (pendingDeleteProfile.value && replacementID.value) await deleteProfile(pendingDeleteProfile.value.id, replacementID.value) }
async function deleteProfile(id: number, replacement?: number) {
  deleting.value = true; drawerSaving.value = true; deleteError.value = ''; drawerError.value = ''
  try {
    const query = replacement ? `?replacement_id=${replacement}` : ''
    await request(`/api/v1/customers/${id}${query}`, {method: 'DELETE'}, token.value)
    replacementVisible.value = false; drawerVisible.value = false
    await loadCodes(); ElMessage.success('客户资料已删除')
  } catch (cause) { const message = cause instanceof Error ? cause.message : '删除失败'; if (replacementVisible.value) deleteError.value = message; else drawerError.value = message }
  finally { deleting.value = false; drawerSaving.value = false }
}

async function openCodeDialog(mode: 'create' | 'edit', code?: CustomerCodeItem) {
  codeDialogMode.value = mode; editingCode.value = code || null; codeError.value = ''; codeFieldError.value = ''
  if (mode === 'edit' && code) codeInput.value = code.code
  else { try { codeInput.value = (await request<{code: string}>('/api/v1/customer-codes/next', {}, token.value)).code } catch { codeInput.value = 'BB-001' } }
  codeInitial.value = codeInput.value; codeDialogVisible.value = true
}
function normalizeCodeInput() { const match = /^(?:BB-)?(\d+)$/i.exec(codeInput.value.trim()); if (!match) return; const value = Number(match[1]); if (Number.isSafeInteger(value) && value > 0) codeInput.value = `BB-${String(value).padStart(3, '0')}` }
async function saveCode() {
  normalizeCodeInput(); codeFieldError.value = ''; codeError.value = ''
  if (!/^BB-\d{3,}$/.test(codeInput.value) || Number(codeInput.value.slice(3)) <= 0) { codeFieldError.value = '请输入 BB- 加至少 3 位正整数'; return }
  if (codeDialogMode.value === 'edit') { try { await appMessageBox.confirm('客户编码会同步影响所有关联资料的显示，确认修改？', '确认修改客户编码', {type: 'warning'}) } catch { return } }
  codeSaving.value = true
  try {
    if (codeDialogMode.value === 'create') await request('/api/v1/customer-codes', {method: 'POST', body: {code: codeInput.value}}, token.value)
    else await request(`/api/v1/customer-codes/${editingCode.value?.id}`, {method: 'PATCH', body: {code: codeInput.value}}, token.value)
    codeDialogVisible.value = false; await loadCodes(); ElMessage.success(codeDialogMode.value === 'create' ? '客户编码已新增' : '客户编码已修改')
  } catch (cause) { codeError.value = cause instanceof Error ? cause.message : '客户编码保存失败' }
  finally { codeSaving.value = false }
}
async function deleteCode(code: CustomerCodeItem) {
  if (code.profile_count > 0 || deletingCodeID.value !== null) return
  try { await appMessageBox.confirm(`确认物理删除未使用的客户编码 ${code.code}？`, '删除客户编码', {type: 'warning', confirmButtonText: '确认删除'}) } catch { return }
  deletingCodeID.value = code.id
  try { await request(`/api/v1/customer-codes/${code.id}`, {method: 'DELETE'}, token.value); await loadCodes(); ElMessage.success('客户编码已删除') } catch (cause) { ElMessage.error(cause instanceof Error ? cause.message : '客户编码删除失败') }
  finally { deletingCodeID.value = null }
}
async function closeCodeDialog() { if (!(await confirmCodeClose())) return; codeDialogVisible.value = false }
async function confirmCodeClose() { if (!codeDirty.value) return true; try { await appMessageBox.confirm('客户编码尚未保存，确认放弃？', '放弃修改', {type: 'warning'}); return true } catch { return false } }
async function beforeCloseCodeDialog(done: () => void) { if (!codeSaving.value && await confirmCodeClose()) done() }
async function handleImportCompleted() { page.value = 1; await loadCodes() }

async function leaveGuard() {
  if (drawerSaving.value || codeSaving.value || deleting.value || deletingCodeID.value !== null) {
    ElMessage.warning('客户资料正在提交，请等待完成后再离开')
    return false
  }
  if (drawerVisible.value && !(await profileDrawer.value?.requestClose())) return false
  if (codeDialogVisible.value && !(await confirmCodeClose())) return false
  if (importVisible.value) { try { await appMessageBox.confirm('当前 Excel 导入流程将被关闭，确认离开客户模块？', '放弃导入', {type: 'warning'}) } catch { return false } }
  codeDialogVisible.value = false; importVisible.value = false; exportVisible.value = false
  return true
}
function beforeUnload(event: BeforeUnloadEvent) { if (drawerSaving.value || codeSaving.value || deleting.value || deletingCodeID.value !== null || profileDrawer.value?.dirty || codeDirty.value || importVisible.value) { event.preventDefault(); event.returnValue = '' } }
watch([keyword, filter], () => { page.value = 1 })
watch(drawerVisible, (visible) => { if (!visible) profileDetailGeneration.value += 1 }, {flush: 'sync'})
watch([() => filteredCodes.value.length, pageSize], () => {
  const lastPage = Math.max(1, Math.ceil(filteredCodes.value.length / pageSize.value))
  if (page.value > lastPage) page.value = lastPage
})
onMounted(() => { registerModuleLeaveGuard(leaveGuard); window.addEventListener('beforeunload', beforeUnload); void loadCodes() })
onBeforeUnmount(() => { profileDetailGeneration.value += 1; registerModuleLeaveGuard(null); window.removeEventListener('beforeunload', beforeUnload) })
</script>

<style scoped>
.customer-page { min-width: 0; }
.customer-tabs { margin-bottom: var(--bb-space-3); }
.customer-tabs :deep(.el-tabs__item) { min-height: 44px; font-weight: var(--bb-font-weight-semibold); }
.customer-filter-bar { display: flex; min-height: 62px; align-items: center; justify-content: space-between; gap: var(--bb-space-3); margin-bottom: var(--bb-space-3); border: 1px solid var(--bb-border-default); border-radius: var(--bb-radius-lg); background: var(--bb-bg-surface); padding: var(--bb-space-2) var(--bb-space-3); box-shadow: var(--bb-shadow-xs); }
.customer-filter-controls,
.customer-filter-actions { display: flex; align-items: center; gap: var(--bb-space-2); }
.customer-filter-controls { min-width: 0; flex: 1 1 auto; }
.customer-filter-controls .el-input { width: min(480px, 48vw); }
.customer-filter-controls .el-select { width: 190px; }
.customer-filter-actions { flex: 0 0 auto; }
.customer-filter-actions span { color: var(--bb-text-secondary); font-size: var(--bb-font-size-13); }
.customer-summary { display: flex; min-height: 40px; align-items: center; margin-bottom: var(--bb-space-3); border: 1px solid var(--bb-border-default); border-radius: var(--bb-radius-md); background: var(--bb-bg-surface); padding: var(--bb-space-2) var(--bb-space-3); color: var(--bb-text-secondary); font-size: var(--bb-font-size-13); }
.customer-summary strong { color: var(--bb-text-primary); font-size: var(--bb-font-size-14); font-variant-numeric: tabular-nums; }
.customer-desktop-table { overflow-x: auto; overflow-y: hidden; border: 1px solid var(--bb-border-default); border-radius: var(--bb-radius-xl); background: var(--bb-bg-surface); box-shadow: var(--bb-shadow-xs); }
.customer-code { color: var(--bb-brand-700); font-family: var(--bb-font-mono); font-weight: var(--bb-font-weight-bold); white-space: nowrap; }
.primary-profile { display: grid; gap: var(--bb-space-1); }
.primary-profile small { overflow: hidden; color: var(--bb-text-secondary); text-overflow: ellipsis; white-space: nowrap; }
.text-cell { font-family: var(--bb-font-mono); }
.code-toolbar { display: flex; align-items: center; justify-content: space-between; gap: var(--bb-space-4); margin-bottom: var(--bb-space-3); border: 1px solid var(--bb-border-default); border-radius: var(--bb-radius-lg); background: var(--bb-bg-surface); padding: var(--bb-space-3) var(--bb-space-4); }
.code-toolbar > div { display: grid; gap: var(--bb-space-1); }
.code-toolbar span { color: var(--bb-text-secondary); font-size: var(--bb-font-size-13); }
.customer-pagination { display: flex; justify-content: flex-end; overflow-x: auto; margin-top: var(--bb-space-4); padding-bottom: var(--bb-space-1); }
.dialog-actions { display: flex; justify-content: flex-end; gap: var(--bb-space-2); }
.field-help { display: block; margin-top: var(--bb-space-1); color: var(--bb-text-secondary); line-height: var(--bb-line-height-base); }
.replacement-tip { margin: 0 0 var(--bb-space-4); color: var(--bb-text-secondary); line-height: var(--bb-line-height-relaxed); }
.replacement-list { display: grid; gap: var(--bb-space-2); margin-top: var(--bb-space-3); }
.replacement-list :deep(.el-radio) { width: 100%; height: auto; min-height: 64px; align-items: center; margin: 0; padding: var(--bb-space-3); }
.replacement-list :deep(.el-radio__label) { min-width: 0; white-space: normal; }
.replacement-list span { display: grid; gap: var(--bb-space-1); }
.replacement-list small { color: var(--bb-text-secondary); }

@media (max-width: 900px) {
  .customer-filter-bar { align-items: stretch; flex-direction: column; }
  .customer-filter-controls,
  .customer-filter-actions { align-items: stretch; flex-wrap: wrap; }
  .customer-filter-controls .el-input,
  .customer-filter-controls .el-select { width: 100%; }
  .customer-filter-actions { justify-content: space-between; }
  .customer-desktop-table { overflow-x: auto; }
  .customer-pagination { justify-content: center; }
  .code-toolbar { align-items: stretch; flex-direction: column; }
  .code-toolbar .el-button { width: 100%; margin: 0; }
}
</style>
