import type {ServerIdentity} from './types'

type IdentityCoordinates = Pick<ServerIdentity, 'origin' | 'instance_id'>

export type DesktopStartupTarget = 'saved-server' | 'discovery'

export const workspaceSessionStorageKeys = [
  'bb_erp_access_token',
  'bb_erp_refresh_token',
  'bb_erp_access_token_expires_at',
] as const

export function canonicalServerOrigin(value: string): string {
  const input = value.trim()
  if (!input) return ''
  try {
    return new URL(input).origin
  } catch {
    return input.replace(/\/+$/, '')
  }
}

export function serverIdentityKey(identity: IdentityCoordinates): string {
  return `${canonicalServerOrigin(identity.origin)}\n${identity.instance_id.trim()}`
}

export function isSameServerIdentity(left: IdentityCoordinates | null, right: IdentityCoordinates | null): boolean {
  return Boolean(left && right && serverIdentityKey(left) === serverIdentityKey(right))
}

export function shouldClearWorkspaceSession(previous: IdentityCoordinates | null, next: IdentityCoordinates): boolean {
  return !isSameServerIdentity(previous, next)
}

export function uniqueServerIdentities(identities: ServerIdentity[]): ServerIdentity[] {
  const unique = new Map<string, ServerIdentity>()
  for (const identity of identities) {
    const key = serverIdentityKey(identity)
    if (!unique.has(key)) unique.set(key, identity)
  }
  return [...unique.values()]
}

export function automaticServerCandidate(identities: ServerIdentity[]): ServerIdentity | null {
  const candidates = uniqueServerIdentities(identities)
  return candidates.length === 1 ? candidates[0] : null
}

// 仅保存了可规范化 origin 与实例 ID 的服务才可在启动时优先直连。
export function desktopStartupTarget(saved: IdentityCoordinates | null): DesktopStartupTarget {
  return saved && canonicalServerOrigin(saved.origin) && saved.instance_id.trim()
    ? 'saved-server'
    : 'discovery'
}

export function clearStoredWorkspaceSession(storage: Pick<Storage, 'removeItem'>): void {
  for (const key of workspaceSessionStorageKeys) storage.removeItem(key)
}
