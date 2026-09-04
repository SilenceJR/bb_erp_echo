<template><div class="property-item"><dt>{{ label }}</dt><dd><slot /></dd></div></template>
<script setup lang="ts">
defineProps<{label?: string}>()
</script>
<style scoped>
.property-item { display: grid; grid-template-columns: 96px minmax(0, 1fr); gap: 16px; min-height: 36px; padding: 8px 0; align-items: baseline; }
dt { color: var(--bb-text-secondary); font-size: 13px; line-height: 20px; font-weight: 400; }
dd { margin: 0; min-width: 0; overflow-wrap: anywhere; color: var(--bb-text-primary); font-size: 14px; line-height: 20px; }
</style>
