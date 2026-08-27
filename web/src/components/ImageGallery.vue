<template>
  <section class="image-gallery" aria-label="业务图片">
    <div class="image-gallery-heading">
      <div>
        <h3>图片资料</h3>
        <small>支持 JPG、PNG、WebP、GIF，单张不超过 20 MiB</small>
      </div>
      <div class="image-gallery-actions">
        <el-button :loading="loading" @click="loadImages">刷新</el-button>
        <el-button v-if="canWrite" type="primary" :loading="saving" @click="openUpload">上传图片</el-button>
      </div>
    </div>

    <input
      ref="uploadInput"
      class="image-gallery-input"
      type="file"
      accept="image/jpeg,image/png,image/webp,image/gif,.jpg,.jpeg,.png,.webp,.gif"
      @change="uploadSelected"
    />
    <input
      ref="replaceInput"
      class="image-gallery-input"
      type="file"
      accept="image/jpeg,image/png,image/webp,image/gif,.jpg,.jpeg,.png,.webp,.gif"
      @change="replaceSelected"
    />

    <el-alert v-if="errorMessage" :title="errorMessage" type="error" :closable="false" show-icon/>

    <div v-loading="loading" class="image-gallery-grid">
      <article v-for="item in images" :key="item.id" class="image-gallery-item">
        <div class="image-gallery-preview">
          <el-image
            v-if="previewUrls[item.id]"
            :src="previewUrls[item.id]"
            :preview-src-list="previewSources"
            :initial-index="previewIndex(item.id)"
            fit="cover"
            preview-teleported
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
          <el-button link type="primary" :disabled="saving" @click="openReplace(item)">替换</el-button>
          <el-button link type="danger" :disabled="saving" @click="deleteImage(item)">删除</el-button>
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
const saving = ref(false)
const errorMessage = ref('')
const uploadInput = ref<HTMLInputElement | null>(null)
const replaceInput = ref<HTMLInputElement | null>(null)
const replaceTarget = ref<ImageFile | null>(null)
let loadSequence = 0

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

async function loadImages() {
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
      return
    }
    releasePreviewUrls()
    images.value = result
    previewUrls.value = Object.fromEntries(loaded)
  } catch (error) {
    if (sequence !== loadSequence) return
    releasePreviewUrls()
    images.value = []
    errorMessage.value = error instanceof Error ? error.message : '图片加载失败'
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
  if (!props.canWrite) return
  if (uploadInput.value) {
    uploadInput.value.value = ''
    uploadInput.value.click()
  }
}

function openReplace(item: ImageFile) {
  if (!props.canWrite) return
  replaceTarget.value = item
  if (replaceInput.value) {
    replaceInput.value.value = ''
    replaceInput.value.click()
  }
}

async function uploadSelected(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  await saveFile(file)
}

async function replaceSelected(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  const target = replaceTarget.value
  replaceTarget.value = null
  if (!file || !target) return
  await saveFile(file, target)
}

async function saveFile(file: File, target?: ImageFile) {
  const validationMessage = validateFile(file)
  if (validationMessage) {
    ElMessage.warning(validationMessage)
    return
  }
  const body = new FormData()
  body.append('file', file)
  if (target) {
    if (props.category) body.append('category', props.category)
  } else {
    body.append('owner_type', props.ownerType)
    body.append('owner_id', String(props.ownerId))
    if (props.category) body.append('category', props.category)
  }

  saving.value = true
  errorMessage.value = ''
  try {
    await request<ImageFile>(target ? `/api/v1/files/${target.id}/content` : '/api/v1/files/images', {
      method: target ? 'PUT' : 'POST',
      body,
    }, props.token)
    ElMessage.success(target ? '图片已替换' : '图片已上传')
    await loadImages()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '图片保存失败'
    ElMessage.error(errorMessage.value)
  } finally {
    saving.value = false
  }
}

async function deleteImage(item: ImageFile) {
  if (!props.canWrite) return
  try {
    await appMessageBox.confirm(`确定删除“${item.original_name}”吗？`, '删除图片', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  saving.value = true
  errorMessage.value = ''
  try {
    await request<void>(`/api/v1/files/${item.id}`, {method: 'DELETE'}, props.token)
    ElMessage.success('图片已删除')
    await loadImages()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '图片删除失败'
    ElMessage.error(errorMessage.value)
  } finally {
    saving.value = false
  }
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
