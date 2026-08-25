<template>
  <section
    class="ui-page-state"
    :class="`is-${kind}`"
    :role="kind === 'error' ? 'alert' : 'status'"
    :aria-live="kind === 'error' ? 'assertive' : 'polite'"
  >
    <template v-if="kind === 'loading'">
      <span class="bb-sr-only">{{ title }}</span>
      <el-skeleton :rows="4" animated />
    </template>
    <template v-else>
      <span class="ui-page-state__icon" aria-hidden="true">{{ icon }}</span>
      <strong>{{ title }}</strong>
      <p v-if="description">{{ description }}</p>
      <el-button v-if="actionLabel" :type="kind === 'error' ? 'primary' : undefined" @click="emit('action')">
        {{ actionLabel }}
      </el-button>
    </template>
  </section>
</template>

<script setup lang="ts">
import {computed} from 'vue'

export type PageStateKind = 'loading' | 'empty' | 'error' | 'permission' | 'readonly'

const props = withDefaults(defineProps<{
  kind: PageStateKind
  title: string
  description?: string
  actionLabel?: string
}>(), {
  description: '',
  actionLabel: '',
})

const emit = defineEmits<{
  action: []
}>()

const icon = computed(() => ({
  empty: '—',
  error: '!',
  permission: '×',
  readonly: 'i',
  loading: '',
})[props.kind])
</script>

<style scoped>
.ui-page-state {
  display: grid;
  min-height: 240px;
  place-items: center;
  align-content: center;
  gap: var(--bb-space-2);
  border: 1px dashed var(--bb-border-strong);
  border-radius: var(--bb-radius-xl);
  background: var(--bb-bg-surface);
  padding: var(--bb-space-8);
  color: var(--bb-text-secondary);
  text-align: center;
}

.ui-page-state.is-loading {
  display: block;
  min-height: 220px;
  border-style: solid;
}

.ui-page-state__icon {
  display: grid;
  width: 48px;
  height: 48px;
  place-items: center;
  border-radius: 50%;
  background: var(--bb-info-bg);
  color: var(--bb-info);
  font-size: var(--bb-font-size-20);
  font-weight: var(--bb-font-weight-bold);
}

.ui-page-state.is-error .ui-page-state__icon {
  background: var(--bb-danger-bg);
  color: var(--bb-danger);
}

.ui-page-state.is-permission .ui-page-state__icon {
  background: var(--bb-warning-bg);
  color: var(--bb-warning);
}

.ui-page-state strong {
  color: var(--bb-text-primary);
  font-size: var(--bb-font-size-16);
}

.ui-page-state p {
  max-width: 520px;
  margin: 0;
  line-height: var(--bb-line-height-relaxed);
}
</style>
