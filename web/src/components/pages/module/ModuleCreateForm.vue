<template>
  <ResponsiveDetailCarrier v-if="formSchema.length" :model-value="showCreateForm" :docked="docked" :size="size" :title="panelTitle" :before-close="closeForm" :close-on-click-modal="!panelBusy" :close-on-press-escape="!panelBusy" :docked-auto-focus="savedItem ? 'preserve' : 'first-editable'" destroy-on-close @update:model-value="requestClose">
  <section v-if="savedItem" class="module-saved-detail" :aria-label="`${savedDetailTitle}内容`">
    <div class="module-saved-detail__heading"><span>{{ createEntityTitle }}</span><h3>{{ savedDetailPrimary }}</h3><p v-if="savedDetailSubtitle">{{ savedDetailSubtitle }}</p></div>
    <PropertyList>
      <PropertyItem v-for="field in savedDetailFields" :key="field.key" :label="field.label"><span :class="{'module-saved-detail__mono': field.mono}">{{ field.value }}</span></PropertyItem>
    </PropertyList>
  </section>
  <template v-else>
    <WorkorderTemporaryProductStep v-if="temporaryProductDialogVisible" />
    <el-form v-else id="module-editor" class="module-editor" label-position="top" :disabled="loading || Boolean(moduleUnavailable) || !canWriteActive" @submit.prevent="submitForm">
      <FormPanelContent>
        <el-alert v-if="moduleUnavailable" :title="moduleUnavailable.message || '此功能暂不可用，当前无法保存'" type="warning" :closable="false" show-icon />
        <el-alert v-else-if="!canWriteActive" title="当前账号没有该功能的写入权限，表单仅保留当前内容。" type="warning" :closable="false" show-icon />
        <el-alert v-if="formError" id="workorder-create-error" :title="formError" type="error" :closable="false" show-icon />
        <el-alert v-if="activeKey === 'users' && !canCreateDepartmentTerminalUser" title="当前权限只能创建个人账号" description="创建部门终端账号还需要部门查看和终端查看权限。" type="info" :closable="false" show-icon />
        <FormSection v-for="group in formGroups" :key="group.key" :title="group.title" :description="group.description">
          <FormGrid :columns="group.columns">
            <template v-for="field in group.fields" :key="field.key">
              <WorkorderProductField v-if="field.kind === 'workorder-product'" class="form-grid-full" />
              <OperatorSelect
                v-else-if="field.kind === 'operator'"
                id="create-form-operator"
                class="form-grid-full"
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
              <el-form-item v-else :class="{'form-grid-full': field.kind === 'textarea'}" :label="field.label" :required="field.required">
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
          </FormGrid>
        </FormSection>
      </FormPanelContent>
    </el-form>
  </template>
    <template #footer>
      <template v-if="savedItem">
        <el-button @click="toggleCreateForm">关闭</el-button>
        <el-button v-if="activeKey === 'suppliers' && canWriteActive" type="primary" plain @click="editSavedSupplier">编辑资料</el-button>
        <el-button v-if="hasAssignmentAction" type="primary" plain :disabled="assignmentTargetDisabled(savedItem)" :title="assignmentTargetHint(savedItem)" @click="openSavedAssignment">{{ assignmentConfigs[activeKey]?.buttonLabel }}</el-button>
        <el-button v-if="activeKey === 'users' && canWriteActive" type="primary" plain :disabled="!canEditUserAffiliation" @click="openSavedAffiliation">账号归属</el-button>
      </template>
      <template v-else-if="temporaryProductDialogVisible">
        <el-button :disabled="temporaryProductSubmitting" @click="closeTemporaryProductWithGuard()">返回任务单</el-button>
        <el-button type="primary" native-type="submit" form="temporary-product-form" :loading="temporaryProductSubmitting" :disabled="!temporaryProductForm.operator_employee_id || Boolean(operatorDirectory.unavailableReason.value)">保存并选择</el-button>
      </template>
      <template v-else>
        <el-button @click="toggleCreateForm">取消</el-button>
        <el-button type="primary" form="module-editor" native-type="submit" :loading="loading" :disabled="Boolean(moduleUnavailable) || !canWriteActive || (['warehouses', 'workorder'].includes(activeKey) && Boolean(operatorDirectory.unavailableReason.value))">保存</el-button>
      </template>
    </template>
  </ResponsiveDetailCarrier>
</template>

<script setup lang="ts">
import {computed, nextTick, ref, watch} from 'vue'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import {useWorkorderContext} from '../../../composables/workorderContext'
import type {FormField} from '../../../composables/useModuleConfiguration'
import OperatorSelect from '../../ui/OperatorSelect.vue'
import WorkorderProductField from '../WorkorderProductField.vue'
import WorkorderTemporaryProductStep from '../WorkorderTemporaryProductStep.vue'
import ResponsiveDetailCarrier from '../../ui/ResponsiveDetailCarrier.vue'
import {useResponsiveDetailPanel} from '../../../composables/useResponsiveDetailPanel'
import type {BasicItem} from '../../../types'
import PropertyItem from '../../ui/PropertyItem.vue'
import PropertyList from '../../ui/PropertyList.vue'
import FormPanelContent from '../../ui/FormPanelContent.vue'
import FormSection from '../../ui/FormSection.vue'
import FormGrid from '../../ui/FormGrid.vue'

type SavedDetailField = {key: string; label: string; value: string; mono?: boolean}
type FormFieldGroup = {key: string; title: string; description: string; columns: 'one' | 'two'; fields: FormField[]}

const {
  formSchema, columns, canWriteActive, showCreateForm, createItem, editingSupplier, editSupplier, rows,
  createEntityTitle, formError, activeKey, canCreateDepartmentTerminalUser,
  formState, operatorDirectory, toggleCreateForm, loading, moduleUnavailable,
  columnLabel, formatGenericCell, genericStatusLabel, hasAssignmentAction, assignmentConfigs,
  openAssignment, canEditUserAffiliation, openUserAffiliation, assignmentTargetDisabled, assignmentTargetHint,
} = useWorkspaceContext()
const {workorderProductStock, temporaryProductDialogVisible, temporaryProductSubmitting, temporaryProductForm, closeTemporaryProductWithGuard} = useWorkorderContext().product
const savedItem = ref<BasicItem | null>(null)
const {docked, size} = useResponsiveDetailPanel(showCreateForm, computed(() => !savedItem.value ? {complexity: 'standard-form' as const} : {complexity: 'detail' as const}))
const savedDetailTitle = computed(() => `${createEntityTitle.value}详情`)
const panelTitle = computed(() => temporaryProductDialogVisible.value ? '临时添加产品档案' : savedItem.value ? savedDetailTitle.value : editingSupplier.value ? '编辑供应商' : `新增${createEntityTitle.value}`)
const panelBusy = computed(() => loading.value || temporaryProductSubmitting.value)
let workorderFormScrollTop = 0
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

const visibleFormFields = computed(() => formSchema.value.filter((field) => {
  if (activeKey.value !== 'users') return true
  return formState.account_type === 'department_terminal' || !['department_id', 'terminal_id'].includes(field.key)
}))

function fieldGroup(key: string, title: string, description: string, fields: FormField[], columns: 'one' | 'two' = 'two'): FormFieldGroup {
  return {key, title, description, fields, columns}
}

const formGroups = computed<FormFieldGroup[]>(() => {
  const fields = visibleFormFields.value
  const byKeys = (keys: string[]) => fields.filter((field) => keys.includes(field.key))
  const remaining = (known: string[]) => fields.filter((field) => !known.includes(field.key))
  switch (activeKey.value) {
    case 'users': {
      const account = byKeys(['username', 'password', 'account_type', 'name'])
      const affiliation = byKeys(['department_id', 'terminal_id'])
      return [
        fieldGroup('account', '账号信息', '用于登录和识别操作者的基础信息。', account),
        ...(!affiliation.length ? [] : [fieldGroup('affiliation', '归属信息', '部门终端账号需要明确的部门与终端归属。', affiliation, 'one')]),
      ]
    }
    case 'terminals':
      return [fieldGroup('terminal', '终端信息', '终端编码、名称和位置用于识别现场设备。', fields)]
    case 'roles':
      return [fieldGroup('role', '角色信息', '角色名称和编码用于权限分配与审计识别。', fields)]
    case 'suppliers':
      return [fieldGroup('supplier', '供应商资料', '维护供应商的基本联系信息。', fields)]
    case 'warehouses': {
      const operator = byKeys(['operator_employee_id'])
      return [
        fieldGroup('item', '物品信息', '名称、编码和规格用于库存识别。', fields.filter((field) => field.key !== 'operator_employee_id')),
        ...(!operator.length ? [] : [fieldGroup('operator', '操作记录', '每次库存资料写入都需要记录实际操作人。', operator, 'one')]),
      ]
    }
    case 'workorder': {
      const basic = byKeys(['type', 'code', 'title', 'customer_id'])
      const product = byKeys(['product_id', 'planned_quantity'])
      const routing = byKeys(['due_at', 'priority', 'target_department_ids'])
      const notes = byKeys(['description'])
      const operator = byKeys(['operator_employee_id'])
      return [
        fieldGroup('basic', '任务信息', '先确定任务类型和业务对象，再填写执行信息。', basic),
        ...(!product.length ? [] : [fieldGroup('product', '产品与数量', '生产单需要选择产品并填写计划数量。', product, 'one')]),
        ...(!routing.length ? [] : [fieldGroup('routing', '流转安排', '设置交期、优先级和需要处理的部门。', routing)]),
        ...(!notes.length ? [] : [fieldGroup('notes', '任务说明', '补充现场执行需要关注的说明。', notes, 'one')]),
        ...(!operator.length ? [] : [fieldGroup('operator', '操作记录', '记录创建本任务的实际操作人。', operator, 'one')]),
      ]
    }
    default:
      return [fieldGroup('default', '基本信息', '填写该资料的业务信息。', remaining([]))]
  }
})

watch(() => formState.account_type, (accountType) => {
  if (activeKey.value !== 'users' || accountType === 'department_terminal') return
  delete formState.department_id
  delete formState.terminal_id
})

watch([showCreateForm, activeKey], ([open, key], [wasOpen, previousKey]) => {
  if (!open || key !== previousKey || (!wasOpen && open)) savedItem.value = null
})

watch(temporaryProductDialogVisible, async (open, wasOpen) => {
  const body = document.querySelector<HTMLElement>('.workspace-detail-aside .detail-body')
  if (open) workorderFormScrollTop = body?.scrollTop || 0
  await nextTick()
  if (open) {
    document.querySelector<HTMLElement>('#temporary-product-name input, #temporary-product-name')?.focus({preventScroll: true})
    return
  }
  if (!wasOpen) return
  const restoredBody = document.querySelector<HTMLElement>('.workspace-detail-aside .detail-body')
  if (restoredBody) restoredBody.scrollTop = workorderFormScrollTop
  document.querySelector<HTMLElement>('.workorder-temporary-product-trigger, #workorder-product-select input, #workorder-product-select')?.focus({preventScroll: true})
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

async function closeForm(done: () => void) {
  if (temporaryProductDialogVisible.value) {
    let allowed = false
    await closeTemporaryProductWithGuard(() => {
      allowed = true
      temporaryProductDialogVisible.value = false
    })
    if (!allowed) return
  }
  await toggleCreateForm()
  if (!showCreateForm.value) done()
}
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
