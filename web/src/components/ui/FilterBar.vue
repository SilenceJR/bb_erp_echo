<template>
  <form class="ui-filter-bar" role="search" :aria-label="ariaLabel" @submit.prevent="emit('submit')">
    <div class="ui-filter-bar__controls">
      <slot name="prepend"></slot>
      <slot></slot>
      <el-button v-if="resettable" @click="emit('reset')">重置</el-button>
      <el-button native-type="submit" type="primary" plain>查询</el-button>
    </div>
    <div class="ui-filter-bar__actions">
      <span v-if="message" class="ui-filter-bar__message" aria-live="polite">{{ message }}</span>
      <slot name="actions"></slot>
      <el-button :loading="loading" @click="emit('refresh')">刷新数据</el-button>
    </div>
  </form>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  message?: string
  loading?: boolean
  ariaLabel?: string
  resettable?: boolean
}>(), {
  message: '',
  loading: false,
  ariaLabel: '列表筛选',
  resettable: false,
})

const emit = defineEmits<{
  submit: []
  refresh: []
  reset: []
}>()
</script>

<style scoped>
.ui-filter-bar {
  display: flex;
  min-height: 56px;
  align-items: center;
  justify-content: space-between;
  gap: var(--bb-space-3);
  margin-bottom: var(--bb-space-3);
  border-bottom: 1px solid var(--bb-border-subtle);
  padding: 12px 0;
  flex-wrap: wrap;
}

.ui-filter-bar__controls,
.ui-filter-bar__actions {
  display: flex;
  align-items: center;
  gap: var(--bb-space-2);
}

.ui-filter-bar__controls {
  min-width: 0;
  flex: 1 1 auto;
  flex-wrap: wrap;
}

.ui-filter-bar__controls :deep(> .el-input) { width: 320px; min-width: min(240px, 100%); max-width: 480px; flex: 1 1 280px; }
.ui-filter-bar__controls :deep(> .el-select) { width: 180px; flex: 0 1 180px; }

.ui-filter-bar__actions {
  flex: 0 0 auto;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.ui-filter-bar__message {
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-13);
}

.ui-filter-bar :deep(.el-button) {
  margin: 0;
}

@media (max-width: 760px) {
  .ui-filter-bar { align-items: stretch; flex-direction: column; }
  .ui-filter-bar__controls,
  .ui-filter-bar__actions { width: 100%; }
  .ui-filter-bar__actions { justify-content: space-between; }
  .ui-filter-bar__message { max-width: 100%; }
}
</style>
