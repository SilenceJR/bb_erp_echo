import {ref} from 'vue'
import type {BasicItem} from '../types'

/** Owns mold detail and lifecycle-action state, including stale-request guards. */
export function useMold() {
  const moldDetailDrawerVisible = ref(false)
  const selectedMoldDetail = ref<BasicItem | null>(null)
  const selectedMoldID = ref<number | null>(null)
  const moldDetailLoading = ref(false)
  const moldDetailError = ref('')
  const moldActionSubmitting = ref(false)
  const moldActionError = ref('')
  return {
    moldDetailDrawerVisible,
    selectedMoldDetail,
    selectedMoldID,
    moldDetailLoading,
    moldDetailError,
    moldActionSubmitting,
    moldActionError,
  }
}
