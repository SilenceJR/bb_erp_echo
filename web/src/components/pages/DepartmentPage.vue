<template>
  <div class="data-page department-page">
    <PageHeader title="部门" description="维护部门状态和成员关系；同一员工可以加入多个部门。" :readonly="!canWrite" @back="switchModule('dashboard')"><template #actions><el-button v-if="canWrite" type="primary" @click="openDepartment('create')">＋ 新增部门</el-button></template></PageHeader>
    <el-alert v-if="canWrite && !canManageMembers" title="当前账号缺少员工档案查看权限，不能管理部门成员；部门基础信息仍可维护。" type="info" :closable="false" show-icon />
    <PageState v-if="state.error.value && !state.departments.value.length" kind="error" title="部门加载失败" :description="state.error.value" action-label="重新加载" @action="state.load" />
    <el-alert v-else-if="state.error.value" :title="state.error.value" type="error" :closable="false" show-icon><template #default><el-button link type="primary" @click="state.load">重新加载</el-button></template></el-alert>
    <DataTableShell v-else :loading="state.loading.value" :rows-count="state.departments.value.length" :total="state.departments.value.length" :page="1" :page-size="200" :pagination="false" empty-title="暂无记录" @retry="state.load">
      <div class="responsive-table-desktop"><el-table :data="state.departments.value" row-key="id" stripe><el-table-column label="部门" min-width="200"><template #default="{row}"><span class="item-name">{{ row.name }}</span><small class="item-code">{{ row.code }}</small></template></el-table-column><el-table-column label="在职员工" width="120"><template #default="{row}">{{ row.employee_count || 0 }} 人</template></el-table-column><el-table-column label="状态" width="100"><template #default="{row}"><StatusTag :label="row.status === 'disabled' ? '已停用' : '启用'" :tone="row.status === 'disabled' ? 'info' : 'success'" /></template></el-table-column><el-table-column label="操作" min-width="330" fixed="right"><template #default="{row}"><el-button link type="primary" :disabled="statusUpdatingID === row.id" @click="openDepartment('view', row)">详情</el-button><el-button v-if="canWrite" link type="primary" :disabled="statusUpdatingID === row.id" @click="openDepartment('edit', row)">编辑</el-button><el-button v-if="canWrite" link type="primary" :disabled="!canManageMembers || statusUpdatingID === row.id" :title="canManageMembers ? '' : '需要员工档案查看权限'" @click="openDepartment('members', row)">管理员工</el-button><el-button v-if="canWrite" link :type="row.status === 'disabled' ? 'success' : 'danger'" :loading="statusUpdatingID === row.id" :disabled="statusUpdatingID !== null && statusUpdatingID !== row.id" @click="toggleStatus(row)">{{ row.status === 'disabled' ? '启用' : '停用' }}</el-button></template></el-table-column></el-table></div>
    </DataTableShell>
    <ResponsiveDetailCarrier v-model="state.formVisible.value" drawer-class="business-form-drawer" :title="state.formReadonly.value ? '部门详情' : state.editing.value ? '编辑部门' : '新增部门'" :size="formPanel.size.value" :docked="formPanel.docked.value" :docked-auto-focus="state.formReadonly.value ? 'preserve' : 'first-editable'" :close-on-click-modal="!state.saving.value" :close-on-press-escape="!state.saving.value" :before-close="requestDepartmentClose" destroy-on-close>
      <section v-if="state.formReadonly.value && state.editing.value" class="department-detail" aria-label="部门详情">
        <PropertyList>
          <PropertyItem label="部门名称">{{ state.editing.value.name }}</PropertyItem>
          <PropertyItem label="部门编码"><span class="text-cell">{{ state.editing.value.code || '未填写' }}</span></PropertyItem>
          <PropertyItem label="在职员工">{{ state.editing.value.employee_count || 0 }} 人</PropertyItem>
          <PropertyItem label="状态">{{ state.editing.value.status === 'disabled' ? '已停用' : '启用' }}</PropertyItem>
        </PropertyList>
      </section>
      <el-form v-else id="department-editor" label-position="top" :disabled="state.saving.value" @submit.prevent="saveDepartment"><el-alert v-if="state.saveError.value" :title="state.saveError.value" type="error" :closable="false" show-icon/><el-form-item label="部门名称" required><el-input v-model.trim="state.form.name" autofocus /></el-form-item><el-form-item label="部门编码" required><el-input v-model.trim="state.form.code" /></el-form-item></el-form>
      <template #footer><div class="form-actions"><el-button :disabled="state.saving.value" @click="requestDepartmentClose()">{{ state.formReadonly.value ? '关闭' : '取消' }}</el-button><el-button v-if="state.formReadonly.value && canManageMembers" type="primary" plain @click="openDepartment('members', state.editing.value || undefined)">管理员工</el-button><el-button v-if="state.formReadonly.value && canWrite" type="primary" plain @click="state.formReadonly.value = false">编辑部门</el-button><el-button v-if="!state.formReadonly.value" type="primary" native-type="submit" form="department-editor" :loading="state.saving.value">保存</el-button></div></template>
    </ResponsiveDetailCarrier>
    <DepartmentEmployeesDrawer v-model="state.memberVisible.value" :department="state.memberDepartment.value" :employees="state.employees.value" v-model:selectedEmployeeIDs="state.selectedEmployeeIDs.value" :original-employee-i-ds="state.originalEmployeeIDs.value" v-model:keyword="state.memberKeyword.value" :loading="state.memberLoading.value" :saving="state.saving.value" :load-error="state.memberLoadError.value" :save-error="state.saveError.value" @retry="state.loadMembers()" @save="state.saveMembers" />
  </div>
</template>
<script setup lang="ts">
import ResponsiveDetailCarrier from '../ui/ResponsiveDetailCarrier.vue'
import {useResponsiveDetailPanel} from '../../composables/useResponsiveDetailPanel'
import {computed, onBeforeUnmount, onMounted, ref} from 'vue'
import {ElMessage} from 'element-plus'
import {appMessageBox} from '../../composables/useAppMessageBox'
import {useDepartments} from '../../composables/useDepartments'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import {dirtyGuardRegistry} from '../../platform/dirtyGuard'
import type {DepartmentItem} from '../../types'
import DataTableShell from '../ui/DataTableShell.vue'; import PageHeader from '../ui/PageHeader.vue'; import PageState from '../ui/PageState.vue'; import StatusTag from '../ui/StatusTag.vue'; import DepartmentEmployeesDrawer from './DepartmentEmployeesDrawer.vue'
import PropertyItem from '../ui/PropertyItem.vue'
import PropertyList from '../ui/PropertyList.vue'
const {token, hasPermission, switchModule, loadList} = useWorkspaceContext(); const canWrite = hasPermission('system:departments:write'); const canManageMembers = canWrite && hasPermission('system:employees:read'); const state = useDepartments(token); const statusUpdatingID = ref<number | null>(null)
let switchingDepartment = false
async function openDepartment(mode: 'create' | 'view' | 'edit' | 'members', item?: any) {
  if (switchingDepartment) return
  switchingDepartment = true
  try {
    if (!(await dirtyGuardRegistry.confirmLeave('dialog-close'))) return
    state.formVisible.value = false
    state.memberVisible.value = false
    if (mode === 'create') state.openCreate()
    else if (mode === 'view' && item) state.openView(item)
    else if (mode === 'edit' && item) state.openEdit(item)
    else if (item) await state.openMembers(item)
  } finally { switchingDepartment = false }
}
const formPanel = useResponsiveDetailPanel(state.formVisible, computed(() => !state.formReadonly.value))
const departmentFormDirty = computed(() => { if (!state.formVisible.value || state.formReadonly.value) return false; const original = state.editing.value ? {name: state.editing.value.name, code: state.editing.value.code || ''} : {name: '', code: ''}; return state.form.name !== original.name || state.form.code !== original.code })
const departmentMembersDirty = computed(() => state.memberVisible.value && [...state.selectedEmployeeIDs.value].sort((a, b) => a - b).join(',') !== [...state.originalEmployeeIDs.value].sort((a, b) => a - b).join(','))
let removeDirtyGuard = () => {}
onMounted(() => {
  removeDirtyGuard = dirtyGuardRegistry.register({id: 'department-forms', blocksUnload: () => state.saving.value || statusUpdatingID.value !== null || departmentFormDirty.value || departmentMembersDirty.value, async confirmLeave() { if (state.saving.value || statusUpdatingID.value !== null) { ElMessage.warning(statusUpdatingID.value !== null ? '部门状态正在更新，请等待完成后再离开' : '部门资料正在保存，请等待完成后再离开'); return false } if (!departmentFormDirty.value && !departmentMembersDirty.value) return true; try { await appMessageBox.confirm('部门资料或成员选择尚未保存，离开后修改将丢失。', '放弃修改？', {type: 'warning'}); return true } catch { return false } }})
  void state.load()
})
onBeforeUnmount(() => removeDirtyGuard())
async function saveDepartment() { await state.save(); if (!state.saveError.value) await loadList('departments', false) }
async function requestDepartmentClose(done?: () => void) { if (state.saving.value) return; const original = state.editing.value ? {name: state.editing.value.name, code: state.editing.value.code || ''} : {name: '', code: ''}; const dirty = !state.formReadonly.value && (state.form.name !== original.name || state.form.code !== original.code); if (dirty) { try { await appMessageBox.confirm('部门信息尚未保存，确认关闭？', '放弃修改', {type: 'warning'}) } catch { return } } if (done) done(); else state.formVisible.value = false }
async function toggleStatus(item: any) {
  if (statusUpdatingID.value !== null) return
  const next = item.status === 'disabled' ? 'active' : 'disabled'
  try { await appMessageBox.confirm(next === 'disabled' ? `停用部门“${item.name}”？该部门账号将不能执行任务和库存写入。` : `重新启用部门“${item.name}”？`, next === 'disabled' ? '确认停用' : '确认启用', {type: next === 'disabled' ? 'warning' : 'success', confirmButtonText: next === 'disabled' ? '确认停用' : '确认启用', confirmButtonClass: next === 'disabled' ? 'el-button--danger' : ''}) } catch { return }
  statusUpdatingID.value = Number(item.id)
  try {
    await state.setStatus(item, next)
    await loadList('departments', false)
    ElMessage.success('部门状态已更新')
  } catch (cause) {
    ElMessage.error(cause instanceof Error ? cause.message : '部门状态更新失败')
  } finally { statusUpdatingID.value = null }
}
</script>
