export type StatisticsSource = 'inventory' | 'workorders' | 'suppliers'
export const moduleUnavailableEvent = 'bb-erp:module-unavailable'

export function isModuleNotInitialized(error: unknown): error is Error & {code: 'module_not_initialized'} {
  return error instanceof Error
    && 'code' in error
    && (error as Error & {code?: string}).code === 'module_not_initialized'
}

export function statisticsSourceIsUnavailable(
  status: string | undefined,
  unavailableSources: string[] | undefined,
  source: StatisticsSource,
): boolean {
  if (status !== 'sources_unavailable') return false
  const aliases = source === 'inventory'
    ? ['inventory', 'warehouse', 'location']
    : source === 'workorders'
      ? ['workorder', 'department_task']
      : ['supplier']
  return (unavailableSources || []).some((value) => aliases.some((alias) => String(value).toLowerCase().includes(alias)))
}

export function deferredModuleForPath(path: string): 'warehouses' | 'workorder' | 'suppliers' | '' {
  const pathname = path.split('?')[0].toLowerCase()
  if (pathname.includes('/warehouse') || pathname.includes('/inventory')) return 'warehouses'
  if (pathname.includes('/workorder')) return 'workorder'
  if (pathname.includes('/supplier')) return 'suppliers'
  return ''
}
