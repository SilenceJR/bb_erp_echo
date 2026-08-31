<template>
  <main
    class="ui-page-scaffold"
    :class="[`is-${width}`, `is-${density}`]"
    :aria-label="ariaLabel || undefined"
  >
    <slot name="header"></slot>
    <slot name="summary"></slot>
    <div class="ui-page-scaffold__body">
      <slot></slot>
    </div>
  </main>
</template>

<script setup lang="ts">
export type PageScaffoldWidth = 'content' | 'wide' | 'fluid'
export type PageScaffoldDensity = 'standard' | 'compact'

withDefaults(defineProps<{
  ariaLabel?: string
  width?: PageScaffoldWidth
  density?: PageScaffoldDensity
}>(), {
  ariaLabel: '',
  width: 'content',
  density: 'standard',
})
</script>

<style scoped>
.ui-page-scaffold {
  width: min(100%, var(--bb-content-max-width));
  margin: 0 auto;
}

.ui-page-scaffold.is-wide { width: min(100%, var(--bb-content-wide-width)); }
.ui-page-scaffold.is-fluid { width: 100%; }

.ui-page-scaffold__body {
  display: grid;
  min-width: 0;
  gap: var(--bb-space-4);
}

.ui-page-scaffold.is-compact .ui-page-scaffold__body {
  gap: var(--bb-space-3);
}
</style>
