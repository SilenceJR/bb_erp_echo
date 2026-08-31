import {inject, type InjectionKey} from 'vue'
import type {useWorkspaceController} from './useWorkspaceController'

export type WorkorderContext = ReturnType<typeof useWorkspaceController>['workorderContext']
export const workorderContextKey: InjectionKey<WorkorderContext> = Symbol('bb-erp-workorder')

export function useWorkorderContext(): WorkorderContext {
  const context = inject(workorderContextKey)
  if (!context) throw new Error('Workorder context is only available inside WorkspaceSession')
  return context
}
