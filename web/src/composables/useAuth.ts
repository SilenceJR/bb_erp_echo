import {apiBaseUrl, isDesktopClient} from '../api/http'
import type {InputInstance} from 'element-plus'
import {reactive, ref} from 'vue'
import type {ClientUpdateStatus, CurrentUser} from '../types'

type HealthState = 'checking' | 'healthy' | 'error'

/** Owns session and connection state shared by the login screen and workspace shell. */
export function useAuth() {
  // Only the access token is persisted; keeping credentials out of storage limits the
  // impact of a copied browser profile or a shared desktop workstation.
  const tokenKey = 'bb_erp_access_token'
  const desktopClient = isDesktopClient()
  const token = ref(localStorage.getItem(tokenKey) || '')
  const currentUser = ref<CurrentUser | null>(null)
  const errorMessage = ref('')
  const healthStatus = ref<HealthState>('checking')
  const serverDialogVisible = ref(false)
  const serverTesting = ref(false)
  const serverUrlInput = ref(apiBaseUrl())
  const serverMessage = ref('')
  const serverMessageType = ref<'success' | 'warning' | 'info' | 'error'>('info')
  const clientUpdate = ref<ClientUpdateStatus>({current_version: '', available: false, cached: false})
  const loginForm = reactive({username: 'admin', password: ''})
  const loginUsernameInput = ref<InputInstance>()
  const formError = ref('')

  return {
    tokenKey,
    desktopClient,
    token,
    currentUser,
    errorMessage,
    healthStatus,
    serverDialogVisible,
    serverTesting,
    serverUrlInput,
    serverMessage,
    serverMessageType,
    clientUpdate,
    loginForm,
    loginUsernameInput,
    formError,
  }
}
