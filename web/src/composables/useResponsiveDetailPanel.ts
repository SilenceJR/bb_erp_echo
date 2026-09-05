import {computed, onBeforeUnmount, onMounted, watch, unref, type MaybeRef, type Ref} from 'vue'
import {clientViewport, detailWidths, activeDetailKey} from './detailLayout'

export type DetailPanelComplexity = 'detail' | 'short-form' | 'standard-form' | 'long-form' | 'image-dense'
export type DetailPanelOptions = {complexity?: DetailPanelComplexity; width?: number | Ref<number>}
export type DetailPanelSpec = MaybeRef<boolean | number | DetailPanelOptions>

function widthForComplexity(complexity: DetailPanelComplexity | undefined) {
  return complexity === 'standard-form' || complexity === 'long-form' || complexity === 'image-dense' ? 520 : 420
}

/** Keeps every active business surface in the shared right-hand dock. */
export function useResponsiveDetailPanel(visible: Ref<boolean>, spec: DetailPanelSpec = false) {
  const key = Symbol('detail')
  const preferredWidth = computed(() => {
    if (typeof spec === 'boolean') return spec ? 520 : 420
    if (typeof spec === 'number') return spec
    const value = unref(spec)
    if (typeof value === 'boolean') return value ? 520 : 420
    if (typeof value === 'number') return value
    const width = value.width === undefined ? undefined : unref(value.width)
    return width ?? widthForComplexity(value.complexity)
  })
  const normalizedWidth = computed(() => Math.max(320, Math.min(720, Math.round(preferredWidth.value))))
  // All current business surfaces share one right-hand dock. The newest
  // surface replaces the current one; the carrier closes the replaced model.
  const docked = computed(() => visible.value && activeDetailKey.value === key)
  const size = computed(() => `min(${normalizedWidth.value}px, calc(100vw - 32px))`)
  watch([visible, normalizedWidth], ([open, width]) => {
    if (open) {
      detailWidths.set(key, width)
      activeDetailKey.value = key
      return
    }
    detailWidths.delete(key)
    if (activeDetailKey.value === key) activeDetailKey.value = undefined
  }, {immediate: true, flush: 'sync'})
  const sync = () => { clientViewport.value = window.innerWidth }
  onMounted(() => window.addEventListener('resize', sync))
  onBeforeUnmount(() => {
    window.removeEventListener('resize', sync)
    detailWidths.delete(key)
    if (activeDetailKey.value === key) activeDetailKey.value = undefined
  })
  return {docked, size, width: normalizedWidth}
}
