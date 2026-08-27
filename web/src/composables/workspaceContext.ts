import {inject, type InjectionKey} from 'vue'
import type {useWorkspaceController} from './useWorkspaceController'

/** Shared setup bindings exposed by the authenticated workspace controller. */
export type WorkspaceContext = ReturnType<typeof useWorkspaceController>

/** Injection key used by shell and domain views without introducing a global store. */
export const workspaceContextKey: InjectionKey<WorkspaceContext> = Symbol('bb-erp-workspace')

/** Returns the current workspace controller or fails fast outside the authenticated shell. */
export function useWorkspaceContext(): WorkspaceContext {
  const context = inject(workspaceContextKey)
  if (!context) throw new Error('Workspace context is only available inside AppWorkspace')
  return context
}
