<template>
  <ResponsiveDetailCarrier
    :model-value="!!assignmentTarget"
    :title="assignmentConfig?.title || '配置'"
    :size="assignmentPanel.size.value" :docked="assignmentPanel.docked.value"
    :close-on-click-modal="!assignmentSaving"
    :close-on-press-escape="!assignmentSaving"
    :show-close="!assignmentSaving"
    :before-close="handleAssignmentBeforeClose"
  >
    <div v-if="assignmentTarget" class="assignment-panel">
      <div class="assignment-heading"><div><strong>{{ assignmentTarget.name || assignmentTarget.username || assignmentTarget.code }}</strong><span>已选择 {{ selectedAssignmentIDs.length }} 项</span></div></div>
      <p class="assignment-tip">{{ assignmentConfig?.tip }}</p>
      <el-alert v-if="assignmentScopeBlockedReason" :title="assignmentScopeBlockedReason" type="warning" :closable="false" show-icon />
      <el-alert v-if="assignmentSaveError" :title="assignmentSaveError" type="error" :closable="false" show-icon />
      <PageState v-if="assignmentOptionsLoading" kind="loading" title="正在加载完整配置项" />
      <PageState v-else-if="assignmentOptionsError" kind="error" title="配置项加载失败" :description="assignmentOptionsError" action-label="重新加载" @action="retryAssignmentOptions" />
      <el-checkbox-group v-else v-model="selectedAssignmentIDs" class="assignment-option-groups">
        <section v-for="group in assignmentOptionGroups" :key="group.key" class="assignment-option-group">
          <div class="assignment-option-group__heading"><strong>{{ group.label }}</strong><small>{{ group.items.length }} 项</small></div>
          <div class="assignment-options">
            <el-checkbox v-for="option in group.items" :key="option.id" :value="option.id" :disabled="isAssignmentOptionDisabled(option)" :title="assignmentOptionDisabledReason(option)" class="check-option">
              <span class="check-option-copy">
                <strong>{{ option.name || option.code }}</strong>
                <small>{{ option.description || option.code }}</small>
                <small v-if="!assignmentScopeBlockedReason && assignmentOptionDisabledReason(option)" class="check-option-scope-hint">{{ assignmentOptionDisabledReason(option) }}</small>
              </span>
            </el-checkbox>
          </div>
        </section>
      </el-checkbox-group>
      <span v-if="assignmentOptionsReady && !assignmentOptions.length" class="assignment-empty">暂无可配置项</span>
    </div>
    <template #footer><div class="assignment-actions"><el-button :disabled="assignmentSaving" @click="requestAssignmentClose">取消</el-button><el-button type="primary" :loading="assignmentSaving" :disabled="!assignmentOptionsReady || assignmentSaving || !!assignmentScopeBlockedReason" @click="saveAssignment">保存配置</el-button></div></template>
  </ResponsiveDetailCarrier>

  <ResponsiveDetailCarrier :model-value="!!affiliationTarget" title="修正账号归属" :size="affiliationPanel.size.value" :docked="affiliationPanel.docked.value" docked-auto-focus="first-editable" :close-on-click-modal="!affiliationSaving" :close-on-press-escape="!affiliationSaving" :before-close="closeUserAffiliation">
    <el-form v-if="affiliationTarget" label-position="top" :disabled="affiliationSaving" @submit.prevent="saveUserAffiliation">
      <p class="assignment-tip">{{ affiliationTarget.username }} · {{ affiliationTarget.name }}。个人管理账号可不绑定部门，但未绑定时不能执行任务或库存写入。</p>
      <el-alert v-if="affiliationError" :title="affiliationError" type="error" :closable="false" show-icon />
      <el-form-item label="所属部门"><el-select v-model="affiliationDepartmentID" clearable placeholder="不绑定部门"><el-option v-for="item in rowsFor('departments')" :key="item.id" :label="String(item.name)" :value="item.id" :disabled="item.status === 'disabled'" /></el-select></el-form-item>
      <el-form-item label="所属终端"><el-select v-model="affiliationTerminalID" clearable :disabled="!affiliationDepartmentID" placeholder="不绑定终端"><el-option v-for="item in affiliationTerminalOptions" :key="item.id" :label="String(item.name)" :value="item.id" /></el-select></el-form-item>
    </el-form>
    <template #footer><div class="form-actions"><el-button :disabled="affiliationSaving" @click="closeUserAffiliation()">取消</el-button><el-button type="primary" :loading="affiliationSaving" :disabled="affiliationTarget?.account_type === 'department_terminal' && (!affiliationDepartmentID || !affiliationTerminalID)" @click="saveUserAffiliation">保存归属</el-button></div></template>
  </ResponsiveDetailCarrier>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useDirtyGuard} from '../../../composables/useDirtyGuard'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import ResponsiveDetailCarrier from '../../ui/ResponsiveDetailCarrier.vue'
import {useResponsiveDetailPanel} from '../../../composables/useResponsiveDetailPanel'
import PageState from '../../ui/PageState.vue'

const {
  assignmentTarget, assignmentConfig, selectedAssignmentIDs, assignmentSaveError,
  assignmentOptionsLoading, assignmentOptionsError, retryAssignmentOptions,
  assignmentOptionGroups, isAssignmentOptionDisabled, assignmentOptionsReady,
  assignmentOptionDisabledReason, assignmentScopeBlockedReason,
  assignmentOptions, assignmentSaving, closeAssignment, saveAssignment,
  affiliationTarget, affiliationSaving, affiliationError, affiliationDepartmentID,
  affiliationTerminalID, affiliationTerminalOptions, rowsFor,
  closeUserAffiliation, saveUserAffiliation,
} = useWorkspaceContext()

const assignmentPanel = useResponsiveDetailPanel(computed(() => !!assignmentTarget.value), true)
const affiliationPanel = useResponsiveDetailPanel(computed(() => !!affiliationTarget.value), true)
const assignmentDirty = computed(() => {
  if (!assignmentTarget.value || !assignmentConfig.value) return false
  const original = Array.isArray(assignmentTarget.value[assignmentConfig.value.selectedKey])
    ? (assignmentTarget.value[assignmentConfig.value.selectedKey] as unknown[]).map(Number)
    : []
  return [...original].sort((left, right) => left - right).join(',')
    !== [...selectedAssignmentIDs.value].sort((left, right) => left - right).join(',')
})
const assignmentGuard = useDirtyGuard('module-assignment', {
  busy: () => assignmentSaving.value,
  dirty: () => assignmentDirty.value,
  busyMessage: '权限配置正在保存，请等待完成后再关闭',
  dirtyMessage: '权限或配置分配尚未保存，关闭后修改将丢失。',
})

async function requestAssignmentClose() {
  if (await assignmentGuard.confirmLeave()) closeAssignment()
}

function handleAssignmentBeforeClose(done: () => void) {
  void assignmentGuard.confirmLeave().then((allowed) => {
    if (!allowed) return
    closeAssignment()
    done()
  })
}
</script>
