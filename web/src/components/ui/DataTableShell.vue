<template>
  <section class="ui-data-table-shell" :class="`bb-table-density-${density}`" :aria-label="ariaLabel" :aria-busy="loading">
    <el-alert
      v-if="error && rowsCount > 0"
      class="ui-data-table-shell__alert"
      :title="error"
      type="error"
      :closable="false"
      show-icon
    />

    <PageState v-if="loading && rowsCount === 0" kind="loading" title="正在加载列表" />
    <PageState
      v-else-if="error && rowsCount === 0"
      kind="error"
      title="加载失败"
      :description="error"
      action-label="重新加载"
      @action="emit('retry')"
    />
    <PageState
      v-else-if="rowsCount === 0"
      kind="empty"
      :title="emptyTitle"
      :description="emptyDescription"
    />
    <div v-else v-loading="loading" class="ui-data-table-shell__table">
      <slot></slot>
    </div>

    <footer v-if="rowsCount > 0" class="ui-data-table-shell__pagination">
      <span>共 {{ total }} 条记录</span>
      <el-pagination
        v-if="pagination"
        :current-page="page"
        :page-size="pageSize"
        :page-sizes="[20, 50, 100]"
        :total="total"
        background
        :pager-count="5"
        layout="sizes, prev, pager, next"
        @current-change="emit('update:page', $event)"
        @size-change="emit('update:pageSize', $event)"
      />
    </footer>
  </section>
</template>

<script setup lang="ts">
import PageState from './PageState.vue'

withDefaults(defineProps<{
  loading?: boolean
  error?: string
  rowsCount: number
  total: number
  page: number
  pageSize: number
  ariaLabel?: string
  emptyTitle?: string
  emptyDescription?: string
  pagination?: boolean
  density?: 'compact' | 'standard' | 'double' | 'complex'
}>(), {
  loading: false,
  error: '',
  ariaLabel: '数据列表',
  emptyTitle: '暂无记录',
  emptyDescription: '',
  pagination: true,
  density: 'standard',
})

const emit = defineEmits<{
  retry: []
  'update:page': [page: number]
  'update:pageSize': [pageSize: number]
}>()
</script>

<style scoped>
.ui-data-table-shell {
  min-width: 0;
}

.ui-data-table-shell__alert {
  margin-bottom: var(--bb-space-3);
}

.ui-data-table-shell__table {
  overflow: hidden;
  border-block: 1px solid var(--bb-border-subtle);
  border-radius: 0;
  background: var(--bb-bg-elevated);
}

.ui-data-table-shell__table :deep(.el-table) {
  border: 0;
  border-radius: 0;
}

.ui-data-table-shell__pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--bb-space-3);
  min-height: 56px;
  padding: 8px 0;
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-13);
}

.ui-data-table-shell__pagination :deep(.el-pagination) {
  min-width: 0;
  flex-wrap: wrap;
  justify-content: flex-end;
}
</style>
