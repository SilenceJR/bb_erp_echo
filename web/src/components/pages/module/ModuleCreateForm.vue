<template>
  <el-form v-if="formSchema.length && canWriteActive && showCreateForm" class="inline-form" label-position="top" @submit.prevent="createItem">
    <div class="form-heading">
      <strong>{{ editingSupplier ? '编辑供应商' : `新增${createEntityTitle}` }}</strong>
      <span>请填写以下信息，带 * 为常用必填项</span>
    </div>
    <el-alert v-if="formError" id="workorder-create-error" class="form-error" :title="formError" type="error" :closable="false" show-icon />
    <el-alert v-if="activeKey === 'users' && !canCreateDepartmentTerminalUser" title="当前权限只能创建个人账号" description="创建部门终端账号还需要部门查看和终端查看权限。" type="info" :closable="false" show-icon />
    <template v-for="field in formSchema" :key="field.key">
      <WorkorderProductField v-if="field.kind === 'workorder-product'" />
      <OperatorSelect
        v-else-if="field.kind === 'operator'"
        id="create-form-operator"
        v-model="formState.operator_employee_id"
        :department="operatorDirectory.department.value"
        :employees="operatorDirectory.employees.value"
        :loading="operatorDirectory.loading.value"
        :unavailable-reason="operatorDirectory.unavailableReason.value"
        :retryable="operatorDirectory.retryable.value"
        :invalid="Boolean(formError && !formState.operator_employee_id)"
        :validation-error="formError && !formState.operator_employee_id ? '请选择本次操作人。' : ''"
        @load="operatorDirectory.load"
        @retry="operatorDirectory.load(true)"
      />
      <el-form-item v-else :label="field.label" :required="field.required">
        <el-select v-if="field.kind === 'select'" v-model="formState[field.key]" placeholder="请选择" clearable>
          <el-option v-for="option in field.options" :key="option.value" :label="option.label" :value="option.value" :disabled="option.disabled" />
        </el-select>
        <el-select v-else-if="field.kind === 'multi-select'" v-model="formState[field.key]" placeholder="请选择" multiple collapse-tags collapse-tags-tooltip>
          <el-option v-for="option in field.options" :key="option.value" :label="option.label" :value="option.value" />
        </el-select>
        <el-date-picker v-else-if="field.kind === 'date'" v-model="formState[field.key]" value-format="YYYY-MM-DD" type="date" placeholder="请选择日期" />
        <el-input v-else-if="field.kind === 'textarea'" v-model="formState[field.key]" type="textarea" :rows="3" />
        <el-input v-else-if="field.kind === 'workorder-quantity'" v-model="formState[field.key]" inputmode="decimal" placeholder="请输入计划数量，最多 4 位小数">
          <template #append>{{ workorderProductStock?.unit || '单位' }}</template>
        </el-input>
        <el-input v-else v-model="formState[field.key]" :type="field.kind === 'password' ? 'password' : 'text'" :show-password="field.kind === 'password'" />
      </el-form-item>
    </template>
    <div class="form-actions">
      <el-button @click="toggleCreateForm">取消</el-button>
      <el-button type="primary" native-type="submit" :loading="loading" :disabled="['warehouses', 'workorder'].includes(activeKey) && Boolean(operatorDirectory.unavailableReason.value)">保存</el-button>
    </div>
  </el-form>
</template>

<script setup lang="ts">
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import {useWorkorderContext} from '../../../composables/workorderContext'
import OperatorSelect from '../../ui/OperatorSelect.vue'
import WorkorderProductField from '../WorkorderProductField.vue'

const {
  formSchema, canWriteActive, showCreateForm, createItem, editingSupplier,
  createEntityTitle, formError, activeKey, canCreateDepartmentTerminalUser,
  formState, operatorDirectory, toggleCreateForm, loading,
} = useWorkspaceContext()
const {workorderProductStock} = useWorkorderContext().product
</script>
