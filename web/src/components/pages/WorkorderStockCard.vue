<template>
  <section class="workorder-stock-card" :aria-busy="loading" aria-live="polite" aria-label="产品实时库存">
    <div class="workorder-stock-card__heading">
      <div>
        <small>仓库实时库存</small>
        <strong>{{ product?.name || '已关联产品' }}</strong>
        <span>{{ productSubtitle }}</span>
      </div>
      <div class="workorder-stock-card__actions">
        <StatusTag v-if="product" :label="stockState(product).label" :tone="stockState(product).tone" />
        <el-button
          class="workorder-stock-card__refresh-button"
          size="small"
          :loading="loading"
          :disabled="loading"
          aria-label="刷新产品库存"
          @click="$emit('refresh')"
        >刷新库存</el-button>
      </div>
    </div>

    <el-skeleton v-if="loading && !product" :rows="2" animated />
    <template v-else-if="product">
      <dl class="workorder-stock-card__metrics">
        <div><dt>当前库存</dt><dd>{{ formatQuantity(product.quantity) }} {{ product.unit || '' }}</dd></div>
        <div><dt>安全库存</dt><dd>{{ formatQuantity(product.safety_stock) }} {{ product.unit || '' }}</dd></div>
        <div><dt>更新时间</dt><dd>{{ updatedAtLabel }}</dd></div>
      </dl>
      <small v-if="loading" class="workorder-stock-card__refreshing">正在刷新，当前保留上次成功数据。</small>
    </template>

    <el-alert
      v-if="error"
      class="workorder-stock-card__error"
      type="error"
      :title="error"
      :closable="false"
      show-icon
    >
      <template #default>
        <el-button link type="primary" :disabled="loading" @click="$emit('refresh')">重新加载库存</el-button>
      </template>
    </el-alert>
  </section>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import type {BasicItem} from '../../types'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import StatusTag from '../ui/StatusTag.vue'

const props = defineProps<{
  product: BasicItem | null
  loading: boolean
  error: string
  updatedAt: string
}>()

defineEmits<{refresh: []}>()

const {formatQuantity, stockState} = useWorkspaceContext()

const productSubtitle = computed(() => {
  if (!props.product) return '库存只用于计划参考，不阻止创建生产单。'
  const code = String(props.product.code || '').trim()
  const spec = String(props.product.spec || '').trim()
  return [code, spec || '无规格'].filter(Boolean).join(' · ')
})

const updatedAtLabel = computed(() => {
  if (!props.updatedAt) return '尚未成功刷新'
  const date = new Date(props.updatedAt)
  return Number.isNaN(date.getTime()) ? props.updatedAt : date.toLocaleString('zh-CN', {hour12: false})
})
</script>
