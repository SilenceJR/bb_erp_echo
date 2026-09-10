<template>
  <div class="workorder-product-field">
    <div class="workorder-product-field__label">
      <div>
        <label for="workorder-product-select"><strong>产品型号 <span aria-hidden="true">*</span></strong></label>
        <small id="workorder-product-help">从产品资料中搜索启用产品；生产单必须关联一个产品型号。</small>
      </div>
    </div>

    <el-select
      id="workorder-product-select"
      v-model="formState.product_id"
      class="workorder-product-select"
      filterable
      remote
      clearable
      :disabled="!canReadProducts"
      :remote-method="searchWorkorderProducts"
      :loading="workorderProductSearchLoading"
      :placeholder="canReadProducts ? '输入产品型号搜索' : '缺少产品查看权限'"
      aria-label="搜索并选择产品型号"
      @visible-change="handleVisibleChange"
      @change="handleWorkorderProductSelect"
    >
      <el-option v-for="item in workorderProductOptions" :key="item.id" :label="String(item.product_model || '')" :value="Number(item.id)" />
      <template #empty><span v-if="workorderProductSearchLoading">正在搜索产品型号…</span><span v-else>{{ workorderProductSearchError || '没有找到匹配的产品型号' }}</span></template>
    </el-select>

    <el-alert v-if="!canReadProducts" type="warning" title="当前账号缺少产品查看权限，无法选择产品或创建生产单。" description="请联系管理员授予 product:read，或将任务类型改为通用任务。" :closable="false" show-icon />
    <el-alert v-else-if="workorderProductSearchError && workorderProductOptions.length" type="error" :title="workorderProductSearchError" :closable="false" show-icon />
    <WorkorderStockCard v-if="canReadWarehouse && formState.product_id" :product="workorderProductStock" :loading="workorderProductStockLoading" :error="workorderProductStockError" :updated-at="workorderProductStockUpdatedAt" @refresh="loadWorkorderProductStock" />
  </div>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useWorkorderContext} from '../../composables/workorderContext'
import WorkorderStockCard from './WorkorderStockCard.vue'

const {
  formState,
  workorderProductOptions,
  workorderProductSearchLoading,
  workorderProductSearchError,
  workorderProductStock,
  workorderProductStockLoading,
  workorderProductStockError,
  workorderProductStockUpdatedAt,
  hasPermission,
  searchWorkorderProducts,
  handleWorkorderProductSelect,
  loadWorkorderProductStock,
} = useWorkorderContext().product
const canReadProducts = computed(() => hasPermission('product:read'))
const canReadWarehouse = computed(() => hasPermission('warehouse:read'))

function handleVisibleChange(visible: boolean) {
  if (visible && !workorderProductOptions.value.length && !workorderProductSearchLoading.value) void searchWorkorderProducts('')
}
</script>
