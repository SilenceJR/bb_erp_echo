import {onBeforeUnmount, onMounted} from 'vue'
import {ElMessage} from 'element-plus'
import {appMessageBox} from './useAppMessageBox'
import {dirtyGuardRegistry} from '../platform/dirtyGuard'

let guardSequence = 0

type DirtyGuardOptions = {
  busy: () => boolean
  dirty?: () => boolean
  busyMessage: string
  dirtyMessage?: string
}

/** Registers component-local form state in the application-wide leave guard. */
export function useDirtyGuard(name: string, options: DirtyGuardOptions) {
  const id = `${name}-${++guardSequence}`
  let remove = () => {}
  const guard = {
    id,
    blocksUnload: () => options.busy() || options.dirty?.() === true,
    async confirmLeave() {
      if (options.busy()) {
        ElMessage.warning(options.busyMessage)
        return false
      }
      if (!options.dirty?.()) return true
      try {
        await appMessageBox.confirm(options.dirtyMessage || '当前表单尚未保存，离开后修改将丢失。', '放弃修改？', {type: 'warning'})
        return true
      } catch {
        return false
      }
    },
  }
  onMounted(() => {
    remove = dirtyGuardRegistry.register(guard)
  })
  onBeforeUnmount(() => remove())
  return {confirmLeave: guard.confirmLeave}
}
