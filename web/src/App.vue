<template>
  <el-config-provider :locale="zhCn">
    <main class="app-shell" :class="{ 'is-login': !token }">
      <LoginScreen v-if="!token" />
      <AppWorkspace v-else>
        <template #page>
          <DashboardPage v-if="activeKey === 'dashboard'" />
          <DepartmentPage v-else-if="activeKey === 'departments'" />
          <EmployeePage v-else-if="activeKey === 'employees'" />
          <ModulePage v-else />
        </template>
        <template #overlays>
          <DetailPanels />
        </template>
      </AppWorkspace>
    </main>
  </el-config-provider>
</template>

<script setup lang="ts">
import {provide} from 'vue'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import AppWorkspace from './components/app/AppWorkspace.vue'
import LoginScreen from './components/app/LoginScreen.vue'
import DashboardPage from './components/pages/DashboardPage.vue'
import DepartmentPage from './components/pages/DepartmentPage.vue'
import EmployeePage from './components/pages/EmployeePage.vue'
import DetailPanels from './components/pages/DetailPanels.vue'
import ModulePage from './components/pages/ModulePage.vue'
import {useWorkspaceController} from './composables/useWorkspaceController'
import {workspaceContextKey} from './composables/workspaceContext'

/**
 * Keeps the root component responsible only for application composition.
 *
 * The controller is provided to Web and Tauri views so both shells share one
 * authenticated session and one set of business API workflows.
 */
const workspace = useWorkspaceController()
const {activeKey, token} = workspace
provide(workspaceContextKey, workspace)
</script>
