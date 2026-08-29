<template>
  <div class="operator-field">
    <div class="operator-field__heading">
      <label :for="inputId"><strong>本次操作人 <span aria-hidden="true">*</span></strong></label>
      <small v-if="department">当前部门：{{ department.name }}</small>
    </div>
    <el-select
      ref="selectRef"
      :id="inputId"
      :model-value="modelValue"
      filterable
      clearable
      :loading="loading"
      :disabled="Boolean(unavailableReason)"
      placeholder="请选择本次实际操作员工"
      aria-label="本次操作人"
      :aria-required="required"
      :aria-invalid="invalid"
      :aria-describedby="`${inputId}-help${validationError ? ` ${inputId}-error` : ''}`"
      @update:model-value="$emit('update:modelValue', Number($event) || undefined)"
      @visible-change="handleVisible"
    >
      <el-option
        v-for="employee in employees"
        :key="employee.id"
        :label="employeeLabel(employee)"
        :value="employee.id"
      />
    </el-select>
    <div :id="`${inputId}-help`" class="operator-field__help" aria-live="polite">
      <span v-if="!unavailableReason">每次提交都需重新选择，系统不会自动记忆。</span>
      <span v-else>{{ unavailableReason }}</span>
      <el-button v-if="retryable && !loading" link type="primary" @click="$emit('retry')">重新加载</el-button>
    </div>
    <small v-if="validationError" :id="`${inputId}-error`" class="operator-field__error" role="alert">{{ validationError }}</small>
  </div>
</template>

<script setup lang="ts">
import {computed, nextTick, onMounted, onUpdated, ref, useId, watch} from 'vue'
import type {SelectInstance} from 'element-plus'
import type {DepartmentSummary, OperatorEmployee} from '../../types'

const props = withDefaults(defineProps<{
  modelValue?: number
  department?: DepartmentSummary | null
  employees: OperatorEmployee[]
  loading?: boolean
  unavailableReason?: string
  retryable?: boolean
  id?: string
  required?: boolean
  invalid?: boolean
  validationError?: string
}>(), {loading: false, unavailableReason: '', retryable: false, id: '', required: true, invalid: false, validationError: ''})

const emit = defineEmits<{(event: 'update:modelValue', value: number | undefined): void; (event: 'retry'): void; (event: 'load'): void}>()
const generatedId = useId()
const inputId = computed(() => props.id || `operator-employee-${generatedId}`)
const selectRef = ref<SelectInstance>()

function comboboxInput(): HTMLInputElement | null {
  const root = (selectRef.value as unknown as {$el?: HTMLElement} | undefined)?.$el
  return root?.querySelector<HTMLInputElement>('input[role="combobox"]') || null
}

function syncComboboxAria() {
  const input = comboboxInput()
  if (!input) return
  input.setAttribute('aria-label', '本次操作人')
  input.setAttribute('aria-required', String(props.required))
  input.setAttribute('aria-invalid', String(props.invalid))
  input.setAttribute('aria-describedby', `${inputId.value}-help${props.validationError ? ` ${inputId.value}-error` : ''}`)
}

function focus() {
  selectRef.value?.focus()
  comboboxInput()?.focus()
}

watch(
  () => [inputId.value, props.required, props.invalid, props.validationError],
  () => void nextTick(syncComboboxAria),
  {immediate: true, flush: 'post'},
)
onMounted(syncComboboxAria)
onUpdated(syncComboboxAria)
defineExpose({focus})

function employeeLabel(employee: OperatorEmployee): string {
  const duplicated = props.employees.filter((item) => item.name === employee.name).length > 1
  return duplicated ? `${employee.name}（#${employee.id}）` : employee.name
}

function handleVisible(visible: boolean) {
  if (visible) emit('load')
}

</script>
