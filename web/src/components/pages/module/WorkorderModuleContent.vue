<template>
  <DataTableShell v-bind="tableState" aria-label="任务单列表" @retry="loadActiveModule" @update:page="handlePageChange" @update:page-size="handlePageSizeChange">
    <div class="responsive-table-desktop">
      <el-table :data="rows" row-key="id" stripe class="data-table">
        <el-table-column label="任务" min-width="220"><template #default="{row}"><span class="item-name">{{ row.title }}</span><small class="item-code">{{ row.code }} · {{ workorderTypeLabel(row.type) }}</small></template></el-table-column>
        <el-table-column label="状态" width="130"><template #default="{row}"><StatusTag :label="workorderStatusLabel(row.status)" :tone="workorderStatusTone(row.status)" /></template></el-table-column>
        <el-table-column label="优先级" width="100"><template #default="{row}"><StatusTag :label="row.priority === 'urgent' ? '加急' : '普通'" :tone="row.priority === 'urgent' ? 'danger' : 'info'" /></template></el-table-column>
        <el-table-column label="产品/数量" min-width="160"><template #default="{row}">{{ row.product_name || '-' }}<br><small>{{ formatQuantity(row.planned_quantity) }} {{ row.unit || '' }}</small></template></el-table-column>
        <el-table-column label="部门进度" min-width="220"><template #default="{row}"><div class="department-progress-cell"><div><span>{{ departmentProgressSummary(row) }}</span><strong>{{ departmentProgressMetrics(row).percentage }}%</strong></div><el-progress :percentage="departmentProgressMetrics(row).percentage" :show-text="false" :stroke-width="8" /></div></template></el-table-column>
        <el-table-column label="交期" width="130"><template #default="{row}"><div class="due-state-cell"><span>{{ formatDate(row.due_at) }}</span><StatusTag v-if="workorderDueState(row).overdue" :label="workorderDueState(row).label" tone="danger" /></div></template></el-table-column>
        <el-table-column label="操作" width="100" fixed="right"><template #default="{row}"><el-button link type="primary" @click="openWorkOrder(row)">详情</el-button></template></el-table-column>
        <template #empty><el-empty description="还没有任务单" /></template>
      </el-table>
    </div>
  </DataTableShell>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useWorkorderContext} from '../../../composables/workorderContext'
import DataTableShell from '../../ui/DataTableShell.vue'
import StatusTag from '../../ui/StatusTag.vue'

const {rows, loading, listError, pageTotal, page, pageSize, filteredEmptyTitle, filteredEmptyDescription, loadActiveModule, handlePageChange, handlePageSizeChange, workorderTypeLabel, workorderStatusLabel, workorderStatusTone, formatQuantity, departmentProgressSummary, departmentProgressMetrics, formatDate, workorderDueState, openWorkOrder} = useWorkorderContext().list
const tableState = computed(() => ({loading: loading.value, error: listError.value, rowsCount: rows.value.length, total: pageTotal.value, page: page.value, pageSize: pageSize.value, emptyTitle: filteredEmptyTitle.value, emptyDescription: filteredEmptyDescription.value}))
</script>
