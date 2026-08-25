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

// 客户端升级状态，由服务端从 GitHub、Gitee 或内网更新源缓存后提供。
export interface ClientUpdateStatus {
  current_version: string
  latest_version?: string
  available: boolean
  cached: boolean
  file_name?: string
  download_path?: string
  message?: string
}

// 更新中心状态兼容服务端、桌面客户端两类包；字段保持可选以容忍禁用更新、
// 首次检查失败以及旧服务端返回的精简状态。
export interface UpdatePackageStatus {
  current_version?: string
  latest_version?: string
  available?: boolean
  cached?: boolean
  file_name?: string
  download_path?: string
  download_url?: string
  size?: number
  sha256?: string
  message?: string
}

export interface SystemUpdateStatus {
  enabled?: boolean
  manifest_url?: string
  source?: string
  reachable?: boolean
  checking?: boolean
  check_interval?: string
  last_attempt_at?: string
  last_success_at?: string
  next_check_at?: string
  last_error?: string
  error?: string
  server?: UpdatePackageStatus
  client?: UpdatePackageStatus
  server_update?: UpdatePackageStatus
  client_update?: UpdatePackageStatus
  [key: string]: unknown
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

// 图片元数据，对应文件服务返回结构；content_url 只作为元数据保留，不直接用于渲染。
export interface ImageFile {
  id: number
  owner_type: string
  owner_id: number
  original_name: string
  mime_type: string
  size: number
  category: string
  content_url: string
  created_at: string
}
