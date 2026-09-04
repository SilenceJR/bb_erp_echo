<template>
  <div class="data-page employee-page">
    <PageHeader title="员工档案" description="维护员工基础信息和在职状态；所属部门由部门模块统一管理。" :readonly="!canWrite" @back="switchModule('dashboard')">
      <template #actions><el-button v-if="canWrite" type="primary" @click="openEmployee('create')">＋ 新增员工</el-button></template>
    </PageHeader>
    <FilterBar :loading="directory.loading.value" :resettable="Boolean(directory.keyword.value || directory.departmentID.value || directory.status.value !== 'active')" @submit="directory.applySearch" @reset="resetFilters" @refresh="directory.load">
      <el-input v-model.trim="directory.keyword.value" clearable placeholder="搜索姓名或电话" aria-label="员工关键词" />
      <el-select v-model="directory.departmentID.value" clearable :loading="directory.departmentsLoading.value" :disabled="!canReadDepartments || Boolean(directory.departmentsError.value)" :placeholder="departmentFilterPlaceholder" aria-label="所属部门" @change="directory.applySearch"><el-option v-for="item in directory.departments.value" :key="item.id" :label="item.name" :value="item.id" /></el-select>
      <el-select v-model="directory.status.value" placeholder="员工状态" aria-label="员工状态" @change="directory.applySearch"><el-option label="在职" value="active"/><el-option label="已停用" value="disabled"/><el-option label="全部" value=""/></el-select>
    </FilterBar>
    <el-alert v-if="!canReadDepartments" title="当前账号无部门查看权限，不能按部门筛选；员工档案仍可正常查看。" type="info" :closable="false" show-icon />
    <el-alert v-else-if="directory.departmentsError.value" :title="`部门筛选项加载失败：${directory.departmentsError.value}`" type="error" :closable="false" show-icon><template #default><el-button link type="primary" @click="directory.loadDepartments">重新加载部门</el-button></template></el-alert>
    <PageState v-if="directory.error.value && !directory.employees.value.length" kind="error" title="员工档案加载失败" :description="directory.error.value" action-label="重新加载" @action="directory.load" />
    <el-alert v-else-if="directory.error.value" :title="directory.error.value" type="error" :closable="false" show-icon><template #default><el-button link type="primary" @click="directory.load">重新加载</el-button></template></el-alert>
    <PageState v-else-if="!directory.loading.value && !directory.employees.value.length" kind="empty" :title="hasFilters ? '没有符合筛选条件的员工' : '暂无记录'" :description="emptyDescription" :action-label="emptyActionLabel" @action="handleEmptyAction" />
    <DataTableShell v-else :loading="directory.loading.value" :rows-count="directory.employees.value.length" :total="directory.total.value" :page="directory.page.value" :page-size="directory.pageSize.value" @update:page="changePage" @update:page-size="changePageSize" @retry="directory.load">
      <div class="responsive-table-desktop"><el-table :data="directory.employees.value" row-key="id" stripe>
        <el-table-column label="员工" min-width="150"><template #default="{row}"><span class="item-name">{{ row.name }}</span><small class="item-code">#{{ row.id }} · {{ row.phone || '未填写电话' }}</small></template></el-table-column>
        <el-table-column prop="hire_date" label="入职日期" width="125" />
        <el-table-column label="出生/年龄" width="160"><template #default="{row}">{{ row.birth_date }} · {{ row.age }} 岁</template></el-table-column>
        <el-table-column label="所属部门" min-width="180"><template #default="{row}"><div class="tag-list"><el-tag v-for="item in row.departments" :key="item.id" effect="plain">{{ item.name }}</el-tag><span v-if="!row.departments?.length">未分配</span></div></template></el-table-column>
        <el-table-column label="状态" width="95"><template #default="{row}"><StatusTag :label="row.status === 'active' ? '在职' : '已停用'" :tone="row.status === 'active' ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="操作" width="210" fixed="right"><template #default="{row}"><el-button link type="primary" :disabled="statusUpdatingID === row.id" @click="openEmployee('view', row)">详情</el-button><el-button v-if="canWrite" link type="primary" :disabled="statusUpdatingID === row.id" @click="openEmployee('edit', row)">编辑</el-button><el-button v-if="canWrite" link :type="row.status === 'active' ? 'danger' : 'success'" :loading="statusUpdatingID === row.id" :disabled="statusUpdatingID !== null && statusUpdatingID !== row.id" @click="toggleStatus(row)">{{ row.status === 'active' ? '停用' : '启用' }}</el-button></template></el-table-column>
      </el-table></div>
    </DataTableShell>
    <EmployeeFormDrawer v-model="directory.drawerVisible.value" :title="directory.title.value" :form="directory.form" :editing="directory.editing.value" :readonly="directory.drawerReadonly.value" :can-write="canWrite" @edit="directory.editing.value && openEmployee('edit', directory.editing.value)" :saving="directory.saving.value" :save-error="directory.saveError.value" @save="directory.save" />
  </div>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref} from 'vue'
import {ElMessage} from 'element-plus'
import {appMessageBox} from '../../composables/useAppMessageBox'
import {useEmployeeDirectory} from '../../composables/useEmployeeDirectory'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import {dirtyGuardRegistry} from '../../platform/dirtyGuard'
import type {EmployeeItem} from '../../types'
import DataTableShell from '../ui/DataTableShell.vue'
import FilterBar from '../ui/FilterBar.vue'
import PageHeader from '../ui/PageHeader.vue'
import PageState from '../ui/PageState.vue'
import StatusTag from '../ui/StatusTag.vue'
import EmployeeFormDrawer from './EmployeeFormDrawer.vue'

const {token, hasPermission, switchModule} = useWorkspaceContext()
const canWrite = hasPermission('system:employees:write')
const canReadDepartments = hasPermission('system:departments:read')
const directory = useEmployeeDirectory(token)
const statusUpdatingID = ref<number | null>(null)
let switchingEmployee = false
async function openEmployee(mode: 'create' | 'view' | 'edit', item?: any) {
  if (switchingEmployee) return
  switchingEmployee = true
  try {
    if (!(await dirtyGuardRegistry.confirmLeave('dialog-close'))) return
    if (mode === 'create') directory.openCreate()
    else if (item && mode === 'view') directory.openView(item)
    else if (item) directory.openEdit(item)
  } finally { switchingEmployee = false }
}
const hasFilters = computed(() => Boolean(directory.keyword.value || directory.departmentID.value || directory.status.value !== 'active'))
const emptyDescription = computed(() => hasFilters.value ? '请调整筛选条件。' : '')
const emptyActionLabel = computed(() => hasFilters.value ? '清除筛选' : canWrite ? '新增员工' : '')
const departmentFilterPlaceholder = computed(() => !canReadDepartments ? '无部门查看权限' : directory.departmentsError.value ? '部门加载失败' : '所属部门')
const employeeInitial = computed(() => directory.editing.value ? JSON.stringify({name: directory.editing.value.name, phone: directory.editing.value.phone || '', hire_date: directory.editing.value.hire_date, birthplace: directory.editing.value.birthplace || '', residential_address: directory.editing.value.residential_address || '', birth_date: directory.editing.value.birth_date}) : JSON.stringify({name: '', phone: '', hire_date: '', birthplace: '', residential_address: '', birth_date: ''}))
const employeeDirty = computed(() => directory.drawerVisible.value && !directory.drawerReadonly.value && JSON.stringify(directory.form) !== employeeInitial.value)
let removeDirtyGuard = () => {}
onMounted(() => {
  removeDirtyGuard = dirtyGuardRegistry.register({
    id: 'employee-form',
    blocksUnload: () => directory.saving.value || statusUpdatingID.value !== null || employeeDirty.value,
    async confirmLeave() {
      if (directory.saving.value || statusUpdatingID.value !== null) { ElMessage.warning(statusUpdatingID.value !== null ? '员工状态正在更新，请等待完成后再离开' : '员工档案正在保存，请等待完成后再离开'); return false }
      if (!employeeDirty.value) return true
      try { await appMessageBox.confirm('尚有未保存的员工信息，离开后填写内容将丢失。', '放弃修改？', {type: 'warning'}); return true } catch { return false }
    },
  })
  void Promise.all([directory.load(), canReadDepartments ? directory.loadDepartments() : Promise.resolve()])
})
onBeforeUnmount(() => removeDirtyGuard())
function resetFilters() { directory.keyword.value = ''; directory.departmentID.value = undefined; directory.status.value = 'active'; directory.applySearch() }
function handleEmptyAction() { if (hasFilters.value) resetFilters(); else if (canWrite) void openEmployee('create') }
function changePage(value: number) { directory.page.value = value; void directory.load() }
function changePageSize(value: number) { directory.pageSize.value = value; directory.page.value = 1; void directory.load() }
async function toggleStatus(row: unknown) {
  const item = row as EmployeeItem
  if (statusUpdatingID.value !== null) return
  try { await appMessageBox.confirm(item.status === 'active' ? `停用员工“${item.name}”？历史记录和部门关系会保留。` : `重新启用员工“${item.name}”？`, item.status === 'active' ? '确认停用' : '确认启用', {type: item.status === 'active' ? 'warning' : 'success', confirmButtonText: item.status === 'active' ? '确认停用' : '确认启用', confirmButtonClass: item.status === 'active' ? 'el-button--danger' : ''}) } catch { return }
  statusUpdatingID.value = item.id
  try {
    await directory.setStatus(item, item.status === 'active' ? 'disabled' : 'active')
    ElMessage.success('员工状态已更新')
  } catch (cause) {
    ElMessage.error(cause instanceof Error ? cause.message : '员工状态更新失败')
  } finally { statusUpdatingID.value = null }
}
</script>
