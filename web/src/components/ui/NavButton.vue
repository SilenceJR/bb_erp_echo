<template>
  <el-tooltip :content="item.title" placement="right" :disabled="!collapsed">
    <button
      type="button"
      :class="['navigation-button', {active}]"
      :aria-current="active ? 'page' : undefined"
      :aria-label="collapsed ? item.title : undefined"
      @click="emit('select')"
    >
      <span class="navigation-button__icon" aria-hidden="true"><component :is="item.icon" /></span>
      <span v-if="!collapsed" class="navigation-button__label">{{ item.title }}</span>
    </button>
  </el-tooltip>
</template>

<script setup lang="ts">
import type {Component} from 'vue'

export type NavigationDisplayItem = {key: string; title: string; icon: Component}
defineProps<{item: NavigationDisplayItem; active: boolean; collapsed: boolean}>()
const emit = defineEmits<{select: []}>()
</script>

<style scoped>
.navigation-button {
  position: relative;
  display: flex;
  width: 100%;
  height: 40px;
  align-items: center;
  gap: var(--bb-space-3);
  border: 0;
  border-radius: var(--bb-radius-md);
  background: transparent;
  padding: 0 var(--bb-space-3);
  color: var(--bb-text-regular);
  text-align: left;
  transition: background-color var(--bb-duration-fast) var(--bb-ease-standard), color var(--bb-duration-fast) var(--bb-ease-standard);
}
.navigation-button::before {
  position: absolute;
  inset: 10px auto 10px 0;
  width: 2px;
  border-radius: 0 var(--bb-radius-xs) var(--bb-radius-xs) 0;
  background: transparent;
  content: '';
}
.navigation-button:hover { background: color-mix(in srgb, var(--bb-bg-sunken) 54%, transparent); color: var(--bb-text-primary); }
.navigation-button.active { background: var(--bb-accent-selected-bg); color: var(--bb-accent-selected-text); font-weight: var(--bb-font-weight-semibold); }
.navigation-button.active::before { background: var(--bb-action-primary); }
.navigation-button__icon { display: grid; width: 20px; flex: 0 0 20px; place-items: center; color: var(--bb-text-secondary); font-size: 18px; }
.navigation-button.active .navigation-button__icon { color: var(--bb-accent-text); }
.navigation-button__icon svg { width: 1em; height: 1em; }
.navigation-button__label { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
