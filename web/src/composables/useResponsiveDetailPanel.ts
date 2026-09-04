import {computed, onBeforeUnmount, onMounted, ref, type Ref} from 'vue'

/** Keeps business detail in one carrier: docked on wide screens, overlay elsewhere. */
export function useResponsiveDetailPanel(visible: Ref<boolean>, imageDense = false) {
  const viewportWidth = ref(window.innerWidth)
  const preferredWidth = computed(() => imageDense && viewportWidth.value >= 1464 ? 520 : 420)
  const docked = computed(() => visible.value && viewportWidth.value >= 1440 && viewportWidth.value - 224 - preferredWidth.value >= 720)
  const size = computed(() => docked.value ? `${preferredWidth.value}px` : 'min(520px, 100%)')
  const sync = () => { viewportWidth.value = window.innerWidth }
  onMounted(() => window.addEventListener('resize', sync))
  onBeforeUnmount(() => window.removeEventListener('resize', sync))
  return {docked, size}
}
