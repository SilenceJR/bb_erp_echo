<template>
  <DataTableShell v-bind="tableState" :aria-label="`${activeModule?.title || '数据'}列表`" @retry="loadActiveModule" @update:page="handlePageChange" @update:page-size="handlePageSizeChange">
    <div class="generic-list-desktop">
      <el-table :data="rows" row-key="id" stripe class="data-table generic-data-table">
        <el-table-column v-for="column in visibleColumns" :key="column" :label="columnLabel(column)" :min-width="column === 'name' || column === 'description' ? 200 : 130">
          <template #default="{row}"><StatusTag v-if="isGenericStatusColumn(column)" :label="genericStatusLabel(row[column])" :tone="genericStatusTone(row[column])" /><span v-else>{{ formatGenericCell(column, row[column]) }}</span></template>
        </el-table-column>
        <el-table-column :label="actionColumnLabel" :width="actionColumnWidth" fixed="right">
          <template #default="{row}">
            <el-button link type="primary" @click="openDetail(row)">详情</el-button>
            <span v-if="hasAssignmentAction" :title="assignmentTargetHint(row)">
              <el-button link type="primary" :disabled="assignmentTargetDisabled(row)" @click="openAssignment(row)">{{ assignmentConfigs[activeKey]?.buttonLabel }}</el-button>
            </span>
            <span v-if="activeKey === 'users' && canWriteActive" :title="canEditUserAffiliation ? '修正账号部门和终端归属' : '需要部门查看和终端查看权限'"><el-button link type="primary" :disabled="!canEditUserAffiliation" @click="openUserAffiliation(row)">账号归属</el-button></span>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </DataTableShell>

  <GenericRecordDetail
    v-model="detailVisible"
    :title="`${activeModule?.title || '数据'}详情`"
    :eyebrow="activeModule?.title || '数据详情'"
    :primary="detailPrimary"
    :subtitle="detailSubtitle"
    :item="detailRow"
    :fields="detailFields"
    @closed="resolveDetailClosed"
  >
    <template #footer>
      <el-button @click="detailVisible = false">关闭</el-button>
      <el-button v-if="hasAssignmentAction" type="primary" plain :disabled="detailRow ? assignmentTargetDisabled(detailRow) : true" :title="detailRow ? assignmentTargetHint(detailRow) : ''" @click="openDetailAssignment">{{ assignmentConfigs[activeKey]?.buttonLabel }}</el-button>
      <el-button v-if="activeKey === 'users' && canWriteActive" type="primary" plain :disabled="!canEditUserAffiliation" @click="openDetailAffiliation">账号归属</el-button>
    </template>
  </GenericRecordDetail>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, onMounted, ref, watch} from 'vue'
import {ElMessage} from 'element-plus'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import {normalizeModuleKey, notificationOpenEvent, notificationRefreshEvent, type NotificationOpenEventDetail, type NotificationRefreshEventDetail} from '../../../platform/notifications'
import DataTableShell from '../../ui/DataTableShell.vue'
import StatusTag from '../../ui/StatusTag.vue'
import GenericRecordDetail, {type GenericRecordDetailField} from './GenericRecordDetail.vue'

const {rows, columns, loading, listError, pageTotal, page, pageSize, filteredEmptyTitle, filteredEmptyDescription, activeModule, loadActiveModule, handlePageChange, handlePageSizeChange, columnLabel, isGenericStatusColumn, genericStatusLabel, genericStatusTone, formatGenericCell, hasAssignmentAction, activeKey, canWriteActive, assignmentConfigs, openAssignment, canEditUserAffiliation, openUserAffiliation, assignmentTargetDisabled, assignmentTargetHint, showCreateForm, toggleCreateForm, setPageDetailPanelVisible, genericRowTitle, genericRowSubtitle, assignmentTarget, affiliationTarget} = useWorkspaceContext()
const tableState = computed(() => ({loading: loading.value, error: listError.value, rowsCount: rows.value.length, total: pageTotal.value, page: page.value, pageSize: pageSize.value, emptyTitle: filteredEmptyTitle.value, emptyDescription: filteredEmptyDescription.value}))
// Internal timestamps and transport-only fields are not user-facing columns.
const visibleColumns = computed(() => {
  const source = [...columns.value]
  if (activeKey.value !== 'audits' || !source.includes('action')) return source
  // Keep the audit action next to the operator identity so the table reads as
  // one event record instead of splitting the business action from its actor.
  const actionIndex = source.indexOf('action')
  source.splice(actionIndex, 1)
  const operatorIndex = source.findIndex((column) => column === 'operator_employee_name' || column === 'operator_department_name')
  source.splice(operatorIndex >= 0 ? operatorIndex + 1 : Math.min(actionIndex, source.length), 0, 'action')
  return source
})
const detailVisible = ref(false)
const detailRow = ref<Record<string, unknown> | null>(null)
const detailPrimary = computed(() => detailRow.value ? genericRowTitle(detailRow.value as never) : '')
const detailSubtitle = computed(() => detailRow.value ? genericRowSubtitle(detailRow.value as never) : '')
const detailFields = computed<GenericRecordDetailField[]>(() => {
  const row = detailRow.value
  if (!row) return []
  return visibleColumns.value.filter((column) => column in row).map((column) => ({
    key: column,
    label: columnLabel(column),
    value: column === 'status' || column === 'result' ? genericStatusLabel(row[column]) : formatGenericCell(column, row[column]),
    mono: column === 'id' || column.endsWith('_id') || ['code', 'username'].includes(column),
  }))
})
const actionColumnWidth = computed(() => hasAssignmentAction.value || (activeKey.value === 'users' && canWriteActive.value) ? 250 : 100)
const actionColumnLabel = computed(() => hasAssignmentAction.value || (activeKey.value === 'users' && canWriteActive.value) ? '操作' : '查看')
let detailCloseResolver: (() => void) | null = null

watch(detailVisible, (visible) => setPageDetailPanelVisible(visible), {immediate: true, flush: 'sync'})
watch(activeKey, () => { detailVisible.value = false; detailRow.value = null })

async function handleNotificationOpen(event: Event) {
  const detail = (event as CustomEvent<NotificationOpenEventDetail>).detail
  if (!detail || normalizeModuleKey(detail.module.module) !== normalizeModuleKey(activeKey.value) || detail.module.action.type !== 'open_entity') return
  const entityID = detail.module.action.entity_id ?? detail.module.items[0]?.entity_id
  if (entityID === undefined || entityID === null) return
  const findRow = () => rows.value.find((row) => String(row.id) === String(entityID))
  let row = findRow()
  if (!row) {
    await loadActiveModule()
    row = findRow()
  }
  if (row) await openDetail(row)
  else ElMessage.info('该记录已不存在或当前账号无权查看')
}

function handleNotificationRefresh(event: Event) {
  const detail = (event as CustomEvent<NotificationRefreshEventDetail>).detail
  if (!detail || normalizeModuleKey(detail.module.module) !== normalizeModuleKey(activeKey.value)) return
  detail.handled = true
  if (detail.deferred || showCreateForm.value || assignmentTarget.value || affiliationTarget.value) {
    detail.deferred = true
    setPageDetailPanelVisible(detailVisible.value)
    ElMessage.warning('数据已由其他用户更新，请保存或关闭当前修改后刷新')
    return
  }
  const selectedID = detailRow.value?.id
  void loadActiveModule().then(() => {
    if (!detailVisible.value || selectedID === undefined || selectedID === null) return
    const refreshed = rows.value.find((row) => String(row.id) === String(selectedID))
    if (refreshed) detailRow.value = refreshed
    else {
      detailVisible.value = false
      detailRow.value = null
      ElMessage.info('该记录已删除或当前账号无权查看')
    }
  }).catch(() => ElMessage.warning('外部更新同步失败，请手动刷新'))
}

onMounted(() => {
  window.addEventListener(notificationOpenEvent, handleNotificationOpen)
  window.addEventListener(notificationRefreshEvent, handleNotificationRefresh)
})
onBeforeUnmount(() => {
  window.removeEventListener(notificationOpenEvent, handleNotificationOpen)
  window.removeEventListener(notificationRefreshEvent, handleNotificationRefresh)
  setPageDetailPanelVisible(false)
})

async function openDetail(row: Record<string, unknown>) {
  if (showCreateForm.value) {
    await toggleCreateForm()
    if (showCreateForm.value) return
  }
  detailRow.value = row
  detailVisible.value = true
}

async function openDetailAssignment() {
  const row = detailRow.value
  if (!row) return
  await closeDetailPanel()
  await openAssignment(row)
}

async function openDetailAffiliation() {
  const row = detailRow.value
  if (!row) return
  await closeDetailPanel()
  openUserAffiliation(row)
}

function closeDetailPanel() {
  if (!detailVisible.value) return Promise.resolve()
  detailVisible.value = false
  return new Promise<void>((resolve) => { detailCloseResolver = resolve })
}

function resolveDetailClosed() {
  detailCloseResolver?.()
  detailCloseResolver = null
}
</script>
