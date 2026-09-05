import {reactive, ref} from 'vue'
import type {BasicItem} from '../types'

type AssignmentConfig = {
  title: string
  tip: string
  buttonLabel: string
  optionKey: 'permissions' | 'roles'
  /** Endpoint used to load assignment options; it is not necessarily a navigable module. */
  optionsPath: string
  selectedKey: 'permission_ids' | 'role_ids'
  payloadKey: 'permission_ids' | 'role_ids'
  endpoint: (id: number) => string
  requiredPermissions: string[]
  isDisabled?: (target: BasicItem, option: BasicItem) => boolean
}

/** Owns role and permission assignment state while the generic list remains reusable. */
export function useAssignment() {
  const assignmentConfigs: Partial<Record<string, AssignmentConfig>> = {
    roles: {
      title: '配置角色权限',
      tip: '勾选该角色可以使用的功能；写入权限通常应同时保留对应的查看权限。',
      buttonLabel: '配置权限',
      optionKey: 'permissions',
      optionsPath: '/api/v1/system/permissions',
      selectedKey: 'permission_ids',
      payloadKey: 'permission_ids',
      endpoint: (id) => `/api/v1/system/roles/${id}/permissions`,
      requiredPermissions: ['system:roles:write'],
    },
    users: {
      title: '分配账号角色',
      tip: '只能分配当前管理员自己已拥有的角色；新角色首次分配需由超级管理员完成，终端账号不能获得超级管理员角色。',
      buttonLabel: '分配角色',
      optionKey: 'roles',
      optionsPath: '/api/v1/system/roles',
      selectedKey: 'role_ids',
      payloadKey: 'role_ids',
      endpoint: (id) => `/api/v1/system/users/${id}/roles`,
      requiredPermissions: ['system:users:write'],
      isDisabled: (target, option) => target.account_type === 'department_terminal' && option.code === 'super_admin',
    },
  }

  const assignmentTarget = ref<BasicItem | null>(null)
  const assignmentModuleKey = ref('')
  const selectedAssignmentIDs = ref<number[]>([])
  const assignmentOptionsCache = reactive<Record<string, BasicItem[]>>({})
  const assignmentOptionsLoading = ref(false)
  const assignmentOptionsError = ref('')
  const assignmentSaving = ref(false)
  const assignmentSaveError = ref('')

  return {
    assignmentConfigs,
    assignmentTarget,
    assignmentModuleKey,
    selectedAssignmentIDs,
    assignmentOptionsCache,
    assignmentOptionsLoading,
    assignmentOptionsError,
    assignmentSaving,
    assignmentSaveError,
  }
}
