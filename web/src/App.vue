<template>
  <main class="app-shell" :class="{ 'is-login': !token }">
    <section v-if="!token" class="login-screen" aria-label="博邦光电登录">
      <div class="login-panel">
        <img class="login-logo" src="/bobang-logo-hd.png" alt="博邦光电"/>
        <form class="login-form" @submit.prevent="login">
          <label>
            <span>账号</span>
            <input v-model.trim="loginForm.username" autocomplete="username"/>
          </label>
          <label>
            <span>密码</span>
            <input v-model="loginForm.password" type="password" autocomplete="current-password"/>
          </label>
          <button class="primary-button" type="submit" :disabled="loading">
            {{ loading ? '登录中' : '登录' }}
          </button>
          <p v-if="errorMessage" class="error-text">{{ errorMessage }}</p>
        </form>
      </div>
    </section>

    <section v-else class="workspace">
      <header class="topbar">
        <div class="brand">
          <img src="/bobang-logo-hd.png" alt="博邦光电"/>
          <div>
            <strong>博邦光电</strong>
            <span>ERP 管理系统</span>
          </div>
        </div>
        <div class="user-chip">
          <span>{{ currentUser?.name || currentUser?.username }}</span>
          <small>{{ accountTypeText }}</small>
          <button class="ghost-button" type="button" @click="logout">退出</button>
        </div>
      </header>

      <div class="mobile-tabs" aria-label="模块导航">
        <button
            v-for="item in navItems"
            :key="item.key"
            type="button"
            :class="{ active: activeKey === item.key }"
            @click="switchModule(item.key)"
        >
          {{ item.title }}
        </button>
      </div>

      <aside class="sidebar" aria-label="系统导航">
        <nav>
          <button
              v-for="item in navItems"
              :key="item.key"
              type="button"
              :class="{ active: activeKey === item.key }"
              @click="switchModule(item.key)"
          >
            <span>{{ item.title }}</span>
            <small>{{ item.status === 'available' ? '可用' : '骨架' }}</small>
          </button>
        </nav>
      </aside>

      <section class="content">
        <div v-if="activeModule?.key === 'dashboard'" class="dashboard">
          <div class="section-title">
            <h1>运行概览</h1>
            <p>{{ apiBase }}</p>
          </div>

          <div class="metric-grid">
            <article class="metric-card">
              <span>健康检查</span>
              <strong>{{ healthStatus }}</strong>
            </article>
            <article class="metric-card">
              <span>可用接口</span>
              <strong>{{ availableCount }}</strong>
            </article>
            <article class="metric-card">
              <span>骨架模块</span>
              <strong>{{ skeletonCount }}</strong>
            </article>
            <article class="metric-card">
              <span>账号类型</span>
              <strong>{{ accountTypeText }}</strong>
            </article>
          </div>

          <div class="progress-list">
            <article v-for="item in moduleCards" :key="item.key" class="progress-row">
              <div>
                <strong>{{ item.title }}</strong>
                <span>{{ item.description }}</span>
              </div>
              <em :class="item.status">{{ item.status === 'available' ? '已接入' : '待迭代' }}</em>
            </article>
          </div>
        </div>

        <div v-else class="data-page">
          <div class="section-title">
            <h1>{{ activeModule?.title }}</h1>
            <p>{{ activeModule?.description }}</p>
          </div>

          <div v-if="activeModule?.key === 'warehouses'" class="mobile-tabs" aria-label="仓库分类">
            <button
                v-for="tab in warehouseTabs"
                :key="tab.key"
                type="button"
                :class="{ active: activeWarehouseTab === tab.key }"
                @click="switchWarehouseTab(tab.key)"
            >
              {{ tab.title }}
            </button>
          </div>

          <form v-if="formSchema.length" class="inline-form" @submit.prevent="createItem">
            <label v-for="field in formSchema" :key="field.key">
              <span>{{ field.label }}</span>
              <select v-if="field.kind === 'select'" v-model="formState[field.key]">
                <option value="">请选择</option>
                <option v-for="option in field.options" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
              <input v-else v-model="formState[field.key]" :type="field.kind === 'password' ? 'password' : 'text'"/>
            </label>
            <button class="primary-button" type="submit" :disabled="loading">新增</button>
          </form>

          <div class="toolbar">
            <button class="secondary-button" type="button" :disabled="loading" @click="loadActiveModule">刷新</button>
            <span v-if="panelMessage">{{ panelMessage }}</span>
          </div>

          <div v-if="skeletonResult" class="skeleton-state">
            <strong>{{ skeletonResult.name }}</strong>
            <span>{{ skeletonResult.message }}</span>
          </div>

          <div v-else class="table-wrap">
            <table>
              <thead>
              <tr>
                <th v-for="column in columns" :key="column">{{ column }}</th>
              </tr>
              </thead>
              <tbody>
              <tr v-for="row in rows" :key="String(row.id || JSON.stringify(row))">
                <td v-for="column in columns" :key="column">
                  {{ formatCell(row[column]) }}
                </td>
              </tr>
              <tr v-if="!rows.length">
                <td :colspan="columns.length || 1">暂无数据</td>
              </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>
    </section>
  </main>
</template>

<script setup lang="ts">
import {computed, onMounted, reactive, ref} from 'vue'
import {apiBaseUrl, request} from './api/http'
import {type ModuleItem, modules} from './data/modules'
import type {BasicItem, CurrentUser, SkeletonResponse} from './types'

type FormField = {
  key: string
  label: string
  kind?: 'text' | 'password' | 'select'
  options?: Array<{ label: string; value: string | number }>
}

// 本地存储键：只保存 access token，不保存密码。
const tokenKey = 'bb_erp_access_token'

const token = ref(localStorage.getItem(tokenKey) || '')
const currentUser = ref<CurrentUser | null>(null)
const activeKey = ref('dashboard')
const rows = ref<BasicItem[]>([])
const columns = ref<string[]>([])
const skeletonResult = ref<SkeletonResponse | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const panelMessage = ref('')
const healthStatus = ref('检查中')
const apiBase = apiBaseUrl()

const loginForm = reactive({
  username: 'admin',
  password: '',
})

const formState = reactive<Record<string, string | number>>({})
const activeWarehouseTab = ref('product')
const warehouseTabs = [
  {key: 'product', title: '产品'},
  {key: 'production_material', title: '生产物资'},
  {key: 'regular_product', title: '常规产品'},
  {key: 'daily_supply', title: '生活物资'},
]

const navItems = computed(() => modules)
const activeModule = computed(() => modules.find((item) => item.key === activeKey.value))
const moduleCards = computed(() => modules.filter((item) => item.key !== 'dashboard'))
const availableCount = computed(() => modules.filter((item) => item.status === 'available').length)
const skeletonCount = computed(() => modules.filter((item) => item.status === 'skeleton').length)
const accountTypeText = computed(() => {
  if (!currentUser.value) return '未登录'
  return currentUser.value.account_type === 'department_terminal' ? '部门终端账号' : '个人账号'
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
  const warehouseOptions = rowsFor('warehouse_records').map((item) => ({
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
    case 'warehouses':
      return [
        {key: 'name', label: `${activeWarehouseTabTitle.value}名称`},
        {key: 'code', label: `${activeWarehouseTabTitle.value}编码`},
        {key: 'unit', label: '单位'},
        {key: 'spec', label: '规格'},
        {key: 'safety_stock', label: '安全库存'},
        {key: 'default_cost', label: '默认成本'},
      ]
    case 'inventory_documents':
      return [
        {key: 'code', label: '单据编号'},
        {
          key: 'type',
          label: '类型',
          kind: 'select',
          options: [
            {label: '入库', value: 'inbound'},
            {label: '出库', value: 'outbound'},
            {label: '调拨', value: 'transfer'},
          ],
        },
        {key: 'warehouse_id', label: '仓库', kind: 'select', options: warehouseOptions},
        {key: 'to_warehouse_id', label: '目标仓库', kind: 'select', options: warehouseOptions},
        {
          key: 'item_type',
          label: '对象类型',
          kind: 'select',
          options: [
            {label: '物料', value: 'material'},
            {label: '产品', value: 'product'},
          ],
        },
        {key: 'item_id', label: '物料/产品ID'},
        {key: 'quantity', label: '数量'},
        {key: 'unit_cost', label: '单价'},
        {key: 'reason', label: '原因'},
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

function switchModule(key: string) {
  activeKey.value = key
  clearForm()
  void loadActiveModule()
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
  const keys = ['departments', 'terminals', 'warehouse_records', 'materials', 'products']
  await Promise.allSettled(keys.map((key) => loadList(key, false)))
}

async function loadActiveModule() {
  const item = activeModule.value
  if (!item || item.key === 'dashboard') return
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
  if (!item?.path) return
  loading.value = true
  panelMessage.value = ''
  try {
    await request(item.path, {
      method: 'POST',
      body: normalizedForm(),
    }, token.value)
    clearForm()
    await preloadBaseData()
    await loadActiveModule()
    panelMessage.value = '已新增'
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
  if (activeKey.value === 'inventory_documents') {
    const line: Record<string, unknown> = {}
    for (const key of ['item_type', 'item_id', 'quantity', 'unit_cost']) {
      const value = formState[key]
      if (value === '' || value === undefined) continue
      line[key] = numericKeys.has(key) ? Number(value) : value
    }
    for (const key of ['code', 'type', 'warehouse_id', 'to_warehouse_id', 'reason']) {
      const value = formState[key]
      if (value === '' || value === undefined) continue
      body[key] = numericKeys.has(key) || key.endsWith('_id') ? Number(value) : value
    }
    body.lines = [line]
    return body
  }
  for (const field of formSchema.value) {
    const value = formState[field.key]
    if (value === '' || value === undefined) continue
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

function inferColumns(data: BasicItem[], item: ModuleItem): string[] {
  const preferred: Record<string, string[]> = {
    users: ['id', 'username', 'account_type', 'name', 'organization_id', 'department_id', 'terminal_id', 'status'],
    departments: ['id', 'organization_id', 'name', 'code', 'status'],
    terminals: ['id', 'department_id', 'code', 'name', 'location', 'status'],
    roles: ['id', 'name', 'code', 'description'],
    customers: ['id', 'name', 'code', 'phone', 'address'],
    warehouses: ['id', 'item_type', 'category', 'name', 'code', 'unit', 'spec', 'safety_stock', 'status'],
    materials: ['id', 'name', 'code', 'category', 'unit', 'spec', 'safety_stock', 'status'],
    products: ['id', 'name', 'code', 'unit', 'spec', 'safety_stock', 'status'],
    inventory_documents: ['id', 'code', 'type', 'status', 'warehouse_id', 'to_warehouse_id', 'reason'],
    inventory_balances: ['id', 'warehouse_id', 'location_id', 'item_type', 'item_id', 'quantity', 'avg_cost', 'amount'],
    inventory_ledgers: ['id', 'document_id', 'type', 'warehouse_id', 'item_type', 'item_id', 'quantity', 'balance_qty', 'amount'],
    molds: ['id', 'code', 'name', 'status', 'current_location', 'storage_location', 'cavity_count', 'mold_material', 'steel', 'maintenance_cycle_days', 'next_maintenance_at'],
    permissions: ['id', 'name', 'code', 'object', 'action'],
    audits: ['id', 'actor_username', 'actor_account_type', 'person_name', 'department_id', 'terminal_id', 'action', 'object', 'result'],
  }
  if (preferred[item.key]) return preferred[item.key]
  return Object.keys(data[0] || {})
}

function formatCell(value: unknown): string {
  if (value === null || value === undefined || value === '') return '-'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

onMounted(() => {
  if (token.value) {
    void bootstrap()
  } else {
    void loadHealth()
  }
})
</script>
