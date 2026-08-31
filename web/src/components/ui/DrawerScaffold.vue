<template>
  <el-drawer
    :model-value="modelValue"
    class="ui-drawer-scaffold"
    :title="title"
    :size="size"
    :with-header="true"
    :destroy-on-close="destroyOnClose"
    :close-on-click-modal="!busy && closeOnClickModal"
    :close-on-press-escape="!busy && closeOnPressEscape"
    @update:model-value="emit('update:modelValue', $event)"
    @opened="emit('opened')"
    @closed="emit('closed')"
  >
    <template #header>
      <slot name="header">
        <div class="ui-drawer-scaffold__header">
          <div>
            <h2>{{ title }}</h2>
            <p v-if="description">{{ description }}</p>
          </div>
          <el-button :disabled="busy" aria-label="关闭抽屉" @click="emit('update:modelValue', false)">关闭</el-button>
        </div>
      </slot>
    </template>

    <div class="ui-drawer-scaffold__body" :aria-busy="busy">
      <slot></slot>
    </div>

    <template v-if="slots.footer" #footer>
      <div class="ui-drawer-scaffold__footer">
        <slot name="footer"></slot>
      </div>
    </template>
  </el-drawer>
</template>

<script setup lang="ts">
import {useSlots} from 'vue'

withDefaults(defineProps<{
  modelValue: boolean
  title: string
  description?: string
  size?: string | number
  busy?: boolean
  destroyOnClose?: boolean
  closeOnClickModal?: boolean
  closeOnPressEscape?: boolean
}>(), {
  description: '',
  size: 'min(680px, 72vw)',
  busy: false,
  destroyOnClose: true,
  closeOnClickModal: true,
  closeOnPressEscape: true,
})

const slots = useSlots()
const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  opened: []
  closed: []
}>()
</script>

<style scoped>
.ui-drawer-scaffold__header {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--bb-space-4);
  border-bottom: 1px solid var(--bb-border-subtle);
  padding-bottom: var(--bb-space-4);
}

.ui-drawer-scaffold__header > div { min-width: 0; }
.ui-drawer-scaffold__header h2 { margin: 0; color: var(--bb-text-primary); font-size: var(--bb-font-size-20); }
.ui-drawer-scaffold__header p { margin: var(--bb-space-2) 0 0; color: var(--bb-text-secondary); line-height: var(--bb-line-height-base); }
.ui-drawer-scaffold__body { min-width: 0; }
.ui-drawer-scaffold__footer { display: flex; align-items: center; justify-content: flex-end; gap: var(--bb-space-2); }
.ui-drawer-scaffold__footer :deep(.el-button) { margin: 0; }
</style>

<style>
.ui-drawer-scaffold.el-drawer .el-drawer__header {
  margin-bottom: 0;
  padding: var(--bb-space-6) var(--bb-space-6) var(--bb-space-4);
}

.ui-drawer-scaffold.el-drawer .el-drawer__body {
  min-width: 0;
  padding: var(--bb-space-4) var(--bb-space-6) var(--bb-space-6);
}

.ui-drawer-scaffold.el-drawer .el-drawer__footer {
  border-top: 1px solid var(--bb-border-subtle);
  padding: var(--bb-space-4) var(--bb-space-6);
}
</style>
