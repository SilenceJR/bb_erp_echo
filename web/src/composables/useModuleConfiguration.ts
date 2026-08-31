import {computed, type ComputedRef, type Ref} from 'vue'
import type {ModuleItem} from '../data/modules'
import type {BasicItem} from '../types'

export type FormField = {
  key: string
  label: string
  kind?: 'text' | 'password' | 'select' | 'multi-select' | 'textarea' | 'date' | 'workorder-product' | 'workorder-quantity' | 'operator'
  options?: Array<{label: string; value: string | number; disabled?: boolean}>
  required?: boolean
}

export type MovementDefinition = {
  key: string
  title: string
  requiredAny?: string[]
  requiredAll?: string[]
}

export const warehouseTabs = [
  {key: 'product', title: '产品'},
  {key: 'production_material', title: '生产物资'},
  {key: 'regular_product', title: '常规产品'},
  {key: 'daily_supply', title: '生活物资'},
]

export const warehouseTabOptions = warehouseTabs.map((item) => ({label: item.title, value: item.key}))
export const movementDefinitions: MovementDefinition[] = [
  {key: 'purchase_inbound', title: '采购入库', requiredAll: ['suppliers:read']},
  {key: 'return_rework_inbound', title: '退货返工', requiredAny: ['customers:read', 'system:departments:read']},
  {key: 'customer_outbound', title: '客户出库', requiredAll: ['customers:read']},
  {key: 'department_outbound', title: '部门出库', requiredAll: ['system:departments:read']},
]
export const workorderStatusOptions = [
  {label: '全部状态', value: ''}, {label: '草稿', value: 'draft'}, {label: '正在处理', value: 'processing'},
  {label: '暂停', value: 'paused'}, {label: '待办公室确认', value: 'pending_close'}, {label: '正常完成', value: 'completed_normal'},
  {label: '强制完成', value: 'completed_forced'}, {label: '取消', value: 'cancelled'},
]
export const workorderTypeOptions = [{label: '全部类型', value: ''}, {label: '生产单', value: 'production'}, {label: '通用任务', value: 'general'}]
export const workorderPriorityOptions = [{label: '全部优先级', value: ''}, {label: '普通', value: 'normal'}, {label: '加急', value: 'urgent'}]

type ConfigurationDependencies = {
  activeKey: Ref<string>
  activeWarehouseTab: Ref<string>
  activeModule: ComputedRef<ModuleItem | undefined>
  formState: Record<string, any>
  canCreateDepartmentTerminalUser: ComputedRef<boolean>
  rowsFor: (key: string) => BasicItem[]
  hasPermission: (code?: string) => boolean
}

export function useModuleConfiguration(deps: ConfigurationDependencies) {
  const activeWarehouseTabTitle = computed(() => warehouseTabs.find((tab) => tab.key === deps.activeWarehouseTab.value)?.title || '物品')
  const createEntityTitle = computed(() => deps.activeKey.value === 'warehouses' ? '物品' : (deps.activeModule.value?.title || ''))
  const formSchema = computed<FormField[]>(() => {
    const departmentOptions = deps.rowsFor('departments').map((item) => ({label: item.name || item.code || `#${item.id}`, value: item.id}))
    const selectedDepartmentID = Number(deps.formState.department_id || 0)
    const terminalOptions = deps.rowsFor('terminals')
      .filter((item) => selectedDepartmentID > 0 && Number(item.department_id) === selectedDepartmentID)
      .map((item) => ({label: item.name || item.code || `#${item.id}`, value: item.id}))
    switch (deps.activeKey.value) {
      case 'departments': return [{key: 'name', label: '部门名称', required: true}, {key: 'code', label: '部门编码', required: true}]
      case 'terminals': return [
        {key: 'department_id', label: '所属部门', kind: 'select', options: departmentOptions, required: true},
        {key: 'code', label: '终端编码', required: true}, {key: 'name', label: '终端名称', required: true}, {key: 'location', label: '位置说明'},
      ]
      case 'users': return [
        {key: 'username', label: '账号', required: true}, {key: 'password', label: '密码', kind: 'password', required: true},
        {key: 'account_type', label: '账号类型', kind: 'select', required: true, options: [
          {label: '个人账号', value: 'personal'},
          {label: deps.canCreateDepartmentTerminalUser.value ? '部门终端账号' : '部门终端账号（需要部门和终端查看权限）', value: 'department_terminal', disabled: !deps.canCreateDepartmentTerminalUser.value},
        ]},
        {key: 'name', label: '姓名/终端名', required: true},
        {key: 'department_id', label: '所属部门', kind: 'select', options: departmentOptions, required: deps.formState.account_type === 'department_terminal'},
        {key: 'terminal_id', label: '所属终端', kind: 'select', options: terminalOptions, required: deps.formState.account_type === 'department_terminal'},
      ]
      case 'roles': return [{key: 'name', label: '角色名称', required: true}, {key: 'code', label: '角色编码', required: true}, {key: 'description', label: '说明'}]
      case 'suppliers': return [{key: 'name', label: '供应商名称'}, {key: 'code', label: '供应商编码'}, {key: 'contact', label: '联系人'}, {key: 'phone', label: '联系电话'}, {key: 'address', label: '地址'}]
      case 'warehouses': return [
        {key: 'name', label: `${activeWarehouseTabTitle.value}名称`}, {key: 'code', label: `${activeWarehouseTabTitle.value}编码`},
        {key: 'unit', label: '单位'}, {key: 'spec', label: '规格'}, {key: 'safety_stock', label: '安全库存'},
        ...(deps.hasPermission('cost:view') ? [{key: 'default_cost', label: '默认成本（元）'}] : []),
        {key: 'operator_employee_id', label: '本次操作人', kind: 'operator', required: true},
      ]
      case 'molds': return [
        {key: 'code', label: '模具编号'}, {key: 'name', label: '模具名称'}, {key: 'customer_id', label: '客户ID'},
        {key: 'product_id', label: '产品ID'}, {key: 'cavity_count', label: '穴数'}, {key: 'mold_material', label: '成型材料'},
        {key: 'steel', label: '钢材'}, {key: 'size', label: '尺寸'}, {key: 'weight_gram', label: '重量g'},
        {key: 'manufacturer', label: '制造商'}, {key: 'storage_location', label: '存放位置'}, {key: 'maintenance_cycle_days', label: '保养周期天'},
      ]
      case 'workorder': return [
        {key: 'type', label: '任务类型', kind: 'select', options: [{label: '生产单', value: 'production'}, {label: '通用任务', value: 'general'}], required: true},
        {key: 'code', label: '任务编号'}, {key: 'title', label: '标题', required: deps.formState.type === 'general'},
        {key: 'customer_id', label: '客户', kind: 'select', options: deps.rowsFor('customers').map((item) => ({label: item.name || item.code || `#${item.id}`, value: item.id}))},
        ...(deps.formState.type === 'production' ? [{key: 'product_id', label: '仓库产品', kind: 'workorder-product' as const, required: true}] : []),
        {key: 'planned_quantity', label: '计划数量', kind: deps.formState.type === 'production' ? 'workorder-quantity' : 'text', required: deps.formState.type === 'production'},
        {key: 'due_at', label: '交期', kind: 'date'},
        {key: 'priority', label: '优先级', kind: 'select', options: [{label: '普通', value: 'normal'}, {label: '加急', value: 'urgent'}]},
        {key: 'target_department_ids', label: '流转部门', kind: 'multi-select', options: departmentOptions, required: true},
        {key: 'description', label: '说明', kind: 'textarea', required: deps.formState.type === 'general'},
        {key: 'operator_employee_id', label: '本次操作人', kind: 'operator', required: true},
      ]
      default: return []
    }
  })
  return {activeWarehouseTabTitle, createEntityTitle, formSchema}
}
