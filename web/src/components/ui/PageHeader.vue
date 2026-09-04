<template>
  <header class="ui-page-header">
    <nav v-if="showBack" class="ui-page-header__breadcrumb" aria-label="面包屑">
      <el-button link @click="emit('back')">{{ parentLabel }}</el-button>
      <span aria-hidden="true">/</span>
      <span aria-current="page">{{ title }}</span>
    </nav>

    <div class="ui-page-header__main">
      <div class="ui-page-header__copy">
        <span v-if="eyebrow" class="ui-page-header__eyebrow">{{ eyebrow }}</span>
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
  eyebrow?: string
  parentLabel?: string
  showBack?: boolean
}>(), {
  description: '',
  readonly: false,
  eyebrow: '',
  parentLabel: '首页',
  showBack: false,
})

const emit = defineEmits<{
  back: []
}>()
</script>

<style scoped>
.ui-page-header {
  display: grid;
  gap: var(--bb-space-1);
  margin-bottom: var(--bb-space-6);
}

.ui-page-header__breadcrumb {
  display: flex;
  align-items: center;
  gap: var(--bb-space-2);
  min-height: 24px;
  color: var(--bb-text-placeholder);
  font-size: var(--bb-font-size-12);
}

.ui-page-header__breadcrumb .el-button {
  padding: 0;
  color: var(--bb-text-secondary);
}

.ui-page-header__main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--bb-space-4);
  flex-wrap: wrap;
}

.ui-page-header__copy h1 {
  margin: 0;
  color: var(--bb-text-primary);
  font-size: 24px;
  letter-spacing: -.018em;
  line-height: 32px;
}

.ui-page-header__eyebrow {
  display: block;
  margin-bottom: var(--bb-space-1);
  color: var(--bb-accent-text);
  font-size: var(--bb-font-size-12);
  font-weight: var(--bb-font-weight-bold);
  letter-spacing: .08em;
  text-transform: uppercase;
}

.ui-page-header__copy p {
  max-width: 680px;
  margin: var(--bb-space-1) 0 0;
  color: var(--bb-text-secondary);
  font-size: 13px;
}

.ui-page-header__actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: var(--bb-space-2);
}

.ui-page-header__actions :deep(.el-button) {
  margin: 0;
}

@media (max-width: 1023px) {
  .ui-page-header__main {
    align-items: flex-start;
    flex-direction: column;
    gap: var(--bb-space-4);
  }

  .ui-page-header__actions {
    width: 100%;
    flex-wrap: wrap;
  }
}
</style>
