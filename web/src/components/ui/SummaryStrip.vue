<template>
  <section
    class="ui-summary-strip"
    :class="[`has-${columns}-columns`, { 'is-compact': compact }]"
    :aria-label="ariaLabel"
  >
    <slot></slot>
  </section>
</template>

<script setup lang="ts">
export type SummaryStripColumns = 2 | 3 | 4 | 5 | 6

withDefaults(defineProps<{
  ariaLabel?: string
  columns?: SummaryStripColumns
  compact?: boolean
}>(), {
  ariaLabel: '关键指标',
  columns: 4,
  compact: false,
})
</script>

<style scoped>
.ui-summary-strip {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--bb-space-3);
  margin-bottom: var(--bb-space-4);
}

.ui-summary-strip.has-2-columns { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.ui-summary-strip.has-3-columns { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.ui-summary-strip.has-5-columns { grid-template-columns: repeat(5, minmax(0, 1fr)); }
.ui-summary-strip.has-6-columns { grid-template-columns: repeat(6, minmax(0, 1fr)); }
.ui-summary-strip.is-compact { gap: var(--bb-space-2); }

@media (max-width: 1180px) {
  .ui-summary-strip.has-5-columns,
  .ui-summary-strip.has-6-columns {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
</style>
