<template>
  <div class="form-panel-grid" :class="`is-${columns}`">
    <slot />
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  columns?: 'one' | 'two'
}>(), {columns: 'one'})
</script>

<style scoped>
.form-panel-grid {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(0, 1fr);
  gap: 0 var(--bb-space-4);
}

.form-panel-grid :deep(.form-grid-full) {
  grid-column: 1 / -1;
}

@container (min-width: 440px) {
  .form-panel-grid.is-two {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
