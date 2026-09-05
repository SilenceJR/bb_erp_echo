import {computed, ref, toValue, type MaybeRefOrGetter} from 'vue'
import {ElMessage} from 'element-plus'
import {downloadApiFile} from '../api/http'
import type {FileSaveResult} from '../platform/types'
import {appendTransferQuery, getTransferDefinition, type TransferKind, type TransferModule} from '../data/transferRegistry'

/**
 * Web/Tauri 共用的导入导出文件下载状态与反馈。
 * 同一组件内同一时刻只允许一个传输任务，避免重复点击生成多个保存对话框。
 */
export function useTransferDownload(token: MaybeRefOrGetter<string>) {
  const currentTransferKey = ref('')
  const activeKey = computed(() => currentTransferKey.value)

  function key(module: TransferModule, kind: TransferKind): string {
    return `${module}.${kind}`
  }

  function isLoading(module: TransferModule, kind: TransferKind): boolean {
    return currentTransferKey.value === key(module, kind)
  }

  async function download(
    module: TransferModule,
    kind: TransferKind,
    query?: URLSearchParams | Record<string, string | number | undefined>,
  ): Promise<FileSaveResult> {
    const definition = getTransferDefinition(module, kind)
    const transferKey = key(module, kind)
    if (currentTransferKey.value) return {status: 'cancelled'}
    currentTransferKey.value = transferKey
    try {
      const result = await downloadApiFile(appendTransferQuery(definition.path, query), definition.fileName, toValue(token))
      if (result.status === 'error') throw new Error(result.message)
      if (result.status === 'saved') {
        ElMessage.success(result.path ? `${definition.successLabel}已保存到：${result.path}` : `浏览器已开始下载${definition.successLabel}`)
      } else if (result.status === 'cancelled') {
        ElMessage.info(`已取消保存${definition.successLabel}`)
      }
      return result
    } catch (cause) {
      ElMessage.error(cause instanceof Error && cause.message ? cause.message : definition.failureLabel)
      throw cause
    } finally {
      currentTransferKey.value = ''
    }
  }

  return {
    activeKey,
    isLoading,
    download,
  }
}
