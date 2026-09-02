<template>
  <el-drawer
    v-model="visible"
    class="business-form-drawer customer-profile-drawer"
    size="min(720px, 100%)"
    :title="drawerTitle"
    :close-on-click-modal="!saving"
    :close-on-press-escape="!saving"
    :before-close="beforeClose"
    destroy-on-close
    @closed="restoreFocus"
  >
    <div v-if="visible" :key="`${mode}-${profile?.id || 'new'}`" class="customer-profile-motion">
        <section v-if="mode === 'view' && profile" class="customer-drawer-content" aria-label="客户资料详情">
          <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
          <div class="customer-drawer-heading">
            <div>
              <span class="customer-code-chip">{{ code?.code || profile.code }}</span>
              <h2>{{ profile.short_name || profile.name || '未填写客户名称' }}</h2>
              <p v-if="profile.name && profile.short_name">{{ profile.name }}</p>
            </div>
            <el-tag v-if="profile.is_default" type="success" effect="plain">默认资料</el-tag>
          </div>

      <el-descriptions :column="1" border>
        <el-descriptions-item label="客户简称">{{ display(profile.short_name) }}</el-descriptions-item>
        <el-descriptions-item label="客户名称">{{ display(profile.name) }}</el-descriptions-item>
        <el-descriptions-item label="地址">{{ display(profile.address) }}</el-descriptions-item>
        <el-descriptions-item label="电话"><span class="text-cell">{{ display(profile.phone) }}</span></el-descriptions-item>
        <el-descriptions-item label="联系人">{{ display(profile.contact_name) }}</el-descriptions-item>
        <el-descriptions-item label="联系人电话"><span class="text-cell">{{ display(profile.contact_phone) }}</span></el-descriptions-item>
        <el-descriptions-item label="业务员">{{ display(profile.salesperson) }}</el-descriptions-item>
      </el-descriptions>

      <section v-if="code" class="sibling-profiles" aria-labelledby="sibling-profile-title">
        <div class="section-heading">
          <div><h3 id="sibling-profile-title">同编码其他资料</h3><p>共 {{ code.profiles.length }} 条，业务单据可选择具体资料。</p></div>
          <el-button v-if="canWrite" @click="emit('add-same', code)">新增同码资料</el-button>
        </div>
        <div class="sibling-profile-list">
          <button
            v-for="item in code.profiles"
            :key="item.id"
            type="button"
            :class="{active: item.id === profile.id}"
            @click="emit('select-profile', item, code)"
          >
            <span><strong>{{ item.short_name || item.name || `资料 #${item.id}` }}</strong><small>{{ item.contact_name || '未填写联系人' }}</small></span>
            <el-tag v-if="item.is_default" size="small" type="success" effect="plain">默认</el-tag>
          </button>
        </div>
      </section>

      <div class="drawer-sticky-actions customer-drawer-actions">
        <el-button @click="requestClose()">关闭</el-button>
        <template v-if="canWrite">
          <el-button v-if="!profile.is_default" :loading="saving" @click="emit('set-default', profile)">设为默认</el-button>
          <el-button type="danger" plain :disabled="saving" @click="emit('delete', profile, code)">删除资料</el-button>
          <el-button type="primary" :disabled="saving" @click="emit('edit', profile, code)">编辑资料</el-button>
        </template>
      </div>
        </section>

        <el-form v-else label-position="top" :disabled="saving" class="customer-profile-form" @submit.prevent="submit">
      <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
      <section v-if="mode === 'create' && code" class="form-code-summary">
        <span>客户编码</span><strong>{{ code.code }}</strong>
        <small>当前从客户详情新增同码资料，编码不可更换。</small>
      </section>
      <section v-else-if="mode === 'create' && !code" class="form-section">
        <div class="section-heading"><div><h3>客户编码</h3><p>可选择已有编码，或用系统建议值创建新编码。</p></div></div>
        <el-radio-group v-model="codeMode" class="code-mode-switch">
          <el-radio-button value="existing">选择已有编码</el-radio-button>
          <el-radio-button value="new">创建新编码</el-radio-button>
        </el-radio-group>
        <el-form-item v-if="codeMode === 'existing'" label="客户编码" required :error="fieldErrors.customer_code_id">
          <el-select v-model="form.customer_code_id" filterable placeholder="请选择客户编码" style="width:100%">
            <el-option v-for="item in codes" :key="item.id" :value="item.id" :label="codeOptionLabel(item)" />
          </el-select>
        </el-form-item>
        <el-form-item v-else label="新客户编码" required :error="fieldErrors.new_code">
          <el-input v-model.trim="form.new_code" maxlength="40" placeholder="例如 BB-001，可修改建议值" @blur="normalizeCodeInput" />
          <small class="field-help">格式为 BB- 加至少 3 位正整数；1、BB-1 会规范为 BB-001。</small>
        </el-form-item>
      </section>
      <section v-else class="form-code-summary">
        <span>客户编码</span><strong>{{ code?.code || profile?.code }}</strong>
        <small>资料创建后不可更换所属编码。</small>
      </section>

      <section class="form-section">
        <div class="section-heading"><div><h3>客户资料</h3><p>除客户编码外，其他字段均可留空。</p></div></div>
        <div class="customer-form-grid">
          <el-form-item label="客户简称"><el-input v-model.trim="form.short_name" maxlength="160" autofocus /></el-form-item>
          <el-form-item label="客户名称"><el-input v-model.trim="form.name" maxlength="160" /></el-form-item>
          <el-form-item class="span-two" label="地址"><el-input v-model.trim="form.address" maxlength="255" /></el-form-item>
          <el-form-item label="电话"><el-input v-model.trim="form.phone" maxlength="60" inputmode="tel" /></el-form-item>
          <el-form-item label="业务员"><el-input v-model.trim="form.salesperson" maxlength="120" /></el-form-item>
        </div>
      </section>

      <section class="form-section">
        <div class="section-heading"><div><h3>联系人信息</h3><p>联系人及联系电话随当前客户资料一起维护。</p></div></div>
        <div class="customer-form-grid">
          <el-form-item label="联系人"><el-input v-model.trim="form.contact_name" maxlength="120" /></el-form-item>
          <el-form-item label="联系人电话"><el-input v-model.trim="form.contact_phone" maxlength="60" inputmode="tel" /></el-form-item>
        </div>
      </section>

      <div class="drawer-sticky-actions customer-drawer-actions">
        <el-button :disabled="saving" @click="requestClose()">取消</el-button>
        <el-button type="primary" native-type="submit" :loading="saving">保存客户资料</el-button>
      </div>
        </el-form>
    </div>
  </el-drawer>
</template>

<script setup lang="ts">
import {computed, nextTick, reactive, ref, watch} from 'vue'
import {appMessageBox} from '../../composables/useAppMessageBox'
import {useDirtyGuard} from '../../composables/useDirtyGuard'
import type {CustomerCodeItem, CustomerProfile} from '../../types'

export interface CustomerProfileFormValue {
  customer_code_id: number | undefined
  new_code: string
  short_name: string
  name: string
  address: string
  phone: string
  contact_name: string
  contact_phone: string
  salesperson: string
}

const props = defineProps<{
  modelValue: boolean
  mode: 'view' | 'create' | 'edit'
  profile: CustomerProfile | null
  code: CustomerCodeItem | null
  codes: CustomerCodeItem[]
  suggestedCode: string
  saving: boolean
  error: string
  canWrite: boolean
  returnFocus?: HTMLElement | null
}>()

const emit = defineEmits<{
  (event: 'update:modelValue', value: boolean): void
  (event: 'save', value: CustomerProfileFormValue): void
  (event: 'edit', profile: CustomerProfile, code: CustomerCodeItem | null): void
  (event: 'delete', profile: CustomerProfile, code: CustomerCodeItem | null): void
  (event: 'set-default', profile: CustomerProfile): void
  (event: 'add-same', code: CustomerCodeItem): void
  (event: 'select-profile', profile: CustomerProfile, code: CustomerCodeItem): void
}>()

const visible = computed({get: () => props.modelValue, set: (value) => emit('update:modelValue', value)})
const codeMode = ref<'existing' | 'new'>('existing')
const form = reactive<CustomerProfileFormValue>(emptyForm())
const baseline = ref('')
const fieldErrors = reactive({customer_code_id: '', new_code: ''})
const drawerTitle = computed(() => props.mode === 'create' ? '新增客户资料' : props.mode === 'edit' ? '编辑客户资料' : '客户资料详情')
const dirty = computed(() => props.mode !== 'view' && JSON.stringify(form) !== baseline.value)

useDirtyGuard('customer-profile', {
  busy: () => props.modelValue && props.saving,
  dirty: () => props.modelValue && dirty.value,
  busyMessage: '客户资料正在保存，请等待完成后再离开',
  dirtyMessage: '当前客户资料尚未保存，离开后修改将丢失。',
})

watch(() => [props.modelValue, props.mode, props.profile?.id, props.code?.id, props.suggestedCode] as const, () => {
  if (!props.modelValue) return
  Object.assign(form, props.profile ? fromProfile(props.profile) : emptyForm())
  if (props.mode === 'create' && props.code) {
    codeMode.value = 'existing'
    form.customer_code_id = props.code.id
    form.new_code = ''
  } else if (props.mode === 'create') {
    codeMode.value = 'new'
    form.customer_code_id = undefined
    form.new_code = props.suggestedCode
  } else {
    codeMode.value = 'existing'
  }
  if (props.mode === 'create' && !form.new_code) form.new_code = props.suggestedCode
  fieldErrors.customer_code_id = ''
  fieldErrors.new_code = ''
  nextTick(() => { baseline.value = JSON.stringify(form) })
}, {immediate: true})

function emptyForm(): CustomerProfileFormValue {
  return {customer_code_id: undefined, new_code: '', short_name: '', name: '', address: '', phone: '', contact_name: '', contact_phone: '', salesperson: ''}
}
function fromProfile(profile: CustomerProfile): CustomerProfileFormValue {
  return {customer_code_id: profile.customer_code_id, new_code: '', short_name: profile.short_name || '', name: profile.name || '', address: profile.address || '', phone: profile.phone || '', contact_name: profile.contact_name || '', contact_phone: profile.contact_phone || '', salesperson: profile.salesperson || ''}
}
function display(value?: string) { return value || '未填写' }
function codeOptionLabel(item: CustomerCodeItem) { return `${item.code} · ${item.profile_count || item.profiles.length} 条资料` }

function normalizeCodeInput() {
  const raw = form.new_code.trim()
  const match = /^(?:BB-)?(\d+)$/i.exec(raw)
  if (!match) return
  const number = Number(match[1])
  if (!Number.isSafeInteger(number) || number <= 0) return
  form.new_code = `BB-${String(number).padStart(3, '0')}`
}

function submit() {
  fieldErrors.customer_code_id = ''
  fieldErrors.new_code = ''
  if (props.mode === 'create' && props.code) {
    form.customer_code_id = props.code.id
    form.new_code = ''
    emit('save', {...form})
    return
  }
  if (props.mode === 'create' && codeMode.value === 'existing' && !form.customer_code_id) {
    fieldErrors.customer_code_id = '请选择客户编码'
    return
  }
  if (props.mode === 'create' && codeMode.value === 'new') {
    normalizeCodeInput()
    if (!/^BB-\d{3,}$/.test(form.new_code) || Number(form.new_code.slice(3)) <= 0) {
      fieldErrors.new_code = '请输入 BB- 加至少 3 位正整数'
      return
    }
    form.customer_code_id = undefined
  }
  emit('save', {...form})
}

async function beforeClose(done: () => void) {
  if (props.saving) return
  if (await confirmClose()) done()
}
async function confirmClose(): Promise<boolean> {
  if (!dirty.value) return true
  try {
    await appMessageBox.confirm('当前客户资料尚未保存，确认放弃修改？', '放弃修改', {type: 'warning'})
    return true
  } catch { return false }
}
async function requestClose() {
  if (props.saving) return false
  if (!(await confirmClose())) return false
  visible.value = false
  return true
}
function restoreFocus() { props.returnFocus?.focus?.() }

defineExpose({dirty, requestClose})
</script>

<style scoped>
.customer-drawer-content,
.customer-profile-form { display: grid; gap: var(--bb-space-5); padding-bottom: 76px; }
.customer-profile-motion { min-height: 100%; }
.customer-drawer-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--bb-space-4); }
.customer-drawer-heading h2 { margin: var(--bb-space-2) 0 0; font-size: var(--bb-font-size-24); }
.customer-drawer-heading p { margin: var(--bb-space-1) 0 0; color: var(--bb-text-secondary); }
.customer-code-chip { display: inline-flex; border-radius: var(--bb-radius-pill); background: var(--bb-brand-50); padding: var(--bb-space-1) var(--bb-space-2); color: var(--bb-brand-700); font-family: var(--bb-font-mono); font-size: var(--bb-font-size-13); font-weight: var(--bb-font-weight-bold); }
.text-cell { font-family: var(--bb-font-mono); }
.form-section,
.sibling-profiles { display: grid; gap: var(--bb-space-4); border: 1px solid var(--bb-border-default); border-radius: var(--bb-radius-xl); background: var(--bb-bg-surface); padding: var(--bb-space-5); }
.section-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--bb-space-3); }
.section-heading h3 { margin: 0; font-size: var(--bb-font-size-16); }
.section-heading p { margin: var(--bb-space-1) 0 0; color: var(--bb-text-secondary); font-size: var(--bb-font-size-13); }
.sibling-profile-list { display: grid; gap: var(--bb-space-2); }
.sibling-profile-list button { display: flex; min-height: 56px; align-items: center; justify-content: space-between; gap: var(--bb-space-3); border: 1px solid var(--bb-border-default); border-radius: var(--bb-radius-md); background: var(--bb-bg-surface); padding: var(--bb-space-2) var(--bb-space-3); color: var(--bb-text-primary); text-align: left; }
.sibling-profile-list button.active { border-color: var(--bb-brand-300); background: var(--bb-brand-50); }
.sibling-profile-list span { display: grid; gap: var(--bb-space-1); }
.sibling-profile-list small { color: var(--bb-text-secondary); }
.code-mode-switch { width: 100%; }
.code-mode-switch :deep(.el-radio-button) { flex: 1 1 50%; }
.code-mode-switch :deep(.el-radio-button__inner) { width: 100%; min-height: 44px; }
.customer-form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 var(--bb-space-4); }
.customer-form-grid .span-two { grid-column: 1 / -1; }
.form-code-summary { display: grid; grid-template-columns: auto 1fr; gap: var(--bb-space-1) var(--bb-space-3); border-radius: var(--bb-radius-lg); background: var(--bb-brand-50); padding: var(--bb-space-4); }
.form-code-summary span,
.form-code-summary small { color: var(--bb-text-secondary); }
.form-code-summary strong { font-family: var(--bb-font-mono); }
.form-code-summary small { grid-column: 1 / -1; }
.field-help { display: block; margin-top: var(--bb-space-1); color: var(--bb-text-secondary); line-height: var(--bb-line-height-base); }
.customer-drawer-actions { display: flex; justify-content: flex-end; gap: var(--bb-space-2); }
</style>
