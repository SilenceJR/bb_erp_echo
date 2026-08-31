<template>
  <footer class="ui-form-action-bar" :class="[{ 'is-sticky': sticky }, `is-${align}`]">
    <div class="ui-form-action-bar__message" aria-live="polite">
      <slot name="message">{{ message }}</slot>
    </div>
    <div class="ui-form-action-bar__actions">
      <slot></slot>
    </div>
  </footer>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  message?: string
  sticky?: boolean
  align?: 'start' | 'end' | 'between'
}>(), {
  message: '',
  sticky: false,
  align: 'end',
})
</script>

<style scoped>
.ui-form-action-bar {
  display: flex;
  min-height: var(--bb-control-lg);
  align-items: center;
  gap: var(--bb-space-3);
  margin-top: var(--bb-space-5);
  border-top: 1px solid var(--bb-border-subtle);
  background: var(--bb-bg-surface);
  padding: var(--bb-space-4) 0;
}

.ui-form-action-bar.is-sticky {
  position: sticky;
  bottom: 0;
  z-index: var(--bb-z-sticky);
}

.ui-form-action-bar.is-start { justify-content: flex-start; }
.ui-form-action-bar.is-end { justify-content: flex-end; }
.ui-form-action-bar.is-between { justify-content: space-between; }

.ui-form-action-bar__message {
  margin-right: auto;
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-13);
}

.ui-form-action-bar__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: var(--bb-space-2);
}

.ui-form-action-bar__actions :deep(.el-button) { margin: 0; }
</style>
