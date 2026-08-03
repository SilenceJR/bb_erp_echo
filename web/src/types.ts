// 当前登录用户结构，对应后端 /api/v1/auth/me 返回。
export interface CurrentUser {
  id: number
  username: string
  account_type: 'personal' | 'department_terminal'
  name: string
  organization_id: number
  department_id?: number | null
  terminal_id?: number | null
  roles: string[]
  permissions: string[]
}

// 后端统一错误响应结构。
export interface ApiErrorBody {
  code: string
  message: string
  request_id: string
}

// 系统管理列表中的通用基础字段。
export interface BasicItem {
  id: number
  name?: string
  code?: string
  username?: string
  account_type?: string
  status?: string
  created_at?: string
  [key: string]: unknown
}

// 业务模块骨架接口返回结构。
export interface SkeletonResponse {
  module: string
  name: string
  status: string
  message: string
}

// 分页列表响应，后端列表接口统一返回该结构。
export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  keyword?: string
}
