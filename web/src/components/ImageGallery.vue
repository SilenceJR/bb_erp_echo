<template>
  <section class="image-gallery" :aria-label="`${title}图片`">
    <div class="image-gallery-heading">
      <div>
        <h3>{{ title }}</h3>
        <small>单次最多选择 100 张；支持 JPG、JFIF、PNG、GIF、WebP、HEIC、HEIF、AVIF、BMP、TIFF、SVG；仅生成静态预览，GIF/动态照片只显示封面；支持高清大图，实际可处理范围以服务器安全校验为准</small>
      </div>
      <div class="image-gallery-actions">
        <el-button :loading="loading" :disabled="saving" @click="refreshImages">刷新</el-button>
        <el-button
          v-if="canWrite"
          type="primary"
          :loading="operation === 'upload'"
          :disabled="loading || saving"
          @click="openUpload"
        >
          上传图片
        </el-button>
      </div>
    </div>

    <input
      ref="uploadInput"
      class="image-gallery-input"
      type="file"
      accept="image/*,.jpg,.jpeg,.jfif,.png,.gif,.webp,.heic,.heif,.avif,.bmp,.tif,.tiff,.svg"
      aria-label="选择要批量上传的图片"
      multiple
      @change="uploadSelected"
    />
    <input
      ref="replaceInput"
      class="image-gallery-input"
      type="file"
      accept="image/*,.jpg,.jpeg,.jfif,.png,.gif,.webp,.heic,.heif,.avif,.bmp,.tif,.tiff,.svg"
      aria-label="选择用于替换的单张图片"
      @change="replaceSelected"
    />

    <el-alert
      v-if="errorMessage"
      title="图片操作未完成"
      :description="errorMessage"
      type="error"
      :closable="false"
      show-icon
      role="alert"
    />
    <el-alert
      v-if="!canWrite"
      title="当前账号仅可查看图片"
      description="上传、替换或删除需要对应业务写入权限；部门子任务还需属于当前部门。"
      type="info"
      :closable="false"
      show-icon
    />

    <p
      v-if="statusMessage"
      class="image-gallery-status"
      :class="{'is-busy': saving, 'is-error': statusTone === 'error'}"
      role="status"
      aria-live="polite"
      aria-atomic="true"
    >
      {{ statusMessage }}
    </p>

    <div
      class="image-gallery-dropzone"
      data-file-drop-target
      :class="{'is-dragging': isDragging}"
      @dragenter.prevent="handleDragEnter"
      @dragover.prevent="handleDragOver"
      @dragleave.prevent="handleDragLeave"
      @drop.prevent="handleDrop"
      @bb-native-file-drag="handleNativeFileDrag"
    >
      <div v-loading="loading" class="image-gallery-grid" :aria-busy="loading || saving">
        <article v-for="item in images" :key="item.id" class="image-gallery-item">
          <div class="image-gallery-preview">
            <el-image
              v-if="previewUrls[item.id]"
              :src="previewUrls[item.id]"
              :alt="item.original_name"
              :aria-label="`预览图片：${item.original_name}`"
              tabindex="0"
              role="button"
              :preview-src-list="previewSources"
              :initial-index="previewIndex(item.id)"
              fit="cover"
              preview-teleported
              @keydown.enter.prevent="openPreviewFromKeyboard"
              @keydown.space.prevent="openPreviewFromKeyboard"
            >
              <template #error><span class="image-gallery-error">图片加载失败</span></template>
            </el-image>
            <span v-else class="image-gallery-error">图片加载失败</span>
          </div>
          <div class="image-gallery-meta">
            <span :title="item.original_name">{{ item.original_name }}</span>
            <small>{{ formatSize(item.size) }} · {{ formatDate(item.created_at) }}</small>
          </div>
          <div v-if="canWrite" class="image-gallery-item-actions">
            <el-button link type="primary" :disabled="loading || saving" @click="openReplace(item)">替换</el-button>
            <el-button link type="danger" :disabled="loading || saving" @click="deleteImage(item)">删除</el-button>
          </div>
        </article>
        <p v-if="!loading && !images.length && !errorMessage" class="image-gallery-empty">暂无图片资料</p>
      </div>
      <button
        v-if="canWrite"
        type="button"
        class="image-gallery-drop-hint"
        :class="{'is-dragging': isDragging}"
        :disabled="loading || saving"
        @click="openUpload"
      >
        <strong>{{ isDragging ? '松开以上传图片' : '拖入图片到此处' }}</strong>
        <span>或点击选择多张图片</span>
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref, watch} from 'vue'
import {ElMessage} from 'element-plus'
import {ApiError, RequestTransportError, request, requestBlob, uploadNativeFiles} from '../api/http'
import {appMessageBox} from '../composables/useAppMessageBox'
import {useDirtyGuard} from '../composables/useDirtyGuard'
import type {ImageFile, NativeFileDragDetail} from '../types'

const props = defineProps<{
  ownerType: 'product' | 'mold' | 'workorder' | 'department_task'
  ownerId: number
  token: string
  canWrite: boolean
  category?: string
  title?: string
}>()

const title = props.title || '图片资料'

const allowedExtensions = new Set([
  'jpg', 'jpeg', 'jfif', 'png', 'gif', 'webp',
  'heic', 'heif', 'avif', 'bmp', 'tif', 'tiff', 'svg',
])
const maxImageBatch = 100
const allowedFormatMessage = '仅支持 JPG、JFIF、PNG、GIF、WebP、HEIC、HEIF、AVIF、BMP、TIFF、SVG 静态图片（客户端仅按文件扩展名预检查，文件内容由服务器校验）'

const images = ref<ImageFile[]>([])
const previewUrls = ref<Record<number, string>>({})
const loading = ref(false)
const errorMessage = ref('')
const statusMessage = ref('')
const statusTone = ref<'success' | 'error'>('success')
const uploadInput = ref<HTMLInputElement | null>(null)
const replaceInput = ref<HTMLInputElement | null>(null)
const replaceTarget = ref<ImageFile | null>(null)
const operation = ref<'upload' | 'replace' | 'delete' | null>(null)
const isDragging = ref(false)
let dragDepth = 0
let loadSequence = 0

const saving = computed(() => operation.value !== null)

useDirtyGuard('image-gallery', {
  busy: () => saving.value,
  busyMessage: '图片正在上传、替换或删除，请等待完成后再离开',
})

const previewSources = computed(() => images.value
  .map((item) => previewUrls.value[item.id])
  .filter((url): url is string => Boolean(url)))

function queryPath(): string {
  const query = new URLSearchParams({
    owner_type: props.ownerType,
    owner_id: String(props.ownerId),
  })
  if (props.category) query.set('category', props.category)
  return `/api/v1/files?${query.toString()}`
}

function releasePreviewUrls() {
  for (const url of Object.values(previewUrls.value)) URL.revokeObjectURL(url)
  previewUrls.value = {}
}

type LoadResult = 'complete' | 'partial' | 'failed'

async function loadImages(allowDuringSave = false): Promise<LoadResult> {
  if (saving.value && !allowDuringSave) {
    return 'failed'
  }
  const sequence = ++loadSequence
  loading.value = true
  errorMessage.value = ''
  if (!allowDuringSave) statusMessage.value = ''
  try {
    const result = await request<ImageFile[]>(queryPath(), {}, props.token)
    const previewFailures: Array<{item: ImageFile, error: unknown}> = []
    const loaded = await Promise.all(result.map(async (item) => {
      try {
        const blob = await requestBlob(item.preview_url || item.content_url, {}, props.token)
        return [item.id, URL.createObjectURL(blob)] as const
      } catch (error) {
        previewFailures.push({item, error})
        return [item.id, ''] as const
      }
    }))
    if (sequence !== loadSequence) {
      for (const [, url] of loaded) if (url) URL.revokeObjectURL(url)
      return 'failed'
    }
    releasePreviewUrls()
    images.value = result
    previewUrls.value = Object.fromEntries(loaded)
    if (previewFailures.length) {
      const examples = previewFailures.slice(0, 3).map(({item, error}) => {
        const reason = operationErrorMessage(error, '静态预览读取失败')
        const requestId = error instanceof ApiError && error.requestId.trim() ? `，请求编号：${error.requestId}` : ''
        return `“${item.original_name}”：${reason}${requestId}`
      })
      const omitted = previewFailures.length - examples.length
      errorMessage.value = `图片列表已载入，但有 ${previewFailures.length} 张静态预览无法显示：${examples.join('；')}${omitted > 0 ? `；另有 ${omitted} 张失败` : ''}。请点击“刷新”重试；若仍失败，请将请求编号提供给管理员。本次没有改用高清原图，避免大文件占用页面内存。`
      return 'partial'
    }
    return 'complete'
  } catch (error) {
    if (sequence !== loadSequence) return 'failed'
    // 刷新失败保留已渲染图片，避免临时网络错误误显示为空图库。
    errorMessage.value = error instanceof Error ? error.message : '图片加载失败'
    return 'failed'
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

async function refreshImages() {
  statusMessage.value = ''
  const result = await loadImages()
  if (result === 'complete') {
    statusTone.value = 'success'
    statusMessage.value = '图片列表已刷新。'
  }
}

function validateFile(file: File): string {
  const extension = file.name.split('.').pop()?.toLowerCase() || ''
  // Windows WebView 等运行环境可能不给 File.type，或给出不可靠的 MIME。
  // 客户端只按扩展名做选择前提示，真实图片内容统一交给服务器校验。
  if (!allowedExtensions.has(extension)) return allowedFormatMessage
  if (file.size <= 0) return '图片文件不能为空'
  return ''
}

function operationErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim()) return error.message
  if (typeof error === 'string' && error.trim()) return error
  return fallback
}

function imageFileName(path: string): string {
  return path.split(/[\\/]/).filter(Boolean).pop() || path
}

function fileNamesForMessage(names: string[]): string {
  const visibleNames = names.slice(0, 3).map((name) => `“${imageFileName(name)}”`)
  const omittedCount = names.length - visibleNames.length
  return `${visibleNames.join('、')}${omittedCount > 0 ? `等 ${names.length} 个文件` : ''}`
}

function uploadFailureSuggestion(error: unknown): string {
  if (error instanceof RequestTransportError && error.resultMayBeUnknown) {
    return '请先核对刷新后的图片列表；确认没有这些文件后再重试'
  }
  if (error instanceof ApiError) {
    if (error.status === 401) return '请重新登录后再试'
    if (error.status === 403) return '请确认当前账号具有对应业务的写入权限'
    if (error.status === 404 || error.status === 409) return '请刷新当前业务资料后重新选择图片'
    if (error.status === 400 || error.status === 413 || error.status === 415 || error.status === 422) {
      return '请按原因移除异常文件，或将其转换为受支持的静态图片后重试；不要只修改文件扩展名'
    }
    if (error.status >= 500) return '请稍后重试；若仍失败，请将请求编号提供给管理员排查服务器存储或转换服务'
  }
  return '请检查网络连接后重试；若仍失败，请将错误详情提供给管理员'
}

function uploadFailureMessage(error: unknown, fallback: string, names: string[]): string {
  const reason = operationErrorMessage(error, fallback)
  const requestId = error instanceof ApiError && error.requestId.trim() ? `；请求编号：${error.requestId}` : ''
  return `文件：${fileNamesForMessage(names)}；原因：${reason}；建议：${uploadFailureSuggestion(error)}${requestId}。`
}

function uploadResultMayBeUnknown(error: unknown): boolean {
  return error instanceof RequestTransportError && error.resultMayBeUnknown
}

async function reconcileUnknownUpload(error: unknown, names: string[], fallback: string): Promise<boolean> {
  if (!uploadResultMayBeUnknown(error)) return false
  const refreshed = await loadImages(true)
  const refreshText = refreshed === 'failed'
    ? '图片列表也未能刷新'
    : '图片列表已重新载入'
  errorMessage.value = `${uploadFailureMessage(error, fallback, names)} 由于连接中断，服务器是否已完成入库暂时无法确认；${refreshText}，请先核对这些文件是否已出现，再决定是否重试。`
  statusTone.value = 'error'
  statusMessage.value = ''
  return true
}

function openUpload() {
  if (!props.canWrite || loading.value || saving.value) return
  if (uploadInput.value) {
    uploadInput.value.value = ''
    uploadInput.value.click()
  }
}

function openReplace(item: ImageFile) {
  if (!props.canWrite || loading.value || saving.value) return
  replaceTarget.value = item
  if (replaceInput.value) {
    replaceInput.value.value = ''
    replaceInput.value.click()
  }
}

function hasDraggedFiles(event: DragEvent): boolean {
  return Array.from(event.dataTransfer?.types || []).includes('Files')
}

function handleDragEnter(event: DragEvent) {
  if (!props.canWrite || !hasDraggedFiles(event)) return
  dragDepth++
  isDragging.value = true
}

function handleDragOver(event: DragEvent) {
  if (props.canWrite && hasDraggedFiles(event)) event.dataTransfer!.dropEffect = 'copy'
}

function handleDragLeave() {
  dragDepth = Math.max(0, dragDepth - 1)
  if (!dragDepth) isDragging.value = false
}

function handleDrop(event: DragEvent) {
  dragDepth = 0
  isDragging.value = false
  if (!props.canWrite || saving.value) return
  const files = Array.from(event.dataTransfer?.files || [])
  if (files.length) void uploadSelectedFiles(files)
}

function handleNativeFileDrag(event: Event) {
  const detail = (event as CustomEvent<NativeFileDragDetail>).detail
  if (!detail) return
  if (detail.phase === 'enter' || detail.phase === 'over') {
    if (props.canWrite) isDragging.value = true
    return
  }
  isDragging.value = false
  if (detail.phase !== 'drop' || !props.canWrite || saving.value) return
  if (detail.error) {
    errorMessage.value = detail.error
    statusMessage.value = ''
    return
  }
  if (detail.paths.length) void uploadNativePaths(detail.paths)
  if (detail.files.length) void uploadSelectedFiles(detail.files)
}

async function uploadSelectedFiles(files: File[]) {
  if (!files.length || !props.canWrite || saving.value) return
  if (files.length > maxImageBatch) {
    errorMessage.value = `本次选择了 ${files.length} 张图片，一次最多上传 ${maxImageBatch} 张，请分批上传。本次未发起上传。`
    statusMessage.value = ''
    return
  }
  const validationErrors = files
    .map((file) => ({file, message: validateFile(file)}))
    .filter((result) => result.message)
  if (validationErrors.length) {
    const details = validationErrors
      .slice(0, 3)
      .map(({file, message}) => `“${file.name}”：${message}`)
    const omittedCount = validationErrors.length - details.length
    errorMessage.value = `已检查所选 ${files.length} 张图片，其中 ${validationErrors.length} 张未通过校验：${details.join('；')}${omittedCount > 0 ? `；另有 ${omittedCount} 张未通过` : ''}。建议移除空文件，或转换为受支持的静态图片后重试；不要只修改文件扩展名。本次未发起上传。`
    statusMessage.value = ''
    return
  }
  await uploadFiles(files)
}

async function uploadSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  await uploadSelectedFiles(files)
}

async function replaceSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  const target = replaceTarget.value
  replaceTarget.value = null
  if (!file || !target || !props.canWrite || saving.value) return
  await replaceFile(file, target)
}

async function uploadFiles(files: File[]) {
  if (!files.length || !props.canWrite || saving.value) return
  const body = new FormData()
  for (const file of files) body.append('file', file)
  body.append('owner_type', props.ownerType)
  body.append('owner_id', String(props.ownerId))
  if (props.category) body.append('category', props.category)

  operation.value = 'upload'
  errorMessage.value = ''
  statusTone.value = 'success'
  statusMessage.value = `正在上传 ${files.length} 张图片，完成前无法刷新或修改图片。`
  try {
    const uploaded = await request<ImageFile[]>('/api/v1/files/images', {
      method: 'POST',
      body,
    }, props.token)
    const uploadedCount = uploaded.length
    statusMessage.value = `已上传 ${uploadedCount} 张图片，正在刷新图片列表。`
    const refreshed = await loadImages(true)
    statusTone.value = refreshed === 'complete' ? 'success' : 'error'
    statusMessage.value = refreshed === 'complete'
      ? `已成功上传 ${uploadedCount} 张图片，图片列表已刷新。`
      : ''
    if (refreshed === 'complete') {
      ElMessage.success(`已成功上传 ${uploadedCount} 张图片`)
    }
  } catch (error) {
    if (await reconcileUnknownUpload(error, files.map((file) => file.name), `批量上传 ${files.length} 张图片失败`)) return
    errorMessage.value = uploadFailureMessage(error, `批量上传 ${files.length} 张图片失败`, files.map((file) => file.name))
    statusTone.value = 'error'
    statusMessage.value = ''
  } finally {
    operation.value = null
  }
}

async function uploadNativePaths(paths: string[]) {
  if (!paths.length || !props.canWrite || saving.value) return
  if (paths.length > maxImageBatch) {
    errorMessage.value = `本次拖入了 ${paths.length} 张图片，一次最多上传 ${maxImageBatch} 张，请分批上传。本次未发起上传。`
    statusMessage.value = ''
    return
  }
  operation.value = 'upload'
  errorMessage.value = ''
  statusTone.value = 'success'
  statusMessage.value = `正在上传 ${paths.length} 张图片，完成前无法刷新或修改图片。`
  try {
    const fields: Record<string, string> = {owner_type: props.ownerType, owner_id: String(props.ownerId)}
    if (props.category) fields.category = props.category
    const uploaded = await uploadNativeFiles<ImageFile[]>('/api/v1/files/images', paths, fields, props.token)
    statusMessage.value = `已上传 ${uploaded.length} 张图片，正在刷新图片列表。`
    const refreshed = await loadImages(true)
    statusTone.value = refreshed === 'complete' ? 'success' : 'error'
    statusMessage.value = refreshed === 'complete' ? `已成功上传 ${uploaded.length} 张图片，图片列表已刷新。` : ''
    if (refreshed === 'complete') ElMessage.success(`已成功上传 ${uploaded.length} 张图片`)
  } catch (error) {
    if (await reconcileUnknownUpload(error, paths, `批量上传 ${paths.length} 张图片失败`)) return
    errorMessage.value = uploadFailureMessage(error, `批量上传 ${paths.length} 张图片失败`, paths)
    statusTone.value = 'error'
    statusMessage.value = ''
  } finally {
    operation.value = null
  }
}

async function replaceFile(file: File, target: ImageFile) {
  const validationMessage = validateFile(file)
  if (validationMessage) {
    errorMessage.value = `“${file.name}”：${validationMessage}。本次未发起替换。`
    statusMessage.value = ''
    return
  }
  const body = new FormData()
  body.append('file', file)
  if (props.category) body.append('category', props.category)

  operation.value = 'replace'
  errorMessage.value = ''
  statusTone.value = 'success'
  statusMessage.value = `正在替换“${target.original_name}”。`
  try {
    await request<ImageFile>(`/api/v1/files/${target.id}/content`, {
      method: 'PUT',
      body,
    }, props.token)
    const refreshed = await loadImages(true)
    statusTone.value = refreshed === 'complete' ? 'success' : 'error'
    statusMessage.value = refreshed === 'complete' ? '图片已替换，图片列表已刷新。' : ''
    if (refreshed === 'complete') ElMessage.success('图片已替换')
  } catch (error) {
    if (await reconcileUnknownUpload(error, [file.name], '图片替换失败')) return
    errorMessage.value = uploadFailureMessage(error, '图片替换失败', [file.name])
    statusTone.value = 'error'
    statusMessage.value = ''
  } finally {
    operation.value = null
  }
}

async function deleteImage(item: ImageFile) {
  if (!props.canWrite || loading.value || saving.value) return
  try {
    await appMessageBox.confirm(`确定删除“${item.original_name}”吗？`, '删除图片', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  if (saving.value) return
  operation.value = 'delete'
  errorMessage.value = ''
  statusTone.value = 'success'
  statusMessage.value = `正在删除“${item.original_name}”。`
  try {
    await request<void>(`/api/v1/files/${item.id}`, {method: 'DELETE'}, props.token)
    const refreshed = await loadImages(true)
    statusTone.value = refreshed === 'complete' ? 'success' : 'error'
    statusMessage.value = refreshed === 'complete' ? '图片已删除，图片列表已刷新。' : ''
    ElMessage.success('图片已删除')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '图片删除失败'
    statusTone.value = 'error'
    statusMessage.value = ''
  } finally {
    operation.value = null
  }
}

function openPreviewFromKeyboard(event: KeyboardEvent) {
  const image = (event.currentTarget as HTMLElement).querySelector('img')
  image?.click()
}

function previewIndex(id: number): number {
  const url = previewUrls.value[id]
  return Math.max(0, previewSources.value.indexOf(url))
}

function formatSize(value: number): string {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${(value / 1024 / 1024).toFixed(1)} MiB`
}

function formatDate(value: string): string {
  return new Date(value).toLocaleString('zh-CN', {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'})
}

watch(() => [props.ownerType, props.ownerId, props.category, props.token], () => {
  void loadImages()
})

onMounted(() => void loadImages())
onBeforeUnmount(() => {
  loadSequence++
  releasePreviewUrls()
})
</script>
