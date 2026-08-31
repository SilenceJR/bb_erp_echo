import {defineAsyncComponent, type Component} from 'vue'

const GenericModuleContent = defineAsyncComponent(() => import('./GenericModuleContent.vue'))
const MoldModuleContent = defineAsyncComponent(() => import('./MoldModuleContent.vue'))
const StatisticsModuleContent = defineAsyncComponent(() => import('./StatisticsModuleContent.vue'))
const SupplierModuleContent = defineAsyncComponent(() => import('./SupplierModuleContent.vue'))
const WarehouseModuleContent = defineAsyncComponent(() => import('./WarehouseModuleContent.vue'))
const WorkorderModuleContent = defineAsyncComponent(() => import('./WorkorderModuleContent.vue'))

const modulePages: Record<string, Component> = {
  warehouses: WarehouseModuleContent,
  workorder: WorkorderModuleContent,
  molds: MoldModuleContent,
  statistics: StatisticsModuleContent,
  suppliers: SupplierModuleContent,
}

export function resolveModulePage(key: string): Component {
  return modulePages[key] || GenericModuleContent
}
