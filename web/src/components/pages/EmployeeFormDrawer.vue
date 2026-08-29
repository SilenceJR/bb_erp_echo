<template>
  <el-drawer v-model="visible" class="business-form-drawer" :title="title" size="min(560px, 100%)" :close-on-click-modal="!saving" :close-on-press-escape="!saving" :before-close="beforeClose" destroy-on-close>
    <section v-if="readonly && editing" class="employee-readonly-detail" aria-label="员工档案详情">
      <el-descriptions :column="1" border>
        <el-descriptions-item label="姓名">{{ editing.name }}</el-descriptions-item>
        <el-descriptions-item label="电话">{{ editing.phone || '未填写' }}</el-descriptions-item>
        <el-descriptions-item label="入职日期">{{ editing.hire_date }}</el-descriptions-item>
        <el-descriptions-item label="出生日期">{{ editing.birth_date }}（{{ ageText }}）</el-descriptions-item>
        <el-descriptions-item label="籍贯">{{ editing.birthplace || '未填写' }}</el-descriptions-item>
        <el-descriptions-item label="居住地址">{{ editing.residential_address || '未填写' }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ editing.status === 'active' ? '在职' : '已停用' }}</el-descriptions-item>
        <el-descriptions-item label="所属部门"><div class="tag-list"><el-tag v-for="department in editing.departments" :key="department.id" effect="plain">{{ department.name }}</el-tag><span v-if="!editing.departments.length">暂未加入部门</span></div></el-descriptions-item>
      </el-descriptions>
      <div class="form-actions drawer-sticky-actions"><el-button @click="requestClose()">关闭</el-button></div>
    </section>
    <el-form v-else label-position="top" :disabled="saving" @submit.prevent="$emit('save')">
      <el-alert v-if="saveError" :title="saveError" type="error" :closable="false" show-icon />
      <div class="employee-form-grid">
        <el-form-item label="姓名" required><el-input v-model.trim="form.name" maxlength="80" autofocus /></el-form-item>
        <el-form-item label="电话"><el-input v-model.trim="form.phone" maxlength="32" inputmode="tel" /></el-form-item>
        <el-form-item label="入职日期" required><el-date-picker v-model="form.hire_date" type="date" value-format="YYYY-MM-DD" placeholder="请选择入职日期" /></el-form-item>
        <el-form-item label="出生日期" required>
          <el-date-picker v-model="form.birth_date" type="date" value-format="YYYY-MM-DD" :disabled-date="futureDate" placeholder="请选择出生日期" />
          <small class="field-help">{{ ageText }}</small>
        </el-form-item>
        <el-form-item label="籍贯"><el-input v-model.trim="form.birthplace" maxlength="120" /></el-form-item>
        <el-form-item label="居住地址"><el-input v-model.trim="form.residential_address" maxlength="240" /></el-form-item>
      </div>
      <section v-if="editing" class="employee-departments-readonly">
        <strong>所属部门</strong>
        <p>成员关系请前往“部门 → 管理员工”统一维护。</p>
        <div><el-tag v-for="department in editing.departments" :key="department.id" effect="plain">{{ department.name }}</el-tag><span v-if="!editing.departments.length">暂未加入部门</span></div>
      </section>
      <div class="form-actions drawer-sticky-actions">
        <el-button :disabled="saving" @click="requestClose()">取消</el-button>
        <el-button type="primary" native-type="submit" :loading="saving">保存档案</el-button>
      </div>
    </el-form>
  </el-drawer>
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {ElDescriptions, ElDescriptionsItem} from 'element-plus'
import {appMessageBox} from '../../composables/useAppMessageBox'
import type {EmployeeItem} from '../../types'
import type {EmployeeFormValue} from '../../composables/useEmployeeDirectory'

const props = defineProps<{modelValue: boolean; title: string; form: EmployeeFormValue; editing: EmployeeItem | null; saving: boolean; saveError: string; readonly: boolean}>()
const emit = defineEmits<{(event: 'update:modelValue', value: boolean): void; (event: 'save'): void}>()
const visible = computed({get: () => props.modelValue, set: (value) => emit('update:modelValue', value)})
const initial = computed(() => props.editing ? JSON.stringify({name: props.editing.name, phone: props.editing.phone || '', hire_date: props.editing.hire_date, birthplace: props.editing.birthplace || '', residential_address: props.editing.residential_address || '', birth_date: props.editing.birth_date}) : JSON.stringify({name: '', phone: '', hire_date: '', birthplace: '', residential_address: '', birth_date: ''}))
const dirty = computed(() => JSON.stringify(props.form) !== initial.value)
const ageText = computed(() => {
  if (!props.form.birth_date) return '选择后自动计算周岁'
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(props.form.birth_date)
  if (!match) return '日期格式无效'
  const now = new Intl.DateTimeFormat('en-CA', {timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit'}).formatToParts(new Date())
  const values = Object.fromEntries(now.map((part) => [part.type, Number(part.value)]))
  const birthYear = Number(match[1]); const birthMonth = Number(match[2]); const birthDay = Number(match[3])
  let age = values.year - birthYear
  if (values.month < birthMonth || (values.month === birthMonth && values.day < birthDay)) age -= 1
  return age >= 0 ? `当前 ${age} 周岁` : '出生日期不能晚于今天'
})
const futureDate = (date: Date) => {
  const value = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
  const now = new Intl.DateTimeFormat('en-CA', {timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit'}).formatToParts(new Date())
  const parts = Object.fromEntries(now.map((part) => [part.type, part.value]))
  return value > `${parts.year}-${parts.month}-${parts.day}`
}

async function beforeClose(done: () => void) {
  if (!props.saving) await requestClose(done)
}
async function requestClose(done?: () => void) {
  if (props.readonly || !dirty.value) { if (done) done(); else visible.value = false; return }
  try { await appMessageBox.confirm('尚有未保存的员工信息，确认关闭？', '放弃修改', {type: 'warning'}) } catch { return }
  if (done) done(); else visible.value = false
}
</script>
