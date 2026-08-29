import {reactive, ref} from 'vue'
import type {BasicItem} from '../types'

/** Owns task detail and log state shared by office and department operations. */
export function useWorkorder() {
  const selectedWorkOrder = ref<BasicItem | null>(null)
  const workorderDrawerVisible = ref(false)
  const workorderLogs = ref<BasicItem[]>([])
  const workorderLogsLoading = ref(false)
  const workorderLogsError = ref('')
  const workorderProductOptions = ref<BasicItem[]>([])
  const workorderProductSearchLoading = ref(false)
  const workorderProductSearchError = ref('')
  const workorderProductStock = ref<BasicItem | null>(null)
  const workorderProductStockLoading = ref(false)
  const workorderProductStockError = ref('')
  const workorderProductStockUpdatedAt = ref('')
  const workorderDrawerProductStock = ref<BasicItem | null>(null)
  const workorderDrawerProductStockLoading = ref(false)
  const workorderDrawerProductStockError = ref('')
  const workorderDrawerProductStockUpdatedAt = ref('')
  const temporaryProductDialogVisible = ref(false)
  const temporaryProductSubmitting = ref(false)
  const temporaryProductError = ref('')
  const temporaryProductForm = reactive({name: '', code: '', unit: '个', spec: ''})
  return {
    selectedWorkOrder,
    workorderDrawerVisible,
    workorderLogs,
    workorderLogsLoading,
    workorderLogsError,
    workorderProductOptions,
    workorderProductSearchLoading,
    workorderProductSearchError,
    workorderProductStock,
    workorderProductStockLoading,
    workorderProductStockError,
    workorderProductStockUpdatedAt,
    workorderDrawerProductStock,
    workorderDrawerProductStockLoading,
    workorderDrawerProductStockError,
    workorderDrawerProductStockUpdatedAt,
    temporaryProductDialogVisible,
    temporaryProductSubmitting,
    temporaryProductError,
    temporaryProductForm,
  }
}
