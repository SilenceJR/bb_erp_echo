export type LeaveReason = 'module-switch' | 'logout' | 'change-server' | 'client-update' | 'window-close' | 'dialog-close'

export interface DirtyGuard {
  id: string
  confirmLeave(reason: LeaveReason): Promise<boolean>
  blocksUnload?: () => boolean
}

class DirtyGuardRegistry {
  private readonly guards = new Map<string, DirtyGuard>()

  register(guard: DirtyGuard): () => void {
    this.guards.set(guard.id, guard)
    return () => {
      if (this.guards.get(guard.id) === guard) this.guards.delete(guard.id)
    }
  }

  remove(id: string) {
    this.guards.delete(id)
  }

  async confirmLeave(reason: LeaveReason): Promise<boolean> {
    for (const guard of [...this.guards.values()]) {
      if (!(await guard.confirmLeave(reason))) return false
    }
    return true
  }

  blocksUnload(): boolean {
    return [...this.guards.values()].some((guard) => guard.blocksUnload?.() === true)
  }
}

/** All destructive navigation paths share one guard registry. */
export const dirtyGuardRegistry = new DirtyGuardRegistry()
