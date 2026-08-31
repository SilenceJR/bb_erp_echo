<template>
  <DataTableShell v-bind="tableState" :aria-label="`${activeModule?.title || '档案'}列表`" @retry="loadActiveModule" @update:page="handlePageChange" @update:page-size="handlePageSizeChange">
    <div class="master-data-desktop">
      <el-table :data="rows" row-key="id" stripe class="data-table master-data-table">
        <el-table-column label="供应商" min-width="190"><template #default="{row}"><span class="item-name">{{ row.name }}</span><small class="item-code">{{ row.code || '未设置编码' }}</small></template></el-table-column>
        <el-table-column prop="contact" label="联系人" min-width="120"><template #default="{row}">{{ formatCell(row.contact) }}</template></el-table-column>
        <el-table-column prop="phone" label="联系电话" min-width="150"><template #default="{row}">{{ formatCell(row.phone) }}</template></el-table-column>
        <el-table-column prop="address" label="地址" min-width="220" show-overflow-tooltip><template #default="{row}">{{ formatCell(row.address) }}</template></el-table-column>
        <el-table-column label="状态" width="110" align="center"><template #default="{row}"><StatusTag :label="genericStatusLabel(row.status)" :tone="genericStatusTone(row.status)" /></template></el-table-column>
        <el-table-column v-if="canWriteActive" label="操作" width="90" fixed="right"><template #default="{row}"><el-button link type="primary" @click="editSupplier(row)">编辑</el-button></template></el-table-column>
      </el-table>
    </div>
  </DataTableShell>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import DataTableShell from '../../ui/DataTableShell.vue'
import StatusTag from '../../ui/StatusTag.vue'

const {rows, loading, listError, pageTotal, page, pageSize, masterDataEmptyTitle, masterDataEmptyDescription, activeModule, loadActiveModule, handlePageChange, handlePageSizeChange, formatCell, genericStatusLabel, genericStatusTone, canWriteActive, editSupplier} = useWorkspaceContext()
const tableState = computed(() => ({loading: loading.value, error: listError.value, rowsCount: rows.value.length, total: pageTotal.value, page: page.value, pageSize: pageSize.value, emptyTitle: masterDataEmptyTitle.value, emptyDescription: masterDataEmptyDescription.value}))
</script>
