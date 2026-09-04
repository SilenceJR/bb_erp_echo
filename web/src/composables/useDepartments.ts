import {reactive, ref, type Ref} from 'vue'
import {request} from '../api/http'
import {ElMessage} from 'element-plus'
import type {DepartmentItem, EmployeeItem, PaginatedResponse} from '../types'

export function useDepartments(token: Ref<string>) {
  const departments = ref<DepartmentItem[]>([])
  const employees = ref<EmployeeItem[]>([])
  const loading = ref(false)
  const error = ref('')
  const formVisible = ref(false)
  const memberVisible = ref(false)
  const editing = ref<DepartmentItem | null>(null)
  const formReadonly = ref(false)
  const memberDepartment = ref<DepartmentItem | null>(null)
  const form = reactive({name: '', code: ''})
  const selectedEmployeeIDs = ref<number[]>([])
  const originalEmployeeIDs = ref<number[]>([])
  const memberKeyword = ref('')
  const saving = ref(false)
  const saveError = ref('')
  const memberLoading = ref(false)
  const memberLoadError = ref('')
  let memberGeneration = 0
  let memberAbortController: AbortController | null = null

  async function load() {
    loading.value = true; error.value = ''
    try {
      const result = await request<PaginatedResponse<DepartmentItem> | DepartmentItem[]>('/api/v1/system/departments?page=1&page_size=200', {}, token.value)
      departments.value = Array.isArray(result) ? result : result.items || []
    } catch (cause) { error.value = cause instanceof Error ? cause.message : '部门加载失败' } finally { loading.value = false }
  }
  function openCreate() { editing.value = null; formReadonly.value = false; form.name = ''; form.code = ''; saveError.value = ''; formVisible.value = true }
  function openEdit(item: any) { editing.value = item; formReadonly.value = false; form.name = item.name; form.code = item.code || ''; saveError.value = ''; formVisible.value = true }
  function openView(item: DepartmentItem) { editing.value = item; formReadonly.value = true; form.name = item.name; form.code = item.code || ''; saveError.value = ''; formVisible.value = true }
  async function save() {
    if (!form.name.trim() || !form.code.trim()) { saveError.value = '请填写部门名称和编码。'; return }
    saving.value = true; saveError.value = ''
    try {
      const wasEditing = Boolean(editing.value)
      const saved = await request<DepartmentItem>(editing.value ? `/api/v1/system/departments/${editing.value.id}` : '/api/v1/system/departments', {method: editing.value ? 'PUT' : 'POST', body: {name: form.name.trim(), code: form.code.trim()}}, token.value)
      editing.value = saved
      form.name = saved.name
      form.code = saved.code || ''
      formReadonly.value = true
      await load()
      ElMessage.success(wasEditing ? '部门已更新' : '部门已新增')
    }
    catch (cause) { saveError.value = cause instanceof Error ? cause.message : '部门保存失败' } finally { saving.value = false }
  }
  async function setStatus(item: any, status: 'active' | 'disabled') { await request(`/api/v1/system/departments/${item.id}/status`, {method: 'PATCH', body: {status}}, token.value); await load() }
  async function loadAllEmployees(signal: AbortSignal): Promise<EmployeeItem[]> {
    const items: EmployeeItem[] = []
    let currentPage = 1
    let total = Number.POSITIVE_INFINITY
    while (items.length < total && currentPage <= 100) {
      const result = await request<PaginatedResponse<EmployeeItem>>(`/api/v1/system/employees?page=${currentPage}&page_size=200&status=`, {signal}, token.value)
      items.push(...(result.items || []))
      total = result.total || items.length
      if (!result.items?.length) break
      currentPage += 1
    }
    if (items.length < total) throw new Error('员工数量超过安全加载上限，请缩小管理范围。')
    return items
  }
  async function openMembers(item: any) {
    memberDepartment.value = item; memberVisible.value = true; selectedEmployeeIDs.value = []; originalEmployeeIDs.value = []; employees.value = []; memberKeyword.value = ''; saveError.value = ''
    await loadMembers(item)
  }
  async function loadMembers(item = memberDepartment.value) {
    if (!item) return
    memberAbortController?.abort()
    const controller = new AbortController()
    memberAbortController = controller
    const generation = ++memberGeneration
    const departmentID = Number(item.id)
    memberLoading.value = true
    memberLoadError.value = ''
    try {
      const [allEmployees, current] = await Promise.all([
        loadAllEmployees(controller.signal),
        request<{employees?: EmployeeItem[]; items?: EmployeeItem[]; employee_ids?: number[]} | EmployeeItem[]>(`/api/v1/system/departments/${departmentID}/employees`, {signal: controller.signal}, token.value),
      ])
      if (generation !== memberGeneration || Number(memberDepartment.value?.id) !== departmentID) return
      employees.value = allEmployees
      if (Array.isArray(current)) selectedEmployeeIDs.value = current.map((employee) => employee.id)
      else selectedEmployeeIDs.value = current.employee_ids || current.employees?.map((employee) => employee.id) || current.items?.map((employee) => employee.id) || []
      originalEmployeeIDs.value = [...selectedEmployeeIDs.value]
    } catch (cause) {
      if (generation !== memberGeneration || controller.signal.aborted || Number(memberDepartment.value?.id) !== departmentID) return
      memberLoadError.value = cause instanceof Error ? cause.message : '部门成员加载失败'
    } finally {
      if (generation === memberGeneration) memberLoading.value = false
      if (memberAbortController === controller) memberAbortController = null
    }
  }
  async function saveMembers() {
    if (!memberDepartment.value || saving.value) return
    saving.value = true; saveError.value = ''
    try { await request(`/api/v1/system/departments/${memberDepartment.value.id}/employees`, {method: 'PUT', body: {employee_ids: selectedEmployeeIDs.value}}, token.value); memberVisible.value = false; await load(); ElMessage.success('部门成员已保存') }
    catch (cause) { saveError.value = cause instanceof Error ? cause.message : '部门成员保存失败' } finally { saving.value = false }
  }
  return {departments, employees, loading, error, formVisible, formReadonly, memberVisible, editing, memberDepartment, form, selectedEmployeeIDs, originalEmployeeIDs, memberKeyword, saving, saveError, memberLoading, memberLoadError, load, openCreate, openEdit, openView, save, setStatus, openMembers, loadMembers, saveMembers}
}
