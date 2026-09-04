<template>
  <ResponsiveDetailCarrier
    v-model="visible"
    drawer-class="generic-record-detail-drawer workspace-detail-drawer"
    :docked="docked"
    :size="size"
    :title="title"
    docked-auto-focus="preserve"
    destroy-on-close
    @closed="emit('closed')"
  >
    <section v-if="item" class="generic-record-detail" :aria-label="`${title}内容`">
      <div class="generic-record-detail__heading">
        <span class="generic-record-detail__eyebrow">{{ eyebrow || title }}</span>
        <h3>{{ primary }}</h3>
        <p v-if="subtitle">{{ subtitle }}</p>
      </div>
      <PropertyList>
        <PropertyItem v-for="field in fields" :key="field.key" :label="field.label">
          <span :class="{'generic-record-detail__mono': field.mono}">{{ field.value }}</span>
        </PropertyItem>
      </PropertyList>
    </section>
    <template #footer>
      <div class="generic-record-detail__actions">
        <slot name="footer">
          <el-button @click="visible = false">关闭</el-button>
        </slot>
      </div>
    </template>
  </ResponsiveDetailCarrier>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useResponsiveDetailPanel} from '../../../composables/useResponsiveDetailPanel'
import PropertyItem from '../../ui/PropertyItem.vue'
import PropertyList from '../../ui/PropertyList.vue'
import ResponsiveDetailCarrier from '../../ui/ResponsiveDetailCarrier.vue'

export type GenericRecordDetailField = {key: string; label: string; value: string; mono?: boolean}

const props = defineProps<{
  modelValue: boolean
  title: string
  eyebrow?: string
  primary: string
  subtitle?: string
  item: Record<string, unknown> | null
  fields: GenericRecordDetailField[]
}>()
const emit = defineEmits<{
  (event: 'update:modelValue', value: boolean): void
  (event: 'closed'): void
}>()
const visible = computed({get: () => props.modelValue, set: (value) => emit('update:modelValue', value)})
const {docked, size} = useResponsiveDetailPanel(visible, false)
</script>

<style scoped>
.generic-record-detail { display: grid; min-width: 0; gap: var(--bb-space-5); }
.generic-record-detail__heading { min-width: 0; }
.generic-record-detail__eyebrow { color: var(--bb-accent-text); font-size: var(--bb-font-size-13); font-weight: var(--bb-font-weight-semibold); }
.generic-record-detail__heading h3 { margin: var(--bb-space-1) 0 0; color: var(--bb-text-primary); font-size: var(--bb-font-size-20); line-height: var(--bb-line-height-tight); overflow-wrap: anywhere; }
.generic-record-detail__heading p { margin: var(--bb-space-2) 0 0; color: var(--bb-text-secondary); line-height: var(--bb-line-height-relaxed); overflow-wrap: anywhere; }
.generic-record-detail__mono { font-family: var(--bb-font-mono); overflow-wrap: anywhere; }
.generic-record-detail__actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: var(--bb-space-2); }
</style>
