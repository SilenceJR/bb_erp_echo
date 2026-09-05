import {computed, ref, shallowReactive} from 'vue'
import {normalizeSidebarMode, sidebarStorageKey} from '../platform/appearance'

export const clientViewport = ref(window.innerWidth)
export const clientSidebarMode = ref(normalizeSidebarMode(localStorage.getItem(sidebarStorageKey)))
export const detailWidths = shallowReactive(new Map<symbol, number>())
export const sidebarPixels = computed(() => clientViewport.value < 1024 || clientSidebarMode.value === 'hidden' ? 0 : clientSidebarMode.value === 'icon' ? 64 : 224)
export const activeDetailKey = ref<symbol>()
export const activeDetailWidth = computed(() => activeDetailKey.value ? detailWidths.get(activeDetailKey.value) || 420 : 420)
export const activeDetailDocked = computed(() => activeDetailKey.value !== undefined && detailWidths.has(activeDetailKey.value))

export type DetailCloseRequest = {resolve: (closed: boolean) => void}

/** Requests the currently visible dock surface to pass its normal leave guard. */
export function requestActiveDetailClose() {
  const visibleDock = document.querySelector('.workspace-detail-aside')
  if (!activeDetailDocked.value && !visibleDock) return Promise.resolve(true)
  return new Promise<boolean>((resolve) => {
    document.dispatchEvent(new CustomEvent<DetailCloseRequest>('bb:request-active-detail-close', {detail: {resolve}}))
  })
}
