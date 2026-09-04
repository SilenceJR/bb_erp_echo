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
.navigation-button { display: flex; width: 100%; min-height: 44px; align-items: center; gap: var(--bb-space-3); border: 0; border-radius: var(--bb-radius-md); background: transparent; padding: 0 var(--bb-space-3); color: var(--bb-text-regular); text-align: left; transition: background var(--bb-duration-fast), color var(--bb-duration-fast); }
.navigation-button:hover { background: var(--bb-bg-sunken); color: var(--bb-text-primary); }
.navigation-button.active { background: var(--bb-bg-surface); color: var(--bb-text-primary); font-weight: var(--bb-font-weight-semibold); box-shadow: inset 2px 0 var(--bb-action-primary); }
.navigation-button__icon { display: grid; width: 20px; flex: 0 0 20px; place-items: center; color: var(--bb-text-secondary); font-size: 17px; }
.navigation-button.active .navigation-button__icon { color: var(--bb-accent-text); }
.navigation-button__icon svg { width: 1em; height: 1em; }
.navigation-button__label { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
