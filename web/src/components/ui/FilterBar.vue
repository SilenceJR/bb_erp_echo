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
  min-height: 58px;
  align-items: center;
  justify-content: space-between;
  gap: var(--bb-space-3);
  margin-bottom: var(--bb-space-3);
  border: 1px solid var(--bb-border-default);
  border-radius: var(--bb-radius-md);
  background: var(--bb-bg-surface);
  padding: var(--bb-space-2) var(--bb-space-3);
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

.ui-filter-bar__actions {
  flex: 0 0 auto;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.ui-filter-bar__message {
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-13);
}

.ui-filter-bar :deep(.el-button) {
  margin: 0;
}
</style>
