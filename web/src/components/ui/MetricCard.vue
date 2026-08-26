<template>
  <article class="ui-metric-card" :class="`is-${tone}`" :aria-label="`${label}：${value}`">
    <div class="ui-metric-card__heading">
      <span>{{ label }}</span>
      <StatusTag v-if="statusLabel" :label="statusLabel" :tone="statusTone" />
    </div>
    <strong class="ui-metric-card__value">{{ value }}</strong>
    <p v-if="caption">{{ caption }}</p>
  </article>
</template>

<script setup lang="ts">
import StatusTag, {type StatusTone} from './StatusTag.vue'

export type MetricTone = 'neutral' | StatusTone

withDefaults(defineProps<{
  label: string
  value: string
  caption?: string
  tone?: MetricTone
  statusLabel?: string
  statusTone?: StatusTone
}>(), {
  caption: '',
  tone: 'neutral',
  statusLabel: '',
  statusTone: 'info',
})
</script>

<style scoped>
.ui-metric-card {
  position: relative;
  display: grid;
  min-width: 0;
  min-height: 124px;
  align-content: start;
  gap: var(--bb-space-2);
  overflow: hidden;
  border: 1px solid var(--bb-border-default);
  border-radius: var(--bb-radius-xl);
  background: var(--bb-bg-surface);
  padding: var(--bb-space-4);
  box-shadow: var(--bb-shadow-xs);
}

.ui-metric-card::before {
  position: absolute;
  inset: 0 auto 0 0;
  width: 4px;
  background: var(--ui-metric-accent, var(--bb-brand-500));
  content: '';
}

.ui-metric-card.is-success { --ui-metric-accent: var(--bb-success); }
.ui-metric-card.is-warning { --ui-metric-accent: var(--bb-warning); }
.ui-metric-card.is-danger { --ui-metric-accent: var(--bb-danger); }
.ui-metric-card.is-info { --ui-metric-accent: var(--bb-info); }

.ui-metric-card__heading {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: var(--bb-space-2);
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-12);
}

.ui-metric-card__value {
  overflow-wrap: anywhere;
  color: var(--bb-text-primary);
  font-size: var(--bb-font-size-24);
  font-variant-numeric: tabular-nums;
  line-height: var(--bb-line-height-tight);
}

.ui-metric-card p {
  margin: 0;
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-12);
  line-height: var(--bb-line-height-base);
}
</style>
