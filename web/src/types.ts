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

// 更新中心状态字段保持可选，以容忍服务端停用更新或尚未完成首次检查。
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
  server_update?: UpdatePackageStatus
  client_protocol_version?: number
  client_full_cached?: boolean
  client_cache_bytes?: number
  [key: string]: unknown
}

export type DesktopUpdateState = 'Idle' | 'Checking' | 'Ready' | 'Downloading' | 'Verifying' | 'Applying' | 'Restarting' | 'Failed'

// 桌面升级计划由 Rust 校验后返回。Vue 只展示计划摘要，不读取资源 URL、
// 本地路径或签名内容，避免把安全决策下放到 WebView。
export interface DesktopUpdatePlan {
  current_version?: string
  latest_version?: string
  version?: string
  strategy: 'full'
  download_size?: number
  full_size?: number
  message?: string
  artifact?: Record<string, unknown>
  [key: string]: unknown
}

export interface DesktopUpdateProgress {
  state: DesktopUpdateState
  message?: string
  downloaded_bytes?: number
  total_bytes?: number
}

export interface DesktopUpdateApplyResult {
  success?: boolean
  state?: DesktopUpdateState
  message?: string
  restart_required?: boolean
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

export interface DepartmentSummary {
  id: number
  name: string
  code?: string
  status?: string
}

export interface EmployeeItem {
  id: number
  name: string
  phone?: string
  hire_date: string
  birthplace?: string
  residential_address?: string
  birth_date: string
  age: number
  status: 'active' | 'disabled'
  departments: DepartmentSummary[]
  created_at?: string
}

export interface DepartmentItem extends DepartmentSummary {
  status: 'active' | 'disabled'
  employee_count: number
  created_at?: string
}

export interface OperatorEmployee {
  id: number
  name: string
}

export interface OperatorEmployeesResponse {
  department: DepartmentSummary | null
  employees: OperatorEmployee[]
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

export interface CustomerProfile {
  id: number
  created_at: string
  updated_at: string
  customer_code_id: number
  code?: string
  short_name: string
  name: string
  address: string
  phone: string
  contact_name: string
  contact_phone: string
  salesperson: string
  is_default: boolean
}

export interface CustomerCodeItem {
  id: number
  created_at: string
  updated_at: string
  code: string
  profiles: CustomerProfile[]
  profile_count: number
  default_profile?: CustomerProfile
}

export interface CustomerOption {
  id: number
  code: string
  short_name: string
  name: string
  is_default: boolean
}

export interface SpreadsheetColumn {
  key: string
  title: string
  width: number
  type: 'text' | 'number' | 'date' | 'bool'
  alignment?: 'left' | 'center' | 'right' | string
}

export interface SpreadsheetDocument {
  sheet_name: string
  title?: string
  columns: SpreadsheetColumn[]
  rows: string[][]
  total_rows: number
  page?: number
  page_size?: number
  empty: boolean
  has_more: boolean
}

export interface SpreadsheetCellError {
  row: number
  column: string
  value?: string
  reason: string
}

export interface CustomerImportPreview {
  token?: string
  expires_at?: string
  summary: {
    total_rows: number
    new_codes: number
    new_profiles: number
    multiple_code_groups: number
  }
  errors: SpreadsheetCellError[]
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
