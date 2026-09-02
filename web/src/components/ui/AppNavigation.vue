<template>
  <nav class="ui-app-navigation" :aria-label="ariaLabel">
    <el-menu class="ui-app-navigation__menu" :default-active="activeKey" @select="handleSelect">
      <el-menu-item index="dashboard">首页</el-menu-item>
      <el-menu-item-group v-if="operationItems.length" title="业务办理">
        <el-menu-item v-for="item in operationItems" :key="item.key" :index="item.key">{{ item.title }}</el-menu-item>
      </el-menu-item-group>
      <el-menu-item-group v-if="masterDataItems.length" title="基础资料">
        <el-menu-item v-for="item in masterDataItems" :key="item.key" :index="item.key">{{ item.title }}</el-menu-item>
      </el-menu-item-group>
      <el-menu-item-group v-if="reportItems.length" title="数据与报表">
        <el-menu-item v-for="item in reportItems" :key="item.key" :index="item.key">{{ item.title }}</el-menu-item>
      </el-menu-item-group>
      <el-sub-menu v-if="systemItems.length" index="system">
        <template #title>系统设置</template>
        <el-menu-item v-for="item in systemItems" :key="item.key" :index="item.key">
          {{ item.title }}
        </el-menu-item>
      </el-sub-menu>
    </el-menu>

    <div class="ui-app-navigation__help">
      <span>遇到问题？</span>
      <small>请联系系统管理员</small>
    </div>
  </nav>
</template>

<script setup lang="ts">
import {computed, toRefs} from 'vue'

type NavigationItem = {
  key: string
  title: string
}

const props = withDefaults(defineProps<{
  activeKey: string
  businessItems: NavigationItem[]
  systemItems: NavigationItem[]
  ariaLabel?: string
}>(), {
  ariaLabel: '系统导航',
})
const {activeKey, systemItems, ariaLabel} = toRefs(props)

const operationKeys = ['workorder', 'warehouses', 'molds']
const masterDataKeys = ['customers', 'suppliers']
const operationItems = computed(() => operationKeys.map((key) => props.businessItems.find((item) => item.key === key)).filter((item): item is NavigationItem => Boolean(item)))
const masterDataItems = computed(() => masterDataKeys.map((key) => props.businessItems.find((item) => item.key === key)).filter((item): item is NavigationItem => Boolean(item)))
const reportItems = computed(() => props.businessItems.filter((item) => item.key === 'statistics'))

const emit = defineEmits<{
  select: [key: string]
}>()

function handleSelect(key: string) {
  emit('select', key)
}
</script>

<style scoped>
.ui-app-navigation {
  display: flex;
  min-height: 100%;
  flex-direction: column;
}

.ui-app-navigation__menu {
  border-right: 0;
}

.ui-app-navigation__menu :deep(.el-menu-item),
.ui-app-navigation__menu :deep(.el-sub-menu__title) {
  min-height: 42px;
  height: 42px;
  margin: var(--bb-space-1) 0;
  border-radius: var(--bb-radius-md);
  line-height: 42px;
}

.ui-app-navigation__menu :deep(.el-menu-item.is-active) {
  background: var(--bb-brand-50);
  color: var(--bb-brand-700);
  font-weight: var(--bb-font-weight-bold);
}

.ui-app-navigation__menu :deep(.el-menu-item-group__title) {
  padding: var(--bb-space-5) var(--bb-space-3) var(--bb-space-2) !important;
  color: var(--bb-text-placeholder);
  font-size: var(--bb-font-size-12);
  font-weight: var(--bb-font-weight-bold);
  letter-spacing: 0.1em;
}

.ui-app-navigation__help {
  display: grid;
  gap: var(--bb-space-1);
  margin-top: auto;
  border-radius: var(--bb-radius-lg);
  background: var(--bb-bg-sunken);
  padding: var(--bb-space-3);
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-12);
}

.ui-app-navigation__help small {
  color: var(--bb-text-placeholder);
}
</style>
