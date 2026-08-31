<template>
  <DataTableShell v-bind="tableState" aria-label="模具台账列表" @retry="loadActiveModule" @update:page="handlePageChange" @update:page-size="handlePageSizeChange">
    <div class="responsive-table-desktop">
      <el-table :data="rows" row-key="id" stripe class="data-table">
        <el-table-column label="模具" min-width="190"><template #default="{row}"><span class="item-name">{{ row.name }}</span><small class="item-code">{{ row.code }}</small></template></el-table-column>
        <el-table-column label="状态" width="120"><template #default="{row}"><StatusTag :label="moldStatusLabel(row.status)" :tone="moldStatusTone(row.status)" /></template></el-table-column>
        <el-table-column prop="current_location" label="当前位置" min-width="150"><template #default="{row}">{{ formatCell(row.current_location) }}</template></el-table-column>
        <el-table-column prop="storage_location" label="存放位置" min-width="150"><template #default="{row}">{{ formatCell(row.storage_location) }}</template></el-table-column>
        <el-table-column label="保养计划" min-width="190"><template #default="{row}"><div class="maintenance-state-cell"><StatusTag :label="moldMaintenanceState(row).label" :tone="moldMaintenanceState(row).tone" /><small>{{ formatDate(row.next_maintenance_at) }}</small></div></template></el-table-column>
        <el-table-column label="操作" width="100" fixed="right"><template #default="{row}"><el-button link type="primary" @click="openMold(row)">详情</el-button></template></el-table-column>
      </el-table>
    </div>
  </DataTableShell>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import DataTableShell from '../../ui/DataTableShell.vue'
import StatusTag from '../../ui/StatusTag.vue'

const {rows, loading, listError, pageTotal, page, pageSize, filteredEmptyTitle, filteredEmptyDescription, loadActiveModule, handlePageChange, handlePageSizeChange, moldStatusLabel, moldStatusTone, formatCell, moldMaintenanceState, formatDate, openMold} = useWorkspaceContext()
const tableState = computed(() => ({loading: loading.value, error: listError.value, rowsCount: rows.value.length, total: pageTotal.value, page: page.value, pageSize: pageSize.value, emptyTitle: filteredEmptyTitle.value, emptyDescription: filteredEmptyDescription.value}))
</script>
