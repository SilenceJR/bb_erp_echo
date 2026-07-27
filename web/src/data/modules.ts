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
  { key: 'organizations', title: '组织', group: 'system', path: '/api/v1/system/organizations', status: 'available', description: '组织基础数据。' },
  { key: 'departments', title: '部门', group: 'system', path: '/api/v1/system/departments', status: 'available', description: '部门基础数据。' },
  { key: 'terminals', title: '终端', group: 'system', path: '/api/v1/system/terminals', status: 'available', description: '公共电脑和部门终端。' },
  { key: 'roles', title: '角色', group: 'system', path: '/api/v1/system/roles', status: 'available', description: '角色清单与权限绑定入口。' },
  { key: 'permissions', title: '权限', group: 'system', path: '/api/v1/system/permissions', status: 'available', description: 'Casbin 接口权限清单。' },
  { key: 'audits', title: '操作审计', group: 'system', path: '/api/v1/system/audits', status: 'available', description: '最近 200 条组织内操作审计。' },
  { key: 'customers', title: '客户与联系人', group: 'business', path: '/api/v1/customers', status: 'skeleton', description: '模块骨架已注册。' },
  { key: 'warehouse', title: '仓库', group: 'business', path: '/api/v1/warehouse', status: 'skeleton', description: '模块骨架已注册。' },
  { key: 'inventory', title: '库存', group: 'business', path: '/api/v1/inventory', status: 'skeleton', description: '模块骨架已注册。' },
  { key: 'material', title: '物料', group: 'business', path: '/api/v1/material', status: 'skeleton', description: '模块骨架已注册。' },
  { key: 'product', title: '产品', group: 'business', path: '/api/v1/product', status: 'skeleton', description: '模块骨架已注册。' },
  { key: 'mold', title: '模具管理', group: 'business', path: '/api/v1/mold', status: 'skeleton', description: '模块骨架已注册。' },
  { key: 'workorder', title: '任务单', group: 'business', path: '/api/v1/workorder', status: 'skeleton', description: '模块骨架已注册，并兼容 /api/v1/tasks。' },
  { key: 'statistics', title: '统计报表', group: 'business', path: '/api/v1/statistics', status: 'skeleton', description: '模块骨架已注册。' },
]
