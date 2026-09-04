<template>
  <section
    class="ui-page-state"
    :class="[`is-${kind}`, { 'is-compact': compact }]"
    :role="kind === 'error' ? 'alert' : 'status'"
    :aria-live="kind === 'error' ? 'assertive' : 'polite'"
  >
    <template v-if="kind === 'loading'">
      <span class="bb-sr-only">{{ title }}</span>
      <el-skeleton :rows="4" animated />
    </template>
    <template v-else>
      <el-icon class="ui-page-state__icon" aria-hidden="true"><component :is="icon" /></el-icon>
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
import {CircleClose, FolderOpened, InfoFilled, Lock} from '@element-plus/icons-vue'

export type PageStateKind = 'loading' | 'empty' | 'error' | 'permission' | 'readonly'

const props = withDefaults(defineProps<{
  kind: PageStateKind
  title: string
  description?: string
  actionLabel?: string
  compact?: boolean
}>(), {
  description: '',
  actionLabel: '',
  compact: false,
})

const emit = defineEmits<{
  action: []
}>()

const icon = computed(() => ({
  empty: FolderOpened,
  error: CircleClose,
  permission: Lock,
  readonly: InfoFilled,
  loading: InfoFilled,
})[props.kind])
</script>

<style scoped>
.ui-page-state {
  display: grid;
  min-height: 240px;
  place-items: center;
  align-content: center;
  gap: var(--bb-space-2);
  border: 1px solid var(--bb-border-default);
  border-radius: var(--bb-radius-md);
  background: transparent;
  padding: var(--bb-space-8);
  color: var(--bb-text-secondary);
  text-align: center;
}

.ui-page-state.is-loading {
  display: block;
  min-height: 220px;
  border-style: solid;
}

.ui-page-state.is-compact {
  min-height: 160px;
  padding: var(--bb-space-5);
}

.ui-page-state__icon {
  display: grid;
  width: 48px;
  height: 48px;
  place-items: center;
  background: transparent;
  color: var(--bb-info);
  font-size: var(--bb-font-size-24);
  font-weight: var(--bb-font-weight-bold);
}

.ui-page-state.is-error .ui-page-state__icon {
  color: var(--bb-danger);
}

.ui-page-state.is-permission .ui-page-state__icon {
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
