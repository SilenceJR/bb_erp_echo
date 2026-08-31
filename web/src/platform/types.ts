export const discoveryProtocol = 1 as const
export const erpProduct = 'bb-erp' as const

export interface ServerIdentity {
  product: typeof erpProduct
  discovery_protocol: typeof discoveryProtocol
  instance_id: string
  server_name: string
  server_version: string
  origin: string
}

export interface ConnectionPort {
  readonly canChangeServer: boolean
  currentOrigin(): string
  savedIdentity(): ServerIdentity | null
  validate(origin?: string): Promise<ServerIdentity>
  connect(identity: ServerIdentity): ServerIdentity
}

export interface DiscoveryPort {
  readonly supported: boolean
  discover(): Promise<ServerIdentity[]>
}

export type FileSaveResult =
  | {status: 'saved'; path: string}
  | {status: 'cancelled'}
  | {status: 'error'; message: string}

// FileSavePort 屏蔽浏览器下载与 Tauri 原生保存的差异；调用方必须根据
// saved/cancelled/error 给出对应反馈，不能把“已开始下载”当作保存成功。
export interface FileSavePort {
  save(path: string, fileName: string, token?: string): Promise<FileSaveResult>
}

export interface PlatformCapabilities {
  readonly kind: 'tauri' | 'web'
  readonly connection: ConnectionPort
  readonly discovery: DiscoveryPort
  readonly fileSave: FileSavePort
  appVersion(): Promise<string>
}

export type StartupPhase =
  | 'Booting'
  | 'Discovering'
  | 'Validating'
  | 'AutoConnected'
  | 'SelectServer'
  | 'ManualSetup'
  | 'LoginReady'

export type StartupFailure = 'none' | 'no-server' | 'discovery-failed' | 'validation-failed'
