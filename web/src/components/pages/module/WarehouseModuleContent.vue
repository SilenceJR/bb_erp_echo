<template>
  <DataTableShell v-bind="tableState" aria-label="仓库物品列表" @retry="loadActiveModule" @update:page="handlePageChange" @update:page-size="handlePageSizeChange">
    <div class="responsive-table-desktop">
      <el-table :data="rows" row-key="id" stripe class="data-table">
        <el-table-column label="物品" min-width="190"><template #default="{row}"><span class="item-name">{{ row.name }}</span><small class="item-code">{{ row.code }}</small></template></el-table-column>
        <el-table-column prop="spec" label="规格" min-width="130"><template #default="{row}">{{ formatCell(row.spec) }}</template></el-table-column>
        <el-table-column prop="unit" label="单位" width="90" />
        <el-table-column label="当前库存" width="140"><template #default="{row}"><div class="stock-state-cell"><strong>{{ formatQuantity(row.quantity) }}</strong><StatusTag :label="stockState(row).label" :tone="stockState(row).tone" /></div></template></el-table-column>
        <el-table-column label="安全库存" width="120"><template #default="{row}">{{ formatQuantity(row.safety_stock) }}</template></el-table-column>
        <el-table-column label="操作" width="130" fixed="right"><template #default="{row}"><el-button link type="primary" @click="openWarehouseItem(row)">查看与办理</el-button></template></el-table-column>
        <template #empty><el-empty description="该分类还没有物品" /></template>
      </el-table>
    </div>
  </DataTableShell>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import DataTableShell from '../../ui/DataTableShell.vue'
import StatusTag from '../../ui/StatusTag.vue'

const {rows, loading, listError, pageTotal, page, pageSize, filteredEmptyTitle, filteredEmptyDescription, loadActiveModule, handlePageChange, handlePageSizeChange, formatCell, formatQuantity, stockState, openWarehouseItem} = useWorkspaceContext()
const tableState = computed(() => ({loading: loading.value, error: listError.value, rowsCount: rows.value.length, total: pageTotal.value, page: page.value, pageSize: pageSize.value, emptyTitle: filteredEmptyTitle.value, emptyDescription: filteredEmptyDescription.value}))
</script>
