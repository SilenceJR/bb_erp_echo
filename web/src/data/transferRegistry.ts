/**
 * 客户与模具导入导出固定契约。
 *
 * 这些信息属于客户端交互契约，不从服务端动态读取。领域模型、列定义或
 * 资料包目录发生变化时，应与对应的后端处理器一起更新这里的声明和回归测试。
 */
export type TransferModule = 'customers' | 'molds'
export type TransferKind = 'template' | 'export'
export type TransferFormat = 'xlsx' | 'zip'

export interface TransferDefinition {
  readonly module: TransferModule
  readonly kind: TransferKind
  readonly path: string
  readonly previewPath?: string
  readonly fileName: string
  readonly format: TransferFormat
  readonly mimeType: string
  readonly accept: string
  readonly permission: string
  readonly label: string
  readonly successLabel: string
  readonly failureLabel: string
}

export interface TransferModuleDefinition {
  readonly template: TransferDefinition
  readonly export: TransferDefinition
  readonly import: {
    readonly accept: string
    readonly format: TransferFormat
    readonly permission: string
    readonly previewPath: string
    readonly commitPath: string
    readonly label: string
  }
  readonly archiveLayout?: {
    readonly rootFiles: readonly string[]
    readonly directories: readonly string[]
  }
}

const customerTemplate: TransferDefinition = {
  module: 'customers',
  kind: 'template',
  path: '/api/v1/customers/import-template',
  fileName: '客户资料导入模板.xlsx',
  format: 'xlsx',
  mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  accept: '.xls,.xlsx',
  permission: 'customers:import',
  label: '客户资料导入模板',
  successLabel: '客户资料模板',
  failureLabel: '客户资料模板下载失败',
}

const customerExport: TransferDefinition = {
  module: 'customers',
  kind: 'export',
  path: '/api/v1/customers/export',
  previewPath: '/api/v1/customers/export/preview',
  fileName: '客户资料.xlsx',
  format: 'xlsx',
  mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  accept: '',
  permission: 'customers:read',
  label: '导出客户资料',
  successLabel: '客户资料 XLSX',
  failureLabel: '客户资料导出失败',
}

const moldTemplate: TransferDefinition = {
  module: 'molds',
  kind: 'template',
  path: '/api/v1/molds/import-template',
  fileName: '博邦模具导入模板.zip',
  format: 'zip',
  mimeType: 'application/zip',
  accept: '.zip,application/zip',
  permission: 'mold:import',
  label: '下载模具导入模板',
  successLabel: '模具导入模板',
  failureLabel: '模具导入模板下载失败',
}

const moldExport: TransferDefinition = {
  module: 'molds',
  kind: 'export',
  path: '/api/v1/molds/export',
  fileName: '博邦模具资料包.zip',
  format: 'zip',
  mimeType: 'application/zip',
  accept: '',
  permission: 'mold:read',
  label: '导出模具资料包',
  successLabel: '模具资料包 ZIP',
  failureLabel: '模具资料包导出失败',
}

export const transferRegistry: Readonly<Record<TransferModule, TransferModuleDefinition>> = Object.freeze({
  customers: Object.freeze({
    template: Object.freeze(customerTemplate),
    export: Object.freeze(customerExport),
    import: Object.freeze({
      accept: '.xls,.xlsx',
      format: 'xlsx' as const,
      permission: 'customers:import',
      previewPath: '/api/v1/customers/import/preview',
      commitPath: '/api/v1/customers/import/commit',
      label: '导入客户资料',
    }),
  }),
  molds: Object.freeze({
    template: Object.freeze(moldTemplate),
    export: Object.freeze(moldExport),
    import: Object.freeze({
      accept: '.zip,application/zip',
      format: 'zip' as const,
      permission: 'mold:import',
      previewPath: '/api/v1/molds/import/preview',
      commitPath: '/api/v1/molds/import/commit',
      label: '导入模具资料包',
    }),
    archiveLayout: Object.freeze({
      rootFiles: Object.freeze(['molds.xlsx', 'locations.json']),
      directories: Object.freeze([
        'images/',
        'images/MOLD-001/',
        'images/MOLD-001/product_material/',
        'images/MOLD-001/supplement/',
        'drawings/',
        'drawings/MOLD-001/',
      ]),
    }),
  }),
})

export function getTransferDefinition(module: TransferModule, kind: TransferKind): TransferDefinition {
  return transferRegistry[module][kind]
}

export function getTransferImportDefinition(module: TransferModule) {
  return transferRegistry[module].import
}

export function appendTransferQuery(path: string, query?: URLSearchParams | Record<string, string | number | undefined>): string {
  if (!query) return path
  const params = query instanceof URLSearchParams
    ? query
    : new URLSearchParams(Object.entries(query).flatMap(([key, value]) => value === undefined ? [] : [[key, String(value)]]))
  const serialized = params.toString()
  return serialized ? `${path}${path.includes('?') ? '&' : '?'}${serialized}` : path
}
