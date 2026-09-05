<template>
  <div class="form-panel-content">
    <slot />
  </div>
</template>

<style scoped>
.form-panel-content {
  display: grid;
  min-width: 0;
  gap: var(--bb-space-6);
}

.form-panel-content :deep(.el-alert) {
  margin: 0;
}
</style>
