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
        <span>已选 {{ selectedAssignableCount }} / 可配置 {{ assignableAssignmentCount }}</span>
      </div>
      <p class="assignment-tip">{{ assignmentConfig?.tip }}</p>
      <span v-if="assignmentDirty" class="assignment-mobile-dirty">有未保存修改</span>
      <div class="assignment-selection-toolbar" aria-label="批量选择" :aria-busy="assignmentOptionsLoading">
        <el-checkbox
          class="assignment-select-all"
          :model-value="allAssignableSelected"
          :indeterminate="someAssignableSelected && !allAssignableSelected"
          :disabled="!assignmentOptionsReady || !canBulkEdit || !assignableAssignmentCount"
          @change="setAllSelection(Boolean($event))"
        >全选可配置{{ assignmentObjectLabel }}（{{ assignmentOptionsLoading ? '加载中' : assignableAssignmentCount }}）</el-checkbox>
        <button class="assignment-clear-all" type="button" :disabled="!assignmentOptionsReady || !canBulkEdit || !selectedAssignableCount" @click="clearAllSelection">清空可配置{{ assignmentObjectLabel }}</button>
      </div>
      <el-input v-model.trim="assignmentSearch" class="assignment-search" clearable :placeholder="`搜索${assignmentObjectLabel}名称、编码或说明`" aria-label="搜索配置项" />
      <el-alert v-if="assignmentScopeBlockedReason" :title="assignmentScopeBlockedReason" type="warning" :closable="false" show-icon />
      <el-alert v-if="assignmentSaveError" :title="assignmentSaveError" type="error" :closable="false" show-icon />
      <PageState v-if="assignmentOptionsLoading" kind="loading" title="正在加载完整配置项" />
      <PageState v-else-if="assignmentOptionsError" kind="error" title="配置项加载失败" :description="assignmentOptionsError" action-label="重新加载" @action="retryAssignmentOptions" />
      <div v-else class="assignment-option-groups">
        <section v-for="group in visibleAssignmentOptionGroups" :key="group.key" class="assignment-option-group">
          <div class="assignment-option-group__heading">
            <el-checkbox
              class="assignment-group-checkbox"
              :model-value="groupSelectionState(group).checked"
              :indeterminate="groupSelectionState(group).indeterminate"
              :disabled="!assignmentOptionsReady || assignmentSaving || !!assignmentScopeBlockedReason || !groupAssignableCount(group)"
              :aria-label="`全选${group.label}可配置${assignmentObjectLabel}`"
              @click.stop
              @change="setGroupSelection(group, Boolean($event))"
            />
            <button
              :id="`${assignmentGroupID(group.key)}-toggle`"
              class="assignment-group-toggle"
              type="button"
              :aria-expanded="isAssignmentGroupExpanded(group)"
              :aria-controls="assignmentGroupID(group.key)"
              :disabled="assignmentSaving || Boolean(assignmentSearch.trim())"
              @click="toggleAssignmentGroup(group.key)"
            >
              <strong>{{ group.label }}</strong>
              <span>{{ groupSelectedCount(group) }} / {{ groupAssignableCount(group) }}</span>
              <small v-if="assignmentSearch.trim()">匹配 {{ group.visibleItems.length }}</small>
              <i aria-hidden="true" class="assignment-group-chevron" />
            </button>
          </div>
          <el-checkbox-group
            v-show="isAssignmentGroupExpanded(group)"
            :id="assignmentGroupID(group.key)"
            v-model="selectedAssignmentIDs"
            class="assignment-options"
            :aria-label="`${group.label}${assignmentObjectLabel}`"
          >
            <el-checkbox v-for="option in group.visibleItems" :key="option.id" :value="option.id" :disabled="isAssignmentOptionDisabled(option) || assignmentSaving" class="check-option">
              <el-tooltip :content="assignmentOptionTooltip(option)" placement="top-start" :show-after="350" :disabled="!assignmentOptionTooltip(option)">
                <span class="check-option-copy">
                  <strong>{{ option.name || option.code }}</strong>
                  <small v-if="option.code">{{ option.code }}</small>
                  <small v-else-if="option.description">{{ option.description }}</small>
                  <small v-if="!assignmentScopeBlockedReason && assignmentOptionDisabledReason(option)" class="check-option-scope-hint">{{ assignmentOptionDisabledReason(option) }}</small>
                </span>
              </el-tooltip>
            </el-checkbox>
          </el-checkbox-group>
        </section>
      </div>
      <span v-if="assignmentOptionsReady && !visibleAssignmentOptionGroups.length" class="assignment-empty">{{ assignmentOptions.length ? '没有符合搜索条件的配置项' : '暂无可配置项' }}</span>
    </div>
    <template #footer><div class="assignment-actions"><span class="assignment-dirty-status" :class="{'is-dirty': assignmentDirty}">{{ assignmentDirty ? '有未保存修改' : '未修改' }}</span><el-button :disabled="assignmentSaving" @click="requestAssignmentClose">取消</el-button><el-button type="primary" :loading="assignmentSaving" :disabled="!assignmentOptionsReady || assignmentSaving || !assignmentDirty || !!assignmentScopeBlockedReason" @click="saveAssignment">保存 {{ selectedAssignmentIDs.length }} 项{{ assignmentObjectLabel }}</el-button></div></template>
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
  assignmentTarget, assignmentModuleKey, assignmentConfig, selectedAssignmentIDs, assignmentSaveError,
  assignmentOptionsLoading, assignmentOptionsError, retryAssignmentOptions,
  assignmentOptionGroups, isAssignmentOptionDisabled, assignmentOptionsReady,
  assignmentOptionDisabledReason, assignmentScopeBlockedReason,
  assignmentOptions, assignmentSaving, closeAssignment, saveAssignment,
  affiliationTarget, affiliationSaving, affiliationError, affiliationDepartmentID,
  affiliationTerminalID, affiliationTerminalOptions, rowsFor,
  closeUserAffiliation, saveUserAffiliation,
} = useWorkspaceContext()

const assignmentPanel = useResponsiveDetailPanel(computed(() => !!assignmentTarget.value), {width: 480})
const affiliationPanel = useResponsiveDetailPanel(computed(() => !!affiliationTarget.value), {complexity: 'short-form'})
const assignmentSearch = ref('')
type AssignmentGroup = {key: string; label: string; items: BasicItem[]}
type VisibleAssignmentGroup = AssignmentGroup & {visibleItems: BasicItem[]}

const expandedAssignmentGroups = ref<Set<string>>(new Set())
const assignmentExpansionInitializedFor = ref('')
watch(assignmentTarget, () => {
  assignmentSearch.value = ''
  expandedAssignmentGroups.value = new Set()
  assignmentExpansionInitializedFor.value = ''
})
const assignmentObjectLabel = computed(() => assignmentConfig.value?.optionKey === 'permissions' ? '权限' : '角色')
const visibleAssignmentOptionGroups = computed<VisibleAssignmentGroup[]>(() => {
  const query = assignmentSearch.value.trim().toLocaleLowerCase('zh-CN')
  return assignmentOptionGroups.value.map((group) => ({
    ...group,
    visibleItems: group.items.filter((option) => !query || [option.name, option.code, option.description].some((value) => String(value || '').toLocaleLowerCase('zh-CN').includes(query))),
  })).filter((group) => group.visibleItems.length)
})
const canBulkEdit = computed(() => assignmentOptionsReady.value && !assignmentSaving.value && !assignmentScopeBlockedReason.value)
const assignableAssignmentOptions = computed(() => assignmentOptions.value.filter((option) => !isAssignmentOptionDisabled(option)))
const assignableAssignmentCount = computed(() => assignableAssignmentOptions.value.length)
const selectedAssignableCount = computed(() => {
  const selected = new Set(selectedAssignmentIDs.value.map(Number))
  return assignableAssignmentOptions.value.filter((option) => selected.has(Number(option.id))).length
})
const allAssignableSelected = computed(() => assignableAssignmentCount.value > 0 && selectedAssignableCount.value === assignableAssignmentCount.value)
const someAssignableSelected = computed(() => selectedAssignableCount.value > 0 && !allAssignableSelected.value)
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

watch(
  () => [assignmentTarget.value?.id, assignmentModuleKey.value, assignmentOptionsReady.value] as const,
  () => {
    if (!assignmentTarget.value || !assignmentOptionsReady.value) return
    const targetKey = `${assignmentModuleKey.value}:${assignmentTarget.value.id}`
    if (assignmentExpansionInitializedFor.value === targetKey) return
    const selected = new Set(selectedAssignmentIDs.value.map(Number))
    expandedAssignmentGroups.value = new Set(assignmentOptionGroups.value
      .filter((group) => group.items.some((option) => selected.has(Number(option.id))))
      .map((group) => group.key))
    assignmentExpansionInitializedFor.value = targetKey
  },
  {immediate: true},
)

function isOptionAssignable(option: BasicItem) {
  return !isAssignmentOptionDisabled(option)
}

function groupAssignableCount(group: AssignmentGroup) {
  return group.items.filter(isOptionAssignable).length
}

function groupSelectedCount(group: AssignmentGroup) {
  const selected = new Set(selectedAssignmentIDs.value.map(Number))
  return group.items.filter((item) => isOptionAssignable(item) && selected.has(Number(item.id))).length
}

function groupSelectionState(group: AssignmentGroup) {
  const available = groupAssignableCount(group)
  const selected = groupSelectedCount(group)
  return {checked: available > 0 && selected === available, indeterminate: selected > 0 && selected < available}
}

function setAllSelection(selected: boolean) {
  if (!canBulkEdit.value) return
  const next = new Set(selectedAssignmentIDs.value.map(Number))
  for (const option of assignableAssignmentOptions.value) {
    if (selected) next.add(Number(option.id))
    else next.delete(Number(option.id))
  }
  selectedAssignmentIDs.value = [...next]
}

function clearAllSelection() {
  setAllSelection(false)
}

function setGroupSelection(group: AssignmentGroup, selected: boolean) {
  if (!canBulkEdit.value) return
  const next = new Set(selectedAssignmentIDs.value.map(Number))
  for (const option of group.items) {
    if (!isOptionAssignable(option)) continue
    if (selected) next.add(Number(option.id))
    else next.delete(Number(option.id))
  }
  selectedAssignmentIDs.value = [...next]
}

function assignmentOptionTooltip(option: BasicItem) {
  return [option.code, option.description].filter((value) => String(value || '').trim()).map(String).join(' · ')
}

function assignmentGroupID(key: string) {
  return `assignment-group-${key.replace(/[^a-zA-Z0-9_-]+/g, '-')}`
}

function isAssignmentGroupExpanded(group: VisibleAssignmentGroup) {
  const query = assignmentSearch.value.trim()
  return Boolean(query) ? group.visibleItems.length > 0 : expandedAssignmentGroups.value.has(group.key)
}

function toggleAssignmentGroup(key: string) {
  if (assignmentSaving.value || assignmentSearch.value.trim()) return
  const next = new Set(expandedAssignmentGroups.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expandedAssignmentGroups.value = next
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
  container: assignment-panel / inline-size;
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

.assignment-selection-toolbar {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: var(--bb-space-3);
  border: 1px solid var(--bb-border-subtle);
  border-radius: var(--bb-radius-md);
  background: var(--bb-bg-subtle);
  padding: 8px 10px;
}

.assignment-select-all {
  min-width: 0;
  min-height: 40px;
  margin: 0;
  padding: 0 4px;
}

.assignment-select-all :deep(.el-checkbox__label) {
  overflow-wrap: anywhere;
  color: var(--bb-text-primary);
  font-size: var(--bb-font-size-13);
  font-weight: var(--bb-font-weight-semibold);
}

.assignment-clear-all {
  min-height: 32px;
  flex: 0 0 auto;
  border: 0;
  background: transparent;
  padding: 0 4px;
  color: var(--bb-accent-text);
  font-size: var(--bb-font-size-12);
  cursor: pointer;
}

.assignment-clear-all:hover:not(:disabled),
.assignment-clear-all:focus-visible:not(:disabled) {
  color: var(--bb-action-primary);
  text-decoration: underline;
}

.assignment-clear-all:focus-visible {
  outline: 2px solid var(--bb-focus-color);
  outline-offset: 2px;
}

.assignment-clear-all:disabled {
  color: var(--bb-text-disabled);
  cursor: not-allowed;
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
  min-height: 40px;
  align-items: center;
  gap: var(--bb-space-2);
  border-bottom: 1px solid var(--bb-border-subtle);
  color: var(--bb-text-primary);
}

.assignment-group-checkbox {
  display: inline-flex;
  width: 40px;
  min-width: 40px;
  height: 40px;
  min-height: 40px;
  flex: 0 0 40px;
  align-items: center;
  justify-content: center;
  margin: 0;
  padding: 0;
}

.assignment-group-toggle {
  display: flex;
  min-width: 0;
  min-height: 36px;
  flex: 1 1 auto;
  align-items: center;
  gap: var(--bb-space-2);
  border: 0;
  background: transparent;
  padding: 0 2px;
  color: var(--bb-text-primary);
  text-align: left;
  cursor: pointer;
}

.assignment-group-toggle:hover,
.assignment-group-toggle:focus-visible {
  color: var(--bb-action-primary);
}

.assignment-group-toggle:focus-visible {
  outline: 2px solid var(--bb-focus-color);
  outline-offset: 2px;
}

.assignment-group-toggle:disabled {
  color: var(--bb-text-primary);
  cursor: default;
}

.assignment-group-toggle strong {
  min-width: 0;
  overflow-wrap: anywhere;
  margin-right: auto;
  font-size: var(--bb-font-size-14);
}

.assignment-group-toggle > span,
.assignment-group-toggle > small {
  flex: 0 0 auto;
  color: var(--bb-text-secondary);
  font-size: var(--bb-font-size-12);
  font-variant-numeric: tabular-nums;
}

.assignment-group-chevron {
  width: 7px;
  height: 7px;
  flex: 0 0 7px;
  border-right: 1.5px solid currentColor;
  border-bottom: 1.5px solid currentColor;
  transform: rotate(45deg) translateY(-2px);
  transition: transform 160ms var(--bb-ease-standard);
}

.assignment-group-toggle[aria-expanded='true'] .assignment-group-chevron {
  transform: rotate(225deg) translate(-1px, -1px);
}

.assignment-options {
  display: grid;
  min-width: 0;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 var(--bb-space-2);
}

.check-option {
  display: flex;
  width: 100%;
  min-width: 0;
  height: auto;
  min-height: 40px;
  box-sizing: border-box;
  align-items: center;
  margin: 0;
  border: 0;
  border-bottom: 1px solid var(--bb-border-subtle);
  border-radius: 0;
  background: transparent;
  padding: 4px 6px;
  white-space: normal;
  cursor: pointer;
  transition: background-color 120ms ease, border-color 120ms ease;
}

.check-option:hover { background: var(--bb-bg-hover); }
.check-option.is-checked { background: var(--bb-accent-selected-bg); }
.check-option:focus-within { outline: 2px solid var(--bb-focus-color); outline-offset: -2px; }
.check-option.is-disabled { cursor: not-allowed; }

.check-option :deep(.el-checkbox__input) { flex: 0 0 auto; }
.check-option :deep(.el-checkbox__label) { min-width: 0; flex: 1 1 auto; padding-left: var(--bb-space-2); line-height: var(--bb-line-height-base); }
.check-option-copy { display: grid; min-width: 0; gap: 1px; }
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

@container assignment-panel (max-width: 420px) {
  .assignment-options { grid-template-columns: 1fr; }
}

@media (max-width: 520px) {
  .assignment-heading { display: grid; }
  .assignment-heading > span { justify-self: start; }
  .assignment-selection-toolbar { align-items: flex-start; flex-direction: column; gap: var(--bb-space-1); }
  .assignment-clear-all { align-self: flex-end; }
  .assignment-actions { justify-content: stretch; }
  .assignment-actions .el-button { flex: 1 1 0; }
  .assignment-dirty-status { display: none; }
  .assignment-mobile-dirty { display: block; }
}

@media (prefers-reduced-motion: reduce) {
  .assignment-group-chevron,
  .check-option { transition-duration: 0ms; }
}
</style>
