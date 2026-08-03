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
        <div class="brand">
          <img src="/bobang-logo-hd.png" alt="博邦光电"/>
          <div>
            <strong>博邦光电</strong>
            <span>业务工作台</span>
          </div>
        </div>
        <div class="user-chip">
          <div class="user-avatar">{{ userInitial }}</div>
          <div class="user-copy">
            <span>{{ currentUser?.name || currentUser?.username }}</span>
            <small>{{ accountTypeText }}</small>
          </div>
          <el-button v-if="desktopClient" text @click="openServerSettings">服务器</el-button>
          <el-button text type="primary" @click="logout">退出登录</el-button>
        </div>
      </header>

      <div class="mobile-tabs" aria-label="模块导航">
        <el-button
            v-for="item in navItems"
            :key="item.key"
            :class="{ active: activeKey === item.key }"
            @click="switchModule(item.key)"
        >
          {{ item.title }}
        </el-button>
      </div>

      <aside class="sidebar" aria-label="系统导航">
        <el-menu class="main-nav" :default-active="activeKey" @select="switchModule">
          <el-menu-item index="dashboard">首页</el-menu-item>
          <el-menu-item-group v-if="businessItems.length" title="日常业务">
            <el-menu-item v-for="item in businessItems" :key="item.key" :index="item.key">{{ item.title }}</el-menu-item>
          </el-menu-item-group>
          <el-sub-menu v-if="systemItems.length" index="system">
            <template #title>系统设置</template>
            <el-menu-item v-for="item in systemItems" :key="item.key" :index="item.key">{{ item.title }}</el-menu-item>
          </el-sub-menu>
        </el-menu>
        <div class="sidebar-help">
          <span>遇到问题？</span>
          <small>请联系系统管理员</small>
        </div>
      </aside>

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
                  @click="switchModule(item.key)"
                  @keyup.enter="switchModule(item.key)"
              >
                <span class="quick-icon">{{ item.icon }}</span>
                <span class="quick-copy">
                  <strong>{{ item.title }}</strong>
                  <small>{{ item.description }}</small>
                </span>
                <span class="quick-arrow">→</span>
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
                  <span>{{ group.icon }}</span>
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
          <div v-if="!businessItems.length" class="permission-empty">
            <span class="permission-empty-icon">✓</span>
            <strong>当前没有待处理的业务</strong>
            <p>如需使用其他功能，请联系管理员为你的账号配置权限。</p>
          </div>
        </div>

        <div v-else class="data-page">
          <div class="page-heading">
            <div class="section-title">
              <el-button class="back-home" link @click="switchModule('dashboard')">首页</el-button>
              <span>/</span>
              <h1>{{ activeModule?.title }}</h1>
              <p>{{ activeModule?.description }}</p>
            </div>
            <el-button
                v-if="formSchema.length && canWriteActive"
                class="add-button"
                type="primary"
                @click="toggleCreateForm"
            >
              {{ showCreateForm ? '收起' : `＋ 新增${activeModule?.title || ''}` }}
            </el-button>
            <el-tag v-else-if="activeModule?.writePermission" type="info" round>仅查看</el-tag>
          </div>

          <div v-if="activeModule?.key === 'warehouses'" class="warehouse-tabs" aria-label="仓库分类">
            <el-segmented v-model="activeWarehouseTab" :options="warehouseTabOptions" @change="switchWarehouseTab"/>
          </div>

          <el-form v-if="formSchema.length && canWriteActive && showCreateForm" class="inline-form" label-position="top" @submit.prevent="createItem">
            <div class="form-heading">
              <strong>{{ editingSupplier ? '编辑供应商' : `新增${activeModule?.title}` }}</strong>
              <span>请填写以下信息，带 * 为常用必填项</span>
            </div>
            <el-form-item v-for="field in formSchema" :key="field.key" :label="field.label">
              <el-select v-if="field.kind === 'select'" v-model="formState[field.key]" placeholder="请选择" clearable>
                <el-option v-for="option in field.options" :key="option.value" :label="option.label" :value="option.value"/>
              </el-select>
              <el-input v-else v-model="formState[field.key]" :type="field.kind === 'password' ? 'password' : 'text'" :show-password="field.kind === 'password'"/>
            </el-form-item>
            <div class="form-actions">
              <el-button @click="showCreateForm = false">取消</el-button>
              <el-button type="primary" native-type="submit" :loading="loading">保存</el-button>
            </div>
          </el-form>

          <el-dialog :model-value="!!assignmentTarget" :title="assignmentConfig?.title" width="min(620px, 92vw)" @close="closeAssignment">
            <div v-if="assignmentTarget" class="assignment-panel">
            <div class="assignment-heading">
              <div>
                <span>{{ assignmentTarget.name || assignmentTarget.username || assignmentTarget.code }}</span>
              </div>
            </div>
            <p class="assignment-tip">
              {{ assignmentConfig?.tip }}
            </p>
            <div class="assignment-options">
              <el-checkbox-group v-model="selectedAssignmentIDs">
                <el-checkbox
                    v-for="option in assignmentOptions"
                    :key="option.id"
                    :value="option.id"
                    :disabled="isAssignmentOptionDisabled(option)"
                    class="check-option"
                >
                <span>
                  <strong>{{ option.name || option.code }}</strong>
                  <small>{{ option.description || option.code }}</small>
                </span>
                </el-checkbox>
              </el-checkbox-group>
              <span v-if="!assignmentOptions.length" class="assignment-empty">暂无可配置项</span>
            </div>
            <div class="assignment-actions">
              <el-button @click="closeAssignment">取消</el-button>
              <el-button type="primary" :loading="loading" @click="saveAssignment">保存配置</el-button>
            </div>
            </div>
          </el-dialog>

          <div class="toolbar">
            <span class="result-count">共 {{ rows.length }} 条记录</span>
            <div>
              <span v-if="panelMessage" class="panel-message">{{ panelMessage }}</span>
              <el-button :loading="loading" @click="loadActiveModule">刷新数据</el-button>
            </div>
          </div>

          <div v-if="skeletonResult" class="skeleton-state">
            <strong>{{ skeletonResult.name }}</strong>
            <span>{{ skeletonResult.message }}</span>
          </div>

          <el-table v-else-if="activeKey === 'warehouses'" v-loading="loading" :data="rows" row-key="id" stripe class="data-table">
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
                <el-tag :type="Number(row.quantity || 0) <= Number(row.safety_stock || 0) ? 'danger' : 'success'" effect="light">
                  {{ formatQuantity(row.quantity) }}
                </el-tag>
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
            <template #empty><el-empty description="这里还没有记录"/></template>
          </el-table>
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

      <el-drawer v-model="warehouseDrawerVisible" size="min(620px, 100%)" :with-header="false" destroy-on-close @closed="resetWarehouseItem">
        <div v-if="selectedWarehouseItem" class="item-drawer" aria-label="物品详情">
          <div class="drawer-heading">
            <div>
              <small>{{ selectedWarehouseItem.category }}</small>
              <h2>{{ selectedWarehouseItem.name }}</h2>
              <span>{{ selectedWarehouseItem.code }} · {{ selectedWarehouseItem.spec || '无规格' }}</span>
            </div>
            <el-button circle @click="closeWarehouseItem">×</el-button>
          </div>

          <div class="stock-summary">
            <div><span>当前库存</span><strong>{{ formatQuantity(warehouseDetail?.quantity) }} {{ selectedWarehouseItem.unit }}</strong></div>
            <div><span>安全库存</span><strong>{{ formatQuantity(selectedWarehouseItem.safety_stock) }} {{ selectedWarehouseItem.unit }}</strong></div>
            <div v-if="hasPermission('cost:view')"><span>库存金额</span><strong>{{ formatMoney(warehouseDetail?.amount) }}</strong></div>
          </div>
          <p v-if="panelMessage" class="drawer-message">{{ panelMessage }}</p>

          <section v-if="hasPermission('inventory:documents:write')" class="movement-section">
            <h3>办理出入库</h3>
            <div class="movement-actions">
              <el-button v-for="definition in availableMovementDefinitions" :key="definition.key" plain type="primary" @click="startMovement(definition.key)">
                {{ definition.title }}
              </el-button>
            </div>
            <p v-if="movementDependencyMessage" class="permission-hint">{{ movementDependencyMessage }}</p>
          </section>

          <el-form v-if="movementMode" class="movement-form" label-position="top" @submit.prevent="submitMovement">
            <div class="form-heading">
              <strong>{{ movementTitle }}</strong>
              <span>库存将在提交后立即生效</span>
            </div>
            <el-form-item v-if="movementMode === 'purchase_inbound'" label="供应商" required>
              <el-select v-model="movementForm.supplier_id" filterable placeholder="请选择供应商">
                <el-option v-for="item in rowsFor('suppliers')" :key="item.id" :label="`${item.name}（${item.code}）`" :value="item.id"/>
              </el-select>
            </el-form-item>
            <el-button v-if="movementMode === 'purchase_inbound' && hasPermission('suppliers:write')" link type="primary" class="inline-link" @click="showQuickSupplier = !showQuickSupplier">
              {{ showQuickSupplier ? '取消新增供应商' : '＋ 快捷新增供应商' }}
            </el-button>
            <div v-if="showQuickSupplier" class="quick-supplier">
              <el-input v-model.trim="quickSupplier.name" placeholder="供应商名称"/>
              <el-input v-model.trim="quickSupplier.code" placeholder="供应商编码"/>
              <el-input v-model.trim="quickSupplier.contact" placeholder="联系人"/>
              <el-input v-model.trim="quickSupplier.phone" placeholder="联系电话"/>
              <el-button :loading="loading" @click="createQuickSupplier">保存供应商</el-button>
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
              <el-input-number v-model="movementForm.quantity" :min="0" :precision="4" :controls="false" placeholder="请输入数量"/>
            </el-form-item>
            <el-form-item v-if="movementMode === 'purchase_inbound' && hasPermission('cost:view')" label="采购单价（元）">
              <el-input-number v-model="movementForm.unit_cost" :min="0" :precision="2" :controls="false" placeholder="选填"/>
            </el-form-item>
            <el-form-item :label="movementMode === 'return_rework_inbound' ? '返工原因' : '备注'" :required="movementMode === 'return_rework_inbound'">
              <el-input v-model.trim="movementForm.reason" type="textarea" :rows="2" placeholder="补充业务说明"/>
            </el-form-item>
            <div class="form-actions">
              <el-button @click="cancelMovement">取消</el-button>
              <el-button type="primary" native-type="submit" :loading="loading">确认并更新库存</el-button>
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
        </div>
      </el-drawer>
    </section>
  </main>
  </el-config-provider>
</template>

<script setup lang="ts">
import {computed, onMounted, reactive, ref} from 'vue'
import {ElMessage} from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import {apiBaseUrl, isDesktopClient, request, saveDesktopServerUrl, testDesktopServerUrl} from './api/http'
import {type ModuleItem, modules} from './data/modules'
import type {BasicItem, CurrentUser, SkeletonResponse} from './types'

type FormField = {
  key: string
  label: string
  kind?: 'text' | 'password' | 'select'
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
const loading = ref(false)
const errorMessage = ref('')
const panelMessage = ref('')
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
const healthStatus = ref('检查中')
const serverDialogVisible = ref(false)
const serverTesting = ref(false)
const serverUrlInput = ref(apiBaseUrl())
const serverMessage = ref('')
const serverMessageType = ref<'success' | 'warning' | 'info' | 'error'>('info')
const loginForm = reactive({
  username: 'admin',
  password: '',
})

const formState = reactive<Record<string, string | number>>({})
const movementForm = reactive<Record<string, any>>({})
const quickSupplier = reactive({name: '', code: '', contact: '', phone: ''})
const activeWarehouseTab = ref('product')
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

const navItems = computed(() => modules.filter(canReadModule))
const businessItems = computed(() => navItems.value.filter((item) => item.group === 'business'))
const systemItems = computed(() => navItems.value.filter((item) => item.group === 'system'))
const activeModule = computed(() => modules.find((item) => item.key === activeKey.value))
const canWriteActive = computed(() => !!activeModule.value && canWriteModule(activeModule.value))
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
  {key: 'warehouses', title: '仓库', description: '查询库存并办理物品出入库', icon: '▦'},
  {key: 'customers', title: '客户档案', description: '查找或新增客户资料', icon: '◎'},
  {key: 'suppliers', title: '供应商', description: '维护采购供应商资料', icon: '↙'},
  {key: 'molds', title: '模具台账', description: '查询模具位置与保养状态', icon: '◇'},
]
const quickActions = computed(() => quickActionDefinitions.filter((item) => {
  const moduleItem = modules.find((candidate) => candidate.key === item.key)
  return !!moduleItem && canReadModule(moduleItem)
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
  const missing: string[] = []
  if (!hasPermission('suppliers:read')) missing.push('采购入库需要供应商查看权限')
  if (!hasPermission('customers:read')) missing.push('客户出库需要客户查看权限')
  if (!hasPermission('system:departments:read')) missing.push('部门出库需要部门查看权限')
  return missing.join('；')
})
const movementTitle = computed(() => movementDefinitions.find((item) => item.key === movementMode.value)?.title || '办理出入库')
const displayedItemMovements = computed(() => showAllItemMovements.value ? itemMovements.value : itemMovements.value.slice(0, 10))
const eligibleOriginalDocuments = computed(() => itemMovements.value.filter((item) => {
  if (item.type !== 'outbound' || item.status !== 'posted') return false
  if (movementForm.source_type === 'customer') return Number(item.customer_id) === Number(movementForm.customer_id)
  if (movementForm.source_type === 'department') return Number(item.department_id) === Number(movementForm.department_id)
  return false
}))

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
    default:
      return []
  }
})

// cache 保存基础资料列表，为用户、部门、终端表单提供选项。
const cache = reactive<Record<string, BasicItem[]>>({})
const activeWarehouseTabTitle = computed(() => warehouseTabs.find((tab) => tab.key === activeWarehouseTab.value)?.title || '物品')

function rowsFor(key: string): BasicItem[] {
  return cache[key] || []
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

function switchModule(key: string) {
  const target = modules.find((item) => item.key === key)
  if (!target || !canReadModule(target)) {
    panelMessage.value = '你的账号暂无该功能权限'
    activeKey.value = 'dashboard'
    return
  }
  activeKey.value = key
  closeWarehouseItem()
  closeAssignment()
  showCreateForm.value = false
  editingSupplier.value = null
  clearForm()
  void loadActiveModule()
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
  clearForm()
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
  await Promise.allSettled([loadHealth(), loadMe(), preloadBaseData()])
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
  skeletonResult.value = null
  try {
    await loadList(item.key, true)
    panelMessage.value = '已同步'
  } catch (error) {
    panelMessage.value = error instanceof Error ? error.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function loadList(key: string, applyToPanel: boolean) {
  const item = modules.find((moduleItem) => moduleItem.key === key)
  let path = item?.path
  if (key === 'warehouse_records') {
    path = '/api/v1/warehouses'
  }
  if (!path) return
  if (key === 'warehouses') {
    path = `${path}?tab=${activeWarehouseTab.value}`
  }
  const data = await request<BasicItem[] | SkeletonResponse>(path, {}, token.value)
  if (!Array.isArray(data)) {
    if (applyToPanel) {
      skeletonResult.value = data
      rows.value = []
      columns.value = []
    }
    return
  }
  cache[key] = data
  if (applyToPanel) {
    rows.value = data
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

function closeWarehouseItem() {
  warehouseDrawerVisible.value = false
}

function resetWarehouseItem() {
  selectedWarehouseItem.value = null
  warehouseDetail.value = null
  itemMovements.value = []
  movementMode.value = ''
  showQuickSupplier.value = false
  clearMovementForm()
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
  movementMode.value = mode
  showQuickSupplier.value = false
  clearMovementForm()
  if (mode === 'return_rework_inbound') {
    movementForm.source_type = hasPermission('customers:read') ? 'customer' : 'department'
  }
}

function cancelMovement() {
  movementMode.value = ''
  showQuickSupplier.value = false
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
  const item = selectedWarehouseItem.value
  if (!item || !movementMode.value) return
  const quantity = decimalToScaled(movementForm.quantity)
  if (quantity <= 0) {
    panelMessage.value = '请输入大于 0 的数量'
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
  loading.value = true
  panelMessage.value = ''
  try {
    await request(`/api/v1/warehouse/items/${item.item_type}/${item.id}/movements`, {
      method: 'POST',
      headers: {'Idempotency-Key': crypto.randomUUID()},
      body,
    }, token.value)
    cancelMovement()
    await Promise.all([loadActiveModule(), loadWarehouseItemDetail(), loadItemMovements()])
    const refreshed = rows.value.find((row) => row.id === item.id && row.item_type === item.item_type)
    if (refreshed) selectedWarehouseItem.value = refreshed
    panelMessage.value = '库存已更新'
    ElMessage.success('库存已更新')
  } catch (error) {
    panelMessage.value = error instanceof Error ? error.message : '办理失败'
    ElMessage.error(panelMessage.value)
  } finally {
    loading.value = false
  }
}

async function createQuickSupplier() {
  if (!quickSupplier.name || !quickSupplier.code) {
    panelMessage.value = '请填写供应商名称和编码'
    return
  }
  loading.value = true
  try {
    const created = await request<BasicItem>('/api/v1/suppliers', {method: 'POST', body: {...quickSupplier}}, token.value)
    await loadList('suppliers', false)
    movementForm.supplier_id = created.id
    Object.assign(quickSupplier, {name: '', code: '', contact: '', phone: ''})
    showQuickSupplier.value = false
    panelMessage.value = '供应商已新增'
    ElMessage.success('供应商已新增')
  } catch (error) {
    panelMessage.value = error instanceof Error ? error.message : '供应商新增失败'
    ElMessage.error(panelMessage.value)
  } finally {
    loading.value = false
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

onMounted(() => {
  if (token.value) {
    void bootstrap()
  } else {
    void loadHealth()
  }
})
</script>
