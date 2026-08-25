<template>
  <header class="ui-page-header">
    <nav class="ui-page-header__breadcrumb" aria-label="面包屑">
      <el-button link @click="emit('back')">首页</el-button>
      <span aria-hidden="true">/</span>
      <span aria-current="page">{{ title }}</span>
    </nav>

    <div class="ui-page-header__main">
      <div class="ui-page-header__copy">
        <h1>{{ title }}</h1>
        <p v-if="description">{{ description }}</p>
      </div>
      <div class="ui-page-header__actions">
        <StatusTag v-if="readonly" label="仅查看" tone="info" />
        <slot name="actions"></slot>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import StatusTag from './StatusTag.vue'

withDefaults(defineProps<{
  title: string
  description?: string
  readonly?: boolean
}>(), {
  description: '',
  readonly: false,
})

const emit = defineEmits<{
  back: []
}>()
</script>

<style scoped>
.ui-page-header {
  display: grid;
  gap: var(--bb-space-2);
  margin-bottom: var(--bb-space-6);
}

.ui-page-header__breadcrumb {
  display: flex;
  align-items: center;
  gap: var(--bb-space-2);
  color: var(--bb-text-placeholder);
  font-size: var(--bb-font-size-13);
}

.ui-page-header__breadcrumb .el-button {
  padding: 0;
  color: var(--bb-text-secondary);
}

.ui-page-header__main {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--bb-space-6);
}

.ui-page-header__copy h1 {
  margin: 0;
  color: var(--bb-text-primary);
  font-size: var(--bb-font-size-30);
  line-height: var(--bb-line-height-tight);
}

.ui-page-header__copy p {
  margin: var(--bb-space-2) 0 0;
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-14);
}

.ui-page-header__actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: var(--bb-space-2);
}

@media (max-width: 560px) {
  .ui-page-header__main {
    align-items: stretch;
    flex-direction: column;
  }

  .ui-page-header__copy h1 {
    font-size: var(--bb-font-size-24);
  }

  .ui-page-header__actions {
    align-items: stretch;
    flex-direction: column;
  }

  .ui-page-header__actions :deep(.el-button) {
    width: 100%;
  }
}
</style>
