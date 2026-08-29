<template>
  <el-drawer v-model="visible" class="business-form-drawer member-management-drawer" size="min(620px, 100%)" :title="`管理员工 · ${department?.name || ''}`" :close-on-click-modal="!saving" :close-on-press-escape="!saving" :before-close="requestClose" destroy-on-close>
    <el-alert v-if="department?.status !== 'active'" title="停用部门不能新增成员；你仍可移除现有成员或先重新启用部门。" type="warning" :closable="false" show-icon />
    <el-alert v-if="saveError" :title="saveError" type="error" :closable="false" show-icon />
    <el-input v-model.trim="keyword" clearable placeholder="搜索员工姓名或电话" aria-label="搜索员工" />
    <PageState v-if="loading" kind="loading" title="正在加载部门成员" />
    <PageState v-else-if="loadError" kind="error" title="部门成员加载失败" :description="loadError" action-label="重新加载" @action="$emit('retry')" />
    <el-checkbox-group v-else v-model="selected" class="department-employee-options">
      <el-checkbox v-for="employee in filtered" :key="employee.id" :value="employee.id" :disabled="(department?.status !== 'active' || employee.status !== 'active') && !selected.includes(employee.id)">
        <span class="check-option-copy"><strong>{{ employee.name }} <small v-if="duplicates(employee.name)">#{{ employee.id }}</small></strong><small>{{ employee.phone || '未填写电话' }} · {{ employee.status === 'active' ? '在职' : '已停用（保留关系）' }}</small></span>
      </el-checkbox>
    </el-checkbox-group>
    <p v-if="!loading && !filtered.length" class="drawer-empty">没有符合条件的员工</p>
    <template #footer><div class="form-actions"><el-button :disabled="saving" @click="requestClose()">取消</el-button><el-button type="primary" :loading="saving" :disabled="loading || Boolean(loadError)" @click="$emit('save')">保存 {{ selected.length }} 名成员</el-button></div></template>
  </el-drawer>
</template>
<script setup lang="ts">
import {computed} from 'vue'
import type {DepartmentItem, EmployeeItem} from '../../types'
import PageState from '../ui/PageState.vue'
import {appMessageBox} from '../../composables/useAppMessageBox'
const props = defineProps<{modelValue: boolean; department: DepartmentItem | null; employees: EmployeeItem[]; selectedEmployeeIDs: number[]; originalEmployeeIDs: number[]; keyword: string; loading: boolean; saving: boolean; loadError: string; saveError: string}>()
const emit = defineEmits<{(event: 'update:modelValue', value: boolean): void; (event: 'update:selectedEmployeeIDs', value: number[]): void; (event: 'update:keyword', value: string): void; (event: 'save'): void; (event: 'retry'): void}>()
const visible = computed({get: () => props.modelValue, set: (value) => emit('update:modelValue', value)})
const selected = computed({get: () => props.selectedEmployeeIDs, set: (value) => emit('update:selectedEmployeeIDs', value)})
const keyword = computed({get: () => props.keyword, set: (value) => emit('update:keyword', value)})
const filtered = computed(() => { const q = props.keyword.trim().toLocaleLowerCase('zh-CN'); return q ? props.employees.filter((item) => `${item.name} ${item.phone || ''}`.toLocaleLowerCase('zh-CN').includes(q)) : props.employees })
const duplicates = (name: string) => props.employees.filter((item) => item.name === name).length > 1
const dirty = computed(() => [...props.selectedEmployeeIDs].sort((a, b) => a - b).join(',') !== [...props.originalEmployeeIDs].sort((a, b) => a - b).join(','))
async function requestClose(done?: () => void) { if (props.saving) return; if (dirty.value) { try { await appMessageBox.confirm('部门成员选择尚未保存，确认关闭？', '放弃修改', {type: 'warning'}) } catch { return } } if (done) done(); else visible.value = false }
</script>
