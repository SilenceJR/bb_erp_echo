import {isDesktopClient} from '../api/http'
import type {InputInstance} from 'element-plus'
import {reactive, ref} from 'vue'
import type {CurrentUser} from '../types'

type HealthState = 'checking' | 'healthy' | 'error'

/** Owns session and connection state shared by the login screen and workspace shell. */
export function useAuth() {
  // Access and refresh tokens are persisted so an internal-workstation session can
  // survive page reloads; passwords are never stored in browser or desktop storage.
  const tokenKey = 'bb_erp_access_token'
  const refreshTokenKey = 'bb_erp_refresh_token'
  const tokenExpiresAtKey = 'bb_erp_access_token_expires_at'
  const desktopClient = isDesktopClient()
  const token = ref(localStorage.getItem(tokenKey) || '')
  const refreshToken = ref(localStorage.getItem(refreshTokenKey) || '')
  const tokenExpiresAt = ref(localStorage.getItem(tokenExpiresAtKey) || '')
  const currentUser = ref<CurrentUser | null>(null)
  const errorMessage = ref('')
  const healthStatus = ref<HealthState>('checking')
  const loginForm = reactive({username: 'admin', password: ''})
  const loginUsernameInput = ref<InputInstance>()
  const formError = ref('')

  return {
    tokenKey,
    refreshTokenKey,
    tokenExpiresAtKey,
    desktopClient,
    token,
    refreshToken,
    tokenExpiresAt,
    currentUser,
    errorMessage,
    healthStatus,
    loginForm,
    loginUsernameInput,
    formError,
  }
}
