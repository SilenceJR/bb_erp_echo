<template>
      <ResponsiveDetailCarrier
        v-model="warehouseDrawerVisible"
        drawer-class="workspace-detail-drawer"
        :docked="detailPanelDocked"
        :size="detailPanelSize"
        :title="showQuickSupplier ? '新增供应商' : '物品详情'"
        :before-close="handleWarehousePanelBeforeClose"
        :close-on-press-escape="!movementSubmitting && !quickSupplierSubmitting"
        :show-close="!movementSubmitting && !quickSupplierSubmitting"
        :docked-auto-focus="showQuickSupplier ? 'first-editable' : 'panel'"
        destroy-on-close
        @closed="resetWarehouseItem"
      >
        <el-form v-if="showQuickSupplier" id="quick-supplier-editor" class="quick-supplier-editor" label-position="top" :disabled="quickSupplierSubmitting" @submit.prevent="createQuickSupplier">
          <FormPanelContent>
            <FormSection title="供应商资料" description="保存后将自动选入当前采购入库单。">
              <FormGrid columns="one">
                <el-form-item label="供应商名称" required><el-input id="quick-supplier-name" v-model.trim="quickSupplier.name" /></el-form-item>
                <el-form-item label="供应商编码" required><el-input v-model.trim="quickSupplier.code" /></el-form-item>
                <el-form-item label="联系人"><el-input v-model.trim="quickSupplier.contact" /></el-form-item>
                <el-form-item label="联系电话"><el-input v-model.trim="quickSupplier.phone" /></el-form-item>
              </FormGrid>
            </FormSection>
            <el-alert v-if="quickSupplierError" :title="quickSupplierError" type="error" :closable="false" show-icon />
          </FormPanelContent>
        </el-form>
        <div v-else-if="selectedWarehouseItem" class="item-drawer" aria-label="物品详情">
          <el-alert v-if="moduleUnavailable" title="仓库暂不可用" description="已填写内容仍保留，当前不能提交。请确认后再关闭。" type="warning" :closable="false" show-icon />
          <div class="drawer-heading">
            <div>
              <small>{{ selectedWarehouseItem.category }}</small>
              <h2>{{ selectedWarehouseItem.name }}</h2>
              <span>{{ selectedWarehouseItem.code }} · {{ selectedWarehouseItem.spec || '无规格' }}</span>
            </div>
          </div>

          <PageState v-if="warehouseDetailLoading" kind="loading" title="正在加载库存详情" />
          <PageState
            v-else-if="warehouseDetailError"
            kind="error"
            title="库存详情加载失败"
            :description="warehouseDetailError"
            action-label="重新加载"
            @action="loadWarehouseItemDetail"
          />
          <template v-else-if="warehouseDetail">
          <el-alert
            class="drawer-status-alert"
            :title="warehouseQuantityAvailable ? selectedWarehouseStockState.label : '库存数量未返回'"
            :description="!warehouseQuantityAvailable ? '当前详情缺少可核对的库存数量，已暂停依赖库存的办理操作。' : selectedWarehouseStockState.tone === 'success' ? '当前库存高于安全库存，可以按权限继续办理。' : '请先核对现有库存和本次办理数量，避免造成负库存。'"
            :type="warehouseQuantityAvailable ? selectedWarehouseAlertType : 'info'"
            :closable="false"
            show-icon
          />

          <div class="stock-summary">
            <div><span>当前库存</span><strong>{{ warehouseQuantityAvailable ? `${formatQuantity(warehouseDetail.quantity)} ${selectedWarehouseItem.unit}` : '—' }}</strong></div>
            <div><span>安全库存</span><strong>{{ formatQuantity(selectedWarehouseItem.safety_stock) }} {{ selectedWarehouseItem.unit }}</strong></div>
            <div v-if="hasPermission('cost:view')"><span>库存金额</span><strong>{{ warehouseDetail.amount === null || warehouseDetail.amount === undefined ? '—' : formatMoney(warehouseDetail.amount) }}</strong></div>
          </div>
          <ImageGallery v-if="activeWarehouseTab === 'product'" owner-type="product" :owner-id="selectedWarehouseItem.id" :token="token" :can-write="hasPermission('warehouse:write')" category="product"/>
          <p v-if="panelMessage" class="drawer-message">{{ panelMessage }}</p>

          <section class="movement-section">
            <h3>办理出入库</h3>
            <el-alert v-if="!warehouseQuantityAvailable" title="库存数量未返回，暂不能办理出入库。" type="warning" :closable="false" show-icon/>
            <div v-else-if="hasPermission('inventory:documents:write')" class="movement-actions">
              <el-button v-for="definition in availableMovementDefinitions" :key="definition.key" plain type="primary" :disabled="movementSubmitting || Boolean(moduleUnavailable)" @click="startMovement(definition.key)">
                {{ definition.title }}
              </el-button>
            </div>
            <el-alert v-else title="当前账号只能查看库存，办理出入库需要库存单据写入权限。" type="info" :closable="false" show-icon/>
            <p v-if="movementDependencyMessage" class="permission-hint">{{ movementDependencyMessage }}</p>
          </section>

          <el-form v-if="movementMode" id="warehouse-movement-editor" class="movement-form" label-position="top" :disabled="movementSubmitting" @submit.prevent="!moduleUnavailable && submitMovement()">
            <div class="form-heading">
              <strong>{{ movementTitle }}</strong>
              <span>本次只办理当前物品，提交后立即过账</span>
            </div>
            <el-alert v-if="movementFormError" :title="movementFormError" type="error" :closable="false" show-icon/>
            <el-form-item v-if="movementMode === 'purchase_inbound'" label="供应商" required>
              <el-select v-model="movementForm.supplier_id" filterable placeholder="请选择供应商">
                <el-option v-for="item in rowsFor('suppliers')" :key="item.id" :label="`${item.name}（${item.code}）`" :value="item.id"/>
              </el-select>
            </el-form-item>
            <el-button v-if="movementMode === 'purchase_inbound' && hasPermission('suppliers:write')" link type="primary" class="inline-link" :disabled="movementSubmitting" @click="showQuickSupplier = true">
              新增供应商
            </el-button>

            <el-form-item v-if="movementMode === 'return_rework_inbound'" label="退回来源">
              <el-radio-group v-model="movementForm.source_type" @change="resetMovementSource">
                <el-radio-button v-if="hasPermission('customers:read')" value="customer">客户退回</el-radio-button>
                <el-radio-button v-if="hasPermission('system:departments:read')" value="department">部门退回</el-radio-button>
              </el-radio-group>
            </el-form-item>
            <el-form-item v-if="movementMode === 'customer_outbound' || (movementMode === 'return_rework_inbound' && movementForm.source_type === 'customer')" label="客户" required>
              <el-select v-model="movementForm.customer_id" filterable placeholder="请选择客户">
                <el-option v-for="item in rowsFor('customers')" :key="item.id" :label="String(item.name)" :value="item.id"/>
              </el-select>
            </el-form-item>
            <el-form-item v-if="movementMode === 'department_outbound' || (movementMode === 'return_rework_inbound' && movementForm.source_type === 'department')" :label="movementMode === 'department_outbound' ? '目标部门' : '退回部门'" required>
              <el-select v-model="movementForm.department_id" filterable placeholder="请选择部门">
                <el-option v-for="item in rowsFor('departments')" :key="item.id" :label="String(item.name)" :value="item.id"/>
              </el-select>
            </el-form-item>
            <el-form-item v-if="movementMode === 'return_rework_inbound'" label="原出库记录（可选）">
              <el-select v-model="movementForm.original_document_id" clearable placeholder="不关联原记录">
                <el-option v-for="item in eligibleOriginalDocuments" :key="item.id" :label="`${item.code} · ${formatDate(item.posted_at)}`" :value="item.id"/>
              </el-select>
            </el-form-item>
            <el-form-item :label="`数量（${selectedWarehouseItem.unit}）`" required>
              <el-input-number v-model="movementForm.quantity" :min="0" :max="999999999" :step="0.0001" :precision="4" :controls="false" :input-attrs="{'aria-invalid': Boolean(movementQuantityError), 'aria-describedby': 'movement-quantity-help movement-quantity-error'}" placeholder="请输入 0–999999999，最多 4 位小数"/>
              <small id="movement-quantity-help" class="field-help">可填 0–999999999，最多 4 位小数；0 不可提交。当前库存 {{ formatQuantity(warehouseDetail?.quantity) }}；办理后预计 {{ formatQuantity(expectedStockQuantity) }} {{ selectedWarehouseItem.unit }}</small>
              <small v-show="movementQuantityError" id="movement-quantity-error" class="field-error" aria-live="polite">{{ movementQuantityError }}</small>
            </el-form-item>
            <el-form-item v-if="movementMode === 'purchase_inbound' && hasPermission('cost:view')" label="采购单价（元）">
              <el-input-number v-model="movementForm.unit_cost" :min="0" :precision="2" :controls="false" placeholder="选填"/>
            </el-form-item>
            <el-form-item :label="movementMode === 'return_rework_inbound' ? '返工原因' : '备注'" :required="movementMode === 'return_rework_inbound'">
              <el-input v-model.trim="movementForm.reason" type="textarea" :rows="2" placeholder="补充业务说明"/>
            </el-form-item>
            <OperatorSelect
              v-model="movementForm.operator_employee_id"
              :department="operatorDirectory.department.value"
              :employees="operatorDirectory.employees.value"
              :loading="operatorDirectory.loading.value"
              :unavailable-reason="operatorDirectory.unavailableReason.value"
              :retryable="operatorDirectory.retryable.value"
              @load="operatorDirectory.load"
              @retry="operatorDirectory.load(true)"
            />
            <aside class="movement-confirm-summary" aria-label="本次办理摘要">
              <strong>本次办理摘要</strong>
              <dl>
                <div><dt>物品</dt><dd>{{ selectedWarehouseItem.name }}</dd></div>
                <div><dt>类型</dt><dd>{{ movementTitle }}</dd></div>
                <div><dt>对象</dt><dd>{{ movementCounterpartyLabel }}</dd></div>
                <div><dt>数量</dt><dd>{{ formatMovementInputQuantity }} {{ selectedWarehouseItem.unit }}</dd></div>
                <div><dt>操作人</dt><dd>{{ operatorDirectory.employees.value.find((item) => item.id === Number(movementForm.operator_employee_id))?.name || '尚未选择' }}</dd></div>
              </dl>
            </aside>
          </el-form>

          <section v-if="hasPermission('inventory:documents:read')" class="movement-history">
            <div class="drawer-section-title"><h3>最近出入库记录</h3><el-button link type="primary" :loading="itemMovementsLoading" :disabled="itemMovementsLoading" @click="loadAllItemMovements">查看全部</el-button></div>
            <PageState v-if="itemMovementsLoading" kind="loading" title="正在加载出入库记录" />
            <PageState
              v-else-if="itemMovementsError"
              kind="error"
              title="出入库记录加载失败"
              :description="itemMovementsError"
              action-label="重新加载"
              @action="loadItemMovements"
            />
            <div v-else-if="itemMovements.length" class="movement-list">
              <article v-for="item in displayedItemMovements" :key="item.id">
                <span class="movement-kind">{{ businessTypeLabel(item.business_type) }}</span>
                <div><strong>{{ item.posted_by_employee_name || item.created_by_employee_name || '历史记录未记录员工' }} · {{ movementQuantity(item) }}</strong><small>{{ item.posted_by || item.created_by ? `登录账号#${item.posted_by || item.created_by}` : '历史账号未记录' }}{{ item.posted_by_terminal_id || item.created_by_terminal_id ? ` · 终端#${item.posted_by_terminal_id || item.created_by_terminal_id}` : '' }} · {{ item.code }} · {{ formatDate(item.posted_at || item.created_at) }}</small></div>
              </article>
            </div>
            <p v-else class="drawer-empty">暂无出入库记录</p>
          </section>
          <el-alert v-else title="出入库记录需要库存单据查看权限。" type="info" :closable="false" show-icon/>
          </template>
        </div>
        <template #footer>
          <template v-if="showQuickSupplier">
            <el-button :disabled="quickSupplierSubmitting" @click="closeSupplier()">返回入库办理</el-button>
            <el-button type="primary" native-type="submit" form="quick-supplier-editor" :loading="quickSupplierSubmitting">保存并选择</el-button>
          </template>
          <template v-else>
            <el-button :disabled="movementSubmitting" @click="closeWarehouseItem">关闭</el-button>
            <template v-if="movementMode">
              <el-button :disabled="movementSubmitting" @click="cancelMovement">取消办理</el-button>
              <el-button type="primary" native-type="submit" form="warehouse-movement-editor" :loading="movementSubmitting" :disabled="!movementCanSubmit || Boolean(moduleUnavailable)">{{ movementSubmitLabel }}</el-button>
            </template>
          </template>
        </template>
      </ResponsiveDetailCarrier>

</template>

<script setup lang="ts">
import {nextTick, watch} from 'vue'
import {useDirtyGuard} from '../../composables/useDirtyGuard'
import ImageGallery from '../ImageGallery.vue'
import PageState from '../ui/PageState.vue'
import OperatorSelect from '../ui/OperatorSelect.vue'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import {useResponsiveDetailPanel} from '../../composables/useResponsiveDetailPanel'
import ResponsiveDetailCarrier from '../ui/ResponsiveDetailCarrier.vue'
import FormPanelContent from '../ui/FormPanelContent.vue'
import FormSection from '../ui/FormSection.vue'
import FormGrid from '../ui/FormGrid.vue'

const {
  moduleUnavailable,
  operatorDirectory,
  token,
  selectedWarehouseItem,
  warehouseDrawerVisible,
  warehouseDetail,
  warehouseDetailLoading,
  warehouseDetailError,
  itemMovements,
  itemMovementsLoading,
  itemMovementsError,
  showAllItemMovements,
  movementMode,
  showQuickSupplier,
  movementSubmitting,
  movementFormError,
  quickSupplierSubmitting,
  quickSupplierError,
  movementForm,
  quickSupplier,
  activeWarehouseTab,
  panelMessage,
  authRequestGeneration,
  statisticsData,
  warehouseTabs,
  warehouseTabOptions,
  movementDefinitions,
  navItems,
  businessItems,
  systemItems,
  activeModule,
  canWriteActive,
  activePageReadonly,
  hasActiveFilters,
  filteredEmptyTitle,
  filteredEmptyDescription,
  isMasterDataValidationPage,
  hasRenderableData,
  listSearchPlaceholder,
  masterDataEmptyTitle,
  masterDataEmptyDescription,
  genericIdentityColumns,
  genericStatusColumn,
  genericCardColumns,
  hasAssignmentAction,
  assignmentConfig,
  assignmentOptions,
  assignmentOptionsReady,
  assignmentOptionGroups,
  userInitial,
  greeting,
  quickActionDefinitions,
  quickActions,
  businessGroups,
  accountTypeText,
  healthStatusLabel,
  dashboardMetricCards,
  dashboardFocusItems,
  availableMovementDefinitions,
  movementDependencyMessage,
  movementTitle,
  movementIsOutbound,
  expectedStockQuantity,
  warehouseQuantityAvailable,
  selectedWarehouseStockState,
  selectedWarehouseAlertType,
  movementQuantityError,
  movementCounterpartyLabel,
  formatMovementInputQuantity,
  movementCanSubmit,
  movementSubmitLabel,
  movementFormDirty,
  displayedItemMovements,
  eligibleOriginalDocuments,
  warehouseSummaryCards,
  moldSummaryCards,
  operationalSummaryCards,
  statisticsCards,
  compactTrendItems,
  formSchema,
  activeWarehouseTabTitle,
  createEntityTitle,
  rowsFor,
  isPaginatedResponse,
  appendQuery,
  hasPermission,
  canReadModule,
  canWriteModule,
  switchModule,
  handleUserCommand,
  resetFilters,
  assignmentOptionsRequestToken,
  openAssignment,
  closeAssignment,
  loadAssignmentOptions,
  retryAssignmentOptions,
  isAssignmentOptionDisabled,
  saveAssignment,
  switchWarehouseTab,
  resetListQuery,
  applySearch,
  handlePageChange,
  handlePageSizeChange,
  login,
  clearAuthSession,
  logout,
  bootstrap,
  loadHealth,
  loadMe,
  preloadBaseData,
  loadActiveModule,
  loadStatistics,
  loadList,
  createItem,
  clearForm,
  toggleCreateForm,
  editSupplier,
  formatCell,
  genericStatusLabel,
  genericStatusTone,
  isGenericStatusColumn,
  formatGenericCell,
  permissionDomainKey,
  permissionDomainLabel,
  genericRowTitle,
  genericRowSubtitle,
  stockState,
  columnLabel,
  invalidateWarehouseRequests,
  openWarehouseItem,
  closeWarehouseItem,
  performWarehouseClose,
  requestWarehouseClose,
  resetWarehouseItem,
  loadWarehouseItemDetail,
  loadItemMovements,
  loadAllItemMovements,
  startMovement,
  cancelMovement,
  resetMovementSource,
  clearMovementForm,
  submitMovement,
  createQuickSupplier,
  decimalToScaled,
  moneyToCents,
  formatQuantity,
  formatMoney,
  formatDate,
  businessTypeLabel,
  movementQuantity,
  departmentName,
  inventoryItemTypeLabel,
  departmentCompletionRate,
  trendNameLabel,
  trendBarPercentage,
} = useWorkspaceContext()
const {docked: detailPanelDocked, size: detailPanelSize} = useResponsiveDetailPanel(warehouseDrawerVisible, true)
let warehouseStepScrollTop = 0
const supplierGuard = useDirtyGuard('warehouse-supplier-editor', {
  busy: () => showQuickSupplier.value && quickSupplierSubmitting.value,
  busyMessage: '供应商正在保存，请稍候。',
  dirty: () => showQuickSupplier.value && Object.values(quickSupplier).some(Boolean),
  dirtyMessage: '供应商信息尚未保存，是否放弃修改？',
})
async function closeSupplier(done?: () => void) {
  if (!(await supplierGuard.confirmLeave())) return
  Object.assign(quickSupplier, {name: '', code: '', contact: '', phone: ''})
  if (done) done()
  else showQuickSupplier.value = false
}
async function handleWarehousePanelBeforeClose(done: () => void) {
  if (showQuickSupplier.value) {
    if (!(await supplierGuard.confirmLeave())) return
    Object.assign(quickSupplier, {name: '', code: '', contact: '', phone: ''})
    showQuickSupplier.value = false
  }
  if (await requestWarehouseClose()) {
    invalidateWarehouseRequests()
    done()
  }
}
watch(showQuickSupplier, async (open, wasOpen) => {
  const body = document.querySelector<HTMLElement>('.workspace-detail-aside .detail-body')
  if (open) warehouseStepScrollTop = body?.scrollTop || 0
  await nextTick()
  if (open) {
    document.querySelector<HTMLElement>('#quick-supplier-name input, #quick-supplier-name')?.focus({preventScroll: true})
    return
  }
  if (!wasOpen) return
  const restoredBody = document.querySelector<HTMLElement>('.workspace-detail-aside .detail-body')
  if (restoredBody) restoredBody.scrollTop = warehouseStepScrollTop
  document.querySelector<HTMLElement>('.inline-link, .movement-form .el-select input')?.focus({preventScroll: true})
})
</script>
