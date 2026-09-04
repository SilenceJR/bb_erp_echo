<template>
  <DataTableShell v-bind="tableState" :aria-label="`${activeModule?.title || '数据'}列表`" @retry="loadActiveModule" @update:page="handlePageChange" @update:page-size="handlePageSizeChange">
    <div class="generic-list-desktop">
      <el-table :data="rows" row-key="id" stripe class="data-table generic-data-table">
        <el-table-column v-for="column in columns" :key="column" :label="columnLabel(column)" min-width="130">
          <template #default="{row}"><StatusTag v-if="isGenericStatusColumn(column)" :label="genericStatusLabel(row[column])" :tone="genericStatusTone(row[column])" /><span v-else>{{ formatGenericCell(column, row[column]) }}</span></template>
        </el-table-column>
        <el-table-column v-if="hasAssignmentAction || (activeKey === 'users' && canWriteActive)" label="配置操作" width="150" fixed="right">
          <template #default="{row}">
            <span v-if="hasAssignmentAction" :title="assignmentTargetHint(row)">
              <el-button link type="primary" :disabled="assignmentTargetDisabled(row)" @click="openAssignment(row)">{{ assignmentConfigs[activeKey]?.buttonLabel }}</el-button>
            </span>
            <span v-if="activeKey === 'users' && canWriteActive" :title="canEditUserAffiliation ? '修正账号部门和终端归属' : '需要部门查看和终端查看权限'"><el-button link type="primary" :disabled="!canEditUserAffiliation" @click="openUserAffiliation(row)">账号归属</el-button></span>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </DataTableShell>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import DataTableShell from '../../ui/DataTableShell.vue'
import StatusTag from '../../ui/StatusTag.vue'

const {rows, columns, loading, listError, pageTotal, page, pageSize, filteredEmptyTitle, filteredEmptyDescription, activeModule, loadActiveModule, handlePageChange, handlePageSizeChange, columnLabel, isGenericStatusColumn, genericStatusLabel, genericStatusTone, formatGenericCell, hasAssignmentAction, activeKey, canWriteActive, assignmentConfigs, openAssignment, canEditUserAffiliation, openUserAffiliation, assignmentTargetDisabled, assignmentTargetHint} = useWorkspaceContext()
const tableState = computed(() => ({loading: loading.value, error: listError.value, rowsCount: rows.value.length, total: pageTotal.value, page: page.value, pageSize: pageSize.value, emptyTitle: filteredEmptyTitle.value, emptyDescription: filteredEmptyDescription.value}))
</script>
