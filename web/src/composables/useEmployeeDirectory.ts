import {computed, onBeforeUnmount, reactive, ref, type Ref} from 'vue'
import {request} from '../api/http'
import {ElMessage} from 'element-plus'
import type {DepartmentItem, EmployeeItem, PaginatedResponse} from '../types'

export type EmployeeFormValue = {
  name: string
  phone: string
  hire_date: string
  birthplace: string
  residential_address: string
  birth_date: string
}

const emptyEmployee = (): EmployeeFormValue => ({name: '', phone: '', hire_date: '', birthplace: '', residential_address: '', birth_date: ''})

export function useEmployeeDirectory(token: Ref<string>) {
  const employees = ref<EmployeeItem[]>([])
  const departments = ref<DepartmentItem[]>([])
  const departmentsLoading = ref(false)
  const departmentsError = ref('')
  const total = ref(0)
  const page = ref(1)
  const pageSize = ref(20)
  const keyword = ref('')
  const departmentID = ref<number | undefined>()
  const status = ref('active')
  const loading = ref(false)
  const error = ref('')
  const drawerVisible = ref(false)
  const drawerReadonly = ref(false)
  const editing = ref<EmployeeItem | null>(null)
  const saving = ref(false)
  const saveError = ref('')
  const form = reactive<EmployeeFormValue>(emptyEmployee())
  let listGeneration = 0
  let listAbortController: AbortController | null = null

  const title = computed(() => drawerReadonly.value ? '员工详情' : editing.value ? '编辑员工档案' : '新增员工')

  async function load() {
    listAbortController?.abort()
    const controller = new AbortController()
    listAbortController = controller
    const generation = ++listGeneration
    const snapshot = {page: page.value, pageSize: pageSize.value, keyword: keyword.value, departmentID: departmentID.value, status: status.value}
    loading.value = true
    error.value = ''
    try {
      const params = new URLSearchParams({page: String(snapshot.page), page_size: String(snapshot.pageSize)})
      if (snapshot.keyword) params.set('q', snapshot.keyword)
      if (snapshot.departmentID) params.set('department_id', String(snapshot.departmentID))
      if (snapshot.status) params.set('status', snapshot.status)
      const result = await request<PaginatedResponse<EmployeeItem>>(`/api/v1/system/employees?${params}`, {signal: controller.signal}, token.value)
      if (generation !== listGeneration || controller.signal.aborted || page.value !== snapshot.page || pageSize.value !== snapshot.pageSize || keyword.value !== snapshot.keyword || departmentID.value !== snapshot.departmentID || status.value !== snapshot.status) return
      employees.value = result.items || []
      total.value = result.total || 0
    } catch (cause) {
      if (generation !== listGeneration || controller.signal.aborted) return
      error.value = cause instanceof Error ? cause.message : '员工档案加载失败'
    } finally {
      if (generation === listGeneration) loading.value = false
      if (listAbortController === controller) listAbortController = null
    }
  }

  async function loadDepartments() {
    departmentsLoading.value = true
    departmentsError.value = ''
    try {
      const result = await request<PaginatedResponse<DepartmentItem> | DepartmentItem[]>('/api/v1/system/departments?page=1&page_size=200', {}, token.value)
      departments.value = Array.isArray(result) ? result : result.items || []
    } catch (cause) {
      departments.value = []
      departmentsError.value = cause instanceof Error ? cause.message : '部门选项加载失败'
    } finally {
      departmentsLoading.value = false
    }
  }

  function resetForm() { Object.assign(form, emptyEmployee()) }
  function fillForm(item: any) {
    editing.value = item
    Object.assign(form, {name: item.name, phone: item.phone || '', hire_date: item.hire_date, birthplace: item.birthplace || '', residential_address: item.residential_address || '', birth_date: item.birth_date})
    saveError.value = ''
    drawerVisible.value = true
  }
  function openCreate() { drawerReadonly.value = false; editing.value = null; resetForm(); saveError.value = ''; drawerVisible.value = true }
  function openEdit(item: any) { drawerReadonly.value = false; fillForm(item) }
  function openView(item: any) { drawerReadonly.value = true; fillForm(item) }

  function validate(): string {
    if (!form.name.trim() || !form.hire_date || !form.birth_date) return '请填写姓名、入职日期和出生日期。'
    if (form.birth_date > shanghaiDate()) return '出生日期不能晚于今天。'
    return ''
  }

  async function save() {
    saveError.value = validate()
    if (saveError.value || saving.value) return
    saving.value = true
    try {
      const body = {...form, name: form.name.trim(), phone: form.phone.trim(), birthplace: form.birthplace.trim(), residential_address: form.residential_address.trim()}
      const wasEditing = Boolean(editing.value)
      const saved = await request<EmployeeItem>(editing.value ? `/api/v1/system/employees/${editing.value.id}` : '/api/v1/system/employees', {method: editing.value ? 'PUT' : 'POST', body}, token.value)
      openView(saved)
      await load()
      ElMessage.success(wasEditing ? '员工档案已更新' : '员工档案已新增')
    } catch (cause) {
      saveError.value = cause instanceof Error ? cause.message : '员工档案保存失败'
    } finally {
      saving.value = false
    }
  }

  async function setStatus(item: any, nextStatus: 'active' | 'disabled') {
    await request(`/api/v1/system/employees/${item.id}/status`, {method: 'PATCH', body: {status: nextStatus}}, token.value)
    await load()
  }

  function applySearch() { page.value = 1; void load() }

  onBeforeUnmount(() => { listAbortController?.abort(); listGeneration += 1 })

  return {employees, departments, departmentsLoading, departmentsError, total, page, pageSize, keyword, departmentID, status, loading, error, drawerVisible, drawerReadonly, editing, saving, saveError, form, title, load, loadDepartments, openCreate, openEdit, openView, save, setStatus, applySearch}
}

function shanghaiDate(): string {
  const parts = new Intl.DateTimeFormat('en-CA', {timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit'}).formatToParts(new Date())
  const value = Object.fromEntries(parts.map((part) => [part.type, part.value]))
  return `${value.year}-${value.month}-${value.day}`
}
