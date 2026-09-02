<template>
  <LoginScreen v-if="!token" />
  <AppWorkspace v-else>
    <template #page>
      <AnimatePresence mode="wait" :initial="false">
        <motion.div
          :key="activeKey"
          class="workspace-page-motion"
          :initial="{opacity: 0, x: 24}"
          :animate="{opacity: 1, x: 0}"
          :exit="{opacity: 0, x: -24}"
          :transition="{duration: 0.22, ease: [0.2, 0, 0, 1]}"
        >
          <DashboardPage v-if="activeKey === 'dashboard'" />
          <DepartmentPage v-else-if="activeKey === 'departments'" />
          <EmployeePage v-else-if="activeKey === 'employees'" />
          <CustomerPage v-else-if="activeKey === 'customers'" />
          <ModulePage v-else />
        </motion.div>
      </AnimatePresence>
    </template>
    <template #overlays>
      <DetailPanels />
    </template>
  </AppWorkspace>
</template>

<script setup lang="ts">
import {defineAsyncComponent, provide} from 'vue'
import {AnimatePresence, motion} from 'motion-v'
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

<style scoped>
.workspace-page-motion { min-width: 0; min-height: 100%; }
</style>
