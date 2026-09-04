<template>
  <nav class="ui-app-navigation" :class="`is-${mode}`" :aria-label="ariaLabel">
    <div class="ui-app-navigation__items">
      <NavButton :item="dashboardItem" :active="activeKey === 'dashboard'" :collapsed="collapsed" @select="emit('select', 'dashboard')" />

      <section v-for="group in navigationGroups" :key="group.key" class="navigation-group" :aria-label="group.label">
        <h2 v-if="!collapsed">{{ group.label }}</h2>
        <NavButton
          v-for="item in group.items"
          :key="item.key"
          :item="item"
          :active="activeKey === item.key"
          :collapsed="collapsed"
          @select="emit('select', item.key)"
        />
      </section>
    </div>

    <div class="ui-app-navigation__footer">
      <el-tooltip content="设置" placement="right" :disabled="!collapsed">
        <button class="navigation-button" type="button" aria-label="打开设置" @click="emit('settings')">
          <span class="navigation-button__icon" aria-hidden="true"><Setting /></span>
          <span v-if="!collapsed" class="navigation-button__label">设置</span>
        </button>
      </el-tooltip>
      <div v-if="!collapsed" class="ui-app-navigation__help">
        <span>遇到问题？</span>
        <small>请联系系统管理员</small>
      </div>
    </div>
  </nav>
</template>

<script setup lang="ts">
import {computed, toRefs, type Component} from 'vue'
import {Avatar, Box, Coin, Document, House, Key, Monitor, OfficeBuilding, Setting, Tickets, TrendCharts, Upload, UserFilled, Van} from '@element-plus/icons-vue'
import type {SidebarMode} from '../../platform/appearance'
import NavButton, {type NavigationDisplayItem} from './NavButton.vue'

type NavigationItem = {key: string; title: string}

const props = withDefaults(defineProps<{
  activeKey: string
  businessItems: NavigationItem[]
  systemItems: NavigationItem[]
  mode?: SidebarMode
  ariaLabel?: string
}>(), {
  mode: 'full',
  ariaLabel: '系统导航',
})
const {activeKey, ariaLabel, mode} = toRefs(props)
const emit = defineEmits<{select: [key: string]; settings: []}>()
const collapsed = computed(() => mode.value === 'icon')

const icons: Record<string, Component> = {
  dashboard: House, workorder: Tickets, warehouses: Box, molds: Coin, customers: UserFilled, suppliers: Van, statistics: TrendCharts,
  departments: OfficeBuilding, employees: Avatar, users: UserFilled, terminals: Monitor, roles: Key, permissions: Key, audits: Document, updates: Upload,
}
const withIcon = (item: NavigationItem): NavigationDisplayItem => ({...item, icon: icons[item.key] || Document})
const dashboardItem = withIcon({key: 'dashboard', title: '首页'})
const keys = {
  operation: ['workorder', 'warehouses', 'molds'],
  master: ['customers', 'suppliers'],
  report: ['statistics'],
}
const navigationGroups = computed(() => [
  {key: 'operation', label: '业务办理', items: keys.operation.map(findBusiness).filter(Boolean)},
  {key: 'master', label: '基础资料', items: keys.master.map(findBusiness).filter(Boolean)},
  {key: 'report', label: '数据与报表', items: keys.report.map(findBusiness).filter(Boolean)},
  {key: 'system', label: '系统管理', items: props.systemItems.map(withIcon)},
].filter((group) => group.items.length) as {key: string; label: string; items: NavigationDisplayItem[]}[])

function findBusiness(key: string) {
  const item = props.businessItems.find((candidate) => candidate.key === key)
  return item ? withIcon(item) : null
}
</script>

<style scoped>
.ui-app-navigation { display: flex; min-height: 100%; flex-direction: column; }
.ui-app-navigation__items { display: grid; gap: var(--bb-space-1); }
.navigation-group { display: grid; gap: var(--bb-space-1); padding-top: var(--bb-space-3); }
.navigation-group h2 { margin: var(--bb-space-2) var(--bb-space-3) var(--bb-space-1); color: var(--bb-text-placeholder); font-size: 11px; font-weight: var(--bb-font-weight-semibold); letter-spacing: .06em; }
.ui-app-navigation__footer { display: grid; gap: var(--bb-space-2); margin-top: auto; border-top: 1px solid var(--bb-border-subtle); padding-top: var(--bb-space-3); }
.navigation-button { display: flex; width: 100%; min-height: 44px; align-items: center; gap: var(--bb-space-3); border: 0; border-radius: var(--bb-radius-md); background: transparent; padding: 0 var(--bb-space-3); color: var(--bb-text-regular); text-align: left; }
.navigation-button { position: relative; transition: background-color var(--bb-duration-fast) var(--bb-ease-standard), color var(--bb-duration-fast) var(--bb-ease-standard); }
.navigation-button:hover { background: color-mix(in srgb, var(--bb-bg-sunken) 54%, transparent); color: var(--bb-text-primary); }
.navigation-button__icon { display: grid; width: 20px; flex: 0 0 20px; place-items: center; color: var(--bb-text-secondary); font-size: 17px; }
.navigation-button__icon svg { width: 1em; height: 1em; }
.navigation-button__label { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ui-app-navigation__help { display: grid; gap: var(--bb-space-1); border-top: 1px solid var(--bb-border-subtle); padding: var(--bb-space-3); color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); }
.ui-app-navigation__help small { color: var(--bb-text-placeholder); }
.is-icon .navigation-group { padding-top: var(--bb-space-1); }
.is-icon :deep(.navigation-button) { justify-content: center; padding: 0; }
</style>
