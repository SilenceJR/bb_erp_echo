<template>
  <section class="image-gallery" aria-label="业务图片">
    <div class="image-gallery-heading">
      <div>
        <h3>图片资料</h3>
        <small>可一次选择多张；支持 JPG、PNG、WebP、GIF，单张不超过 20 MiB</small>
      </div>
      <div class="image-gallery-actions">
        <el-button :loading="loading" :disabled="saving" @click="loadImages()">刷新</el-button>
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
      accept="image/jpeg,image/png,image/webp,image/gif,.jpg,.jpeg,.png,.webp,.gif"
      aria-label="选择要批量上传的图片"
      multiple
      @change="uploadSelected"
    />
    <input
      ref="replaceInput"
      class="image-gallery-input"
      type="file"
      accept="image/jpeg,image/png,image/webp,image/gif,.jpg,.jpeg,.png,.webp,.gif"
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
      v-loading="loading"
      class="image-gallery-grid"
      :aria-busy="loading || saving"
    >
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
      <p v-if="!loading && !images.length" class="image-gallery-empty">暂无图片资料</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref, watch} from 'vue'
import {ElMessage} from 'element-plus'
import {request, requestBlob} from '../api/http'
import {appMessageBox} from '../composables/useAppMessageBox'
import {useDirtyGuard} from '../composables/useDirtyGuard'
import type {ImageFile} from '../types'

const props = defineProps<{
  ownerType: 'product' | 'mold' | 'workorder' | 'department_task'
  ownerId: number
  token: string
  canWrite: boolean
  category?: string
}>()

const allowedMimeTypes = new Set(['image/jpeg', 'image/png', 'image/webp', 'image/gif'])
const allowedExtensions = new Set(['jpg', 'jpeg', 'png', 'webp', 'gif'])
const maxImageSize = 20 * 1024 * 1024

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

async function loadImages(allowDuringSave = false): Promise<boolean> {
  if (saving.value && !allowDuringSave) {
    return false
  }
  const sequence = ++loadSequence
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await request<ImageFile[]>(queryPath(), {}, props.token)
    const loaded = await Promise.all(result.map(async (item) => {
      try {
        const blob = await requestBlob(item.content_url, {}, props.token)
        return [item.id, URL.createObjectURL(blob)] as const
      } catch {
        return [item.id, ''] as const
      }
    }))
    if (sequence !== loadSequence) {
      for (const [, url] of loaded) if (url) URL.revokeObjectURL(url)
      return false
    }
    releasePreviewUrls()
    images.value = result
    previewUrls.value = Object.fromEntries(loaded)
    return true
  } catch (error) {
    if (sequence !== loadSequence) return false
    if (!allowDuringSave) {
      releasePreviewUrls()
      images.value = []
    }
    errorMessage.value = error instanceof Error ? error.message : '图片加载失败'
    return false
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

function validateFile(file: File): string {
  const extension = file.name.split('.').pop()?.toLowerCase() || ''
  if (!allowedMimeTypes.has(file.type) || !allowedExtensions.has(extension)) {
    return '仅支持 JPG、PNG、WebP、GIF 图片'
  }
  if (file.size <= 0) return '图片文件不能为空'
  if (file.size > maxImageSize) return '图片大小不能超过 20 MiB'
  return ''
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

async function uploadSelected(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (!files.length || !props.canWrite || saving.value) return

  const validationErrors = files
    .map((file) => ({file, message: validateFile(file)}))
    .filter((result) => result.message)
  if (validationErrors.length) {
    const details = validationErrors
      .slice(0, 3)
      .map(({file, message}) => `“${file.name}”：${message}`)
    const omittedCount = validationErrors.length - details.length
    errorMessage.value = `已检查所选 ${files.length} 张图片，其中 ${validationErrors.length} 张未通过校验：${details.join('；')}${omittedCount > 0 ? `；另有 ${omittedCount} 张未通过` : ''}。本次未发起上传。`
    statusMessage.value = ''
    ElMessage.warning('批量上传未开始，请检查文件格式和大小')
    return
  }

  await uploadFiles(files)
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
    statusTone.value = refreshed ? 'success' : 'error'
    statusMessage.value = refreshed
      ? `已成功上传 ${uploadedCount} 张图片，图片列表已刷新。`
      : `已成功上传 ${uploadedCount} 张图片，但列表刷新失败，请稍后手动刷新。`
    if (refreshed) {
      ElMessage.success(`已成功上传 ${uploadedCount} 张图片`)
    } else {
      ElMessage.warning(`已上传 ${uploadedCount} 张图片，但图片列表刷新失败`)
    }
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : `批量上传 ${files.length} 张图片失败`
    statusTone.value = 'error'
    statusMessage.value = `本次 ${files.length} 张图片未能完成上传。`
    ElMessage.error(errorMessage.value)
  } finally {
    operation.value = null
  }
}

async function replaceFile(file: File, target: ImageFile) {
  const validationMessage = validateFile(file)
  if (validationMessage) {
    errorMessage.value = `“${file.name}”：${validationMessage}。本次未发起替换。`
    statusMessage.value = ''
    ElMessage.warning('图片替换未开始，请检查文件格式和大小')
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
    statusTone.value = refreshed ? 'success' : 'error'
    statusMessage.value = refreshed ? '图片已替换，图片列表已刷新。' : '图片已替换，但列表刷新失败，请稍后手动刷新。'
    ElMessage.success('图片已替换')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '图片替换失败'
    statusTone.value = 'error'
    statusMessage.value = '图片未能完成替换。'
    ElMessage.error(errorMessage.value)
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
    statusTone.value = refreshed ? 'success' : 'error'
    statusMessage.value = refreshed ? '图片已删除，图片列表已刷新。' : '图片已删除，但列表刷新失败，请稍后手动刷新。'
    ElMessage.success('图片已删除')
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '图片删除失败'
    statusTone.value = 'error'
    statusMessage.value = '图片未能完成删除。'
    ElMessage.error(errorMessage.value)
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
