<template>
  <div class="workorder-product-field">
    <div class="workorder-product-field__label">
      <div>
        <label for="workorder-product-select"><strong>仓库产品 <span aria-hidden="true">*</span></strong></label>
        <small id="workorder-product-help">按名称、编码或规格搜索；生产单必须关联一个启用产品。</small>
      </div>
      <el-button
        v-if="canCreateTemporaryProduct"
        class="workorder-temporary-product-trigger"
        link
        type="primary"
        :disabled="loading"
        @click="openTemporaryProductDialog"
      >＋ 临时添加产品</el-button>
    </div>

    <el-select
      id="workorder-product-select"
      v-product-combobox-a11y="productComboboxA11y"
      v-model="formState.product_id"
      class="workorder-product-select"
      filterable
      remote
      clearable
      :disabled="!canReadWarehouse"
      :remote-method="searchWorkorderProducts"
      :loading="workorderProductSearchLoading"
      :placeholder="canReadWarehouse ? '输入产品名称、编码或规格' : '缺少仓库查看权限'"
      aria-label="搜索并选择仓库产品"
      @visible-change="handleVisibleChange"
      @change="handleWorkorderProductSelect"
    >
      <el-option
        v-for="item in workorderProductOptions"
        :key="item.id"
        class="workorder-product-option-shell"
        :label="`${item.name || ''}${item.code ? `（${item.code}）` : ''}`"
        :value="item.id"
        :disabled="Boolean(item.status) && item.status !== 'active'"
      >
        <div class="workorder-product-option">
          <div>
            <strong>{{ item.name }}</strong>
            <small>{{ item.code }} · {{ item.spec || '无规格' }}</small>
          </div>
          <div>
            <span>{{ formatQuantity(item.quantity) }} {{ item.unit || '' }}</span>
            <StatusTag
              :label="item.status && item.status !== 'active' ? '已停用' : stockState(item).label"
              :tone="item.status && item.status !== 'active' ? 'info' : stockState(item).tone"
            />
          </div>
        </div>
      </el-option>
      <template #empty>
        <div class="workorder-product-empty">
          <span v-if="workorderProductSearchLoading">正在搜索仓库产品…</span>
          <template v-else-if="canReadWarehouse">
            <span>{{ workorderProductSearchError || '没有找到匹配的仓库产品' }}</span>
            <el-button v-if="workorderProductSearchError" link type="primary" @click="searchWorkorderProducts('')">重新搜索</el-button>
            <el-button v-else-if="canCreateTemporaryProduct" class="workorder-temporary-product-trigger" link type="primary" @click="openTemporaryProductDialog">临时添加产品</el-button>
          </template>
        </div>
      </template>
    </el-select>

    <el-alert
      v-if="!canReadWarehouse"
      id="workorder-product-permission-help"
      type="warning"
      title="当前账号缺少仓库查看权限，无法选择产品或创建生产单。"
      description="请联系管理员授予 warehouse:read，或将任务类型改为通用任务。"
      :closable="false"
      show-icon
    />

    <el-alert
      v-if="canReadWarehouse && workorderProductSearchError && workorderProductOptions.length"
      type="error"
      :title="workorderProductSearchError"
      :closable="false"
      show-icon
    />

    <WorkorderStockCard
      v-if="canReadWarehouse && formState.product_id"
      :product="workorderProductStock"
      :loading="workorderProductStockLoading"
      :error="workorderProductStockError"
      :updated-at="workorderProductStockUpdatedAt"
      @refresh="loadWorkorderProductStock"
    />

    <el-dialog
      v-model="temporaryProductDialogVisible"
      title="临时添加产品档案"
      width="min(560px, calc(100vw - 28px))"
      append-to-body
      :close-on-click-modal="false"
      :close-on-press-escape="!temporaryProductSubmitting"
      :show-close="!temporaryProductSubmitting"
      @closed="closeTemporaryProductDialog"
    >
      <el-alert
        title="保存后会成为正式产品档案，初始库存为 0，不会生成入库流水。"
        type="info"
        :closable="false"
        show-icon
      />
      <el-form class="temporary-product-form" label-position="top" :disabled="temporaryProductSubmitting" @submit.prevent="createTemporaryProduct">
        <el-form-item label="产品名称" required>
          <el-input v-model.trim="temporaryProductForm.name" maxlength="100" show-word-limit autocomplete="off" />
        </el-form-item>
        <el-form-item label="产品编码" required>
          <el-input v-model.trim="temporaryProductForm.code" maxlength="100" show-word-limit autocomplete="off" placeholder="请输入唯一编码" />
        </el-form-item>
        <el-form-item label="单位" required>
          <el-input v-model.trim="temporaryProductForm.unit" maxlength="20" autocomplete="off" />
        </el-form-item>
        <el-form-item label="规格">
          <el-input v-model.trim="temporaryProductForm.spec" maxlength="200" show-word-limit autocomplete="off" placeholder="选填" />
        </el-form-item>
        <el-alert v-if="temporaryProductError" :title="temporaryProductError" type="error" :closable="false" show-icon />
      </el-form>
      <template #footer>
        <div class="temporary-product-actions">
          <el-button :disabled="temporaryProductSubmitting" @click="closeTemporaryProductDialog">取消</el-button>
          <el-button type="primary" :loading="temporaryProductSubmitting" @click="createTemporaryProduct">保存并选择</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import {computed, nextTick, type DirectiveBinding, type ObjectDirective} from 'vue'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import StatusTag from '../ui/StatusTag.vue'
import WorkorderStockCard from './WorkorderStockCard.vue'

const {
  formState,
  formError,
  loading,
  temporaryProductForm,
  temporaryProductDialogVisible,
  temporaryProductSubmitting,
  temporaryProductError,
  workorderProductOptions,
  workorderProductSearchLoading,
  workorderProductSearchError,
  workorderProductStock,
  workorderProductStockLoading,
  workorderProductStockError,
  workorderProductStockUpdatedAt,
  hasPermission,
  stockState,
  formatQuantity,
  searchWorkorderProducts,
  handleWorkorderProductSelect,
  loadWorkorderProductStock,
  openTemporaryProductDialog,
  closeTemporaryProductDialog,
  createTemporaryProduct,
} = useWorkspaceContext()

const canCreateTemporaryProduct = computed(() => (
  hasPermission('warehouse:read')
  && hasPermission('workorder:write')
  && hasPermission('workorder:temporary-product:write')
))
const canReadWarehouse = computed(() => hasPermission('warehouse:read'))
const productDescribedBy = computed(() => (
  [
    'workorder-product-help',
    !canReadWarehouse.value ? 'workorder-product-permission-help' : '',
    formError.value ? 'workorder-create-error' : '',
  ].filter(Boolean).join(' ')
))
const productComboboxA11y = computed(() => ({
  describedBy: productDescribedBy.value,
  invalid: !canReadWarehouse.value || (!!formError.value && !formState.product_id),
}))

type ProductComboboxA11y = {describedBy: string; invalid: boolean}

function applyProductComboboxA11y(element: HTMLElement, binding: DirectiveBinding<ProductComboboxA11y>) {
  const apply = () => {
    const input = element.querySelector<HTMLInputElement>('input[role="combobox"]')
    if (!input) return
    input.setAttribute('aria-required', 'true')
    input.setAttribute('aria-describedby', binding.value.describedBy)
    input.setAttribute('aria-invalid', String(binding.value.invalid))
  }
  apply()
  void nextTick(apply)
}

const vProductComboboxA11y: ObjectDirective<HTMLElement, ProductComboboxA11y> = {
  mounted: applyProductComboboxA11y,
  updated: applyProductComboboxA11y,
}

function handleVisibleChange(visible: boolean) {
  if (visible && !workorderProductOptions.value.length && !workorderProductSearchLoading.value) {
    void searchWorkorderProducts('')
  }
}
</script>
