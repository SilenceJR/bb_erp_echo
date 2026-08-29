<template>
  <div class="data-page employee-page">
    <PageHeader title="员工档案" description="维护员工基础信息和在职状态；所属部门由部门模块统一管理。" :readonly="!canWrite" @back="switchModule('dashboard')">
      <template #actions><el-button v-if="canWrite" type="primary" @click="directory.openCreate">＋ 新增员工</el-button></template>
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
    <PageState v-else-if="!directory.loading.value && !directory.employees.value.length" kind="empty" :title="hasFilters ? '没有符合筛选条件的员工' : '还没有员工档案'" :description="emptyDescription" :action-label="emptyActionLabel" @action="handleEmptyAction" />
    <DataTableShell v-else :loading="directory.loading.value" :rows-count="directory.employees.value.length" :total="directory.total.value" :page="directory.page.value" :page-size="directory.pageSize.value" @update:page="changePage" @update:page-size="changePageSize" @retry="directory.load">
      <div class="responsive-table-desktop"><el-table :data="directory.employees.value" row-key="id" stripe>
        <el-table-column label="员工" min-width="150"><template #default="{row}"><span class="item-name">{{ row.name }}</span><small class="item-code">#{{ row.id }} · {{ row.phone || '未填写电话' }}</small></template></el-table-column>
        <el-table-column prop="hire_date" label="入职日期" width="125" />
        <el-table-column label="出生/年龄" width="160"><template #default="{row}">{{ row.birth_date }} · {{ row.age }} 岁</template></el-table-column>
        <el-table-column label="所属部门" min-width="180"><template #default="{row}"><div class="tag-list"><el-tag v-for="item in row.departments" :key="item.id" effect="plain">{{ item.name }}</el-tag><span v-if="!row.departments?.length">未分配</span></div></template></el-table-column>
        <el-table-column label="状态" width="95"><template #default="{row}"><StatusTag :label="row.status === 'active' ? '在职' : '已停用'" :tone="row.status === 'active' ? 'success' : 'info'" /></template></el-table-column>
        <el-table-column label="操作" width="210" fixed="right"><template #default="{row}"><el-button link type="primary" :disabled="statusUpdatingID === row.id" @click="directory.openView(row)">详情</el-button><el-button v-if="canWrite" link type="primary" :disabled="statusUpdatingID === row.id" @click="directory.openEdit(row)">编辑</el-button><el-button v-if="canWrite" link :type="row.status === 'active' ? 'danger' : 'success'" :loading="statusUpdatingID === row.id" :disabled="statusUpdatingID !== null && statusUpdatingID !== row.id" @click="toggleStatus(row)">{{ row.status === 'active' ? '停用' : '启用' }}</el-button></template></el-table-column>
      </el-table></div>
      <div class="responsive-card-list"><article v-for="row in directory.employees.value" :key="row.id" class="employee-card"><div class="responsive-card-heading"><div><strong>{{ row.name }}</strong><small>#{{ row.id }} · {{ row.phone || '未填写电话' }}</small></div><StatusTag :label="row.status === 'active' ? '在职' : '已停用'" :tone="row.status === 'active' ? 'success' : 'info'" /></div><dl><div><dt>入职</dt><dd>{{ row.hire_date }}</dd></div><div><dt>年龄</dt><dd>{{ row.age }} 岁</dd></div><div><dt>部门</dt><dd>{{ row.departments?.map((item: any) => item.name).join('、') || '未分配' }}</dd></div></dl><div class="card-actions"><el-button :disabled="statusUpdatingID === row.id" @click="directory.openView(row)">查看详情</el-button><el-button v-if="canWrite" :disabled="statusUpdatingID === row.id" @click="directory.openEdit(row)">编辑</el-button><el-button v-if="canWrite" :type="row.status === 'active' ? 'danger' : 'success'" plain :loading="statusUpdatingID === row.id" :disabled="statusUpdatingID !== null && statusUpdatingID !== row.id" @click="toggleStatus(row)">{{ row.status === 'active' ? '停用' : '启用' }}</el-button></div></article></div>
    </DataTableShell>
    <EmployeeFormDrawer v-model="directory.drawerVisible.value" :title="directory.title.value" :form="directory.form" :editing="directory.editing.value" :readonly="directory.drawerReadonly.value" :saving="directory.saving.value" :save-error="directory.saveError.value" @save="directory.save" />
  </div>
</template>

<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
import {ElMessage} from 'element-plus'
import {appMessageBox} from '../../composables/useAppMessageBox'
import {useEmployeeDirectory} from '../../composables/useEmployeeDirectory'
import {useWorkspaceContext} from '../../composables/workspaceContext'
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
const hasFilters = computed(() => Boolean(directory.keyword.value || directory.departmentID.value || directory.status.value !== 'active'))
const emptyDescription = computed(() => hasFilters.value ? '可调整关键词、部门或员工状态后重试。' : canWrite ? '新增员工后，可在部门模块中配置成员关系。' : '当前账号仅可查看员工档案；如需建档，请联系管理员授权或代为新增。')
const emptyActionLabel = computed(() => hasFilters.value ? '清除筛选' : canWrite ? '新增员工' : '')
const departmentFilterPlaceholder = computed(() => !canReadDepartments ? '无部门查看权限' : directory.departmentsError.value ? '部门加载失败' : '所属部门')
onMounted(() => { void Promise.all([directory.load(), canReadDepartments ? directory.loadDepartments() : Promise.resolve()]) })
function resetFilters() { directory.keyword.value = ''; directory.departmentID.value = undefined; directory.status.value = 'active'; directory.applySearch() }
function handleEmptyAction() { if (hasFilters.value) resetFilters(); else if (canWrite) directory.openCreate() }
function changePage(value: number) { directory.page.value = value; void directory.load() }
function changePageSize(value: number) { directory.pageSize.value = value; directory.page.value = 1; void directory.load() }
async function toggleStatus(row: unknown) { const item = row as EmployeeItem; if (statusUpdatingID.value !== null) return; try { await appMessageBox.confirm(item.status === 'active' ? `停用员工“${item.name}”？历史记录和部门关系会保留。` : `重新启用员工“${item.name}”？`, item.status === 'active' ? '确认停用' : '确认启用', {type: item.status === 'active' ? 'warning' : 'success', confirmButtonText: item.status === 'active' ? '确认停用' : '确认启用', confirmButtonClass: item.status === 'active' ? 'el-button--danger' : ''}); statusUpdatingID.value = item.id; await directory.setStatus(item, item.status === 'active' ? 'disabled' : 'active'); ElMessage.success('员工状态已更新') } catch (cause) { if (cause instanceof Error) ElMessage.error(cause.message) } finally { statusUpdatingID.value = null } }
</script>
