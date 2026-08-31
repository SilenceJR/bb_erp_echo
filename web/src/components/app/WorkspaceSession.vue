<template>
  <LoginScreen v-if="!token" />
  <AppWorkspace v-else>
    <template #page>
      <DashboardPage v-if="activeKey === 'dashboard'" />
      <DepartmentPage v-else-if="activeKey === 'departments'" />
      <EmployeePage v-else-if="activeKey === 'employees'" />
      <CustomerPage v-else-if="activeKey === 'customers'" />
      <ModulePage v-else />
    </template>
    <template #overlays>
      <DetailPanels />
    </template>
  </AppWorkspace>
</template>

<script setup lang="ts">
import {defineAsyncComponent, provide} from 'vue'
import AppWorkspace from './AppWorkspace.vue'
import LoginScreen from './LoginScreen.vue'
import {useWorkspaceController} from '../../composables/useWorkspaceController'
import {workspaceContextKey} from '../../composables/workspaceContext'
import {workorderContextKey} from '../../composables/workorderContext'

const DashboardPage = defineAsyncComponent(() => import('../pages/DashboardPage.vue'))
const DepartmentPage = defineAsyncComponent(() => import('../pages/DepartmentPage.vue'))
const EmployeePage = defineAsyncComponent(() => import('../pages/EmployeePage.vue'))
const CustomerPage = defineAsyncComponent(() => import('../pages/CustomerPage.vue'))
const DetailPanels = defineAsyncComponent(() => import('../pages/DetailPanels.vue'))
const ModulePage = defineAsyncComponent(() => import('../pages/ModulePage.vue'))

const workspace = useWorkspaceController()
const {activeKey, token} = workspace
provide(workspaceContextKey, workspace)
provide(workorderContextKey, workspace.workorderContext)
</script>
