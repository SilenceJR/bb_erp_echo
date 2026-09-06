<template>
  <LoginScreen v-if="!token" />
  <AppWorkspace v-else>
    <template #page>
      <div class="workspace-page">
          <DashboardPage v-if="activeKey === 'dashboard'" />
          <DepartmentPage v-else-if="activeKey === 'departments'" />
          <EmployeePage v-else-if="activeKey === 'employees'" />
          <CustomerPage v-else-if="activeKey === 'customers'" />
          <ModulePage v-else />
      </div>
    </template>
    <template #overlays>
      <DetailPanels />
    </template>
  </AppWorkspace>
</template>

<script setup lang="ts">
import {defineAsyncComponent, onMounted, provide} from 'vue'
import AppWorkspace from './AppWorkspace.vue'
import LoginScreen from './LoginScreen.vue'
import {useWorkspaceController} from '../../composables/useWorkspaceController'
import {workspaceContextKey} from '../../composables/workspaceContext'
import {workorderContextKey} from '../../composables/workorderContext'
import {desktopBridge} from '../../api/transport'
import {useDesktopUpdate} from '../../composables/useDesktopUpdate'

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

const {checkOnConnection} = useDesktopUpdate()
onMounted(() => {
  // WorkspaceSession is mounted for both restored and freshly authenticated
  // sessions, so returning users also receive the once-per-server check.
  if (desktopBridge()) void checkOnConnection()
})
</script>

<style scoped>
.workspace-page { min-width: 0; min-height: 100%; }
</style>
