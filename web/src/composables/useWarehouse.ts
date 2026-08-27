import {reactive, ref} from 'vue'
import type {BasicItem} from '../types'

/** Owns inventory detail and posting-form state; API workflows stay in the workspace controller. */
export function useWarehouse() {
  const selectedWarehouseItem = ref<BasicItem | null>(null)
  const warehouseDrawerVisible = ref(false)
  const warehouseDetail = ref<Record<string, unknown> | null>(null)
  const warehouseDetailLoading = ref(false)
  const warehouseDetailError = ref('')
  const itemMovements = ref<BasicItem[]>([])
  const itemMovementsLoading = ref(false)
  const itemMovementsError = ref('')
  const showAllItemMovements = ref(false)
  const movementMode = ref('')
  const showQuickSupplier = ref(false)
  const movementSubmitting = ref(false)
  const movementFormError = ref('')
  const quickSupplierSubmitting = ref(false)
  const quickSupplierError = ref('')
  const movementForm = reactive<Record<string, any>>({})
  const quickSupplier = reactive({name: '', code: '', contact: '', phone: ''})
  return {
    selectedWarehouseItem,
    warehouseDrawerVisible,
    warehouseDetail,
    warehouseDetailLoading,
    warehouseDetailError,
    itemMovements,
    itemMovementsLoading,
    itemMovementsError,
    showAllItemMovements,
    movementMode,
    showQuickSupplier,
    movementSubmitting,
    movementFormError,
    quickSupplierSubmitting,
    quickSupplierError,
    movementForm,
    quickSupplier,
  }
}
