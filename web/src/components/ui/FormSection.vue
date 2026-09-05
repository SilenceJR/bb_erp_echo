<template>
  <section class="form-panel-section" :class="{'is-divided': divided}">
    <header v-if="title || description || eyebrow" class="form-panel-section__header">
      <span v-if="eyebrow" class="form-panel-section__eyebrow">{{ eyebrow }}</span>
      <h3 v-if="title">{{ title }}</h3>
      <p v-if="description">{{ description }}</p>
    </header>
    <slot />
  </section>
</template>

<script setup lang="ts">
defineProps<{
  title?: string
  description?: string
  eyebrow?: string
  divided?: boolean
}>()
</script>

<style scoped>
.form-panel-section {
  display: grid;
  min-width: 0;
  container-type: inline-size;
  gap: var(--bb-space-4);
}

.form-panel-section.is-divided {
  border-top: 1px solid var(--bb-border-subtle);
  padding-top: var(--bb-space-5);
}

.form-panel-section__header {
  display: grid;
  min-width: 0;
  gap: var(--bb-space-1);
}

.form-panel-section__eyebrow {
  color: var(--bb-accent-text);
  font-size: var(--bb-font-size-12);
  font-weight: var(--bb-font-weight-semibold);
}

.form-panel-section__header h3 {
  margin: 0;
  color: var(--bb-text-primary);
  font-size: var(--bb-font-size-16);
  line-height: var(--bb-line-height-tight);
}

.form-panel-section__header p {
  margin: 0;
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-13);
  line-height: var(--bb-line-height-relaxed);
}
</style>
