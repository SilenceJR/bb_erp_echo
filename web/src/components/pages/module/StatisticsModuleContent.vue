<template>
  <section v-loading="loading" class="statistics-page">
    <el-alert v-if="listError && statisticsData" :title="listError" type="error" :closable="false" show-icon />
    <PageState v-if="loading && !statisticsData" kind="loading" title="正在生成统计报表" />
    <template v-else>
      <div class="report-overview-heading"><div><h2>经营概览</h2><p>更新时间：{{ formatDate(statisticsData?.generated_at) }}</p></div><StatusTag :label="statisticsData?.can_view_cost ? '可查看成本金额' : '成本金额已隐藏'" :tone="statisticsData?.can_view_cost ? 'success' : 'info'" /></div>
      <div class="stats-grid"><MetricCard v-for="card in statisticsCards" :key="card.label" v-bind="card" /></div>

      <div class="report-grid">
        <section class="report-panel">
          <div class="drawer-section-title"><h3>库存分类</h3><small>{{ statisticsData?.can_view_cost ? '含库存金额' : '金额已按权限隐藏' }}</small></div>
          <div v-if="statisticsData?.inventory?.by_item_type?.length" class="metric-list"><article v-for="item in statisticsData.inventory.by_item_type" :key="String(item.name)"><span>{{ inventoryItemTypeLabel(item.name) }}</span><strong>{{ formatQuantity(item.value) }}</strong><small v-if="statisticsData.can_view_cost">{{ formatMoney(item.amount) }}</small></article></div><p v-else class="report-empty">暂无库存分类数据</p>
        </section>
        <section class="report-panel">
          <div class="drawer-section-title"><h3>任务状态</h3><small>主任务</small></div>
          <div v-if="statisticsData?.workorders?.by_status?.length" class="metric-list"><article v-for="item in statisticsData.workorders.by_status" :key="String(item.name)"><span>{{ workorderStatusLabel(item.name) }}</span><strong>{{ item.value }}</strong></article></div><p v-else class="report-empty">暂无任务状态数据</p>
        </section>
        <section class="report-panel">
          <div class="drawer-section-title"><h3>部门处理</h3><small>子任务</small></div>
          <div v-if="statisticsData?.workorders?.by_department?.length" class="department-stat-list"><article v-for="item in statisticsData.workorders.by_department" :key="Number(item.department_id)"><div><strong>{{ item.name || departmentName(item.department_id) }}</strong><small>共 {{ item.total }} 项</small></div><el-progress :percentage="departmentCompletionRate(item)" :stroke-width="8" /><small>完成 {{ item.completed }} · 处理中 {{ item.processing }} · 部分完成 {{ item.partial }} · 已收到 {{ item.received }}</small></article></div><p v-else class="report-empty">暂无部门处理数据</p>
        </section>
        <section class="report-panel">
          <div class="drawer-section-title"><h3>模具状态</h3><small>台账</small></div>
          <div v-if="statisticsData?.molds?.by_status?.length" class="metric-list"><article v-for="item in statisticsData.molds.by_status" :key="String(item.name)"><span>{{ moldStatusLabel(item.name) }}</span><strong>{{ item.value }}</strong></article></div><p v-else class="report-empty">暂无模具状态数据</p>
        </section>
      </div>

      <div class="report-grid lower">
        <section class="report-panel">
          <div class="drawer-section-title"><h3>低库存</h3><small>安全库存预警</small></div>
          <div v-if="statisticsData?.inventory?.low_stock?.length" class="report-table"><article v-for="item in statisticsData.inventory.low_stock" :key="`${item.item_type}-${item.item_id}`"><div><strong>{{ item.name }}</strong><small>{{ item.code }} · {{ item.category }}</small></div><div class="report-table__status"><StatusTag :label="stockState(item).label" :tone="stockState(item).tone" /><small>{{ formatQuantity(item.quantity) }} / {{ formatQuantity(item.safety_stock) }}</small></div></article></div><p v-else class="drawer-empty">暂无低库存预警</p>
        </section>
        <section class="report-panel">
          <div class="drawer-section-title"><h3>需关注模具</h3><small>借出/维修/保养到期</small></div>
          <div v-if="statisticsData?.molds?.need_care?.length" class="report-table"><article v-for="item in statisticsData.molds.need_care" :key="Number(item.id)"><div><strong>{{ item.name }}</strong><small>{{ item.code }} · {{ item.current_location || '-' }}</small></div><div class="report-table__status"><StatusTag :label="moldStatusLabel(item.status)" :tone="moldStatusTone(item.status)" /><small>{{ moldMaintenanceState(item).label }}</small></div></article></div><p v-else class="drawer-empty">暂无需要关注的模具</p>
        </section>
        <section class="report-panel">
          <div class="drawer-section-title"><h3>最近任务</h3><small>按创建时间</small></div>
          <div v-if="statisticsData?.recent_workorders?.length" class="report-table"><article v-for="item in statisticsData.recent_workorders" :key="Number(item.id)"><div><strong>{{ item.title }}</strong><small>{{ item.code }} · {{ item.product_name || workorderTypeLabel(item.type) }}</small></div><StatusTag :label="workorderStatusLabel(item.status)" :tone="workorderStatusTone(item.status)" /></article></div><p v-else class="drawer-empty">暂无任务单</p>
        </section>
        <section class="report-panel">
          <div class="drawer-section-title"><h3>近 14 天趋势</h3><small>库存流水和任务创建</small></div>
          <div v-if="compactTrendItems.length" class="trend-list"><article v-for="item in compactTrendItems" :key="`${item.date}-${item.name}-${item.value}`"><div><span>{{ item.date }} · {{ trendNameLabel(item.name) }}</span><strong>{{ item.quantity ? formatQuantity(item.quantity) : item.value }}</strong></div><div class="trend-bar" aria-hidden="true"><span :style="{width: `${trendBarPercentage(item)}%`}"></span></div></article></div><p v-else class="report-empty">暂无可展示的趋势数据</p>
        </section>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import {useWorkorderContext} from '../../../composables/workorderContext'
import MetricCard from '../../ui/MetricCard.vue'
import PageState from '../../ui/PageState.vue'
import StatusTag from '../../ui/StatusTag.vue'

const {loading, listError, statisticsData, formatDate, statisticsCards, inventoryItemTypeLabel, formatQuantity, formatMoney, departmentName, departmentCompletionRate, moldStatusLabel, stockState, moldStatusTone, moldMaintenanceState, compactTrendItems, trendNameLabel, trendBarPercentage} = useWorkspaceContext()
const {workorderStatusLabel, workorderTypeLabel, workorderStatusTone} = useWorkorderContext().list
</script>
