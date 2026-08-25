<template>
  <el-config-provider :locale="zhCn">
  <main class="app-shell" :class="{ 'is-login': !token }">
    <section v-if="!token" class="login-screen" aria-label="博邦光电登录">
      <div class="login-panel">
        <img class="login-logo" src="/bobang-logo-hd.png" alt="博邦光电"/>
        <el-form class="login-form" label-position="top" @submit.prevent="login">
          <el-form-item label="账号">
            <el-input v-model.trim="loginForm.username" autocomplete="username" clearable/>
          </el-form-item>
          <el-form-item label="密码">
            <el-input v-model="loginForm.password" type="password" autocomplete="current-password" show-password @keyup.enter="login"/>
          </el-form-item>
          <el-button class="login-submit" type="primary" :loading="loading" native-type="submit">登录</el-button>
          <el-alert v-if="errorMessage" :title="errorMessage" type="error" :closable="false" show-icon/>
          <div v-if="desktopClient" class="server-settings login-server-settings">
            <div class="server-settings-heading">
              <span>服务器</span>
              <small>填写运行 Go 服务的内网电脑地址</small>
            </div>
            <el-input v-model.trim="serverUrlInput" placeholder="例如 http://192.168.1.20:8080" clearable/>
            <div class="server-actions">
              <el-button :loading="serverTesting" @click="testServerSetting">测试连接</el-button>
              <el-button type="primary" plain @click="saveServerSetting">保存地址</el-button>
            </div>
            <el-alert v-if="serverMessage" :title="serverMessage" :type="serverMessageType" :closable="false" show-icon/>
          </div>
        </el-form>
      </div>
    </section>

    <section v-else class="workspace">
      <header class="topbar">
        <el-button
          id="mobile-menu-button"
          class="mobile-menu-button"
          circle
          aria-label="打开系统导航"
          @click="mobileNavOpen = true"
        >
          <span class="mobile-menu-icon" aria-hidden="true"><i></i><i></i><i></i></span>
        </el-button>
        <div class="brand">
          <img src="/bobang-logo-hd.png" alt="博邦光电"/>
          <span class="brand-mark" aria-label="博邦光电">BB</span>
          <div>
            <strong>博邦光电</strong>
            <span>业务工作台</span>
          </div>
        </div>
        <div class="user-chip">
          <div class="user-copy">
            <span>{{ currentUser?.name || currentUser?.username }}</span>
            <small>{{ accountTypeText }}</small>
          </div>
          <el-dropdown trigger="click" @command="handleUserCommand">
            <el-button circle class="user-avatar" :aria-label="`${currentUser?.name || currentUser?.username || '用户'}菜单`">{{ userInitial }}</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-if="desktopClient" command="server">服务器设置</el-dropdown-item>
                <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>

      <aside class="sidebar" aria-label="系统导航">
        <AppNavigation
          :active-key="activeKey"
          :business-items="businessItems"
          :system-items="systemItems"
          @select="switchModule"
        />
      </aside>

      <el-drawer
        v-model="mobileNavOpen"
        class="mobile-navigation-drawer"
        direction="ltr"
        size="min(320px, 88vw)"
        title="系统导航"
        :with-header="false"
        append-to-body
        @closed="restoreMobileMenuFocus"
      >
        <div class="mobile-navigation">
          <div class="mobile-navigation__brand">
            <img src="/bobang-logo-hd.png" alt=""/>
            <div><strong>博邦光电</strong><span>业务工作台</span></div>
            <el-button aria-label="关闭系统导航" @click="mobileNavOpen = false">关闭</el-button>
          </div>
          <AppNavigation
            :active-key="activeKey"
            :business-items="businessItems"
            :system-items="systemItems"
            aria-label="移动端系统导航"
            @select="selectMobileModule"
          />
        </div>
      </el-drawer>

      <section class="content">
        <div v-if="activeModule?.key === 'dashboard'" class="dashboard">
          <div class="welcome-block">
            <div>
              <p class="eyebrow">{{ greeting }}</p>
              <h1>{{ currentUser?.name || currentUser?.username }}，今天要处理什么？</h1>
              <p>从常用功能开始，快速完成手头的工作。</p>
            </div>
            <div class="service-status" :class="{ warning: healthStatus !== '正常' }">
              <span></span> 服务{{ healthStatus === '正常' ? '正常' : '暂不可用' }}
            </div>
            <div v-if="desktopClient && clientUpdate.available && clientUpdate.cached" class="client-update">
              <span>客户端 {{ clientUpdate.latest_version || '新版本' }}</span>
              <el-button link type="primary" @click="downloadClientUpdate">下载更新</el-button>
            </div>
          </div>

          <section v-if="quickActions.length" class="home-section">
            <div class="home-section-title">
              <div>
                <h2>常用功能</h2>
                <p>一步直达最常办理的业务</p>
              </div>
            </div>
            <div class="quick-grid">
              <el-card
                  v-for="item in quickActions"
                  :key="item.key"
                  class="quick-card"
                  shadow="hover"
                  role="button"
                  tabindex="0"
                  :aria-label="`打开${item.title}：${item.description}`"
                  @click="switchModule(item.key)"
                  @keyup.enter="switchModule(item.key)"
                  @keyup.space.prevent="switchModule(item.key)"
              >
                <span class="quick-icon" aria-hidden="true">{{ item.icon }}</span>
                <span class="quick-copy">
                  <strong>{{ item.title }}</strong>
                  <small>{{ item.description }}</small>
                </span>
                <span class="quick-arrow" aria-hidden="true">→</span>
              </el-card>
            </div>
          </section>

          <section v-if="businessGroups.length" class="home-section">
            <div class="home-section-title">
              <div>
                <h2>全部业务</h2>
                <p>按工作场景查找功能</p>
              </div>
            </div>
            <div class="business-grid">
              <el-card v-for="group in businessGroups" :key="group.title" class="business-card" shadow="never">
                <div class="business-card-heading">
                  <span aria-hidden="true">{{ group.icon }}</span>
                  <div>
                    <strong>{{ group.title }}</strong>
                    <small>{{ group.caption }}</small>
                  </div>
                </div>
                <el-button
                    v-for="item in group.items"
                    :key="item.key"
                    link
                    @click="switchModule(item.key)"
                >
                  {{ item.title }} <span>→</span>
                </el-button>
              </el-card>
            </div>
          </section>
          <PageState
            v-if="!businessItems.length"
            kind="permission"
            title="当前账号没有可用的日常业务"
            description="如需使用其他功能，请联系管理员为账号配置对应的查看权限。"
          />
        </div>

        <div v-else class="data-page">
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
            <el-form-item v-for="field in formSchema" :key="field.key" :label="field.label">
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
              <el-checkbox-group v-model="selectedAssignmentIDs" class="assignment-options">
                <el-checkbox
                  v-for="option in assignmentOptions"
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
              </el-checkbox-group>
              <span v-if="!assignmentOptions.length" class="assignment-empty">暂无可配置项</span>
            </div>
            <template #footer>
              <div class="assignment-actions">
              <el-button @click="closeAssignment">取消</el-button>
              <el-button type="primary" :loading="loading" @click="saveAssignment">保存配置</el-button>
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
            <div class="stats-grid">
              <article v-for="card in statisticsCards" :key="card.label" class="stat-card">
                <span>{{ card.label }}</span>
                <strong>{{ card.value }}</strong>
                <small>{{ card.caption }}</small>
              </article>
            </div>

            <div class="report-grid">
              <section class="report-panel">
                <div class="drawer-section-title"><h3>库存分类</h3><small>{{ statisticsData?.can_view_cost ? '含库存金额' : '金额已按权限隐藏' }}</small></div>
                <div class="metric-list">
                  <article v-for="item in statisticsData?.inventory?.by_item_type || []" :key="String(item.name)">
                    <span>{{ inventoryItemTypeLabel(item.name) }}</span>
                    <strong>{{ formatQuantity(item.value) }}</strong>
                    <small v-if="statisticsData?.can_view_cost">{{ formatMoney(item.amount) }}</small>
                  </article>
                </div>
              </section>

              <section class="report-panel">
                <div class="drawer-section-title"><h3>任务状态</h3><small>主任务</small></div>
                <div class="metric-list">
                  <article v-for="item in statisticsData?.workorders?.by_status || []" :key="String(item.name)">
                    <span>{{ workorderStatusLabel(item.name) }}</span>
                    <strong>{{ item.value }}</strong>
                  </article>
                </div>
              </section>

              <section class="report-panel">
                <div class="drawer-section-title"><h3>部门处理</h3><small>子任务</small></div>
                <div class="department-stat-list">
                  <article v-for="item in statisticsData?.workorders?.by_department || []" :key="Number(item.department_id)">
                    <div><strong>{{ item.name || departmentName(item.department_id) }}</strong><small>共 {{ item.total }} 项</small></div>
                    <el-progress :percentage="departmentCompletionRate(item)" :stroke-width="8"/>
                    <small>完成 {{ item.completed }} · 处理中 {{ item.processing }} · 部分完成 {{ item.partial }} · 已收到 {{ item.received }}</small>
                  </article>
                </div>
              </section>

              <section class="report-panel">
                <div class="drawer-section-title"><h3>模具状态</h3><small>台账</small></div>
                <div class="metric-list">
                  <article v-for="item in statisticsData?.molds?.by_status || []" :key="String(item.name)">
                    <span>{{ moldStatusLabel(item.name) }}</span>
                    <strong>{{ item.value }}</strong>
                  </article>
                </div>
              </section>
            </div>

            <div class="report-grid lower">
              <section class="report-panel">
                <div class="drawer-section-title"><h3>低库存</h3><small>安全库存预警</small></div>
                <div v-if="statisticsData?.inventory?.low_stock?.length" class="report-table">
                  <article v-for="item in statisticsData.inventory.low_stock" :key="`${item.item_type}-${item.item_id}`">
                    <div><strong>{{ item.name }}</strong><small>{{ item.code }} · {{ item.category }}</small></div>
                    <span>{{ formatQuantity(item.quantity) }} / {{ formatQuantity(item.safety_stock) }}</span>
                  </article>
                </div>
                <p v-else class="drawer-empty">暂无低库存预警</p>
              </section>

              <section class="report-panel">
                <div class="drawer-section-title"><h3>需关注模具</h3><small>借出/维修/保养到期</small></div>
                <div v-if="statisticsData?.molds?.need_care?.length" class="report-table">
                  <article v-for="item in statisticsData.molds.need_care" :key="Number(item.id)">
                    <div><strong>{{ item.name }}</strong><small>{{ item.code }} · {{ item.current_location || '-' }}</small></div>
                    <span>{{ moldStatusLabel(item.status) }}</span>
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
                <div class="trend-list">
                  <article v-for="item in compactTrendItems" :key="`${item.date}-${item.name}-${item.value}`">
                    <span>{{ item.date }} · {{ trendNameLabel(item.name) }}</span>
                    <strong>{{ item.quantity ? formatQuantity(item.quantity) : item.value }}</strong>
                  </article>
                </div>
              </section>
            </div>
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
                <div class="workorder-task-tags">
                  <StatusTag v-for="task in departmentTasks(row)" :key="task.id" :label="`${departmentName(task.department_id)} · ${departmentTaskStatusLabel(task.status)}`" :tone="departmentTaskStatusTone(task.status)"/>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="交期" width="130">
              <template #default="{row}">{{ formatDate(row.due_at) }}</template>
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
                <div><dt>交期</dt><dd>{{ formatDate(row.due_at) }}</dd></div>
                <div><dt>部门进度</dt><dd>{{ departmentProgressSummary(row) }}</dd></div>
              </dl>
              <el-button type="primary" plain @click="openWorkOrder(row)">查看任务详情</el-button>
            </article>
          </div>
          </DataTableShell>

          <el-table v-else-if="activeKey === 'molds'" v-loading="loading" :data="rows" row-key="id" stripe class="data-table">
            <el-table-column label="模具" min-width="190">
              <template #default="{row}"><span class="item-name">{{ row.name }}</span><small class="item-code">{{ row.code }}</small></template>
            </el-table-column>
            <el-table-column label="状态" width="120"><template #default="{row}"><StatusTag :label="moldStatusLabel(row.status)" :tone="moldStatusTone(row.status)"/></template></el-table-column>
            <el-table-column prop="current_location" label="当前位置" min-width="150"><template #default="{row}">{{ formatCell(row.current_location) }}</template></el-table-column>
            <el-table-column prop="next_maintenance_at" label="下次保养" width="140"><template #default="{row}">{{ formatDate(row.next_maintenance_at) }}</template></el-table-column>
            <el-table-column label="操作" width="100" fixed="right"><template #default="{row}"><el-button link type="primary" @click="openMold(row)">详情</el-button></template></el-table-column>
            <template #empty><el-empty :description="filteredEmptyTitle"/></template>
          </el-table>

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

          <el-table v-else v-loading="loading" :data="rows" row-key="id" stripe class="data-table">
            <el-table-column v-for="column in columns" :key="column" :label="columnLabel(column)" min-width="130">
              <template #default="{row}">{{ formatCell(row[column]) }}</template>
            </el-table-column>
            <el-table-column v-if="hasAssignmentAction" label="权限操作" width="130" fixed="right">
              <template #default="{row}">
                <el-button link type="primary" @click="openAssignment(row)">{{ assignmentConfigs[activeKey]?.buttonLabel }}</el-button>
              </template>
            </el-table-column>
            <el-table-column v-if="activeKey === 'suppliers' && canWriteActive" label="操作" width="90" fixed="right">
              <template #default="{row}"><el-button link type="primary" @click="editSupplier(row)">编辑</el-button></template>
            </el-table-column>
            <template #empty><el-empty :description="filteredEmptyTitle"/></template>
          </el-table>

          <div v-if="!skeletonResult && activeKey !== 'updates' && !isMasterDataValidationPage && !['warehouses', 'workorder'].includes(activeKey)" class="pagination-bar">
            <span>共 {{ pageTotal }} 条记录</span>
            <el-pagination
                v-model:current-page="page"
                v-model:page-size="pageSize"
                :page-sizes="[10, 20, 50, 100]"
                :total="pageTotal"
                background
                layout="sizes, prev, pager, next, jumper"
                @current-change="handlePageChange"
                @size-change="handlePageSizeChange"
            />
          </div>
        </div>
      </section>

      <el-dialog v-model="serverDialogVisible" title="服务器设置" width="min(520px, 92vw)" append-to-body>
        <div class="server-settings">
          <p class="server-description">当前客户端通过此地址访问 Go 服务。内网部署可填写服务器电脑的局域网 IP，后续公网部署可直接改为 HTTPS 地址。切换到其他服务器后需要重新登录。</p>
          <el-form label-position="top">
            <el-form-item label="Go 服务地址">
              <el-input v-model.trim="serverUrlInput" placeholder="例如 http://192.168.1.20:8080" clearable/>
            </el-form-item>
          </el-form>
          <el-alert v-if="serverMessage" :title="serverMessage" :type="serverMessageType" :closable="false" show-icon/>
        </div>
        <template #footer>
          <el-button @click="serverDialogVisible = false">取消</el-button>
          <el-button :loading="serverTesting" @click="testServerSetting">测试连接</el-button>
          <el-button type="primary" @click="saveServerSetting">保存地址</el-button>
        </template>
      </el-dialog>

      <el-drawer v-model="warehouseDrawerVisible" size="min(620px, 100%)" title="物品详情" :with-header="false" :before-close="handleWarehouseBeforeClose" destroy-on-close @closed="resetWarehouseItem">
        <div v-if="selectedWarehouseItem" class="item-drawer" aria-label="物品详情">
          <div class="drawer-heading">
            <div>
              <small>{{ selectedWarehouseItem.category }}</small>
              <h2>{{ selectedWarehouseItem.name }}</h2>
              <span>{{ selectedWarehouseItem.code }} · {{ selectedWarehouseItem.spec || '无规格' }}</span>
            </div>
            <el-button circle aria-label="关闭物品详情" @click="closeWarehouseItem">×</el-button>
          </div>

          <div class="stock-summary">
            <div><span>当前库存</span><strong>{{ formatQuantity(warehouseDetail?.quantity) }} {{ selectedWarehouseItem.unit }}</strong></div>
            <div><span>安全库存</span><strong>{{ formatQuantity(selectedWarehouseItem.safety_stock) }} {{ selectedWarehouseItem.unit }}</strong></div>
            <div v-if="hasPermission('cost:view')"><span>库存金额</span><strong>{{ formatMoney(warehouseDetail?.amount) }}</strong></div>
          </div>
          <ImageGallery v-if="activeWarehouseTab === 'product'" owner-type="product" :owner-id="selectedWarehouseItem.id" :token="token" :can-write="hasPermission('warehouse:write')" category="product"/>
          <p v-if="panelMessage" class="drawer-message">{{ panelMessage }}</p>

          <section class="movement-section">
            <h3>办理出入库</h3>
            <div v-if="hasPermission('inventory:documents:write')" class="movement-actions">
              <el-button v-for="definition in availableMovementDefinitions" :key="definition.key" plain type="primary" :disabled="movementSubmitting" @click="startMovement(definition.key)">
                {{ definition.title }}
              </el-button>
            </div>
            <el-alert v-else title="当前账号只能查看库存，办理出入库需要库存单据写入权限。" type="info" :closable="false" show-icon/>
            <p v-if="movementDependencyMessage" class="permission-hint">{{ movementDependencyMessage }}</p>
          </section>

          <el-form v-if="movementMode" class="movement-form" label-position="top" :disabled="movementSubmitting" @submit.prevent="submitMovement">
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
            <el-button v-if="movementMode === 'purchase_inbound' && hasPermission('suppliers:write')" link type="primary" class="inline-link" :disabled="movementSubmitting" @click="showQuickSupplier = !showQuickSupplier">
              {{ showQuickSupplier ? '取消新增供应商' : '＋ 快捷新增供应商' }}
            </el-button>
            <div v-if="showQuickSupplier" class="quick-supplier">
              <el-form-item label="供应商名称" required><el-input v-model.trim="quickSupplier.name" placeholder="请输入供应商名称"/></el-form-item>
              <el-form-item label="供应商编码" required><el-input v-model.trim="quickSupplier.code" placeholder="请输入唯一编码"/></el-form-item>
              <el-form-item label="联系人"><el-input v-model.trim="quickSupplier.contact" placeholder="选填"/></el-form-item>
              <el-form-item label="联系电话"><el-input v-model.trim="quickSupplier.phone" placeholder="选填"/></el-form-item>
              <el-alert v-if="quickSupplierError" :title="quickSupplierError" type="error" :closable="false" show-icon/>
              <el-button :loading="quickSupplierSubmitting" :disabled="movementSubmitting || quickSupplierSubmitting" @click="createQuickSupplier">保存并选择供应商</el-button>
            </div>
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
              <el-input-number v-model="movementForm.quantity" :min="0.0001" :precision="4" :controls="false" :input-attrs="{'aria-invalid': Boolean(movementQuantityError), 'aria-describedby': 'movement-quantity-help movement-quantity-error'}" placeholder="请输入大于 0 的数量"/>
              <small id="movement-quantity-help" class="field-help">当前库存 {{ formatQuantity(warehouseDetail?.quantity) }}；办理后预计 {{ formatQuantity(expectedStockQuantity) }} {{ selectedWarehouseItem.unit }}</small>
              <small v-show="movementQuantityError" id="movement-quantity-error" class="field-error" aria-live="polite">{{ movementQuantityError }}</small>
            </el-form-item>
            <el-form-item v-if="movementMode === 'purchase_inbound' && hasPermission('cost:view')" label="采购单价（元）">
              <el-input-number v-model="movementForm.unit_cost" :min="0" :precision="2" :controls="false" placeholder="选填"/>
            </el-form-item>
            <el-form-item :label="movementMode === 'return_rework_inbound' ? '返工原因' : '备注'" :required="movementMode === 'return_rework_inbound'">
              <el-input v-model.trim="movementForm.reason" type="textarea" :rows="2" placeholder="补充业务说明"/>
            </el-form-item>
            <aside class="movement-confirm-summary" aria-label="本次办理摘要">
              <strong>本次办理摘要</strong>
              <dl>
                <div><dt>物品</dt><dd>{{ selectedWarehouseItem.name }}</dd></div>
                <div><dt>类型</dt><dd>{{ movementTitle }}</dd></div>
                <div><dt>对象</dt><dd>{{ movementCounterpartyLabel }}</dd></div>
                <div><dt>数量</dt><dd>{{ formatMovementInputQuantity }} {{ selectedWarehouseItem.unit }}</dd></div>
              </dl>
            </aside>
            <div class="form-actions movement-submit-bar">
              <el-button :disabled="movementSubmitting" @click="cancelMovement">取消</el-button>
              <el-button type="primary" native-type="submit" :loading="movementSubmitting" :disabled="!movementCanSubmit">{{ movementSubmitLabel }}</el-button>
            </div>
          </el-form>

          <section v-if="hasPermission('inventory:documents:read')" class="movement-history">
            <div class="drawer-section-title"><h3>最近出入库记录</h3><el-button link type="primary" @click="loadAllItemMovements">查看全部</el-button></div>
            <div v-if="itemMovements.length" class="movement-list">
              <article v-for="item in displayedItemMovements" :key="item.id">
                <span class="movement-kind">{{ businessTypeLabel(item.business_type) }}</span>
                <div><strong>{{ movementQuantity(item) }}</strong><small>{{ item.code }} · {{ formatDate(item.posted_at) }}</small></div>
              </article>
            </div>
            <p v-else class="drawer-empty">暂无出入库记录</p>
          </section>
          <el-alert v-else title="出入库记录需要库存单据查看权限。" type="info" :closable="false" show-icon/>
        </div>
      </el-drawer>

      <el-drawer v-model="moldDetailDrawerVisible" size="min(720px, 100%)" title="模具详情" :with-header="false" destroy-on-close @closed="resetMold">
        <div v-if="selectedMoldDetail" class="item-drawer mold-drawer" aria-label="模具详情">
          <div class="drawer-heading">
            <div><small>{{ selectedMoldDetail.code }}</small><h2>{{ selectedMoldDetail.name }}</h2><span>{{ moldStatusLabel(selectedMoldDetail.status) }} · {{ selectedMoldDetail.current_location || '暂无位置' }}</span></div>
            <el-button circle aria-label="关闭模具详情" @click="moldDetailDrawerVisible = false">×</el-button>
          </div>
          <div class="stock-summary mold-summary">
            <div><span>穴数</span><strong>{{ formatCell(selectedMoldDetail.cavity_count) }}</strong></div>
            <div><span>成型材料</span><strong>{{ formatCell(selectedMoldDetail.mold_material) }}</strong></div>
            <div><span>钢材</span><strong>{{ formatCell(selectedMoldDetail.steel) }}</strong></div>
            <div><span>存放位置</span><strong>{{ formatCell(selectedMoldDetail.storage_location) }}</strong></div>
            <div><span>保养周期</span><strong>{{ formatCell(selectedMoldDetail.maintenance_cycle_days) }} 天</strong></div>
            <div><span>下次保养</span><strong>{{ formatDate(selectedMoldDetail.next_maintenance_at) }}</strong></div>
          </div>
          <ImageGallery owner-type="mold" :owner-id="selectedMoldDetail.id" :token="token" :can-write="hasPermission('mold:write')" category="mold"/>
          <section class="movement-history">
            <div class="drawer-section-title"><h3>模具履历</h3></div>
            <div v-if="Array.isArray(selectedMoldDetail.events) && selectedMoldDetail.events.length" class="movement-list">
              <article v-for="event in selectedMoldDetail.events" :key="event.id"><span class="movement-kind">{{ event.type || '事件' }}</span><div><strong>{{ event.status_before || '-' }} → {{ event.status_after || '-' }}</strong><small>{{ event.description || event.reason || event.remark || '-' }} · {{ formatDate(event.created_at) }}</small></div></article>
            </div>
            <p v-else class="drawer-empty">暂无模具履历</p>
          </section>
        </div>
      </el-drawer>

      <el-drawer v-model="workorderDrawerVisible" size="min(720px, 100%)" title="任务单详情" :with-header="false" destroy-on-close @closed="resetWorkOrder">
        <div v-if="selectedWorkOrder" class="item-drawer workorder-drawer" aria-label="任务单详情">
          <div class="drawer-heading">
            <div>
              <small>{{ selectedWorkOrder.code }} · {{ workorderTypeLabel(selectedWorkOrder.type) }}</small>
              <h2>{{ selectedWorkOrder.title }}</h2>
              <span>{{ selectedWorkOrder.product_name || '通用任务' }} · {{ workorderStatusLabel(selectedWorkOrder.status) }}</span>
            </div>
            <el-button circle aria-label="关闭任务单详情" @click="closeWorkOrder">×</el-button>
          </div>

          <div class="stock-summary">
            <div><span>计划数量</span><strong>{{ formatQuantity(selectedWorkOrder.planned_quantity) }} {{ selectedWorkOrder.unit || '' }}</strong></div>
            <div><span>优先级</span><strong>{{ selectedWorkOrder.priority === 'urgent' ? '加急' : '普通' }}</strong></div>
            <div><span>交期</span><strong>{{ formatDate(selectedWorkOrder.due_at) }}</strong></div>
          </div>
          <p v-if="selectedWorkOrder.description" class="drawer-message">{{ selectedWorkOrder.description }}</p>
          <section class="workorder-stage-card" aria-label="任务当前阶段">
            <span>当前阶段</span>
            <StatusTag :label="workorderStatusLabel(selectedWorkOrder.status)" :tone="workorderStatusTone(selectedWorkOrder.status)"/>
            <strong>下一步：{{ workorderNextAction(selectedWorkOrder) }}</strong>
          </section>
          <ImageGallery owner-type="workorder" :owner-id="selectedWorkOrder.id" :token="token" :can-write="hasPermission('workorder:write')" category="workorder"/>

          <section v-if="canWriteActive" class="movement-section">
            <h3>办公室操作</h3>
            <div class="movement-actions">
              <el-button v-if="selectedWorkOrder.status === 'draft'" type="primary" plain @click="dispatchWorkOrder">派发</el-button>
              <el-button v-if="selectedWorkOrder.status === 'processing' || selectedWorkOrder.status === 'pending_close'" plain @click="pauseWorkOrder">暂停</el-button>
              <el-button v-if="selectedWorkOrder.status === 'paused'" plain @click="resumeWorkOrder">恢复</el-button>
              <el-button :type="selectedWorkOrder.priority === 'urgent' ? 'warning' : 'danger'" plain @click="toggleWorkOrderUrgent">
                {{ selectedWorkOrder.priority === 'urgent' ? '取消加急' : '加急' }}
              </el-button>
              <el-button v-if="selectedWorkOrder.status === 'pending_close'" type="success" plain @click="completeWorkOrder('normal')">确认正常完成</el-button>
              <el-button v-if="['processing', 'paused', 'pending_close'].includes(String(selectedWorkOrder.status))" type="danger" plain @click="completeWorkOrder('forced')">强制完成</el-button>
            </div>
          </section>

          <section class="workorder-department-section">
            <h3>部门子任务</h3>
            <div class="department-task-grid">
              <article v-for="task in departmentTasks(selectedWorkOrder)" :key="task.id" class="department-task-card">
                <div>
                  <strong>{{ departmentName(task.department_id) }}</strong>
                  <StatusTag :label="departmentTaskStatusLabel(task.status)" :tone="departmentTaskStatusTone(task.status)"/>
                </div>
                <p>{{ formatQuantity(task.completed_quantity) }} / {{ formatQuantity(task.planned_quantity) }} {{ selectedWorkOrder.unit || '' }}</p>
                <el-progress :percentage="Number(task.progress || 0)" :stroke-width="8"/>
                <small>{{ task.remark || '暂无备注' }}</small>
                <ImageGallery owner-type="department_task" :owner-id="task.id" :token="token" :can-write="canOperateDepartmentTask(task)" category="department_task"/>
                <div v-if="canOperateDepartmentTask(task)" class="department-task-actions">
                  <el-button v-if="task.status === 'received'" link type="primary" @click="startDepartmentTask(task)">开始处理</el-button>
                  <el-button v-if="['received', 'processing', 'partial_completed'].includes(String(task.status))" link type="warning" @click="partialCompleteDepartmentTask(task)">部分完成</el-button>
                  <el-button v-if="['received', 'processing', 'partial_completed'].includes(String(task.status))" link type="success" @click="completeDepartmentTask(task)">完成</el-button>
                </div>
              </article>
            </div>
          </section>

          <section class="movement-history">
            <div class="drawer-section-title"><h3>流转日志</h3><el-button link type="primary" @click="loadWorkOrderLogs">刷新</el-button></div>
            <div v-if="workorderLogs.length" class="movement-list">
              <article v-for="item in workorderLogs" :key="item.id">
                <span class="movement-kind">{{ workorderActionLabel(item.action) }}</span>
                <div><strong>{{ item.actor_username || '系统' }}</strong><small>{{ item.remark || item.reason || `${item.status_before || '-'} → ${item.status_after || '-'}` }} · {{ formatDate(item.created_at) }}</small></div>
              </article>
            </div>
            <p v-else class="drawer-empty">暂无流转日志</p>
          </section>
        </div>
      </el-drawer>
    </section>
  </main>
  </el-config-provider>
</template>

<script setup lang="ts">
import {computed, onMounted, reactive, ref} from 'vue'
import {ElMessage, ElMessageBox} from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import {apiBaseUrl, desktopAppVersion, downloadApiFile, isDesktopClient, request, saveDesktopServerUrl, testDesktopServerUrl} from './api/http'
import ImageGallery from './components/ImageGallery.vue'
import UpdateCenter from './components/UpdateCenter.vue'
import AppNavigation from './components/ui/AppNavigation.vue'
import DataTableShell from './components/ui/DataTableShell.vue'
import FilterBar from './components/ui/FilterBar.vue'
import PageHeader from './components/ui/PageHeader.vue'
import PageState from './components/ui/PageState.vue'
import StatusTag, {type StatusTone} from './components/ui/StatusTag.vue'
import {type ModuleItem, modules} from './data/modules'
import type {BasicItem, ClientUpdateStatus, CurrentUser, PaginatedResponse, SkeletonResponse} from './types'

type FormField = {
  key: string
  label: string
  kind?: 'text' | 'password' | 'select' | 'multi-select' | 'textarea' | 'date'
  options?: Array<{ label: string; value: string | number }>
}

type MovementDefinition = {
  key: string
  title: string
  requiredAny?: string[]
  requiredAll?: string[]
}

type AssignmentConfig = {
  title: string
  tip: string
  buttonLabel: string
  optionKey: 'permissions' | 'roles'
  selectedKey: 'permission_ids' | 'role_ids'
  payloadKey: 'permission_ids' | 'role_ids'
  endpoint: (id: number) => string
  requiredPermissions: string[]
  isDisabled?: (target: BasicItem, option: BasicItem) => boolean
}

type StatisticNameValue = { name: string; value: number; amount?: number }
type StatisticTrendItem = { date: string; name?: string; value: number; quantity?: number; amount?: number }
type DepartmentStatistic = { department_id: number; name: string; total: number; completed: number; processing: number; partial: number; received: number }
type StockStatisticItem = { item_type: string; item_id: number; name: string; code: string; category: string; quantity: number; safety_stock: number; amount?: number }
type MoldStatisticItem = { id: number; code: string; name: string; status: string; current_location?: string; next_maintenance_at?: string }
type StatisticsDashboard = {
  generated_at: string
  can_view_cost: boolean
  summary: Record<string, number>
  inventory: { by_item_type: StatisticNameValue[]; by_material_type: StatisticNameValue[]; low_stock: StockStatisticItem[]; trend: StatisticTrendItem[] }
  workorders: { by_status: StatisticNameValue[]; by_type: StatisticNameValue[]; by_department: DepartmentStatistic[]; trend: StatisticTrendItem[] }
  molds: { by_status: StatisticNameValue[]; need_care: MoldStatisticItem[] }
  business: { by_master_data: StatisticNameValue[] }
  audit: { by_result: StatisticNameValue[]; trend: StatisticTrendItem[] }
  recent_workorders: BasicItem[]
}

// assignmentConfigs 是通用分配编辑器的策略表，后续可按同一协议扩展其他关联配置。
const assignmentConfigs: Partial<Record<string, AssignmentConfig>> = {
  roles: {
    title: '配置角色权限',
    tip: '勾选该角色可以使用的功能；写入权限通常应同时保留对应的查看权限。',
    buttonLabel: '配置权限',
    optionKey: 'permissions',
    selectedKey: 'permission_ids',
    payloadKey: 'permission_ids',
    endpoint: (id) => `/api/v1/system/roles/${id}/permissions`,
    requiredPermissions: ['system:roles:write', 'system:permissions:read'],
  },
  users: {
    title: '分配账号角色',
    tip: '终端账号通过角色获得权限，不能授予超级管理员角色。',
    buttonLabel: '分配角色',
    optionKey: 'roles',
    selectedKey: 'role_ids',
    payloadKey: 'role_ids',
    endpoint: (id) => `/api/v1/system/users/${id}/roles`,
    requiredPermissions: ['system:users:write', 'system:roles:read'],
    isDisabled: (target, option) => target.account_type === 'department_terminal' && option.code === 'super_admin',
  },
}

// 本地存储键：只保存 access token，不保存密码。
const tokenKey = 'bb_erp_access_token'
const desktopClient = isDesktopClient()

const token = ref(localStorage.getItem(tokenKey) || '')
const currentUser = ref<CurrentUser | null>(null)
const activeKey = ref('dashboard')
const showCreateForm = ref(false)
const editingSupplier = ref<BasicItem | null>(null)
const rows = ref<BasicItem[]>([])
const columns = ref<string[]>([])
const skeletonResult = ref<SkeletonResponse | null>(null)
const searchKeyword = ref('')
const page = ref(1)
const pageSize = ref(20)
const pageTotal = ref(0)
const loading = ref(false)
const errorMessage = ref('')
const panelMessage = ref('')
const listError = ref('')
const assignmentTarget = ref<BasicItem | null>(null)
const assignmentModuleKey = ref('')
const selectedAssignmentIDs = ref<number[]>([])
const selectedWarehouseItem = ref<BasicItem | null>(null)
const warehouseDrawerVisible = ref(false)
const warehouseDetail = ref<Record<string, unknown> | null>(null)
const itemMovements = ref<BasicItem[]>([])
const showAllItemMovements = ref(false)
const movementMode = ref<string>('')
const showQuickSupplier = ref(false)
const movementSubmitting = ref(false)
const movementFormError = ref('')
const quickSupplierSubmitting = ref(false)
const quickSupplierError = ref('')
const healthStatus = ref('检查中')
const mobileNavOpen = ref(false)
const serverDialogVisible = ref(false)
const serverTesting = ref(false)
const serverUrlInput = ref(apiBaseUrl())
const serverMessage = ref('')
const serverMessageType = ref<'success' | 'warning' | 'info' | 'error'>('info')
const clientUpdate = ref<ClientUpdateStatus>({
  current_version: '',
  available: false,
  cached: false,
})
const loginForm = reactive({
  username: 'admin',
  password: '',
})

const formState = reactive<Record<string, any>>({})
const movementForm = reactive<Record<string, any>>({})
const quickSupplier = reactive({name: '', code: '', contact: '', phone: ''})
const activeWarehouseTab = ref('product')
const workorderStatusFilter = ref('')
const workorderTypeFilter = ref('')
const workorderPriorityFilter = ref('')
const selectedWorkOrder = ref<BasicItem | null>(null)
const workorderDrawerVisible = ref(false)
const workorderLogs = ref<BasicItem[]>([])
const moldDetailDrawerVisible = ref(false)
const selectedMoldDetail = ref<BasicItem | null>(null)
const statisticsData = ref<StatisticsDashboard | null>(null)
const warehouseTabs = [
  {key: 'product', title: '产品'},
  {key: 'production_material', title: '生产物资'},
  {key: 'regular_product', title: '常规产品'},
  {key: 'daily_supply', title: '生活物资'},
]
const warehouseTabOptions = warehouseTabs.map((item) => ({label: item.title, value: item.key}))
const movementDefinitions: MovementDefinition[] = [
  {key: 'purchase_inbound', title: '采购入库', requiredAll: ['suppliers:read']},
  {key: 'return_rework_inbound', title: '退货返工', requiredAny: ['customers:read', 'system:departments:read']},
  {key: 'customer_outbound', title: '客户出库', requiredAll: ['customers:read']},
  {key: 'department_outbound', title: '部门出库', requiredAll: ['system:departments:read']},
]
const workorderStatusOptions = [
  {label: '全部状态', value: ''},
  {label: '草稿', value: 'draft'},
  {label: '正在处理', value: 'processing'},
  {label: '暂停', value: 'paused'},
  {label: '待办公室确认', value: 'pending_close'},
  {label: '正常完成', value: 'completed_normal'},
  {label: '强制完成', value: 'completed_forced'},
  {label: '取消', value: 'cancelled'},
]
const workorderTypeOptions = [
  {label: '全部类型', value: ''},
  {label: '生产单', value: 'production'},
  {label: '通用任务', value: 'general'},
]
const workorderPriorityOptions = [
  {label: '全部优先级', value: ''},
  {label: '普通', value: 'normal'},
  {label: '加急', value: 'urgent'},
]

const navItems = computed(() => modules.filter(canReadModule))
const businessItems = computed(() => navItems.value.filter((item) => item.group === 'business'))
const systemItems = computed(() => navItems.value.filter((item) => item.group === 'system'))
const activeModule = computed(() => modules.find((item) => item.key === activeKey.value))
const canWriteActive = computed(() => !!activeModule.value && canWriteModule(activeModule.value))
const activePageReadonly = computed(() => {
  if (activeKey.value === 'warehouses') {
    return !hasPermission('warehouse:write') && !hasPermission('inventory:documents:write')
  }
  return Boolean(activeModule.value?.writePermission && !canWriteActive.value)
})
const hasActiveFilters = computed(() => Boolean(
  searchKeyword.value || workorderStatusFilter.value || workorderTypeFilter.value || workorderPriorityFilter.value,
))
const filteredEmptyTitle = computed(() => hasActiveFilters.value ? '没有符合当前条件的结果' : `还没有${activeModule.value?.title || '业务'}记录`)
const filteredEmptyDescription = computed(() => hasActiveFilters.value
  ? '请调整筛选条件或点击重置查看全部记录。'
  : '当前分类尚未创建可显示的记录。')
const isMasterDataValidationPage = computed(() => ['customers', 'suppliers'].includes(activeKey.value))
const hasRenderableData = computed(() => activeKey.value === 'statistics'
  ? Boolean(statisticsData.value)
  : rows.value.length > 0)
const listSearchPlaceholder = computed(() => {
  if (activeKey.value === 'customers') return '搜索客户名称、编码、电话或地址'
  if (activeKey.value === 'suppliers') return '搜索供应商名称、编码、联系人或电话'
  return '输入名称、编号、电话等关键字'
})
const masterDataEmptyTitle = computed(() => searchKeyword.value
  ? '没有符合当前条件的结果'
  : `还没有${activeModule.value?.title || '档案'}记录`)
const masterDataEmptyDescription = computed(() => searchKeyword.value
  ? '请尝试缩短关键词或清空筛选条件。'
  : canWriteActive.value
    ? '可以使用页面右上角的新增操作创建第一条记录。'
    : '当前账号仅可查看，暂无可显示的记录；如需新增，请联系具备编辑权限的人员。')
const hasAssignmentAction = computed(() => {
  const config = assignmentConfigs[activeKey.value]
  return Boolean(config?.requiredPermissions.every(hasPermission))
})
const assignmentConfig = computed(() => assignmentConfigs[assignmentModuleKey.value])
const assignmentOptions = computed(() => {
  return assignmentConfig.value ? rowsFor(assignmentConfig.value.optionKey) : []
})
const userInitial = computed(() => (currentUser.value?.name || currentUser.value?.username || '用户').slice(0, 1))
const greeting = computed(() => {
  const hour = new Date().getHours()
  if (hour < 11) return '早上好'
  if (hour < 14) return '中午好'
  if (hour < 18) return '下午好'
  return '晚上好'
})
const quickActionDefinitions = [
  {key: 'workorder', title: '任务单', description: '查看当前任务与部门处理进度', icon: '✓'},
  {key: 'warehouses', title: '仓库', description: '查询库存并办理物品出入库', icon: '▦'},
  {key: 'customers', title: '客户档案', description: '查找或新增客户资料', icon: '◎'},
  {key: 'suppliers', title: '供应商', description: '维护采购供应商资料', icon: '↙'},
  {key: 'molds', title: '模具台账', description: '查询模具位置与保养状态', icon: '◇'},
]
const quickActions = computed(() => quickActionDefinitions.filter((item) => {
  const moduleItem = modules.find((candidate) => candidate.key === item.key)
  return !!moduleItem && canReadModule(moduleItem)
}).sort((left, right) => {
  const departmentTerminal = currentUser.value?.account_type === 'department_terminal'
  const order = departmentTerminal
    ? ['workorder', 'warehouses', 'molds', 'customers', 'suppliers']
    : ['warehouses', 'workorder', 'customers', 'suppliers', 'molds']
  return order.indexOf(left.key) - order.indexOf(right.key)
}))
const businessGroups = computed(() => [
  {
    title: '库存与仓储',
    caption: '物品、库存与出入库',
    icon: '▣',
    items: businessItems.value.filter((item) => ['warehouses'].includes(item.key)),
  },
  {
    title: '客户与生产',
    caption: '客户、联系人与生产资料',
    icon: '◫',
    items: businessItems.value.filter((item) => ['customers', 'contacts', 'suppliers', 'molds', 'workorder'].includes(item.key)),
  },
  {
    title: '数据与报表',
    caption: '经营数据与统计结果',
    icon: '↗',
    items: businessItems.value.filter((item) => item.key === 'statistics'),
  },
].filter((group) => group.items.length))
const accountTypeText = computed(() => {
  if (!currentUser.value) return '未登录'
  return currentUser.value.account_type === 'department_terminal' ? '部门终端账号' : '个人账号'
})
const availableMovementDefinitions = computed(() => movementDefinitions.filter((definition) => {
  const allAllowed = (definition.requiredAll || []).every(hasPermission)
  const anyAllowed = !definition.requiredAny?.length || definition.requiredAny.some(hasPermission)
  return allAllowed && anyAllowed
}))
const movementDependencyMessage = computed(() => {
  if (!hasPermission('inventory:documents:write')) return ''
  const missing: string[] = []
  if (!hasPermission('suppliers:read')) missing.push('采购入库需要供应商查看权限')
  if (!hasPermission('customers:read') && !hasPermission('system:departments:read')) missing.push('退货返工需要客户或部门查看权限')
  if (!hasPermission('customers:read')) missing.push('客户出库需要客户查看权限')
  if (!hasPermission('system:departments:read')) missing.push('部门出库需要部门查看权限')
  return missing.length ? `${missing.join('；')}。请联系管理员配置所需权限。` : ''
})
const movementTitle = computed(() => movementDefinitions.find((item) => item.key === movementMode.value)?.title || '办理出入库')
const movementIsOutbound = computed(() => ['customer_outbound', 'department_outbound'].includes(movementMode.value))
const expectedStockQuantity = computed(() => {
  const current = Number(warehouseDetail.value?.quantity || 0)
  const delta = decimalToScaled(movementForm.quantity)
  return movementIsOutbound.value ? current - delta : current + delta
})
const movementQuantityError = computed(() => {
  if (movementForm.quantity === undefined || movementForm.quantity === null || movementForm.quantity === '') return ''
  if (decimalToScaled(movementForm.quantity) <= 0) return '数量必须大于 0。'
  if (movementIsOutbound.value && expectedStockQuantity.value < 0) return '出库数量超过当前可用库存，请减少数量。'
  return ''
})
const movementCounterpartyLabel = computed(() => {
  const source = movementMode.value === 'purchase_inbound' ? rowsFor('suppliers').find((item) => Number(item.id) === Number(movementForm.supplier_id))
    : movementMode.value === 'customer_outbound' || movementForm.source_type === 'customer' ? rowsFor('customers').find((item) => Number(item.id) === Number(movementForm.customer_id))
      : rowsFor('departments').find((item) => Number(item.id) === Number(movementForm.department_id))
  return String(source?.name || '尚未选择')
})
const formatMovementInputQuantity = computed(() => {
  const quantity = Number(movementForm.quantity || 0)
  return Number.isFinite(quantity) && quantity > 0 ? quantity.toLocaleString('zh-CN', {maximumFractionDigits: 4}) : '0'
})
const movementCanSubmit = computed(() => {
  if (movementSubmitting.value || decimalToScaled(movementForm.quantity) <= 0) return false
  if (movementIsOutbound.value && expectedStockQuantity.value < 0) return false
  if (movementMode.value === 'purchase_inbound') return Boolean(movementForm.supplier_id)
  if (movementMode.value === 'customer_outbound') return Boolean(movementForm.customer_id)
  if (movementMode.value === 'department_outbound') return Boolean(movementForm.department_id)
  if (movementMode.value === 'return_rework_inbound') {
    const sourceSelected = movementForm.source_type === 'customer' ? movementForm.customer_id : movementForm.department_id
    return Boolean(sourceSelected && String(movementForm.reason || '').trim())
  }
  return false
})
const movementSubmitLabel = computed(() => `确认${movementTitle.value}并过账`)
const movementFormDirty = computed(() => Boolean(movementMode.value && (
  Object.keys(movementForm).some((key) => key !== 'source_type') || showQuickSupplier.value || Object.values(quickSupplier).some(Boolean)
)))
const displayedItemMovements = computed(() => showAllItemMovements.value ? itemMovements.value : itemMovements.value.slice(0, 10))
const eligibleOriginalDocuments = computed(() => itemMovements.value.filter((item) => {
  if (item.type !== 'outbound' || item.status !== 'posted') return false
  if (movementForm.source_type === 'customer') return Number(item.customer_id) === Number(movementForm.customer_id)
  if (movementForm.source_type === 'department') return Number(item.department_id) === Number(movementForm.department_id)
  return false
}))
const statisticsCards = computed(() => {
  const summary = statisticsData.value?.summary || {}
  return [
    {label: '库存总量', value: formatQuantity(summary.inventory_quantity), caption: statisticsData.value?.can_view_cost ? `金额 ${formatMoney(summary.inventory_amount)}` : '金额按权限隐藏'},
    {label: '低库存', value: String(summary.low_stock_items || 0), caption: '低于或等于安全库存'},
    {label: '进行中任务', value: String(summary.open_workorders || 0), caption: `加急 ${summary.urgent_workorders || 0} · 待确认 ${summary.pending_close_orders || 0}`},
    {label: '模具关注', value: String(summary.molds_need_care || 0), caption: `模具总数 ${summary.molds || 0}`},
    {label: '客户/联系人', value: `${summary.customers || 0}/${summary.contacts || 0}`, caption: '客户档案 / 联系人'},
    {label: '仓库物品', value: String(summary.warehouse_items || 0), caption: '产品与物资档案'},
  ]
})
const compactTrendItems = computed(() => {
  const inventory = statisticsData.value?.inventory?.trend || []
  const workorders = statisticsData.value?.workorders?.trend || []
  return [...inventory.slice(-8), ...workorders.slice(-8)].slice(-12)
})

// formSchema 根据当前模块返回轻量新增表单，和后端已实现能力保持一致。
const formSchema = computed<FormField[]>(() => {
  const departmentOptions = rowsFor('departments').map((item) => ({
    label: item.name || item.code || `#${item.id}`,
    value: item.id
  }))
  const terminalOptions = rowsFor('terminals').map((item) => ({
    label: item.name || item.code || `#${item.id}`,
    value: item.id
  }))
  switch (activeKey.value) {
    case 'departments':
      return [
        {key: 'name', label: '部门名称'},
        {key: 'code', label: '部门编码'},
      ]
    case 'terminals':
      return [
        {key: 'department_id', label: '所属部门', kind: 'select', options: departmentOptions},
        {key: 'code', label: '终端编码'},
        {key: 'name', label: '终端名称'},
        {key: 'location', label: '位置说明'},
      ]
    case 'users':
      return [
        {key: 'username', label: '账号'},
        {key: 'password', label: '密码', kind: 'password'},
        {
          key: 'account_type',
          label: '账号类型',
          kind: 'select',
          options: [
            {label: '个人账号', value: 'personal'},
            {label: '部门终端账号', value: 'department_terminal'},
          ],
        },
        {key: 'name', label: '姓名/终端名'},
        {key: 'department_id', label: '所属部门', kind: 'select', options: departmentOptions},
        {key: 'terminal_id', label: '所属终端', kind: 'select', options: terminalOptions},
      ]
    case 'roles':
      return [
        {key: 'name', label: '角色名称'},
        {key: 'code', label: '角色编码'},
        {key: 'description', label: '说明'},
      ]
    case 'customers':
      return [
        {key: 'name', label: '客户名称'},
        {key: 'code', label: '客户编码'},
        {key: 'phone', label: '座机'},
        {key: 'address', label: '地址'},
      ]
    case 'suppliers':
      return [
        {key: 'name', label: '供应商名称'},
        {key: 'code', label: '供应商编码'},
        {key: 'contact', label: '联系人'},
        {key: 'phone', label: '联系电话'},
        {key: 'address', label: '地址'},
      ]
    case 'warehouses':
      return [
        {key: 'name', label: `${activeWarehouseTabTitle.value}名称`},
        {key: 'code', label: `${activeWarehouseTabTitle.value}编码`},
        {key: 'unit', label: '单位'},
        {key: 'spec', label: '规格'},
        {key: 'safety_stock', label: '安全库存'},
        ...(hasPermission('cost:view') ? [{key: 'default_cost', label: '默认成本（元）'}] : []),
      ]
    case 'molds':
      return [
        {key: 'code', label: '模具编号'},
        {key: 'name', label: '模具名称'},
        {key: 'customer_id', label: '客户ID'},
        {key: 'product_id', label: '产品ID'},
        {key: 'cavity_count', label: '穴数'},
        {key: 'mold_material', label: '成型材料'},
        {key: 'steel', label: '钢材'},
        {key: 'size', label: '尺寸'},
        {key: 'weight_gram', label: '重量g'},
        {key: 'manufacturer', label: '制造商'},
        {key: 'storage_location', label: '存放位置'},
        {key: 'maintenance_cycle_days', label: '保养周期天'},
      ]
    case 'workorder':
      return [
        {
          key: 'type',
          label: '任务类型',
          kind: 'select',
          options: [
            {label: '生产单', value: 'production'},
            {label: '通用任务', value: 'general'},
          ],
        },
        {key: 'code', label: '任务编号'},
        {key: 'title', label: '标题'},
        {key: 'customer_id', label: '客户', kind: 'select', options: rowsFor('customers').map((item) => ({label: item.name || item.code || `#${item.id}`, value: item.id}))},
        {key: 'product_name', label: '产品'},
        {key: 'planned_quantity', label: '计划数量'},
        {key: 'unit', label: '单位'},
        {key: 'due_at', label: '交期', kind: 'date'},
        {
          key: 'priority',
          label: '优先级',
          kind: 'select',
          options: [
            {label: '普通', value: 'normal'},
            {label: '加急', value: 'urgent'},
          ],
        },
        {key: 'target_department_ids', label: '流转部门', kind: 'multi-select', options: departmentOptions},
        {key: 'description', label: '说明', kind: 'textarea'},
      ]
    default:
      return []
  }
})

// cache 保存基础资料列表，为用户、部门、终端表单提供选项。
const cache = reactive<Record<string, BasicItem[]>>({})
const activeWarehouseTabTitle = computed(() => warehouseTabs.find((tab) => tab.key === activeWarehouseTab.value)?.title || '物品')
const createEntityTitle = computed(() => activeKey.value === 'warehouses' ? '物品' : (activeModule.value?.title || ''))

function rowsFor(key: string): BasicItem[] {
  return cache[key] || []
}

function isPaginatedResponse(data: BasicItem[] | PaginatedResponse<BasicItem> | SkeletonResponse): data is PaginatedResponse<BasicItem> {
  return !Array.isArray(data) && Array.isArray((data as PaginatedResponse<BasicItem>).items)
}

function appendQuery(path: string, params: Record<string, string | number | undefined>): string {
  const urlParams = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && String(value) !== '') {
      urlParams.set(key, String(value))
    }
  }
  const query = urlParams.toString()
  if (!query) return path
  return `${path}${path.includes('?') ? '&' : '?'}${query}`
}

function hasPermission(code?: string): boolean {
  return !code || !!currentUser.value?.permissions?.includes(code)
}

function canReadModule(item: ModuleItem): boolean {
  return item.key === 'dashboard' || hasPermission(item.readPermission)
}

function canWriteModule(item: ModuleItem): boolean {
  return !!item.writePermission && hasPermission(item.writePermission)
}

async function switchModule(key: string) {
  const target = modules.find((item) => item.key === key)
  if (!target || !canReadModule(target)) {
    panelMessage.value = '你的账号暂无该功能权限'
    activeKey.value = 'dashboard'
    return
  }
  if (warehouseDrawerVisible.value) {
    const canClose = await requestWarehouseClose()
    if (!canClose) return
    performWarehouseClose()
  }
  activeKey.value = key
  rows.value = []
  columns.value = []
  pageTotal.value = 0
  skeletonResult.value = null
  listError.value = ''
  closeWorkOrder()
  closeMold()
  closeAssignment()
  showCreateForm.value = false
  editingSupplier.value = null
  resetListQuery()
  clearForm()
  void loadActiveModule()
}

function selectMobileModule(key: string) {
  mobileNavOpen.value = false
  void switchModule(key)
}

function restoreMobileMenuFocus() {
  document.getElementById('mobile-menu-button')?.focus()
}

async function handleUserCommand(command: string) {
  if (command === 'server') openServerSettings()
  if (command === 'logout') {
    if (warehouseDrawerVisible.value) {
      const canClose = await requestWarehouseClose()
      if (!canClose) return
      warehouseDrawerVisible.value = false
      resetWarehouseItem()
    }
    logout()
  }
}

function resetFilters() {
  searchKeyword.value = ''
  workorderStatusFilter.value = ''
  workorderTypeFilter.value = ''
  workorderPriorityFilter.value = ''
  applySearch()
}

async function openAssignment(row: any) {
  const config = assignmentConfigs[activeKey.value]
  if (!config) return
  assignmentTarget.value = row
  assignmentModuleKey.value = activeKey.value
  selectedAssignmentIDs.value = Array.isArray(row[config.selectedKey])
    ? (row[config.selectedKey] as unknown[]).map(Number)
    : []
  if (!rowsFor(config.optionKey).length) {
    loading.value = true
    try {
      await loadList(config.optionKey, false)
    } catch (error) {
      panelMessage.value = error instanceof Error ? error.message : '配置项加载失败'
    } finally {
      loading.value = false
    }
  }
}

function closeAssignment() {
  assignmentTarget.value = null
  assignmentModuleKey.value = ''
  selectedAssignmentIDs.value = []
}

function isAssignmentOptionDisabled(option: BasicItem): boolean {
  return Boolean(
    assignmentConfig.value?.isDisabled
    && assignmentTarget.value
    && assignmentConfig.value.isDisabled(assignmentTarget.value, option)
  )
}

async function saveAssignment() {
  const config = assignmentConfig.value
  if (!assignmentTarget.value || !config) return
  loading.value = true
  panelMessage.value = ''
  try {
    await request(config.endpoint(assignmentTarget.value.id), {
      method: 'POST',
      body: {[config.payloadKey]: selectedAssignmentIDs.value},
    }, token.value)
    closeAssignment()
    await loadActiveModule()
    panelMessage.value = '权限配置已保存'
    ElMessage.success('权限配置已保存')
  } catch (error) {
    panelMessage.value = error instanceof Error ? error.message : '权限配置保存失败'
  } finally {
    loading.value = false
  }
}

function switchWarehouseTab(key: string) {
  activeWarehouseTab.value = key
  resetListQuery()
  clearForm()
  void loadActiveModule()
}

function resetListQuery() {
  searchKeyword.value = ''
  page.value = 1
  pageTotal.value = 0
  workorderStatusFilter.value = ''
  workorderTypeFilter.value = ''
  workorderPriorityFilter.value = ''
}

function applySearch() {
  page.value = 1
  void loadActiveModule()
}

function handlePageChange(value: number) {
  page.value = value
  void loadActiveModule()
}

function handlePageSizeChange(value: number) {
  pageSize.value = value
  page.value = 1
  void loadActiveModule()
}

async function login() {
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await request<{ access_token: string; user: CurrentUser }>('/api/v1/auth/login', {
      method: 'POST',
      body: loginForm,
    })
    token.value = data.access_token
    currentUser.value = data.user
    localStorage.setItem(tokenKey, data.access_token)
    await bootstrap()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '登录失败'
  } finally {
    loading.value = false
  }
}

function logout() {
  token.value = ''
  currentUser.value = null
  localStorage.removeItem(tokenKey)
}

function openServerSettings() {
  serverUrlInput.value = apiBaseUrl()
  serverMessage.value = ''
  serverDialogVisible.value = true
}

async function testServerSetting() {
  serverTesting.value = true
  serverMessage.value = ''
  try {
    await testDesktopServerUrl(serverUrlInput.value)
    serverMessageType.value = 'success'
    serverMessage.value = '连接成功，Go 服务可以访问'
  } catch (error) {
    serverMessageType.value = 'error'
    serverMessage.value = error instanceof Error ? error.message : '连接失败，请检查地址和网络'
  } finally {
    serverTesting.value = false
  }
}

function saveServerSetting() {
  try {
    const previous = apiBaseUrl()
    const saved = saveDesktopServerUrl(serverUrlInput.value)
    serverUrlInput.value = saved
    serverMessageType.value = 'success'
    serverMessage.value = '服务器地址已保存'
    serverDialogVisible.value = false
    if (previous !== saved && token.value) {
      logout()
      errorMessage.value = '服务器已切换，请使用新服务器的账号重新登录'
    }
    void loadHealth()
    ElMessage.success('服务器地址已保存')
  } catch (error) {
    serverMessageType.value = 'error'
    serverMessage.value = error instanceof Error ? error.message : '服务器地址保存失败'
  }
}

async function bootstrap() {
  const startupTasks: Promise<unknown>[] = [loadHealth(), loadMe(), preloadBaseData()]
  if (desktopClient) startupTasks.push(loadClientUpdate())
  await Promise.allSettled(startupTasks)
  await loadActiveModule()
}

async function loadHealth() {
  try {
    await request('/health')
    healthStatus.value = '正常'
  } catch {
    healthStatus.value = '异常'
  }
}

async function loadMe() {
  if (!token.value) return
  currentUser.value = await request<CurrentUser>('/api/v1/auth/me', {}, token.value)
}

async function loadClientUpdate() {
  if (!desktopClient) return
  try {
    const currentVersion = await desktopAppVersion()
    const path = appendQuery('/api/v1/updates/client/status', {current_version: currentVersion})
    clientUpdate.value = await request<ClientUpdateStatus>(path)
  } catch {
    clientUpdate.value = {
      current_version: '',
      available: false,
      cached: false,
    }
  }
}

async function downloadClientUpdate() {
  if (!desktopClient || !clientUpdate.value.download_path) return
  try {
    await downloadApiFile(
      clientUpdate.value.download_path,
      clientUpdate.value.file_name || 'bb-erp-client-windows.zip',
      token.value,
    )
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '客户端安装包下载失败')
  }
}

async function preloadBaseData() {
  const candidates = [
    {key: 'departments', permission: 'system:departments:read'},
    {key: 'terminals', permission: 'system:terminals:read'},
    {key: 'materials', permission: 'material:read'},
    {key: 'products', permission: 'product:read'},
    {key: 'customers', permission: 'customers:read'},
    {key: 'suppliers', permission: 'suppliers:read'},
  ]
  const keys = candidates.filter((item) => hasPermission(item.permission)).map((item) => item.key)
  await Promise.allSettled(keys.map((key) => loadList(key, false)))
}

async function loadActiveModule() {
  const item = activeModule.value
  if (!item || item.key === 'dashboard') return
  if (!canReadModule(item)) {
    activeKey.value = 'dashboard'
    panelMessage.value = '你的账号暂无该功能权限'
    return
  }
  loading.value = true
  panelMessage.value = ''
  listError.value = ''
  skeletonResult.value = null
  try {
    if (item.key === 'updates') {
      rows.value = []
      columns.value = []
      pageTotal.value = 0
    } else if (item.key === 'statistics') {
      await loadStatistics()
    } else {
      await loadList(item.key, true)
    }
    panelMessage.value = '已同步'
  } catch (error) {
    listError.value = error instanceof Error ? error.message : '加载失败'
    panelMessage.value = listError.value
  } finally {
    loading.value = false
  }
}

async function loadStatistics() {
  statisticsData.value = await request<StatisticsDashboard>('/api/v1/statistics', {}, token.value)
  rows.value = []
  columns.value = []
  pageTotal.value = 0
}

async function loadList(key: string, applyToPanel: boolean) {
  const item = modules.find((moduleItem) => moduleItem.key === key)
  let path = item?.path
  if (key === 'warehouse_records') {
    path = '/api/v1/warehouses'
  }
  if (!path) return
  if (key === 'warehouses') {
    path = appendQuery(path, {tab: activeWarehouseTab.value})
  }
  if (applyToPanel) {
    path = appendQuery(path, {
      page: page.value,
      page_size: pageSize.value,
      q: searchKeyword.value,
      status: key === 'workorder' ? workorderStatusFilter.value : undefined,
      type: key === 'workorder' ? workorderTypeFilter.value : undefined,
      priority: key === 'workorder' ? workorderPriorityFilter.value : undefined,
    })
  } else {
    path = appendQuery(path, {page: 1, page_size: 200})
  }
  const data = await request<BasicItem[] | PaginatedResponse<BasicItem> | SkeletonResponse>(path, {}, token.value)
  if (isPaginatedResponse(data)) {
    cache[key] = data.items
    if (applyToPanel) {
      rows.value = data.items
      page.value = data.page
      pageSize.value = data.page_size
      pageTotal.value = data.total
      if (item) {
        columns.value = inferColumns(data.items, item)
      }
    }
    return
  }
  if (!Array.isArray(data)) {
    if (applyToPanel) {
      skeletonResult.value = data
      rows.value = []
      columns.value = []
      pageTotal.value = 0
    }
    return
  }
  cache[key] = data
  if (applyToPanel) {
    rows.value = data
    pageTotal.value = data.length
    if (item) {
      columns.value = inferColumns(data, item)
    }
  }
}

async function createItem() {
  const item = activeModule.value
  if (!item?.path || !canWriteModule(item)) {
    panelMessage.value = '你的账号只有查看权限，不能新增数据'
    showCreateForm.value = false
    return
  }
  loading.value = true
  panelMessage.value = ''
  try {
    const isSupplierEdit = activeKey.value === 'suppliers' && editingSupplier.value
    const path = isSupplierEdit ? `${item.path}/${editingSupplier.value?.id}` : item.path
    await request(path, {
      method: isSupplierEdit ? 'PATCH' : 'POST',
      body: normalizedForm(),
    }, token.value)
    clearForm()
    await preloadBaseData()
    await loadActiveModule()
    panelMessage.value = '已新增'
    if (isSupplierEdit) panelMessage.value = '已保存'
    ElMessage.success(isSupplierEdit ? '保存成功' : '新增成功')
    editingSupplier.value = null
    showCreateForm.value = false
  } catch (error) {
    panelMessage.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    loading.value = false
  }
}

function normalizedForm(): Record<string, unknown> {
  const body: Record<string, unknown> = {}
  if (activeKey.value === 'users') {
    body.organization_id = 1
  }
  if (activeKey.value === 'warehouses') {
    body.tab = activeWarehouseTab.value
  }
  for (const field of formSchema.value) {
    const value = formState[field.key]
    if (value === '' || value === undefined) continue
    if (activeKey.value === 'warehouses' && field.key === 'safety_stock') {
      body[field.key] = decimalToScaled(value)
      continue
    }
    if (activeKey.value === 'warehouses' && field.key === 'default_cost') {
      body[field.key] = moneyToCents(value)
      continue
    }
    if (activeKey.value === 'workorder' && field.key === 'planned_quantity') {
      body[field.key] = decimalToScaled(value)
      continue
    }
    if (activeKey.value === 'workorder' && field.key === 'target_department_ids') {
      body[field.key] = Array.isArray(value) ? value.map(Number) : []
      continue
    }
    body[field.key] = numericKeys.has(field.key) || field.key.endsWith('_id') ? Number(value) : value
  }
  return body
}

const numericKeys = new Set(['quantity', 'unit_cost', 'default_cost', 'safety_stock', 'customer_id', 'product_id', 'cavity_count', 'weight_gram', 'maintenance_cycle_days'])

function clearForm() {
  for (const key of Object.keys(formState)) {
    delete formState[key]
  }
}

function toggleCreateForm() {
  editingSupplier.value = null
  clearForm()
  showCreateForm.value = !showCreateForm.value
}

function editSupplier(item: any) {
  editingSupplier.value = item
  clearForm()
  for (const key of ['name', 'code', 'contact', 'phone', 'address', 'status']) {
    const value = item[key]
    if (typeof value === 'string' || typeof value === 'number') formState[key] = value
  }
  showCreateForm.value = true
  window.scrollTo({top: 0, behavior: 'smooth'})
}

function inferColumns(data: BasicItem[], item: ModuleItem): string[] {
  const preferred: Record<string, string[]> = {
    users: ['id', 'username', 'account_type', 'name', 'organization_id', 'department_id', 'terminal_id', 'status'],
    departments: ['id', 'organization_id', 'name', 'code', 'status'],
    terminals: ['id', 'department_id', 'code', 'name', 'location', 'status'],
    roles: ['id', 'name', 'code', 'description'],
    customers: ['id', 'name', 'code', 'phone', 'address'],
    suppliers: ['id', 'name', 'code', 'contact', 'phone', 'address', 'status'],
    warehouses: ['id', 'item_type', 'category', 'name', 'code', 'unit', 'spec', 'safety_stock', 'status'],
    materials: ['id', 'name', 'code', 'category', 'unit', 'spec', 'safety_stock', 'status'],
    products: ['id', 'name', 'code', 'unit', 'spec', 'safety_stock', 'status'],
    molds: ['id', 'code', 'name', 'status', 'current_location', 'storage_location', 'cavity_count', 'mold_material', 'steel', 'maintenance_cycle_days', 'next_maintenance_at'],
    permissions: ['id', 'name', 'code', 'object', 'action'],
    audits: ['id', 'actor_username', 'actor_account_type', 'person_name', 'department_id', 'terminal_id', 'action', 'object', 'result'],
  }
  const inferred = preferred[item.key] || Object.keys(data[0] || {})
  if (!hasPermission('cost:view')) {
    return inferred.filter((column) => !['avg_cost', 'amount', 'unit_cost', 'default_cost'].includes(column))
  }
  return inferred
}

function formatCell(value: unknown): string {
  if (value === null || value === undefined || value === '') return '-'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function genericStatusLabel(value: unknown): string {
  const status = String(value || 'unknown').toLowerCase()
  const labels: Record<string, string> = {
    active: '正常',
    enabled: '正常',
    normal: '正常',
    inactive: '停用',
    disabled: '停用',
    stopped: '停用',
    unknown: '未设置',
  }
  return labels[status] || formatCell(value)
}

function genericStatusTone(value: unknown): StatusTone {
  const status = String(value || 'unknown').toLowerCase()
  return ['active', 'enabled', 'normal'].includes(status) ? 'success' : 'info'
}

function stockState(row: unknown): {label: string; tone: StatusTone} {
  const item = row as Record<string, unknown>
  const quantity = Number(item.quantity || 0)
  const safetyStock = Number(item.safety_stock || 0)
  if (quantity <= 0) return {label: '缺货', tone: 'danger'}
  if (quantity <= safetyStock) return {label: '低于安全库存', tone: 'warning'}
  return {label: '库存正常', tone: 'success'}
}

const columnLabels: Record<string, string> = {
  id: '编号', username: '账号', account_type: '账号类型', name: '名称', organization_id: '组织',
  department_id: '部门', terminal_id: '终端', status: '状态', code: '编码', description: '说明',
  phone: '电话', contact: '联系人', address: '地址', location: '位置', item_type: '对象类型', category: '分类',
  unit: '单位', spec: '规格', safety_stock: '安全库存', type: '业务类型', warehouse_id: '仓库',
  to_warehouse_id: '目标仓库', reason: '原因', location_id: '库位', item_id: '物品',
  quantity: '数量', avg_cost: '平均成本', amount: '金额', document_id: '单据',
  balance_qty: '结存数量', current_location: '当前位置', storage_location: '存放位置',
  cavity_count: '穴数', mold_material: '成型材料', steel: '钢材', maintenance_cycle_days: '保养周期',
  next_maintenance_at: '下次保养', object: '对象', action: '操作', actor_username: '操作账号',
  actor_account_type: '账号类型', person_name: '操作人', result: '结果',
}

function columnLabel(column: string): string {
  return columnLabels[column] || column
}

async function openWarehouseItem(item: any) {
  selectedWarehouseItem.value = item
  warehouseDrawerVisible.value = true
  movementMode.value = ''
  showAllItemMovements.value = false
  panelMessage.value = ''
  await Promise.allSettled([loadWarehouseItemDetail(), loadItemMovements()])
}

let warehouseCloseBypass = false

async function closeWarehouseItem() {
  if (!await requestWarehouseClose()) return
  performWarehouseClose()
}

function performWarehouseClose() {
  warehouseCloseBypass = true
  warehouseDrawerVisible.value = false
  window.setTimeout(() => { warehouseCloseBypass = false }, 0)
}

async function requestWarehouseClose(): Promise<boolean> {
  if (movementSubmitting.value) {
    ElMessage.warning('正在提交库存变动，请等待办理完成后再关闭。')
    return false
  }
  if (!movementFormDirty.value) return true
  try {
    await ElMessageBox.confirm('当前出入库表单尚未提交，关闭后已填写内容将丢失。', '放弃本次办理？', {
      confirmButtonText: '放弃并关闭',
      cancelButtonText: '继续填写',
      type: 'warning',
    })
    return true
  } catch {
    return false
  }
}

function handleWarehouseBeforeClose(done: () => void) {
  if (warehouseCloseBypass) {
    warehouseCloseBypass = false
    done()
    return
  }
  void requestWarehouseClose().then((canClose) => {
    if (canClose) done()
  })
}

function resetWarehouseItem() {
  warehouseCloseBypass = false
  selectedWarehouseItem.value = null
  warehouseDetail.value = null
  itemMovements.value = []
  movementMode.value = ''
  showQuickSupplier.value = false
  movementFormError.value = ''
  quickSupplierError.value = ''
  clearMovementForm()
}

async function openMold(item: any) {
  selectedMoldDetail.value = null
  moldDetailDrawerVisible.value = true
  try {
    selectedMoldDetail.value = await request<BasicItem>(`/api/v1/molds/${item.id}`, {}, token.value)
  } catch (error) {
    panelMessage.value = error instanceof Error ? error.message : '模具详情加载失败'
    ElMessage.error(panelMessage.value)
  }
}

function closeMold() {
  moldDetailDrawerVisible.value = false
}

function resetMold() {
  selectedMoldDetail.value = null
}

async function loadWarehouseItemDetail() {
  const item = selectedWarehouseItem.value
  if (!item) return
  warehouseDetail.value = await request<Record<string, unknown>>(
    `/api/v1/warehouse/items/${item.item_type}/${item.id}`,
    {},
    token.value,
  )
}

async function loadItemMovements() {
  const item = selectedWarehouseItem.value
  if (!item || !hasPermission('inventory:documents:read')) return
  const data = await request<{items: BasicItem[]}>(
    `/api/v1/warehouse/items/${item.item_type}/${item.id}/movements?page_size=100`,
    {},
    token.value,
  )
  itemMovements.value = data.items
}

async function loadAllItemMovements() {
  showAllItemMovements.value = true
  if (itemMovements.value.length < 100) {
    await loadItemMovements()
  }
}

function startMovement(mode: string) {
  if (movementSubmitting.value) return
  movementMode.value = mode
  showQuickSupplier.value = false
  clearMovementForm()
  movementFormError.value = ''
  if (mode === 'return_rework_inbound') {
    movementForm.source_type = hasPermission('customers:read') ? 'customer' : 'department'
  }
}

async function cancelMovement() {
  if (movementSubmitting.value) return
  if (movementFormDirty.value) {
    try {
      await ElMessageBox.confirm('取消后本次填写内容不会保留。', '取消本次办理？', {
        confirmButtonText: '确认取消',
        cancelButtonText: '继续填写',
        type: 'warning',
      })
    } catch {
      return
    }
  }
  movementMode.value = ''
  showQuickSupplier.value = false
  movementFormError.value = ''
  quickSupplierError.value = ''
  clearMovementForm()
}

function resetMovementSource() {
  delete movementForm.customer_id
  delete movementForm.department_id
  delete movementForm.original_document_id
}

function clearMovementForm() {
  for (const key of Object.keys(movementForm)) delete movementForm[key]
}

async function submitMovement() {
  if (movementSubmitting.value) return
  const item = selectedWarehouseItem.value
  if (!item || !movementMode.value) return
  const quantity = decimalToScaled(movementForm.quantity)
  if (quantity <= 0) {
    movementFormError.value = '数量必须大于 0。'
    return
  }
  if (movementIsOutbound.value && expectedStockQuantity.value < 0) {
    movementFormError.value = `当前可用库存为 ${formatQuantity(warehouseDetail.value?.quantity)} ${item.unit}，本次出库数量不能超过可用库存。`
    return
  }
  if (!movementCanSubmit.value) {
    movementFormError.value = '请补全办理对象、数量和必填业务说明后再提交。'
    return
  }
  const body: Record<string, unknown> = {
    business_type: movementMode.value,
    quantity,
    reason: movementForm.reason || '',
    remark: movementForm.reason || '',
  }
  for (const key of ['supplier_id', 'customer_id', 'department_id', 'original_document_id']) {
    if (movementForm[key] !== '' && movementForm[key] !== undefined) body[key] = Number(movementForm[key])
  }
  if (movementMode.value === 'purchase_inbound' && movementForm.unit_cost) {
    body.unit_cost = moneyToCents(movementForm.unit_cost)
  }
  movementSubmitting.value = true
  movementFormError.value = ''
  try {
    await request(`/api/v1/warehouse/items/${item.item_type}/${item.id}/movements`, {
      method: 'POST',
      headers: {'Idempotency-Key': crypto.randomUUID()},
      body,
    }, token.value)
    movementMode.value = ''
    showQuickSupplier.value = false
    clearMovementForm()
    await Promise.all([loadActiveModule(), loadWarehouseItemDetail(), loadItemMovements()])
    const refreshed = rows.value.find((row) => row.id === item.id && row.item_type === item.item_type)
    if (refreshed) selectedWarehouseItem.value = refreshed
    panelMessage.value = '库存已更新'
    ElMessage.success('库存已更新')
  } catch (error) {
    movementFormError.value = error instanceof Error ? error.message : '办理失败，请检查填写内容后重试。'
    ElMessage.error(movementFormError.value)
  } finally {
    movementSubmitting.value = false
  }
}

async function createQuickSupplier() {
  if (movementSubmitting.value || quickSupplierSubmitting.value) return
  if (!quickSupplier.name || !quickSupplier.code) {
    quickSupplierError.value = '请填写供应商名称和唯一编码。'
    return
  }
  quickSupplierSubmitting.value = true
  quickSupplierError.value = ''
  try {
    const created = await request<BasicItem>('/api/v1/suppliers', {method: 'POST', body: {...quickSupplier}}, token.value)
    await loadList('suppliers', false)
    movementForm.supplier_id = created.id
    Object.assign(quickSupplier, {name: '', code: '', contact: '', phone: ''})
    showQuickSupplier.value = false
    panelMessage.value = '供应商已新增'
    ElMessage.success('供应商已新增')
  } catch (error) {
    quickSupplierError.value = error instanceof Error ? error.message : '供应商新增失败，请检查编码是否重复。'
    ElMessage.error(quickSupplierError.value)
  } finally {
    quickSupplierSubmitting.value = false
  }
}

function decimalToScaled(value: unknown): number {
  const number = Number(value)
  return Number.isFinite(number) ? Math.round(number * 10000) : 0
}

function moneyToCents(value: unknown): number {
  const number = Number(value)
  return Number.isFinite(number) ? Math.round(number * 100) : 0
}

function formatQuantity(value: unknown): string {
  const number = Number(value || 0) / 10000
  return number.toLocaleString('zh-CN', {maximumFractionDigits: 4})
}

function formatMoney(value: unknown): string {
  return `¥${(Number(value || 0) / 100).toLocaleString('zh-CN', {minimumFractionDigits: 2, maximumFractionDigits: 2})}`
}

function formatDate(value: unknown): string {
  if (!value) return '-'
  return new Date(String(value)).toLocaleString('zh-CN', {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'})
}

function businessTypeLabel(value: unknown): string {
  return movementDefinitions.find((item) => item.key === value)?.title || (value === 'inbound' ? '入库' : '出库')
}

function movementQuantity(document: BasicItem): string {
  const lines = Array.isArray(document.lines) ? document.lines as Array<Record<string, unknown>> : []
  const quantity = formatQuantity(lines[0]?.quantity)
  return `${document.type === 'outbound' ? '−' : '+'}${quantity} ${selectedWarehouseItem.value?.unit || ''}`
}

function openWorkOrder(item: any) {
  selectedWorkOrder.value = item
  workorderDrawerVisible.value = true
  void loadWorkOrderLogs()
}

function closeWorkOrder() {
  workorderDrawerVisible.value = false
}

function resetWorkOrder() {
  selectedWorkOrder.value = null
  workorderLogs.value = []
}

async function loadWorkOrderLogs() {
  if (!selectedWorkOrder.value) return
  workorderLogs.value = await request<BasicItem[]>(`/api/v1/workorder/${selectedWorkOrder.value.id}/logs`, {}, token.value)
}

async function dispatchWorkOrder() {
  await runWorkOrderAction(`/api/v1/workorder/${selectedWorkOrder.value?.id}/dispatch`, {}, '任务已派发')
}

async function pauseWorkOrder() {
  const reason = await promptText('暂停原因', '请输入暂停原因')
  if (!reason) return
  await runWorkOrderAction(`/api/v1/workorder/${selectedWorkOrder.value?.id}/pause`, {reason}, '任务已暂停')
}

async function resumeWorkOrder() {
  await runWorkOrderAction(`/api/v1/workorder/${selectedWorkOrder.value?.id}/resume`, {}, '任务已恢复')
}

async function toggleWorkOrderUrgent() {
  await runWorkOrderAction(`/api/v1/workorder/${selectedWorkOrder.value?.id}/urgent`, {
    urgent: selectedWorkOrder.value?.priority !== 'urgent',
  }, selectedWorkOrder.value?.priority === 'urgent' ? '已取消加急' : '已设为加急')
}

async function completeWorkOrder(mode: 'normal' | 'forced') {
  let reason = ''
  if (mode === 'forced') {
    reason = await promptText('强制完成原因', '请输入强制完成原因')
    if (!reason) return
  } else {
    try {
      await ElMessageBox.confirm('确认该任务单正常完成？', '确认完成', {type: 'success'})
    } catch {
      return
    }
  }
  await runWorkOrderAction(`/api/v1/workorder/${selectedWorkOrder.value?.id}/complete`, {mode, reason}, mode === 'normal' ? '任务已正常完成' : '任务已强制完成')
}

async function startDepartmentTask(task: BasicItem) {
  await runWorkOrderAction(`/api/v1/workorder/department-tasks/${task.id}/start`, {}, '已开始处理')
}

async function partialCompleteDepartmentTask(task: BasicItem) {
  const quantity = await promptText('部分完成数量', `请输入已完成数量，计划 ${formatQuantity(task.planned_quantity)}`)
  if (!quantity) return
  const remark = await promptText('备注', '可填写本次完成说明', false)
  await runWorkOrderAction(`/api/v1/workorder/department-tasks/${task.id}/partial-complete`, {
    completed_quantity: decimalToScaled(quantity),
    remark,
  }, '部分完成已提交')
}

async function completeDepartmentTask(task: BasicItem) {
  const remark = await promptText('完成备注', '可填写完成说明', false)
  await runWorkOrderAction(`/api/v1/workorder/department-tasks/${task.id}/complete`, {remark}, '部门任务已完成')
}

async function runWorkOrderAction(path: string, body: Record<string, unknown>, successMessage: string) {
  if (!selectedWorkOrder.value) return
  loading.value = true
  panelMessage.value = ''
  try {
    selectedWorkOrder.value = await request<BasicItem>(path, {method: 'POST', body}, token.value)
    await Promise.all([loadActiveModule(), loadWorkOrderLogs()])
    panelMessage.value = successMessage
    ElMessage.success(successMessage)
  } catch (error) {
    panelMessage.value = error instanceof Error ? error.message : '任务操作失败'
    ElMessage.error(panelMessage.value)
  } finally {
    loading.value = false
  }
}

async function promptText(title: string, message: string, required = true): Promise<string> {
  try {
    const result = await ElMessageBox.prompt(message, title, {
      inputType: 'textarea',
      inputValidator: (value) => !required || !!String(value || '').trim(),
      inputErrorMessage: '请填写内容',
    })
    return String(result.value || '').trim()
  } catch {
    return ''
  }
}

function departmentTasks(row: any): BasicItem[] {
  return Array.isArray(row.department_tasks) ? row.department_tasks as BasicItem[] : []
}

function departmentProgressSummary(row: BasicItem): string {
  const tasks = departmentTasks(row)
  if (!tasks.length) return '0% · 尚未分配部门'
  const completed = tasks.filter((task) => task.status === 'completed').length
  const totalProgress = tasks.reduce((sum, task) => {
    const explicit = Number(task.progress)
    if (Number.isFinite(explicit)) return sum + Math.min(100, Math.max(0, explicit))
    const planned = Number(task.planned_quantity || 0)
    const finished = Number(task.completed_quantity || 0)
    return sum + (planned > 0 ? Math.min(100, Math.max(0, finished * 100 / planned)) : 0)
  }, 0)
  const percentage = Math.round(totalProgress / tasks.length)
  return `${percentage}% · ${completed}/${tasks.length} 个部门已完成`
}

function departmentName(id: unknown): string {
  const item = rowsFor('departments').find((department) => Number(department.id) === Number(id))
  return String(item?.name || `部门#${id}`)
}

function canOperateDepartmentTask(task: BasicItem): boolean {
  if (!canWriteActive.value || ['draft', 'completed'].includes(String(task.status))) return false
  if (!selectedWorkOrder.value || selectedWorkOrder.value.status === 'paused') return false
  if (currentUser.value?.department_id) {
    return Number(currentUser.value.department_id) === Number(task.department_id)
  }
  return true
}

function workorderStatusLabel(value: unknown): string {
  const labels: Record<string, string> = {
    draft: '草稿',
    processing: '正在处理',
    paused: '暂停',
    pending_close: '待办公室确认',
    completed_normal: '正常完成',
    completed_forced: '强制完成',
    cancelled: '取消',
  }
  return labels[String(value)] || String(value || '-')
}

function workorderTypeLabel(value: unknown): string {
  return value === 'general' ? '通用任务' : '生产单'
}

function inventoryItemTypeLabel(value: unknown): string {
  return value === 'product' ? '产品' : '物料'
}

function moldStatusLabel(value: unknown): string {
  const labels: Record<string, string> = {
    in_stock: '在库',
    loaned: '已借出',
    repairing: '维修中',
    maintenance: '保养中',
    scrapped: '报废',
  }
  return labels[String(value)] || String(value || '-')
}

function moldStatusTone(value: unknown): StatusTone {
  if (value === 'in_stock') return 'success'
  if (value === 'repairing' || value === 'maintenance') return 'warning'
  if (value === 'scrapped') return 'danger'
  return 'info'
}

function departmentCompletionRate(item: DepartmentStatistic): number {
  if (!item.total) return 0
  return Math.round((Number(item.completed || 0) * 100) / Number(item.total))
}

function trendNameLabel(value: unknown): string {
  const labels: Record<string, string> = {
    inbound: '入库',
    outbound: '出库',
    transfer: '调拨',
    draft: '草稿',
    processing: '处理中',
    pending_close: '待确认',
    completed_normal: '正常完成',
    completed_forced: '强制完成',
  }
  return labels[String(value)] || String(value || '趋势')
}

function departmentTaskStatusLabel(value: unknown): string {
  const labels: Record<string, string> = {
    draft: '待派发',
    received: '已收到',
    processing: '正在处理',
    partial_completed: '部分完成',
    completed: '完成',
  }
  return labels[String(value)] || String(value || '-')
}

function workorderStatusTone(value: unknown): StatusTone {
  if (value === 'completed_normal') return 'success'
  if (value === 'completed_forced' || value === 'cancelled') return 'danger'
  if (value === 'pending_close') return 'warning'
  return 'info'
}

function departmentTaskStatusTone(value: unknown): StatusTone {
  if (value === 'completed') return 'success'
  if (value === 'partial_completed') return 'warning'
  return 'info'
}

function workorderNextAction(item: BasicItem): string {
  const status = String(item.status || '')
  if (status === 'draft') return hasPermission('workorder:write') ? '办公室派发任务' : '等待办公室派发'
  if (status === 'processing') return '各部门继续处理并回报进度'
  if (status === 'paused') return hasPermission('workorder:write') ? '办公室确认后恢复任务' : '等待办公室恢复任务'
  if (status === 'pending_close') return hasPermission('workorder:write') ? '办公室核对并确认完成' : '等待办公室确认完成'
  if (status.startsWith('completed')) return '任务已结束，可查看流转日志'
  return '查看部门进度与流转日志'
}

function workorderActionLabel(value: unknown): string {
  const labels: Record<string, string> = {
    create: '创建',
    dispatch: '派发',
    dispatch_department: '部门收到',
    department_start: '部门处理',
    department_partial_complete: '部分完成',
    department_complete: '部门完成',
    pending_close: '待确认',
    pause: '暂停',
    resume: '恢复',
    urgent: '加急',
    complete_normal: '正常完成',
    complete_forced: '强制完成',
  }
  return labels[String(value)] || String(value || '-')
}

onMounted(() => {
  if (token.value) {
    void bootstrap()
  } else {
    void loadHealth()
    void loadClientUpdate()
  }
})
</script>
