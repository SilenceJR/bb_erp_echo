import {ElMessageBox} from 'element-plus'

type ConfirmOptions = NonNullable<Parameters<typeof ElMessageBox.confirm>[2]>
type PromptOptions = NonNullable<Parameters<typeof ElMessageBox.prompt>[2]>

/**
 * Opens a confirmation box that follows the ERP interaction defaults.
 *
 * Callers may override button labels and semantic type for a specific business action.
 */
export function appConfirm(message: string, title: string, options: ConfirmOptions = {}) {
  return ElMessageBox.confirm(message, title, withDefaults(options))
}

/** Opens a text input box with the ERP interaction defaults. */
export function appPrompt(message: string, title: string, options: PromptOptions = {}) {
  return ElMessageBox.prompt(message, title, withDefaults(options))
}

/** Provides the shared confirmation and prompt API to business modules. */
export const appMessageBox = {
  confirm: appConfirm,
  prompt: appPrompt,
}

function withDefaults<T extends ConfirmOptions | PromptOptions>(options: T): T {
  return {
    confirmButtonText: '确认',
    cancelButtonText: '取消',
    ...options,
    customClass: ['bb-message-box', options.customClass].filter(Boolean).join(' '),
  } as T
}
