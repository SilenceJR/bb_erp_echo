// ModuleItem 描述前端导航和后端接口进度。
export interface ModuleItem {
  key: string
  title: string
  group: 'dashboard' | 'system' | 'business'
  path?: string
  readPermission?: string
  writePermission?: string
  status: 'available' | 'skeleton'
  description: string
}

// modules 根据当前 Echo 后端已注册接口维护，后续后端扩展 CRUD 时同步调整。
export const modules: ModuleItem[] = [
  { key: 'dashboard', title: '首页', group: 'dashboard', status: 'available', description: '常用功能与全部业务入口。' },
  { key: 'users', title: '用户账号', group: 'system', path: '/api/v1/system/users', readPermission: 'system:users:read', writePermission: 'system:users:write', status: 'available', description: '个人账号和部门终端账号基础管理。' },
  { key: 'departments', title: '部门', group: 'system', path: '/api/v1/system/departments', readPermission: 'system:departments:read', writePermission: 'system:departments:write', status: 'available', description: '部门基础数据。' },
  { key: 'terminals', title: '终端', group: 'system', path: '/api/v1/system/terminals', readPermission: 'system:terminals:read', writePermission: 'system:terminals:write', status: 'available', description: '公共电脑和部门终端。' },
  { key: 'roles', title: '角色', group: 'system', path: '/api/v1/system/roles', readPermission: 'system:roles:read', writePermission: 'system:roles:write', status: 'available', description: '角色清单与权限绑定入口。' },
  { key: 'permissions', title: '权限', group: 'system', path: '/api/v1/system/permissions', readPermission: 'system:permissions:read', status: 'available', description: '系统功能权限清单。' },
  { key: 'audits', title: '操作审计', group: 'system', path: '/api/v1/system/audits', readPermission: 'system:audits:read', status: 'available', description: '最近 200 条组织内操作审计。' },
  { key: 'customers', title: '客户', group: 'business', path: '/api/v1/customers', readPermission: 'customers:read', writePermission: 'customers:write', status: 'available', description: '客户档案基础创建和列表。' },
  { key: 'contacts', title: '联系人', group: 'business', path: '/api/v1/contacts', readPermission: 'contacts:read', writePermission: 'contacts:write', status: 'available', description: '客户联系人和电话明细。' },
  { key: 'suppliers', title: '供应商', group: 'business', path: '/api/v1/suppliers', readPermission: 'suppliers:read', writePermission: 'suppliers:write', status: 'available', description: '维护采购入库使用的供应商档案。' },
  { key: 'warehouses', title: '仓库', group: 'business', path: '/api/v1/warehouse/items', readPermission: 'warehouse:read', writePermission: 'warehouse:write', status: 'available', description: '查看单仓库库存，并在具体物品中办理出入库。' },
  { key: 'molds', title: '模具台账', group: 'business', path: '/api/v1/molds', readPermission: 'mold:read', writePermission: 'mold:write', status: 'available', description: '查询模具位置、借出、维修与保养状态。' },
  { key: 'workorder', title: '任务单', group: 'business', path: '/api/v1/workorder', readPermission: 'workorder:read', writePermission: 'workorder:write', status: 'skeleton', description: '查看和处理工作任务。' },
  { key: 'statistics', title: '统计报表', group: 'business', path: '/api/v1/statistics', readPermission: 'statistics:read', writePermission: 'statistics:write', status: 'skeleton', description: '查看业务统计与报表。' },
]
