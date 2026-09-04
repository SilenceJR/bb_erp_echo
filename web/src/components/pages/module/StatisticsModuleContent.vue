<template>
  <section v-loading="loading" class="statistics-page">
    <el-alert v-if="listError && statisticsData" :title="listError" type="error" :closable="false" show-icon />
    <PageState v-if="loading && !statisticsData" kind="loading" title="正在加载统计" />
    <template v-else-if="statisticsData">
      <el-alert
        v-if="statisticsSourcesUnavailable"
        class="statistics-source-alert"
        title="部分统计暂不可用"
        :description="`${unavailableSourceLabels}暂不可用，以下仅显示可用数据。`"
        type="warning"
        :closable="false"
        show-icon
      />
      <div class="report-overview-heading"><div><h2>经营概览</h2><p>更新时间：{{ formatDate(statisticsData?.generated_at) }}</p></div><StatusTag :label="statisticsData?.can_view_cost ? '可查看成本金额' : '成本金额已隐藏'" :tone="statisticsData?.can_view_cost ? 'success' : 'info'" /></div>
      <div v-if="statisticsCards.length" class="stats-grid"><MetricCard v-for="card in statisticsCards" :key="card.label" v-bind="card" /></div>

      <div v-if="statisticsData?.inventory?.by_item_type?.length || statisticsData?.workorders?.by_status?.length || statisticsData?.workorders?.by_department?.length || statisticsData?.molds?.by_type?.length" class="report-grid">
        <section v-if="!statisticsSourceUnavailable('inventory') && statisticsData?.inventory?.by_item_type?.length" class="report-panel">
          <div class="drawer-section-title"><h3>库存分类</h3><small>{{ statisticsData?.can_view_cost ? '含库存金额' : '金额已按权限隐藏' }}</small></div>
          <div v-if="statisticsData?.inventory?.by_item_type?.length" class="metric-list"><article v-for="item in statisticsData.inventory.by_item_type" :key="String(item.name)"><span>{{ inventoryItemTypeLabel(item.name) }}</span><strong>{{ formatQuantity(item.value) }}</strong><small v-if="statisticsData.can_view_cost">{{ formatMoney(item.amount) }}</small></article></div><p v-else class="report-empty">暂无库存分类数据</p>
        </section>
        <section v-if="!statisticsSourceUnavailable('workorders') && statisticsData?.workorders?.by_status?.length" class="report-panel">
          <div class="drawer-section-title"><h3>任务状态</h3><small>主任务</small></div>
          <div v-if="statisticsData?.workorders?.by_status?.length" class="metric-list"><article v-for="item in statisticsData.workorders.by_status" :key="String(item.name)"><span>{{ workorderStatusLabel(item.name) }}</span><strong>{{ item.value }}</strong></article></div><p v-else class="report-empty">暂无任务状态数据</p>
        </section>
        <section v-if="!statisticsSourceUnavailable('workorders') && statisticsData?.workorders?.by_department?.length" class="report-panel">
          <div class="drawer-section-title"><h3>部门处理</h3><small>子任务</small></div>
          <div v-if="statisticsData?.workorders?.by_department?.length" class="department-stat-list"><article v-for="item in statisticsData.workorders.by_department" :key="Number(item.department_id)"><div><strong>{{ item.name || departmentName(item.department_id) }}</strong><small>共 {{ item.total }} 项</small></div><el-progress :percentage="departmentCompletionRate(item)" :stroke-width="8" /><small>完成 {{ item.completed }} · 处理中 {{ item.processing }} · 部分完成 {{ item.partial }} · 已收到 {{ item.received }}</small></article></div><p v-else class="report-empty">暂无部门处理数据</p>
        </section>
        <section v-if="statisticsData?.molds?.by_type?.length" class="report-panel">
          <div class="drawer-section-title"><h3>模具类型</h3><small>产品档案</small></div>
          <div v-if="statisticsData?.molds?.by_type?.length" class="metric-list"><article v-for="item in statisticsData.molds.by_type" :key="String(item.name)"><span>{{ item.name === 'common' ? '共模' : '单模' }}</span><strong>{{ item.value }}</strong></article></div><p v-else class="report-empty">暂无模具类型数据</p>
        </section>
      </div>

      <div v-if="statisticsData?.inventory?.low_stock?.length || statisticsData?.molds?.by_location?.length || statisticsData?.recent_workorders?.length || compactTrendItems.length" class="report-grid lower">
        <section v-if="!statisticsSourceUnavailable('inventory') && statisticsData?.inventory?.low_stock?.length" class="report-panel">
          <div class="drawer-section-title"><h3>低库存</h3><small>安全库存预警</small></div>
          <div v-if="statisticsData?.inventory?.low_stock?.length" class="report-table"><article v-for="item in statisticsData.inventory.low_stock" :key="`${item.item_type}-${item.item_id}`"><div><strong>{{ item.name }}</strong><small>{{ item.code }} · {{ item.category }}</small></div><div class="report-table__status"><StatusTag :label="stockState(item).label" :tone="stockState(item).tone" /><small>{{ formatQuantity(item.quantity) }} / {{ formatQuantity(item.safety_stock) }}</small></div></article></div><p v-else class="drawer-empty">暂无低库存预警</p>
        </section>
        <section v-if="statisticsData?.molds?.by_location?.length" class="report-panel">
          <div class="drawer-section-title"><h3>模具位置</h3><small>固定位置分布</small></div>
          <div v-if="statisticsData?.molds?.by_location?.length" class="metric-list"><article v-for="item in statisticsData.molds.by_location" :key="String(item.name)"><span>{{ item.name || '未设置' }}</span><strong>{{ item.value }}</strong></article></div><p v-else class="drawer-empty">暂无模具位置数据</p>
        </section>
        <section v-if="!statisticsSourceUnavailable('workorders') && statisticsData?.recent_workorders?.length" class="report-panel">
          <div class="drawer-section-title"><h3>最近任务</h3><small>按创建时间</small></div>
          <div v-if="statisticsData?.recent_workorders?.length" class="report-table"><article v-for="item in statisticsData.recent_workorders" :key="Number(item.id)"><div><strong>{{ item.title }}</strong><small>{{ item.code }} · {{ item.product_name || workorderTypeLabel(item.type) }}</small></div><StatusTag :label="workorderStatusLabel(item.status)" :tone="workorderStatusTone(item.status)" /></article></div><p v-else class="drawer-empty">暂无任务单</p>
        </section>
        <section v-if="compactTrendItems.length" class="report-panel">
          <div class="drawer-section-title"><h3>近 14 天趋势</h3><small>库存流水和任务创建</small></div>
          <div v-if="compactTrendItems.length" class="trend-list"><article v-for="item in compactTrendItems" :key="`${item.date}-${item.name}-${item.value}`"><div><span>{{ item.date }} · {{ trendNameLabel(item.name) }}</span><strong>{{ item.quantity ? formatQuantity(item.quantity) : item.value }}</strong></div><div class="trend-bar" aria-hidden="true"><span :style="{width: `${trendBarPercentage(item)}%`}"></span></div></article></div><p v-else class="report-empty">暂无可展示的趋势数据</p>
        </section>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import {useWorkorderContext} from '../../../composables/workorderContext'
import MetricCard from '../../ui/MetricCard.vue'
import PageState from '../../ui/PageState.vue'
import StatusTag from '../../ui/StatusTag.vue'

const {loading, listError, statisticsData, statisticsSourcesUnavailable, statisticsSourceUnavailable, formatDate, statisticsCards, inventoryItemTypeLabel, formatQuantity, formatMoney, departmentName, departmentCompletionRate, stockState, compactTrendItems, trendNameLabel, trendBarPercentage} = useWorkspaceContext()
const {workorderStatusLabel, workorderTypeLabel, workorderStatusTone} = useWorkorderContext().list
const unavailableSourceLabels = computed(() => {
  const labels: Record<string, string> = {suppliers: '供应商', supplier: '供应商', inventory: '仓库与库存', warehouse: '仓库与库存', workorders: '任务单', workorder: '任务单'}
  const sources = statisticsData.value?.unavailable_sources || []
  return [...new Set(sources.map((source) => labels[String(source).toLowerCase()] || String(source)))].join('、') || '仓库与库存、供应商或任务单'
})
</script>

<style scoped>
.statistics-source-alert { margin-bottom: var(--bb-space-5); }
.report-unavailable { margin: 0; border-radius: var(--bb-radius-md); background: var(--bb-warning-bg); padding: var(--bb-space-4); color: var(--bb-warning); line-height: var(--bb-line-height-base); }
</style>
