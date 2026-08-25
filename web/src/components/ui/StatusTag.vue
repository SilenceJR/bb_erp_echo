<template>
  <el-tag class="ui-status-tag" :type="elementType" effect="light" round>
    <span class="ui-status-tag__dot" aria-hidden="true"></span>
    <span>{{ label }}</span>
  </el-tag>
</template>

<script setup lang="ts">
import {computed} from 'vue'

export type StatusTone = 'success' | 'warning' | 'danger' | 'info'

const props = withDefaults(defineProps<{
  label: string
  tone?: StatusTone
}>(), {
  tone: 'info',
})

const elementType = computed(() => props.tone)
</script>

<style scoped>
.ui-status-tag {
  min-height: 24px;
  font-weight: var(--bb-font-weight-semibold);
}

.ui-status-tag :deep(.el-tag__content) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.ui-status-tag__dot {
  width: 6px;
  height: 6px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: currentColor;
}
</style>
