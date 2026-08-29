import {computed, ref, type Ref} from 'vue'
import {ApiError, request} from '../api/http'
import type {OperatorEmployeesResponse} from '../types'

export function useOperatorEmployees(token: Ref<string>) {
  const data = ref<OperatorEmployeesResponse | null>(null)
  const loading = ref(false)
  const error = ref('')
  const retryable = ref(false)
  let generation = 0

  const department = computed(() => data.value?.department || null)
  const employees = computed(() => data.value?.employees || [])
  const unavailableReason = computed(() => {
    if (loading.value) return '正在加载当前部门员工…'
    if (error.value) return error.value
    if (!data.value) return '正在准备操作人；如持续不可用，请联系管理员检查账号部门归属。'
    if (!department.value) return '当前账号未绑定部门，请联系管理员修复账号归属。'
    if (!employees.value.length) return '当前部门没有可用的在职员工，请联系管理员维护部门成员。'
    return ''
  })

  async function load(force = false) {
    if (loading.value || (!force && data.value && !error.value)) return
    const current = ++generation
    loading.value = true
    error.value = ''
    retryable.value = false
    try {
      const response = await request<OperatorEmployeesResponse>('/api/v1/operator-employees', {}, token.value)
      if (current === generation) data.value = response
    } catch (cause) {
      if (current !== generation) return
      retryable.value = !(cause instanceof ApiError) || cause.status >= 500
      error.value = cause instanceof ApiError && cause.status === 403
        ? `${cause.message}；请联系管理员处理账号或部门配置。`
        : cause instanceof Error ? cause.message : '操作人加载失败，请重试。'
      data.value = null
    } finally {
      if (current === generation) loading.value = false
    }
  }

  function invalidate(message = '') {
    generation += 1
    data.value = null
    error.value = message
    retryable.value = false
    loading.value = false
  }

  function handleSubmitError(cause: unknown) {
    const operatorCodes = new Set(['OPERATOR_EMPLOYEE_STALE', 'OPERATOR_DEPARTMENT_STALE'])
    const operatorMessages = new Set([
      '当前账号部门已停用，不能执行业务写入',
      '操作员工不存在或关系已失效',
      '操作员工已停用',
      '操作员工已不属于当前部门',
    ])
    if (cause instanceof ApiError && (operatorCodes.has(cause.code) || (cause.status === 409 && operatorMessages.has(cause.message)))) {
      invalidate('所选员工已失效或不再属于当前部门，请重新加载后选择。')
      void load(true)
      return true
    }
    return false
  }

  return {data, department, employees, loading, error, retryable, unavailableReason, load, invalidate, handleSubmitError}
}
