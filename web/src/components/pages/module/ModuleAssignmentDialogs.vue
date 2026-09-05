<template>
  <ResponsiveDetailCarrier
    :model-value="!!assignmentTarget"
    :title="assignmentConfig?.title || '配置'"
    :size="assignmentPanel.size.value" :docked="assignmentPanel.docked.value"
    docked-auto-focus="first-editable"
    :close-on-click-modal="!assignmentSaving"
    :close-on-press-escape="!assignmentSaving"
    :show-close="!assignmentSaving"
    :before-close="handleAssignmentBeforeClose"
  >
    <div v-if="assignmentTarget" class="assignment-panel">
      <div class="assignment-heading">
        <div>
          <strong>{{ assignmentTarget.name || assignmentTarget.username || assignmentTarget.code }}</strong>
          <small v-if="assignmentTarget.code">{{ assignmentTarget.code }}</small>
        </div>
        <span>已选择 {{ selectedAssignmentIDs.length }} 项</span>
      </div>
      <p class="assignment-tip">{{ assignmentConfig?.tip }}</p>
      <span v-if="assignmentDirty" class="assignment-mobile-dirty">有未保存修改</span>
      <el-input v-model.trim="assignmentSearch" class="assignment-search" clearable placeholder="搜索权限名称、编码或说明" aria-label="搜索配置项" />
      <el-alert v-if="assignmentScopeBlockedReason" :title="assignmentScopeBlockedReason" type="warning" :closable="false" show-icon />
      <el-alert v-if="assignmentSaveError" :title="assignmentSaveError" type="error" :closable="false" show-icon />
      <PageState v-if="assignmentOptionsLoading" kind="loading" title="正在加载完整配置项" />
      <PageState v-else-if="assignmentOptionsError" kind="error" title="配置项加载失败" :description="assignmentOptionsError" action-label="重新加载" @action="retryAssignmentOptions" />
      <el-checkbox-group v-else v-model="selectedAssignmentIDs" class="assignment-option-groups">
        <section v-for="group in visibleAssignmentOptionGroups" :key="group.key" class="assignment-option-group">
          <div class="assignment-option-group__heading">
            <strong>{{ group.label }}</strong>
            <span>{{ groupSelectedCount(group) }} / {{ group.items.length }}</span>
            <button v-if="canBulkEdit && group.items.length" type="button" class="assignment-group-action" @click="setGroupSelection(group, true)">全选</button>
            <button v-if="canBulkEdit && group.items.length" type="button" class="assignment-group-action" @click="setGroupSelection(group, false)">清空</button>
          </div>
          <div class="assignment-options">
            <el-checkbox v-for="option in group.items" :key="option.id" :value="option.id" :disabled="isAssignmentOptionDisabled(option)" :title="assignmentOptionDisabledReason(option)" class="check-option">
              <span class="check-option-copy">
                <strong>{{ option.name || option.code }}</strong>
                <small v-if="option.code || option.description">{{ option.code || option.description }}</small>
                <small v-if="!assignmentScopeBlockedReason && assignmentOptionDisabledReason(option)" class="check-option-scope-hint">{{ assignmentOptionDisabledReason(option) }}</small>
              </span>
            </el-checkbox>
          </div>
        </section>
      </el-checkbox-group>
      <span v-if="assignmentOptionsReady && !visibleAssignmentOptionGroups.length" class="assignment-empty">{{ assignmentOptions.length ? '没有符合搜索条件的配置项' : '暂无可配置项' }}</span>
    </div>
    <template #footer><div class="assignment-actions"><span class="assignment-dirty-status" :class="{'is-dirty': assignmentDirty}">{{ assignmentDirty ? '有未保存修改' : '未修改' }}</span><el-button :disabled="assignmentSaving" @click="requestAssignmentClose">取消</el-button><el-button type="primary" :loading="assignmentSaving" :disabled="!assignmentOptionsReady || assignmentSaving || !!assignmentScopeBlockedReason" @click="saveAssignment">保存 {{ selectedAssignmentIDs.length }} 项{{ assignmentConfig?.optionKey === 'permissions' ? '权限' : '角色' }}</el-button></div></template>
  </ResponsiveDetailCarrier>

  <ResponsiveDetailCarrier :model-value="!!affiliationTarget" title="修正账号归属" :size="affiliationPanel.size.value" :docked="affiliationPanel.docked.value" docked-auto-focus="first-editable" :close-on-click-modal="!affiliationSaving" :close-on-press-escape="!affiliationSaving" :before-close="closeUserAffiliation">
    <el-form v-if="affiliationTarget" label-position="top" :disabled="affiliationSaving" @submit.prevent="saveUserAffiliation">
      <FormPanelContent>
        <p class="assignment-tip">{{ affiliationTarget.username }} · {{ affiliationTarget.name }}。个人管理账号可不绑定部门，但未绑定时不能执行任务或库存写入。</p>
        <el-alert v-if="affiliationError" :title="affiliationError" type="error" :closable="false" show-icon />
        <FormSection title="账号归属" description="部门终端账号必须同时绑定部门和终端。">
          <FormGrid>
            <el-form-item label="所属部门"><el-select v-model="affiliationDepartmentID" clearable placeholder="不绑定部门"><el-option v-for="item in rowsFor('departments')" :key="item.id" :label="String(item.name)" :value="item.id" :disabled="item.status === 'disabled'" /></el-select></el-form-item>
            <el-form-item label="所属终端"><el-select v-model="affiliationTerminalID" clearable :disabled="!affiliationDepartmentID" placeholder="不绑定终端"><el-option v-for="item in affiliationTerminalOptions" :key="item.id" :label="String(item.name)" :value="item.id" /></el-select></el-form-item>
          </FormGrid>
        </FormSection>
      </FormPanelContent>
    </el-form>
    <template #footer><div class="form-actions"><el-button :disabled="affiliationSaving" @click="closeUserAffiliation()">取消</el-button><el-button type="primary" :loading="affiliationSaving" :disabled="affiliationTarget?.account_type === 'department_terminal' && (!affiliationDepartmentID || !affiliationTerminalID)" @click="saveUserAffiliation">保存归属</el-button></div></template>
  </ResponsiveDetailCarrier>
</template>

<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import {useDirtyGuard} from '../../../composables/useDirtyGuard'
import {useWorkspaceContext} from '../../../composables/workspaceContext'
import ResponsiveDetailCarrier from '../../ui/ResponsiveDetailCarrier.vue'
import {useResponsiveDetailPanel} from '../../../composables/useResponsiveDetailPanel'
import PageState from '../../ui/PageState.vue'
import FormPanelContent from '../../ui/FormPanelContent.vue'
import FormSection from '../../ui/FormSection.vue'
import FormGrid from '../../ui/FormGrid.vue'
import type {BasicItem} from '../../../types'

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

const assignmentPanel = useResponsiveDetailPanel(computed(() => !!assignmentTarget.value), {complexity: 'standard-form'})
const affiliationPanel = useResponsiveDetailPanel(computed(() => !!affiliationTarget.value), {complexity: 'short-form'})
const assignmentSearch = ref('')
watch(assignmentTarget, () => { assignmentSearch.value = '' })
const visibleAssignmentOptionGroups = computed(() => {
  const query = assignmentSearch.value.trim().toLocaleLowerCase('zh-CN')
  return assignmentOptionGroups.value.map((group) => ({
    ...group,
    items: group.items.filter((option) => !query || [option.name, option.code, option.description].some((value) => String(value || '').toLocaleLowerCase('zh-CN').includes(query))),
  })).filter((group) => group.items.length)
})
const canBulkEdit = computed(() => assignmentOptionsReady.value && !assignmentSaving.value && !assignmentScopeBlockedReason.value)
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

function groupSelectedCount(group: {items: BasicItem[]}) {
  const selected = new Set(selectedAssignmentIDs.value.map(Number))
  return group.items.filter((item) => selected.has(Number(item.id))).length
}

function setGroupSelection(group: {items: BasicItem[]}, selected: boolean) {
  if (!canBulkEdit.value) return
  const next = new Set(selectedAssignmentIDs.value.map(Number))
  for (const option of group.items) {
    if (isAssignmentOptionDisabled(option)) continue
    if (selected) next.add(Number(option.id))
    else next.delete(Number(option.id))
  }
  selectedAssignmentIDs.value = [...next]
}

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

<style scoped>
.assignment-panel {
  display: grid;
  min-width: 0;
  gap: var(--bb-space-4);
}

.assignment-heading {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--bb-space-3);
}

.assignment-heading > div {
  display: grid;
  min-width: 0;
  gap: var(--bb-space-1);
}

.assignment-heading strong {
  overflow-wrap: anywhere;
  color: var(--bb-text-primary);
  font-size: var(--bb-font-size-20);
}

.assignment-heading small,
.assignment-heading > span,
.assignment-tip,
.assignment-dirty-status {
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-13);
  line-height: var(--bb-line-height-relaxed);
}

.assignment-heading > span {
  flex: 0 0 auto;
  font-variant-numeric: tabular-nums;
}

.assignment-tip {
  margin: 0;
}

.assignment-search {
  width: 100%;
}

.assignment-option-groups {
  display: grid;
  min-width: 0;
  gap: var(--bb-space-4);
}

.assignment-option-group {
  display: grid;
  min-width: 0;
  gap: var(--bb-space-2);
}

.assignment-option-group__heading {
  display: flex;
  min-height: 36px;
  align-items: center;
  gap: var(--bb-space-2);
  border-bottom: 1px solid var(--bb-border-subtle);
  color: var(--bb-text-primary);
}

.assignment-option-group__heading strong {
  margin-right: auto;
  font-size: var(--bb-font-size-14);
}

.assignment-option-group__heading > span {
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-12);
  font-variant-numeric: tabular-nums;
}

.assignment-group-action {
  min-height: 32px;
  border: 0;
  background: transparent;
  padding: 0 4px;
  color: var(--bb-accent-text);
  font-size: var(--bb-font-size-12);
  cursor: pointer;
}

.assignment-group-action:hover,
.assignment-group-action:focus-visible { color: var(--bb-action-primary); text-decoration: underline; }
.assignment-group-action:focus-visible { outline: 2px solid var(--bb-focus-color); outline-offset: 2px; }

.assignment-options {
  display: grid;
  min-width: 0;
  gap: 0;
}

.check-option {
  display: flex;
  width: 100%;
  min-width: 0;
  height: auto;
  min-height: 52px;
  box-sizing: border-box;
  align-items: center;
  margin: 0;
  border: 0;
  border-bottom: 1px solid var(--bb-border-subtle);
  border-radius: 0;
  background: transparent;
  padding: 6px 8px;
  white-space: normal;
  cursor: pointer;
  transition: background-color 120ms ease, border-color 120ms ease;
}

.check-option:hover { background: var(--bb-bg-hover); }
.check-option.is-checked { background: var(--bb-accent-selected-bg); }
.check-option:focus-within { outline: 2px solid var(--bb-focus-color); outline-offset: -2px; }
.check-option.is-disabled { cursor: not-allowed; }

.check-option :deep(.el-checkbox__input) { flex: 0 0 auto; }
.check-option :deep(.el-checkbox__label) { min-width: 0; padding-left: var(--bb-space-2); }
.check-option-copy { display: grid; min-width: 0; gap: 2px; }
.check-option-copy strong { min-width: 0; overflow-wrap: anywhere; color: var(--bb-text-primary); font-size: var(--bb-font-size-14); font-weight: var(--bb-font-weight-semibold); }
.check-option-copy small { min-width: 0; overflow-wrap: anywhere; color: var(--bb-text-secondary); font-size: var(--bb-font-size-12); line-height: var(--bb-line-height-base); }
.check-option-scope-hint { color: var(--bb-warning) !important; }
.assignment-empty { color: var(--bb-text-secondary); font-size: var(--bb-font-size-13); }

.assignment-actions {
  display: flex;
  width: 100%;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: var(--bb-space-2);
}

.assignment-dirty-status { margin-right: auto; }
.assignment-dirty-status.is-dirty { color: var(--bb-warning); }
.assignment-mobile-dirty { display: none; color: var(--bb-warning); font-size: var(--bb-font-size-13); }

@media (max-width: 520px) {
  .assignment-heading { display: grid; }
  .assignment-heading > span { justify-self: start; }
  .assignment-actions { justify-content: stretch; }
  .assignment-actions .el-button { flex: 1 1 0; }
  .assignment-dirty-status { display: none; }
  .assignment-mobile-dirty { display: block; }
}
</style>
