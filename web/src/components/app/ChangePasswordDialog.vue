<template>
  <el-dialog
    :model-value="modelValue"
    class="change-password-dialog"
    title="修改密码"
    width="min(520px, calc(100vw - 32px))"
    append-to-body
    destroy-on-close
    :before-close="handleBeforeClose"
    :close-on-click-modal="!submitting"
    :close-on-press-escape="!submitting"
    :show-close="!submitting"
    @opened="focusCurrentPassword"
    @closed="resetSensitiveFields"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <p class="change-password-dialog__intro">
      修改成功后当前会话将退出，请使用新密码重新登录。
    </p>

    <el-form :model="form" label-position="top" :aria-busy="submitting" @submit.prevent="submit">
      <el-form-item label="当前密码" label-for="current-password" prop="currentPassword">
        <el-input
          id="current-password"
          ref="currentPasswordInput"
          v-model="form.currentPassword"
          :type="passwordVisible.currentPassword ? 'text' : 'password'"
          autocomplete="current-password"
          required
          :aria-invalid="Boolean(fieldErrors.currentPassword)"
          :aria-describedby="fieldErrors.currentPassword ? 'current-password-error' : undefined"
          :disabled="submitting"
          placeholder="请输入当前密码"
          @input="clearFieldError('currentPassword')"
          @blur="validateField('currentPassword')"
          @keydown.enter.prevent="submit"
        >
          <template #suffix>
            <button
              type="button"
              class="password-visibility-toggle"
              :aria-label="passwordVisible.currentPassword ? '隐藏当前密码' : '显示当前密码'"
              :aria-pressed="passwordVisible.currentPassword"
              :disabled="submitting"
              @click="passwordVisible.currentPassword = !passwordVisible.currentPassword"
            >
              {{ passwordVisible.currentPassword ? '隐藏' : '显示' }}
            </button>
          </template>
        </el-input>
        <p v-if="fieldErrors.currentPassword" id="current-password-error" class="password-field-error" role="alert">
          {{ fieldErrors.currentPassword }}
        </p>
      </el-form-item>

      <el-form-item label="新密码" label-for="new-password" prop="newPassword">
        <el-input
          id="new-password"
          ref="newPasswordInput"
          v-model="form.newPassword"
          :type="passwordVisible.newPassword ? 'text' : 'password'"
          autocomplete="new-password"
          required
          :aria-invalid="Boolean(fieldErrors.newPassword)"
          :aria-describedby="describedBy('newPassword', 'new-password-hint')"
          :disabled="submitting"
          placeholder="请输入新密码"
          @input="clearFieldError('newPassword')"
          @blur="validateField('newPassword')"
          @keydown.enter.prevent="submit"
        >
          <template #suffix>
            <button
              type="button"
              class="password-visibility-toggle"
              :aria-label="passwordVisible.newPassword ? '隐藏新密码' : '显示新密码'"
              :aria-pressed="passwordVisible.newPassword"
              :disabled="submitting"
              @click="passwordVisible.newPassword = !passwordVisible.newPassword"
            >
              {{ passwordVisible.newPassword ? '隐藏' : '显示' }}
            </button>
          </template>
        </el-input>
        <p id="new-password-hint" class="password-field-hint">
          至少 8 个字符，且 UTF-8 编码不超过 72 字节。
        </p>
        <p v-if="fieldErrors.newPassword" id="new-password-error" class="password-field-error" role="alert">
          {{ fieldErrors.newPassword }}
        </p>
      </el-form-item>

      <el-form-item label="确认新密码" label-for="confirm-password" prop="confirmPassword">
        <el-input
          id="confirm-password"
          ref="confirmPasswordInput"
          v-model="form.confirmPassword"
          :type="passwordVisible.confirmPassword ? 'text' : 'password'"
          autocomplete="new-password"
          required
          :aria-invalid="Boolean(fieldErrors.confirmPassword)"
          :aria-describedby="fieldErrors.confirmPassword ? 'confirm-password-error' : undefined"
          :disabled="submitting"
          placeholder="请再次输入新密码"
          @input="clearFieldError('confirmPassword')"
          @blur="validateField('confirmPassword')"
          @keydown.enter.prevent="submit"
        >
          <template #suffix>
            <button
              type="button"
              class="password-visibility-toggle"
              :aria-label="passwordVisible.confirmPassword ? '隐藏确认密码' : '显示确认密码'"
              :aria-pressed="passwordVisible.confirmPassword"
              :disabled="submitting"
              @click="passwordVisible.confirmPassword = !passwordVisible.confirmPassword"
            >
              {{ passwordVisible.confirmPassword ? '隐藏' : '显示' }}
            </button>
          </template>
        </el-input>
        <p v-if="fieldErrors.confirmPassword" id="confirm-password-error" class="password-field-error" role="alert">
          {{ fieldErrors.confirmPassword }}
        </p>
      </el-form-item>

      <div v-if="submitError" ref="submitErrorContainer" tabindex="-1">
        <el-alert :title="submitError" type="error" :closable="false" show-icon />
      </div>
    </el-form>

    <template #footer>
      <div class="change-password-dialog__footer">
        <el-button :disabled="submitting" @click="close">取消</el-button>
        <el-button type="primary" :loading="submitting" :disabled="submitting" @click="submit">
          {{ submitting ? '正在修改' : '确认修改' }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import {computed, nextTick, reactive, ref} from 'vue'
import {type InputInstance} from 'element-plus'
import {request} from '../../api/http'
import {useDirtyGuard} from '../../composables/useDirtyGuard'

type ChangePasswordForm = {
  currentPassword: string
  newPassword: string
  confirmPassword: string
}

type PasswordField = keyof ChangePasswordForm

const props = defineProps<{
  modelValue: boolean
  token: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  changed: []
}>()

const currentPasswordInput = ref<InputInstance>()
const newPasswordInput = ref<InputInstance>()
const confirmPasswordInput = ref<InputInstance>()
const submitErrorContainer = ref<HTMLElement>()
const submitting = ref(false)
const submitError = ref('')
const form = reactive<ChangePasswordForm>({currentPassword: '', newPassword: '', confirmPassword: ''})
const fieldErrors = reactive<Record<PasswordField, string>>({currentPassword: '', newPassword: '', confirmPassword: ''})
const passwordVisible = reactive<Record<PasswordField, boolean>>({currentPassword: false, newPassword: false, confirmPassword: false})
const fieldOrder: PasswordField[] = ['currentPassword', 'newPassword', 'confirmPassword']
const fieldInputs: Record<PasswordField, typeof currentPasswordInput> = {
  currentPassword: currentPasswordInput,
  newPassword: newPasswordInput,
  confirmPassword: confirmPasswordInput,
}
const dirty = computed(() => props.modelValue && fieldOrder.some((field) => Boolean(form[field])))
const passwordGuard = useDirtyGuard('change-password-form', {
  busy: () => submitting.value,
  dirty: () => dirty.value,
  busyMessage: '密码正在修改，请等待完成后再离开',
  dirtyMessage: '密码表单尚未提交，关闭后输入内容将被清除。',
})

function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).length
}

function describedBy(field: PasswordField, hintId?: string): string | undefined {
  const errorId = field.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`) + '-error'
  const ids = [hintId, fieldErrors[field] ? errorId : ''].filter(Boolean)
  return ids.length ? ids.join(' ') : undefined
}

function validateField(field: PasswordField): boolean {
  let message = ''
  if (field === 'currentPassword' && !form.currentPassword) message = '请输入当前密码'
  if (field === 'newPassword') {
    if (!form.newPassword) message = '请输入新密码'
    else if ([...form.newPassword].length < 8) message = '新密码至少需要 8 个字符'
    else if (utf8ByteLength(form.newPassword) > 72) message = '新密码 UTF-8 编码不能超过 72 字节'
    else if (form.newPassword === form.currentPassword) message = '新密码不能与当前密码相同'
  }
  if (field === 'confirmPassword') {
    if (!form.confirmPassword) message = '请再次输入新密码'
    else if (form.confirmPassword !== form.newPassword) message = '两次输入的新密码不一致'
  }
  fieldErrors[field] = message
  return !message
}

function validateForm(): boolean {
  return fieldOrder.map(validateField).every(Boolean)
}

function focusField(field: PasswordField) {
  void nextTick(() => fieldInputs[field].value?.focus())
}

function focusFirstErrorField() {
  const firstError = fieldOrder.find((field) => fieldErrors[field])
  if (firstError) focusField(firstError)
}

function clearFieldError(field: PasswordField) {
  fieldErrors[field] = ''
  submitError.value = ''
  if (field === 'currentPassword') fieldErrors.newPassword = ''
  if (field === 'newPassword') fieldErrors.confirmPassword = ''
}

function resetSensitiveFields() {
  for (const field of fieldOrder) {
    form[field] = ''
    fieldErrors[field] = ''
    passwordVisible[field] = false
  }
  submitError.value = ''
}

function focusCurrentPassword() {
  focusField('currentPassword')
}

async function close() {
  if (!(await passwordGuard.confirmLeave())) return
  resetSensitiveFields()
  emit('update:modelValue', false)
}

function handleBeforeClose(done: () => void) {
  void passwordGuard.confirmLeave().then((allowed) => {
    if (!allowed) return
    resetSensitiveFields()
    done()
  })
}

function handleRequestError(error: unknown) {
  const message = error instanceof Error ? error.message : '密码修改失败，请稍后重试'
  if (message.includes('当前密码')) {
    fieldErrors.currentPassword = message
    focusField('currentPassword')
  } else if (message.includes('新密码')) {
    fieldErrors.newPassword = message
    focusField('newPassword')
  } else {
    submitError.value = message
    void nextTick(() => submitErrorContainer.value?.focus())
  }
}

async function submit() {
  if (submitting.value) return
  submitError.value = ''
  if (!validateForm()) {
    focusFirstErrorField()
    return
  }

  submitting.value = true
  try {
    await request<void>('/api/v1/auth/change-password', {
      method: 'POST',
      body: {current_password: form.currentPassword, new_password: form.newPassword},
    }, props.token)
    resetSensitiveFields()
    emit('update:modelValue', false)
    emit('changed')
  } catch (error) {
    handleRequestError(error)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.change-password-dialog__intro {
  margin: 0 0 var(--bb-space-5);
  color: var(--bb-text-regular);
  font-size: var(--bb-font-size-13);
  line-height: var(--bb-line-height-base);
}

.password-field-hint,
.password-field-error {
  width: 100%;
  margin: var(--bb-space-1) 0 0;
  font-size: var(--bb-font-size-12);
  line-height: var(--bb-line-height-base);
}

.password-field-hint { color: var(--bb-text-regular); }
.password-field-error { color: var(--bb-danger); }

.password-visibility-toggle {
  display: inline-grid;
  min-width: 40px;
  min-height: 40px;
  place-items: center;
  border: 0;
  border-radius: var(--bb-radius-xs);
  background: transparent;
  color: var(--bb-action-primary);
  font-size: var(--bb-font-size-12);
  font-weight: var(--bb-font-weight-semibold);
}

.password-visibility-toggle:hover { background: var(--bb-brand-50); }
.password-visibility-toggle:focus-visible { outline: 2px solid var(--bb-focus-color); outline-offset: 1px; }

.change-password-dialog__footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--bb-space-2);
}

.change-password-dialog__footer .el-button { min-height: var(--bb-control-md); }

</style>
