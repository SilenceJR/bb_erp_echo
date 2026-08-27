import {ref} from 'vue'
import type {BasicItem} from '../types'

/** Owns task detail and log state shared by office and department operations. */
export function useWorkorder() {
  const selectedWorkOrder = ref<BasicItem | null>(null)
  const workorderDrawerVisible = ref(false)
  const workorderLogs = ref<BasicItem[]>([])
  const workorderLogsLoading = ref(false)
  const workorderLogsError = ref('')
  return {
    selectedWorkOrder,
    workorderDrawerVisible,
    workorderLogs,
    workorderLogsLoading,
    workorderLogsError,
  }
}
