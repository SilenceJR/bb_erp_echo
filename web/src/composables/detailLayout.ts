import {computed, ref, shallowReactive} from 'vue'
import {normalizeSidebarMode, sidebarStorageKey} from '../platform/appearance'
import {canDockDetail} from '../platform/detailPanel'

export const clientViewport = ref(window.innerWidth)
export const clientSidebarMode = ref(normalizeSidebarMode(localStorage.getItem(sidebarStorageKey)))
export const detailWidths = shallowReactive(new Map<symbol, number>())
export const sidebarPixels = computed(() => clientViewport.value < 1024 || clientSidebarMode.value === 'hidden' ? 0 : clientSidebarMode.value === 'icon' ? 64 : 224)
export const activeDetailKey = computed(() => detailWidths.keys().next().value)
export const activeDetailWidth = computed(() => detailWidths.values().next().value || 420)
export const activeDetailDocked = computed(() => detailWidths.size > 0 && canDockDetail(clientViewport.value, sidebarPixels.value, activeDetailWidth.value))
