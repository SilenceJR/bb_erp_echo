import {computed, onBeforeUnmount, onMounted, watch, unref, type Ref} from 'vue'
import {clientViewport, sidebarPixels, detailWidths, activeDetailKey} from './detailLayout'
import {canDockDetail} from '../platform/detailPanel'

/** Keeps business detail in one carrier: docked on wide screens, overlay elsewhere. */
export function useResponsiveDetailPanel(visible: Ref<boolean>, imageDense: boolean | Ref<boolean> = false) {
  const key = Symbol('detail')
  const preferredWidth = computed(() => unref(imageDense) ? 520 : 420)
  // A secondary editor overlays the existing detail; it must never create a second docked column.
  const docked = computed(() => visible.value && activeDetailKey.value === key && canDockDetail(clientViewport.value, sidebarPixels.value, preferredWidth.value))
  const size = computed(() => `min(${preferredWidth.value}px, 100%)`)
  watch([visible, preferredWidth], ([open, width]) => { if (open) detailWidths.set(key, width); else detailWidths.delete(key) }, {immediate: true, flush: 'sync'})
  const sync = () => { clientViewport.value = window.innerWidth }
  onMounted(() => window.addEventListener('resize', sync))
  onBeforeUnmount(() => { window.removeEventListener('resize', sync); detailWidths.delete(key) })
  return {docked, size}
}
