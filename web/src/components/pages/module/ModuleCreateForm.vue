<template>
  <ResponsiveDetailCarrier v-if="formSchema.length" :model-value="showCreateForm" :docked="docked" :size="size" :title="savedItem ? savedDetailTitle : editingSupplier ? '编辑供应商' : `新增${createEntityTitle}`" :before-close="closeForm" :close-on-click-modal="!loading" :close-on-press-escape="!loading" :docked-auto-focus="savedItem ? 'preserve' : 'first-editable'" destroy-on-close @update:model-value="requestClose">
  <section v-if="savedItem" class="module-saved-detail" :aria-label="`${savedDetailTitle}内容`">
    <div class="module-saved-detail__heading"><span>{{ createEntityTitle }}</span><h3>{{ savedDetailPrimary }}</h3><p v-if="savedDetailSubtitle">{{ savedDetailSubtitle }}</p></div>
    <PropertyList>
      <PropertyItem v-for="field in savedDetailFields" :key="field.key" :label="field.label"><span :class="{'module-saved-detail__mono': field.mono}">{{ field.value }}</span></PropertyItem>
    </PropertyList>
  </section>
  <template v-else>
    <el-alert v-if="moduleUnavailable" :title="moduleUnavailable.message || '此功能暂不可用，当前无法保存'" type="warning" :closable="false" show-icon />
    <el-alert v-else-if="!canWriteActive" title="当前账号没有该功能的写入权限，表单仅保留当前内容。" type="warning" :closable="false" show-icon />
    <el-form id="module-editor" class="module-editor" label-position="top" :disabled="loading || Boolean(moduleUnavailable) || !canWriteActive" @submit.prevent="submitForm">
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
    </el-form>
  </template>
    <template #footer>
      <template v-if="savedItem">
        <el-button @click="toggleCreateForm">关闭</el-button>
        <el-button v-if="activeKey === 'suppliers' && canWriteActive" type="primary" plain @click="editSavedSupplier">编辑资料</el-button>
        <el-button v-if="hasAssignmentAction" type="primary" plain :disabled="assignmentTargetDisabled(savedItem)" :title="assignmentTargetHint(savedItem)" @click="openSavedAssignment">{{ assignmentConfigs[activeKey]?.buttonLabel }}</el-button>
        <el-button v-if="activeKey === 'users' && canWriteActive" type="primary" plain :disabled="!canEditUserAffiliation" @click="openSavedAffiliation">账号归属</el-button>
      </template>
      <template v-else>
        <el-button @click="toggleCreateForm">取消</el-button>
        <el-button type="primary" form="module-editor" native-type="submit" :loading="loading" :disabled="Boolean(moduleUnavailable) || !canWriteActive || (['warehouses', 'workorder'].includes(activeKey) && Boolean(operatorDirectory.unavailableReason.value))">保存</el-button>
      </template>
    </template>
  </ResponsiveDetailCarrier>
</template>

<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import {useWorkorderContext} from '../../../composables/workorderContext'
import OperatorSelect from '../../ui/OperatorSelect.vue'
import WorkorderProductField from '../WorkorderProductField.vue'
import ResponsiveDetailCarrier from '../../ui/ResponsiveDetailCarrier.vue'
import {useResponsiveDetailPanel} from '../../../composables/useResponsiveDetailPanel'
import type {BasicItem} from '../../../types'
import PropertyItem from '../../ui/PropertyItem.vue'
import PropertyList from '../../ui/PropertyList.vue'

type SavedDetailField = {key: string; label: string; value: string; mono?: boolean}

const {
  formSchema, columns, canWriteActive, showCreateForm, createItem, editingSupplier, editSupplier, rows,
  createEntityTitle, formError, activeKey, canCreateDepartmentTerminalUser,
  formState, operatorDirectory, toggleCreateForm, loading, moduleUnavailable,
  columnLabel, formatGenericCell, genericStatusLabel, hasAssignmentAction, assignmentConfigs,
  openAssignment, canEditUserAffiliation, openUserAffiliation, assignmentTargetDisabled, assignmentTargetHint,
} = useWorkspaceContext()
const {workorderProductStock} = useWorkorderContext().product
const savedItem = ref<BasicItem | null>(null)
const {docked, size} = useResponsiveDetailPanel(showCreateForm, computed(() => !savedItem.value))
const savedDetailTitle = computed(() => `${createEntityTitle.value}详情`)
const savedDetailPrimary = computed(() => {
  const item = savedItem.value
  return String(item?.name || item?.username || item?.code || `${createEntityTitle.value} #${item?.id || ''}`)
})
const savedDetailSubtitle = computed(() => {
  const item = savedItem.value
  if (!item) return ''
  const code = item.code ? `编码：${item.code}` : ''
  const id = item.id ? `记录 #${item.id}` : ''
  return [code, id].filter(Boolean).join(' · ')
})
const savedDetailFields = computed<SavedDetailField[]>(() => {
  const item = savedItem.value
  if (!item) return []
  const detailKeysByModule: Record<string, string[]> = {
    departments: ['id', 'name', 'code', 'employee_count', 'status'],
    terminals: ['id', 'department_id', 'code', 'name', 'location', 'status'],
    users: ['id', 'username', 'account_type', 'name', 'organization_id', 'department_id', 'terminal_id', 'status'],
    roles: ['id', 'name', 'code', 'description', 'system'],
    suppliers: ['id', 'name', 'code', 'contact', 'phone', 'address', 'status'],
    warehouses: ['id', 'name', 'code', 'unit', 'spec', 'safety_stock', 'default_cost', 'status'],
    workorder: ['id', 'code', 'type', 'title', 'customer_id', 'planned_quantity', 'due_at', 'priority', 'description', 'status'],
  }
  const schemaKeys = formSchema.value.map((field) => field.key)
  const configuredKeys = detailKeysByModule[activeKey.value] || [...columns.value, ...schemaKeys]
  const keys = [...new Set(['id', ...configuredKeys, ...schemaKeys])]
    .filter((key) => key in item && !key.toLowerCase().includes('password') && !key.toLowerCase().endsWith('_hash') && !Array.isArray(item[key]) && !(key === 'default_cost' && !columns.value.includes('default_cost')))
  return keys.map((key) => ({
    key,
    label: columnLabel(key),
    value: (() => {
      const formatted = key === 'status' ? genericStatusLabel(item[key]) : formatGenericCell(key, item[key])
      return formatted === '-' ? (key === 'status' ? '未设置' : '未填写') : formatted
    })(),
    mono: key === 'id' || key.endsWith('_id') || ['code', 'username'].includes(key),
  }))
})

watch([showCreateForm, activeKey], ([open, key], [wasOpen, previousKey]) => {
  if (!open || key !== previousKey || (!wasOpen && open)) savedItem.value = null
})

async function submitForm() {
  const saved = await createItem()
  if (!saved) return
  savedItem.value = rows.value.find((item) => Number(item.id) === Number(saved.id)) || saved
}

async function openSavedAssignment() {
  if (!savedItem.value) return
  const target = savedItem.value
  await toggleCreateForm()
  if (!showCreateForm.value) await openAssignment(target)
}

async function openSavedAffiliation() {
  if (!savedItem.value) return
  const target = savedItem.value
  await toggleCreateForm()
  if (!showCreateForm.value) openUserAffiliation(target)
}

function editSavedSupplier() {
  if (!savedItem.value) return
  const target = savedItem.value
  savedItem.value = null
  editSupplier(target)
}

async function closeForm(done: () => void) { await toggleCreateForm(); if (!showCreateForm.value) done() }
async function requestClose(open: boolean) { if (!open && showCreateForm.value) await toggleCreateForm() }
</script>

<style scoped>
.module-saved-detail { display: grid; min-width: 0; gap: var(--bb-space-5); }
.module-saved-detail__heading { min-width: 0; }
.module-saved-detail__heading > span { color: var(--bb-accent-text); font-size: var(--bb-font-size-13); font-weight: var(--bb-font-weight-semibold); }
.module-saved-detail__heading h3 { margin: var(--bb-space-1) 0 0; color: var(--bb-text-primary); font-size: var(--bb-font-size-20); line-height: var(--bb-line-height-tight); overflow-wrap: anywhere; }
.module-saved-detail__heading p { margin: var(--bb-space-2) 0 0; color: var(--bb-text-secondary); line-height: var(--bb-line-height-relaxed); overflow-wrap: anywhere; }
.module-saved-detail__mono { font-family: var(--bb-font-mono); overflow-wrap: anywhere; }
</style>
