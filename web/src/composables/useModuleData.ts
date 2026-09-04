import {reactive, ref} from 'vue'
import type {BasicItem, SkeletonResponse} from '../types'

/** Owns navigation, list query and generic form state used by module pages. */
export function useModuleData() {
  const activeKey = ref('dashboard')
  const showCreateForm = ref(false)
  const editingSupplier = ref<BasicItem | null>(null)
  const rows = ref<BasicItem[]>([])
  const columns = ref<string[]>([])
  const skeletonResult = ref<SkeletonResponse | null>(null)
  const searchKeyword = ref('')
  const page = ref(1)
  const pageSize = ref(20)
  const pageTotal = ref(0)
  const loading = ref(false)
  const panelMessage = ref('')
  const listError = ref('')
  const moduleUnavailable = ref<{module: string; message: string} | null>(null)
  const formState = reactive<Record<string, any>>({})
  const activeWarehouseTab = ref('product')
  const workorderStatusFilter = ref('')
  const workorderTypeFilter = ref('')
  const workorderPriorityFilter = ref('')

  // Cached reference data keeps dependent forms usable while the active list changes.
  const cache = reactive<Record<string, BasicItem[]>>({})

  return {
    activeKey,
    showCreateForm,
    editingSupplier,
    rows,
    columns,
    skeletonResult,
    searchKeyword,
    page,
    pageSize,
    pageTotal,
    loading,
    panelMessage,
    listError,
    moduleUnavailable,
    formState,
    activeWarehouseTab,
    workorderStatusFilter,
    workorderTypeFilter,
    workorderPriorityFilter,
    cache,
  }
}
