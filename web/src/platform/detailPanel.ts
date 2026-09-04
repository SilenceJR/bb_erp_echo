export interface DockedDetailEscapeState {
  key: string
  defaultPrevented: boolean
  visible: boolean
  docked: boolean
  escapeEnabled: boolean
  blockedByFloatingLayer: boolean
}

/** Measure usable table space, not the padded workspace track. */
export function canDockDetail(viewport: number, sidebar: number, panel: number): boolean {
  return viewport >= 1440 && viewport - sidebar - panel - 2 - 48 >= 720
}

/** Keeps Escape scoped to the active docked detail, without stealing it from a nested overlay or popper. */
export function shouldRequestDockedDetailClose(state: DockedDetailEscapeState): boolean {
  return state.key === 'Escape'
    && !state.defaultPrevented
    && state.visible
    && state.docked
    && state.escapeEnabled
    && !state.blockedByFloatingLayer
}
