<template>
  <div class="data-page">
    <PageHeader v-if="activeKey !== 'molds'"
      :title="activeModule?.title || '业务页面'"
      :description="activeModule?.description"
      :readonly="activePageReadonly"
      @back="switchModule('dashboard')"
    >
      <template #actions>
        <el-button v-if="activeKey === 'statistics'" :loading="loading" @click="loadActiveModule">刷新</el-button>
        <el-button v-if="activeKey !== 'molds' && formSchema.length && canWriteActive && !moduleUnavailable" class="add-button" type="primary" :disabled="loading || showCreateForm" @click="toggleCreateForm">
          {{ `＋ 新增${createEntityTitle}` }}
        </el-button>
      </template>
    </PageHeader>

    <div v-if="activeKey === 'warehouses' && !moduleUnavailable" class="warehouse-tabs" aria-label="仓库分类">
      <el-segmented v-model="activeWarehouseTab" :options="warehouseTabOptions" @change="switchWarehouseTab" />
    </div>

    <ModuleCreateForm v-if="activeKey !== 'molds'" />
    <ModuleAssignmentDialogs />

    <FilterBar
      v-if="activeKey !== 'updates' && activeKey !== 'molds' && activeKey !== 'statistics' && !moduleUnavailable"
      :message="panelMessage"
      :loading="loading"
      :resettable="hasActiveFilters"
      :aria-label="`${activeModule?.title || '数据'}筛选`"
      @submit="applySearch"
      @reset="resetFilters"
      @refresh="loadActiveModule"
    >
      <el-input v-model.trim="searchKeyword" class="keyword-input" clearable :placeholder="listSearchPlaceholder" aria-label="关键词" />
      <template v-if="activeKey === 'workorder'">
        <el-select v-model="workorderStatusFilter" class="filter-select" placeholder="状态" aria-label="任务状态" @change="applySearch">
          <el-option v-for="option in workorderStatusOptions" :key="option.value" :label="option.label" :value="option.value" />
        </el-select>
        <el-select v-model="workorderTypeFilter" class="filter-select" placeholder="类型" aria-label="任务类型" @change="applySearch">
          <el-option v-for="option in workorderTypeOptions" :key="option.value" :label="option.label" :value="option.value" />
        </el-select>
        <el-select v-model="workorderPriorityFilter" class="filter-select" placeholder="优先级" aria-label="任务优先级" @change="applySearch">
          <el-option v-for="option in workorderPriorityOptions" :key="option.value" :label="option.label" :value="option.value" />
        </el-select>
      </template>
    </FilterBar>

    <section v-if="activeKey !== 'molds' && !moduleUnavailable && operationalSummaryCards.length && !loading && rows.length" class="operational-summary-grid" :aria-label="`${activeModule?.title || '业务'}当前页摘要`">
      <MetricCard v-for="card in operationalSummaryCards" :key="card.label" v-bind="card" />
    </section>

    <PageState
      v-if="moduleUnavailable"
      kind="readonly"
      :title="`${activeModule?.title || '此功能'}暂不可用`"
      description="当前服务尚未启用此功能，暂时无法查看或办理业务。"
      action-label="重新检查"
      @action="loadActiveModule"
    />
    <PageState v-else-if="skeletonResult" kind="readonly" :title="skeletonResult.name" :description="skeletonResult.message" />
    <UpdateCenter v-else-if="activeKey === 'updates'" :token="token" :can-check="hasPermission('system:updates:write')" />
    <PageState v-else-if="listError && !hasRenderableData" kind="error" title="数据加载失败" :description="listError" action-label="重新加载" @action="loadActiveModule" />
    <div v-show="!moduleUnavailable && !skeletonResult && activeKey !== 'updates' && !(listError && !hasRenderableData)" class="module-page-content">
        <component :is="content" />
    </div>
  </div>
</template>

<script setup lang="ts">
import type {Component} from 'vue'
import UpdateCenter from '../../UpdateCenter.vue'
import FilterBar from '../../ui/FilterBar.vue'
import MetricCard from '../../ui/MetricCard.vue'
import PageHeader from '../../ui/PageHeader.vue'
import PageState from '../../ui/PageState.vue'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import {useWorkorderContext} from '../../../composables/workorderContext'
import ModuleAssignmentDialogs from './ModuleAssignmentDialogs.vue'
import ModuleCreateForm from './ModuleCreateForm.vue'

defineProps<{content: Component}>()

const {
  activeKey, activeModule, activePageReadonly, canWriteActive, formSchema,
  showCreateForm, toggleCreateForm, createEntityTitle, activeWarehouseTab,
  warehouseTabOptions, switchWarehouseTab, panelMessage, loading,
  hasActiveFilters, applySearch, resetFilters, loadActiveModule, searchKeyword,
  listSearchPlaceholder, operationalSummaryCards, rows, skeletonResult,
  token, hasPermission, listError, hasRenderableData, switchModule, moduleUnavailable,
} = useWorkspaceContext()
const {
  workorderStatusFilter, workorderTypeFilter, workorderPriorityFilter,
  workorderStatusOptions, workorderTypeOptions, workorderPriorityOptions,
} = useWorkorderContext().list
</script>

<style scoped>
.module-page-content { min-width: 0; }
</style>
