<template>
        <div class="data-page">
          <PageHeader
            :title="activeModule?.title || '业务页面'"
            :description="activeModule?.description"
            :readonly="activePageReadonly"
            @back="switchModule('dashboard')"
          >
            <template #actions>
              <el-button
                v-if="formSchema.length && canWriteActive"
                class="add-button"
                type="primary"
                @click="toggleCreateForm"
            >
              {{ showCreateForm ? '收起' : `＋ 新增${createEntityTitle}` }}
              </el-button>
            </template>
          </PageHeader>

          <div v-if="activeModule?.key === 'warehouses'" class="warehouse-tabs" aria-label="仓库分类">
            <el-segmented v-model="activeWarehouseTab" :options="warehouseTabOptions" @change="switchWarehouseTab"/>
          </div>

          <el-form v-if="formSchema.length && canWriteActive && showCreateForm" class="inline-form" label-position="top" @submit.prevent="createItem">
            <div class="form-heading">
              <strong>{{ editingSupplier ? '编辑供应商' : `新增${createEntityTitle}` }}</strong>
              <span>请填写以下信息，带 * 为常用必填项</span>
            </div>
            <el-alert v-if="formError" class="form-error" :title="formError" type="error" :closable="false" show-icon/>
            <el-form-item v-for="field in formSchema" :key="field.key" :label="field.label" :required="field.required">
              <el-select v-if="field.kind === 'select'" v-model="formState[field.key]" placeholder="请选择" clearable>
                <el-option v-for="option in field.options" :key="option.value" :label="option.label" :value="option.value"/>
              </el-select>
              <el-select v-else-if="field.kind === 'multi-select'" v-model="formState[field.key]" placeholder="请选择" multiple collapse-tags collapse-tags-tooltip>
                <el-option v-for="option in field.options" :key="option.value" :label="option.label" :value="option.value"/>
              </el-select>
              <el-date-picker v-else-if="field.kind === 'date'" v-model="formState[field.key]" value-format="YYYY-MM-DD" type="date" placeholder="请选择日期"/>
              <el-input v-else-if="field.kind === 'textarea'" v-model="formState[field.key]" type="textarea" :rows="3"/>
              <el-input v-else v-model="formState[field.key]" :type="field.kind === 'password' ? 'password' : 'text'" :show-password="field.kind === 'password'"/>
            </el-form-item>
            <div class="form-actions">
              <el-button @click="showCreateForm = false">取消</el-button>
              <el-button type="primary" native-type="submit" :loading="loading">保存</el-button>
            </div>
          </el-form>

          <el-dialog :model-value="!!assignmentTarget" :title="assignmentConfig?.title" width="min(680px, 92vw)" @close="closeAssignment">
            <div v-if="assignmentTarget" class="assignment-panel">
              <div class="assignment-heading">
                <div>
                  <strong>{{ assignmentTarget.name || assignmentTarget.username || assignmentTarget.code }}</strong>
                  <span>已选择 {{ selectedAssignmentIDs.length }} 项</span>
                </div>
              </div>
              <p class="assignment-tip">
                {{ assignmentConfig?.tip }}
              </p>
              <el-alert v-if="assignmentSaveError" :title="assignmentSaveError" type="error" :closable="false" show-icon/>
              <PageState v-if="assignmentOptionsLoading" kind="loading" title="正在加载完整配置项" />
              <PageState v-else-if="assignmentOptionsError" kind="error" title="配置项加载失败" :description="assignmentOptionsError" action-label="重新加载" @action="retryAssignmentOptions" />
              <el-checkbox-group v-else v-model="selectedAssignmentIDs" class="assignment-option-groups">
                <section v-for="group in assignmentOptionGroups" :key="group.key" class="assignment-option-group">
                  <div class="assignment-option-group__heading"><strong>{{ group.label }}</strong><small>{{ group.items.length }} 项</small></div>
                  <div class="assignment-options">
                    <el-checkbox
                      v-for="option in group.items"
                      :key="option.id"
                      :value="option.id"
                      :disabled="isAssignmentOptionDisabled(option)"
                      class="check-option"
                    >
                      <span class="check-option-copy">
                        <strong>{{ option.name || option.code }}</strong>
                        <small>{{ option.description || option.code }}</small>
                      </span>
                    </el-checkbox>
                  </div>
                </section>
              </el-checkbox-group>
              <span v-if="assignmentOptionsReady && !assignmentOptions.length" class="assignment-empty">暂无可配置项</span>
            </div>
            <template #footer>
              <div class="assignment-actions">
              <el-button @click="closeAssignment">取消</el-button>
              <el-button type="primary" :loading="assignmentSaving" :disabled="!assignmentOptionsReady || assignmentSaving" @click="saveAssignment">保存配置</el-button>
              </div>
            </template>
          </el-dialog>

          <FilterBar
            v-if="activeKey !== 'updates'"
            :message="panelMessage"
            :loading="loading"
            :resettable="hasActiveFilters"
            :aria-label="`${activeModule?.title || '数据'}筛选`"
            @submit="applySearch"
            @reset="resetFilters"
            @refresh="loadActiveModule"
          >
            <el-input
              v-model.trim="searchKeyword"
              class="keyword-input"
              clearable
              :placeholder="listSearchPlaceholder"
              aria-label="关键词"
            />
            <template v-if="activeKey === 'workorder'">
              <el-select v-model="workorderStatusFilter" class="filter-select" placeholder="状态" aria-label="任务状态" @change="applySearch">
                <el-option v-for="option in workorderStatusOptions" :key="option.value" :label="option.label" :value="option.value"/>
              </el-select>
              <el-select v-model="workorderTypeFilter" class="filter-select" placeholder="类型" aria-label="任务类型" @change="applySearch">
                <el-option v-for="option in workorderTypeOptions" :key="option.value" :label="option.label" :value="option.value"/>
              </el-select>
              <el-select v-model="workorderPriorityFilter" class="filter-select" placeholder="优先级" aria-label="任务优先级" @change="applySearch">
                <el-option v-for="option in workorderPriorityOptions" :key="option.value" :label="option.label" :value="option.value"/>
              </el-select>
            </template>
          </FilterBar>

          <section
            v-if="operationalSummaryCards.length && !loading && rows.length"
            class="operational-summary-grid"
            :aria-label="`${activeModule?.title || '业务'}当前页摘要`"
          >
            <MetricCard
              v-for="card in operationalSummaryCards"
              :key="card.label"
              :label="card.label"
              :value="card.value"
              :caption="card.caption"
              :tone="card.tone"
            />
          </section>

          <PageState
            v-if="skeletonResult"
            kind="readonly"
            :title="skeletonResult.name"
            :description="skeletonResult.message"
          />

          <UpdateCenter
              v-else-if="activeKey === 'updates'"
              :token="token"
              :can-check="hasPermission('system:updates:write')"
          />

          <PageState
            v-else-if="listError && !hasRenderableData"
            kind="error"
            title="数据加载失败"
            :description="listError"
            action-label="重新加载"
            @action="loadActiveModule"
          />

          <section v-else-if="activeKey === 'statistics'" v-loading="loading" class="statistics-page">
            <el-alert v-if="listError && statisticsData" :title="listError" type="error" :closable="false" show-icon />
            <PageState v-if="loading && !statisticsData" kind="loading" title="正在生成统计报表" />
            <template v-else>
            <div class="report-overview-heading">
              <div>
                <h2>经营概览</h2>
                <p>更新时间：{{ formatDate(statisticsData?.generated_at) }}</p>
              </div>
              <StatusTag
                :label="statisticsData?.can_view_cost ? '可查看成本金额' : '成本金额已隐藏'"
                :tone="statisticsData?.can_view_cost ? 'success' : 'info'"
              />
            </div>
            <div class="stats-grid">
              <MetricCard
                v-for="card in statisticsCards"
                :key="card.label"
                :label="card.label"
                :value="card.value"
                :caption="card.caption"
                :tone="card.tone"
                :status-label="card.statusLabel"
                :status-tone="card.statusTone"
              />
            </div>

            <div class="report-grid">
              <section class="report-panel">
                <div class="drawer-section-title"><h3>库存分类</h3><small>{{ statisticsData?.can_view_cost ? '含库存金额' : '金额已按权限隐藏' }}</small></div>
                <div v-if="statisticsData?.inventory?.by_item_type?.length" class="metric-list">
                  <article v-for="item in statisticsData?.inventory?.by_item_type || []" :key="String(item.name)">
                    <span>{{ inventoryItemTypeLabel(item.name) }}</span>
                    <strong>{{ formatQuantity(item.value) }}</strong>
                    <small v-if="statisticsData?.can_view_cost">{{ formatMoney(item.amount) }}</small>
                  </article>
                </div>
                <p v-else class="report-empty">暂无库存分类数据</p>
              </section>

              <section class="report-panel">
                <div class="drawer-section-title"><h3>任务状态</h3><small>主任务</small></div>
                <div v-if="statisticsData?.workorders?.by_status?.length" class="metric-list">
                  <article v-for="item in statisticsData?.workorders?.by_status || []" :key="String(item.name)">
                    <span>{{ workorderStatusLabel(item.name) }}</span>
                    <strong>{{ item.value }}</strong>
                  </article>
                </div>
                <p v-else class="report-empty">暂无任务状态数据</p>
              </section>

              <section class="report-panel">
                <div class="drawer-section-title"><h3>部门处理</h3><small>子任务</small></div>
                <div v-if="statisticsData?.workorders?.by_department?.length" class="department-stat-list">
                  <article v-for="item in statisticsData?.workorders?.by_department || []" :key="Number(item.department_id)">
                    <div><strong>{{ item.name || departmentName(item.department_id) }}</strong><small>共 {{ item.total }} 项</small></div>
                    <el-progress :percentage="departmentCompletionRate(item)" :stroke-width="8"/>
                    <small>完成 {{ item.completed }} · 处理中 {{ item.processing }} · 部分完成 {{ item.partial }} · 已收到 {{ item.received }}</small>
                  </article>
                </div>
                <p v-else class="report-empty">暂无部门处理数据</p>
              </section>

              <section class="report-panel">
                <div class="drawer-section-title"><h3>模具状态</h3><small>台账</small></div>
                <div v-if="statisticsData?.molds?.by_status?.length" class="metric-list">
                  <article v-for="item in statisticsData?.molds?.by_status || []" :key="String(item.name)">
                    <span>{{ moldStatusLabel(item.name) }}</span>
                    <strong>{{ item.value }}</strong>
                  </article>
                </div>
                <p v-else class="report-empty">暂无模具状态数据</p>
              </section>
            </div>

            <div class="report-grid lower">
              <section class="report-panel">
                <div class="drawer-section-title"><h3>低库存</h3><small>安全库存预警</small></div>
                <div v-if="statisticsData?.inventory?.low_stock?.length" class="report-table">
                  <article v-for="item in statisticsData.inventory.low_stock" :key="`${item.item_type}-${item.item_id}`">
                    <div><strong>{{ item.name }}</strong><small>{{ item.code }} · {{ item.category }}</small></div>
                    <div class="report-table__status">
                      <StatusTag :label="stockState(item).label" :tone="stockState(item).tone"/>
                      <small>{{ formatQuantity(item.quantity) }} / {{ formatQuantity(item.safety_stock) }}</small>
                    </div>
                  </article>
                </div>
                <p v-else class="drawer-empty">暂无低库存预警</p>
              </section>

              <section class="report-panel">
                <div class="drawer-section-title"><h3>需关注模具</h3><small>借出/维修/保养到期</small></div>
                <div v-if="statisticsData?.molds?.need_care?.length" class="report-table">
                  <article v-for="item in statisticsData.molds.need_care" :key="Number(item.id)">
                    <div><strong>{{ item.name }}</strong><small>{{ item.code }} · {{ item.current_location || '-' }}</small></div>
                    <div class="report-table__status">
                      <StatusTag :label="moldStatusLabel(item.status)" :tone="moldStatusTone(item.status)"/>
                      <small>{{ moldMaintenanceState(item).label }}</small>
                    </div>
                  </article>
                </div>
                <p v-else class="drawer-empty">暂无需要关注的模具</p>
              </section>

              <section class="report-panel">
                <div class="drawer-section-title"><h3>最近任务</h3><small>按创建时间</small></div>
                <div v-if="statisticsData?.recent_workorders?.length" class="report-table">
                  <article v-for="item in statisticsData.recent_workorders" :key="Number(item.id)">
                    <div><strong>{{ item.title }}</strong><small>{{ item.code }} · {{ item.product_name || workorderTypeLabel(item.type) }}</small></div>
                    <StatusTag :label="workorderStatusLabel(item.status)" :tone="workorderStatusTone(item.status)"/>
                  </article>
                </div>
                <p v-else class="drawer-empty">暂无任务单</p>
              </section>

              <section class="report-panel">
                <div class="drawer-section-title"><h3>近 14 天趋势</h3><small>库存流水和任务创建</small></div>
                <div v-if="compactTrendItems.length" class="trend-list">
                  <article v-for="item in compactTrendItems" :key="`${item.date}-${item.name}-${item.value}`">
                    <div><span>{{ item.date }} · {{ trendNameLabel(item.name) }}</span><strong>{{ item.quantity ? formatQuantity(item.quantity) : item.value }}</strong></div>
                    <div class="trend-bar" aria-hidden="true"><span :style="{width: `${trendBarPercentage(item)}%`}"></span></div>
                  </article>
                </div>
                <p v-else class="report-empty">暂无可展示的趋势数据</p>
              </section>
            </div>
            </template>
          </section>

          <DataTableShell
            v-else-if="activeKey === 'warehouses'"
            :loading="loading"
            :error="listError"
            :rows-count="rows.length"
            :total="pageTotal"
            :page="page"
            :page-size="pageSize"
            aria-label="仓库物品列表"
            :empty-title="filteredEmptyTitle"
            :empty-description="filteredEmptyDescription"
            @retry="loadActiveModule"
            @update:page="handlePageChange"
            @update:page-size="handlePageSizeChange"
          >
          <div class="responsive-table-desktop">
          <el-table :data="rows" row-key="id" stripe class="data-table">
            <el-table-column label="物品" min-width="190">
              <template #default="{row}">
                <span class="item-name">{{ row.name }}</span>
                <small class="item-code">{{ row.code }}</small>
              </template>
            </el-table-column>
            <el-table-column prop="spec" label="规格" min-width="130">
              <template #default="{row}">{{ formatCell(row.spec) }}</template>
            </el-table-column>
            <el-table-column prop="unit" label="单位" width="90"/>
            <el-table-column label="当前库存" width="140">
              <template #default="{row}">
                <div class="stock-state-cell">
                  <strong>{{ formatQuantity(row.quantity) }}</strong>
                  <StatusTag :label="stockState(row).label" :tone="stockState(row).tone"/>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="安全库存" width="120">
              <template #default="{row}">{{ formatQuantity(row.safety_stock) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="130" fixed="right">
              <template #default="{row}"><el-button link type="primary" @click="openWarehouseItem(row)">查看与办理</el-button></template>
            </el-table-column>
            <template #empty><el-empty description="该分类还没有物品"/></template>
          </el-table>
          </div>
          <div class="responsive-card-list" role="list">
            <article v-for="row in rows" :key="`${row.item_type}-${row.id}`" class="warehouse-list-card" role="listitem">
              <div class="responsive-card-heading">
                <div><strong>{{ row.name }}</strong><small>{{ row.code }} · {{ row.spec || '无规格' }}</small></div>
                <StatusTag :label="stockState(row).label" :tone="stockState(row).tone"/>
              </div>
              <dl>
                <div><dt>当前库存</dt><dd>{{ formatQuantity(row.quantity) }} {{ row.unit }}</dd></div>
                <div><dt>安全库存</dt><dd>{{ formatQuantity(row.safety_stock) }} {{ row.unit }}</dd></div>
              </dl>
              <el-button type="primary" plain @click="openWarehouseItem(row)">查看物品详情</el-button>
            </article>
          </div>
          </DataTableShell>

          <DataTableShell
            v-else-if="activeKey === 'workorder'"
            :loading="loading"
            :error="listError"
            :rows-count="rows.length"
            :total="pageTotal"
            :page="page"
            :page-size="pageSize"
            aria-label="任务单列表"
            :empty-title="filteredEmptyTitle"
            :empty-description="filteredEmptyDescription"
            @retry="loadActiveModule"
            @update:page="handlePageChange"
            @update:page-size="handlePageSizeChange"
          >
          <div class="responsive-table-desktop">
          <el-table :data="rows" row-key="id" stripe class="data-table">
            <el-table-column label="任务" min-width="220">
              <template #default="{row}">
                <span class="item-name">{{ row.title }}</span>
                <small class="item-code">{{ row.code }} · {{ workorderTypeLabel(row.type) }}</small>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="130">
              <template #default="{row}">
                <StatusTag :label="workorderStatusLabel(row.status)" :tone="workorderStatusTone(row.status)"/>
              </template>
            </el-table-column>
            <el-table-column label="优先级" width="100">
              <template #default="{row}">
                <StatusTag :label="row.priority === 'urgent' ? '加急' : '普通'" :tone="row.priority === 'urgent' ? 'danger' : 'info'"/>
              </template>
            </el-table-column>
            <el-table-column label="产品/数量" min-width="160">
              <template #default="{row}">
                {{ row.product_name || '-' }}<br>
                <small>{{ formatQuantity(row.planned_quantity) }} {{ row.unit || '' }}</small>
              </template>
            </el-table-column>
            <el-table-column label="部门进度" min-width="220">
              <template #default="{row}">
                <div class="department-progress-cell">
                  <div><span>{{ departmentProgressSummary(row) }}</span><strong>{{ departmentProgressMetrics(row).percentage }}%</strong></div>
                  <el-progress :percentage="departmentProgressMetrics(row).percentage" :show-text="false" :stroke-width="8"/>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="交期" width="130">
              <template #default="{row}">
                <div class="due-state-cell">
                  <span>{{ formatDate(row.due_at) }}</span>
                  <StatusTag v-if="workorderDueState(row).overdue" :label="workorderDueState(row).label" tone="danger"/>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="100" fixed="right">
              <template #default="{row}"><el-button link type="primary" @click="openWorkOrder(row)">详情</el-button></template>
            </el-table-column>
            <template #empty><el-empty description="还没有任务单"/></template>
          </el-table>
          </div>
          <div class="responsive-card-list" role="list">
            <article v-for="row in rows" :key="row.id" class="workorder-list-card" role="listitem">
              <div class="responsive-card-heading">
                <div><strong>{{ row.title }}</strong><small>{{ row.code }} · {{ workorderTypeLabel(row.type) }}</small></div>
                <StatusTag :label="workorderStatusLabel(row.status)" :tone="workorderStatusTone(row.status)"/>
              </div>
              <div class="responsive-card-tags"><StatusTag :label="row.priority === 'urgent' ? '加急' : '普通'" :tone="row.priority === 'urgent' ? 'danger' : 'info'"/></div>
              <dl>
                <div><dt>产品/数量</dt><dd>{{ row.product_name || '-' }} · {{ formatQuantity(row.planned_quantity) }} {{ row.unit || '' }}</dd></div>
                <div><dt>交期</dt><dd class="due-state-cell"><span>{{ formatDate(row.due_at) }}</span><StatusTag v-if="workorderDueState(row).overdue" :label="workorderDueState(row).label" tone="danger"/></dd></div>
                <div><dt>部门进度</dt><dd>{{ departmentProgressSummary(row) }}</dd></div>
              </dl>
              <el-progress :percentage="departmentProgressMetrics(row).percentage" :stroke-width="8"/>
              <el-button type="primary" plain @click="openWorkOrder(row)">查看任务详情</el-button>
            </article>
          </div>
          </DataTableShell>

          <DataTableShell
            v-else-if="activeKey === 'molds'"
            :loading="loading"
            :error="listError"
            :rows-count="rows.length"
            :total="pageTotal"
            :page="page"
            :page-size="pageSize"
            aria-label="模具台账列表"
            :empty-title="filteredEmptyTitle"
            :empty-description="filteredEmptyDescription"
            @retry="loadActiveModule"
            @update:page="handlePageChange"
            @update:page-size="handlePageSizeChange"
          >
            <div class="responsive-table-desktop">
              <el-table :data="rows" row-key="id" stripe class="data-table">
                <el-table-column label="模具" min-width="190">
                  <template #default="{row}"><span class="item-name">{{ row.name }}</span><small class="item-code">{{ row.code }}</small></template>
                </el-table-column>
                <el-table-column label="状态" width="120"><template #default="{row}"><StatusTag :label="moldStatusLabel(row.status)" :tone="moldStatusTone(row.status)"/></template></el-table-column>
                <el-table-column prop="current_location" label="当前位置" min-width="150"><template #default="{row}">{{ formatCell(row.current_location) }}</template></el-table-column>
                <el-table-column prop="storage_location" label="存放位置" min-width="150"><template #default="{row}">{{ formatCell(row.storage_location) }}</template></el-table-column>
                <el-table-column label="保养计划" min-width="190">
                  <template #default="{row}">
                    <div class="maintenance-state-cell">
                      <StatusTag :label="moldMaintenanceState(row).label" :tone="moldMaintenanceState(row).tone"/>
                      <small>{{ formatDate(row.next_maintenance_at) }}</small>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column label="操作" width="100" fixed="right"><template #default="{row}"><el-button link type="primary" @click="openMold(row)">详情</el-button></template></el-table-column>
              </el-table>
            </div>
            <div class="responsive-card-list" role="list">
              <article v-for="row in rows" :key="row.id" class="mold-list-card" role="listitem">
                <div class="responsive-card-heading">
                  <div><strong>{{ row.name }}</strong><small>{{ row.code }}</small></div>
                  <StatusTag :label="moldStatusLabel(row.status)" :tone="moldStatusTone(row.status)"/>
                </div>
                <div class="responsive-card-tags">
                  <StatusTag :label="moldMaintenanceState(row).label" :tone="moldMaintenanceState(row).tone"/>
                </div>
                <dl>
                  <div><dt>当前位置</dt><dd>{{ formatCell(row.current_location) }}</dd></div>
                  <div><dt>存放位置</dt><dd>{{ formatCell(row.storage_location) }}</dd></div>
                  <div><dt>下次保养</dt><dd>{{ formatDate(row.next_maintenance_at) }}</dd></div>
                </dl>
                <el-button type="primary" plain @click="openMold(row)">查看模具详情</el-button>
              </article>
            </div>
          </DataTableShell>

          <DataTableShell
            v-else-if="isMasterDataValidationPage"
            :loading="loading"
            :error="listError"
            :rows-count="rows.length"
            :total="pageTotal"
            :page="page"
            :page-size="pageSize"
            :aria-label="`${activeModule?.title || '档案'}列表`"
            :empty-title="masterDataEmptyTitle"
            :empty-description="masterDataEmptyDescription"
            @retry="loadActiveModule"
            @update:page="handlePageChange"
            @update:page-size="handlePageSizeChange"
          >
            <div class="master-data-desktop">
              <el-table :data="rows" row-key="id" stripe class="data-table master-data-table">
                <el-table-column :label="activeKey === 'customers' ? '客户' : '供应商'" min-width="190">
                  <template #default="{row}">
                    <span class="item-name">{{ row.name }}</span>
                    <small class="item-code">{{ row.code || '未设置编码' }}</small>
                  </template>
                </el-table-column>
                <el-table-column v-if="activeKey === 'suppliers'" prop="contact" label="联系人" min-width="120">
                  <template #default="{row}">{{ formatCell(row.contact) }}</template>
                </el-table-column>
                <el-table-column prop="phone" label="联系电话" min-width="150">
                  <template #default="{row}">{{ formatCell(row.phone) }}</template>
                </el-table-column>
                <el-table-column prop="address" label="地址" min-width="220" show-overflow-tooltip>
                  <template #default="{row}">{{ formatCell(row.address) }}</template>
                </el-table-column>
                <el-table-column v-if="activeKey === 'suppliers'" label="状态" width="110" align="center">
                  <template #default="{row}">
                    <StatusTag :label="genericStatusLabel(row.status)" :tone="genericStatusTone(row.status)" />
                  </template>
                </el-table-column>
                <el-table-column v-if="activeKey === 'suppliers' && canWriteActive" label="操作" width="90" fixed="right">
                  <template #default="{row}"><el-button link type="primary" @click="editSupplier(row)">编辑</el-button></template>
                </el-table-column>
              </el-table>
            </div>
            <div class="master-data-mobile" role="list">
              <article v-for="row in rows" :key="row.id" class="master-data-card" role="listitem">
                <div class="master-data-card__heading">
                  <div>
                    <strong>{{ row.name }}</strong>
                    <small>{{ row.code || '未设置编码' }}</small>
                  </div>
                  <StatusTag
                    v-if="activeKey === 'suppliers'"
                    :label="genericStatusLabel(row.status)"
                    :tone="genericStatusTone(row.status)"
                  />
                </div>
                <dl>
                  <div v-if="activeKey === 'suppliers'"><dt>联系人</dt><dd>{{ formatCell(row.contact) }}</dd></div>
                  <div><dt>联系电话</dt><dd>{{ formatCell(row.phone) }}</dd></div>
                  <div><dt>地址</dt><dd>{{ formatCell(row.address) }}</dd></div>
                </dl>
                <el-button v-if="activeKey === 'suppliers' && canWriteActive" link type="primary" @click="editSupplier(row)">编辑供应商</el-button>
              </article>
            </div>
          </DataTableShell>

          <DataTableShell
            v-else
            :loading="loading"
            :error="listError"
            :rows-count="rows.length"
            :total="pageTotal"
            :page="page"
            :page-size="pageSize"
            :aria-label="`${activeModule?.title || '数据'}列表`"
            :empty-title="filteredEmptyTitle"
            :empty-description="filteredEmptyDescription"
            @retry="loadActiveModule"
            @update:page="handlePageChange"
            @update:page-size="handlePageSizeChange"
          >
            <div class="generic-list-desktop">
              <el-table :data="rows" row-key="id" stripe class="data-table generic-data-table">
                <el-table-column v-for="column in columns" :key="column" :label="columnLabel(column)" min-width="130">
                  <template #default="{row}">
                    <StatusTag
                      v-if="isGenericStatusColumn(column)"
                      :label="genericStatusLabel(row[column])"
                      :tone="genericStatusTone(row[column])"
                    />
                    <span v-else>{{ formatGenericCell(column, row[column]) }}</span>
                  </template>
                </el-table-column>
                <el-table-column v-if="hasAssignmentAction" label="配置操作" width="130" fixed="right">
                  <template #default="{row}">
                    <el-button link type="primary" @click="openAssignment(row)">{{ assignmentConfigs[activeKey]?.buttonLabel }}</el-button>
                  </template>
                </el-table-column>
              </el-table>
            </div>
            <div class="generic-list-mobile" role="list">
              <article v-for="row in rows" :key="row.id" class="generic-list-card" role="listitem">
                <div class="generic-list-card__heading">
                  <div>
                    <strong>{{ genericRowTitle(row) }}</strong>
                    <small>{{ genericRowSubtitle(row) }}</small>
                  </div>
                  <StatusTag
                    v-if="genericStatusColumn"
                    :label="genericStatusLabel(row[genericStatusColumn])"
                    :tone="genericStatusTone(row[genericStatusColumn])"
                  />
                </div>
                <dl>
                  <div v-for="column in genericCardColumns" :key="column">
                    <dt>{{ columnLabel(column) }}</dt>
                    <dd>{{ formatGenericCell(column, row[column]) }}</dd>
                  </div>
                </dl>
                <el-button v-if="hasAssignmentAction" type="primary" plain @click="openAssignment(row)">
                  {{ assignmentConfigs[activeKey]?.buttonLabel }}
                </el-button>
              </article>
            </div>
          </DataTableShell>
        </div>
</template>

<script setup lang="ts">
import DataTableShell from '../ui/DataTableShell.vue'
import FilterBar from '../ui/FilterBar.vue'
import MetricCard from '../ui/MetricCard.vue'
import PageHeader from '../ui/PageHeader.vue'
import PageState from '../ui/PageState.vue'
import StatusTag from '../ui/StatusTag.vue'
import UpdateCenter from '../UpdateCenter.vue'
import {useWorkspaceContext} from '../../composables/workspaceContext'

const {
  assignmentConfigs,
  tokenKey,
  desktopClient,
  authRequestGeneration,
  token,
  currentUser,
  activeKey,
  showCreateForm,
  editingSupplier,
  rows,
  columns,
  skeletonResult,
  searchKeyword,
  page,
  pageSize,
  pageTotal,
  loading,
  errorMessage,
  panelMessage,
  listError,
  assignmentTarget,
  assignmentModuleKey,
  selectedAssignmentIDs,
  assignmentOptionsCache,
  assignmentOptionsLoading,
  assignmentOptionsError,
  assignmentSaving,
  assignmentSaveError,
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
  healthStatus,
  mobileNavOpen,
  serverDialogVisible,
  serverTesting,
  serverUrlInput,
  serverMessage,
  serverMessageType,
  clientUpdate,
  loginForm,
  loginUsernameInput,
  formError,
  formState,
  movementForm,
  quickSupplier,
  activeWarehouseTab,
  workorderStatusFilter,
  workorderTypeFilter,
  workorderPriorityFilter,
  selectedWorkOrder,
  workorderDrawerVisible,
  workorderLogs,
  workorderLogsLoading,
  workorderLogsError,
  moldDetailDrawerVisible,
  selectedMoldDetail,
  selectedMoldID,
  moldDetailLoading,
  moldDetailError,
  moldActionSubmitting,
  moldActionError,
  statisticsData,
  selectedMoldMaintenanceState,
  selectedMoldAlertType,
  warehouseTabs,
  warehouseTabOptions,
  movementDefinitions,
  workorderStatusOptions,
  workorderTypeOptions,
  workorderPriorityOptions,
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
  workorderSummaryCards,
  moldSummaryCards,
  operationalSummaryCards,
  statisticsCards,
  compactTrendItems,
  formSchema,
  cache,
  activeWarehouseTabTitle,
  createEntityTitle,
  rowsFor,
  isPaginatedResponse,
  appendQuery,
  hasPermission,
  canReadModule,
  canWriteModule,
  switchModule,
  selectMobileModule,
  restoreMobileMenuFocus,
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
  openServerSettings,
  testServerSetting,
  saveServerSetting,
  bootstrap,
  loadHealth,
  loadMe,
  loadClientUpdate,
  downloadClientUpdate,
  preloadBaseData,
  loadActiveModule,
  loadStatistics,
  loadList,
  createItem,
  normalizedForm,
  validateActiveForm,
  numericKeys,
  clearForm,
  toggleCreateForm,
  editSupplier,
  inferColumns,
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
  columnLabels,
  columnLabel,
  warehouseDetailRequestToken,
  itemMovementsRequestToken,
  moldDetailRequestToken,
  workorderLogsRequestToken,
  warehouseDetailAbortController,
  itemMovementsAbortController,
  moldDetailAbortController,
  workorderLogsAbortController,
  invalidateWarehouseRequests,
  isCurrentWarehouseRequest,
  openWarehouseItem,
  warehouseCloseBypass,
  closeWarehouseItem,
  performWarehouseClose,
  requestWarehouseClose,
  handleWarehouseBeforeClose,
  resetWarehouseItem,
  invalidateMoldDetailRequest,
  isCurrentMoldDetailRequest,
  openMold,
  loadMoldDetail,
  closeMold,
  handleMoldBeforeClose,
  resetMold,
  loanMold,
  returnMold,
  repairMold,
  maintainMold,
  runMoldAction,
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
  openWorkOrder,
  invalidateWorkOrderLogsRequest,
  isCurrentWorkOrderLogsRequest,
  closeWorkOrder,
  handleWorkOrderBeforeClose,
  resetWorkOrder,
  loadWorkOrderLogs,
  dispatchWorkOrder,
  pauseWorkOrder,
  resumeWorkOrder,
  toggleWorkOrderUrgent,
  completeWorkOrder,
  startDepartmentTask,
  partialCompleteDepartmentTask,
  completeDepartmentTask,
  runWorkOrderAction,
  promptText,
  promptTextWithDefault,
  promptPositiveInteger,
  departmentTasks,
  departmentProgressMetrics,
  departmentProgressSummary,
  departmentName,
  canOperateDepartmentTask,
  workorderStatusLabel,
  workorderTypeLabel,
  inventoryItemTypeLabel,
  moldStatusLabel,
  moldStatusTone,
  moldMaintenanceState,
  departmentCompletionRate,
  trendNameLabel,
  trendBarPercentage,
  departmentTaskStatusLabel,
  workorderStatusTone,
  workorderDueState,
  departmentTaskStatusTone,
  workorderNextAction,
  workorderActionLabel,
} = useWorkspaceContext()
</script>
