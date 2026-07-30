// ModuleItem 描述前端导航和后端接口进度。
export interface ModuleItem {
  key: string
  title: string
  group: 'dashboard' | 'system' | 'business'
  path?: string
  status: 'available' | 'skeleton'
  description: string
}

// modules 根据当前 Echo 后端已注册接口维护，后续后端扩展 CRUD 时同步调整。
export const modules: ModuleItem[] = [
  { key: 'dashboard', title: '运行概览', group: 'dashboard', status: 'available', description: '健康检查、登录身份和模块进度。' },
  { key: 'users', title: '用户账号', group: 'system', path: '/api/v1/system/users', status: 'available', description: '个人账号和部门终端账号基础管理。' },
  { key: 'departments', title: '部门', group: 'system', path: '/api/v1/system/departments', status: 'available', description: '部门基础数据。' },
  { key: 'terminals', title: '终端', group: 'system', path: '/api/v1/system/terminals', status: 'available', description: '公共电脑和部门终端。' },
  { key: 'roles', title: '角色', group: 'system', path: '/api/v1/system/roles', status: 'available', description: '角色清单与权限绑定入口。' },
  { key: 'permissions', title: '权限', group: 'system', path: '/api/v1/system/permissions', status: 'available', description: 'Casbin 接口权限清单。' },
  { key: 'audits', title: '操作审计', group: 'system', path: '/api/v1/system/audits', status: 'available', description: '最近 200 条组织内操作审计。' },
  { key: 'customers', title: '客户', group: 'business', path: '/api/v1/customers', status: 'available', description: '客户档案基础创建和列表。' },
  { key: 'contacts', title: '联系人', group: 'business', path: '/api/v1/contacts', status: 'available', description: '客户联系人和电话明细。' },
  { key: 'warehouses', title: '仓库', group: 'business', path: '/api/v1/warehouse/items', status: 'available', description: '单仓库内按产品、生产物资、常规产品、生活物资分类管理。' },
  { key: 'inventory_documents', title: '库存单据', group: 'business', path: '/api/v1/inventory-documents', status: 'available', description: '入库、出库、调拨草稿单据。' },
  { key: 'inventory_balances', title: '库存余额', group: 'business', path: '/api/v1/inventory-balances', status: 'available', description: '当前库存结存。' },
  { key: 'inventory_ledgers', title: '库存流水', group: 'business', path: '/api/v1/inventory-ledgers', status: 'available', description: '库存过账流水。' },
  { key: 'molds', title: '模具管理', group: 'business', path: '/api/v1/molds', status: 'available', description: '模具台账、位置、借出、维修和保养状态。' },
  { key: 'workorder', title: '任务单', group: 'business', path: '/api/v1/workorder', status: 'skeleton', description: '模块骨架已注册，并兼容 /api/v1/tasks。' },
  { key: 'statistics', title: '统计报表', group: 'business', path: '/api/v1/statistics', status: 'skeleton', description: '模块骨架已注册。' },
]
