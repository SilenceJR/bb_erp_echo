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
      <p class="report-updated">更新于 {{ formatDate(statisticsData.generated_at) }}</p>

      <div v-if="statisticsData?.workorders?.by_status?.length || statisticsData?.workorders?.by_department?.length || statisticsData?.molds?.by_type?.length" class="report-grid">
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

      <div v-if="statisticsData?.inventory?.low_stock?.length || statisticsData?.molds?.by_location?.length || statisticsData?.recent_workorders?.length" class="report-grid lower">
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
      </div>
      <PageState v-if="!hasRecords && !statisticsSourcesUnavailable" kind="empty" title="暂无记录" />
    </template>
    <PageState v-else-if="listError" kind="error" title="统计加载失败" :description="listError" />
  </section>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import {useWorkorderContext} from '../../../composables/workorderContext'
import PageState from '../../ui/PageState.vue'
import StatusTag from '../../ui/StatusTag.vue'

const {loading, listError, statisticsData, statisticsSourcesUnavailable, statisticsSourceUnavailable, formatDate, statisticsCards, inventoryItemTypeLabel, formatQuantity, formatMoney, departmentName, departmentCompletionRate, stockState, compactTrendItems, trendNameLabel, trendBarPercentage} = useWorkspaceContext()
const {workorderStatusLabel, workorderTypeLabel, workorderStatusTone} = useWorkorderContext().list
const hasRecords = computed(() => {
  const data = statisticsData.value
  return !!(data?.molds?.by_type?.length || data?.molds?.by_location?.length || (!statisticsSourceUnavailable('workorders') && (data?.workorders?.by_status?.length || data?.workorders?.by_department?.length || data?.recent_workorders?.length)) || (!statisticsSourceUnavailable('inventory') && data?.inventory?.low_stock?.length))
})
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
