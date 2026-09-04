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
  { key: 'dashboard', title: '首页', group: 'dashboard', status: 'available', description: '业务入口。' },
  { key: 'departments', title: '部门', group: 'system', path: '/api/v1/system/departments', readPermission: 'system:departments:read', writePermission: 'system:departments:write', status: 'available', description: '部门基础数据。' },
  { key: 'employees', title: '员工档案', group: 'system', path: '/api/v1/system/employees', readPermission: 'system:employees:read', writePermission: 'system:employees:write', status: 'available', description: '维护员工档案、在职状态与所属部门。' },
  { key: 'users', title: '用户账号', group: 'system', path: '/api/v1/system/users', readPermission: 'system:users:read', writePermission: 'system:users:write', status: 'available', description: '个人账号和部门终端账号基础管理。' },
  { key: 'terminals', title: '终端', group: 'system', path: '/api/v1/system/terminals', readPermission: 'system:terminals:read', writePermission: 'system:terminals:write', status: 'available', description: '公共电脑和部门终端。' },
  { key: 'roles', title: '角色', group: 'system', path: '/api/v1/system/roles', readPermission: 'system:roles:read', writePermission: 'system:roles:write', status: 'available', description: '按岗位配置功能权限。' },
  { key: 'permissions', title: '权限', group: 'system', path: '/api/v1/system/permissions', readPermission: 'system:permissions:read', status: 'available', description: '系统功能权限清单。' },
  { key: 'audits', title: '操作审计', group: 'system', path: '/api/v1/system/audits', readPermission: 'system:audits:read', status: 'available', description: '最近 200 条组织内操作审计。' },
  { key: 'updates', title: '版本与更新', group: 'system', path: '/api/v1/system/updates/status', readPermission: 'system:updates:read', writePermission: 'system:updates:write', status: 'available', description: '检查新版本，查看或下载安装包。' },
  { key: 'customers', title: '客户资料', group: 'business', path: '/api/v1/customer-codes', readPermission: 'customers:read', writePermission: 'customers:write', status: 'available', description: '按客户编码维护资料、联系人与 Excel 导入导出。' },
  { key: 'suppliers', title: '供应商', group: 'business', path: '/api/v1/suppliers', readPermission: 'suppliers:read', writePermission: 'suppliers:write', status: 'available', description: '维护采购入库使用的供应商档案。' },
  { key: 'warehouses', title: '仓库', group: 'business', path: '/api/v1/warehouse/items', readPermission: 'warehouse:read', writePermission: 'warehouse:write', status: 'available', description: '查看单仓库库存，并在具体物品中办理出入库。' },
  { key: 'molds', title: '模具', group: 'business', path: '/api/v1/molds', readPermission: 'mold:read', writePermission: 'mold:write', status: 'available', description: '维护模具编号、产品型号、图片资料与固定位置。' },
  { key: 'workorder', title: '任务单', group: 'business', path: '/api/v1/workorder', readPermission: 'workorder:read', writePermission: 'workorder:write', status: 'available', description: '创建生产单，多部门流转并确认结单。' },
  { key: 'statistics', title: '统计报表', group: 'business', path: '/api/v1/statistics', readPermission: 'statistics:read', status: 'available', description: '查看库存、任务、模具和业务数据统计。' },
]
