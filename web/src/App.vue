<template>
  <el-config-provider :locale="zhCn">
    <main class="app-shell" :class="{ 'is-startup': !startup.isReady.value }">
      <StartupScreen v-if="!startup.isReady.value" />
      <WorkspaceSession v-else />
    </main>
  </el-config-provider>
</template>

<script setup lang="ts">
import {defineAsyncComponent, onBeforeUnmount, onMounted, provide} from 'vue'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import StartupScreen from './components/app/StartupScreen.vue'
import {useStartupConnection} from './composables/useStartupConnection'
import {startupConnectionKey} from './composables/workspaceContext'
import {dirtyGuardRegistry} from './platform/dirtyGuard'
import {desktopBridge} from './api/transport'

const WorkspaceSession = defineAsyncComponent(() => import('./components/app/WorkspaceSession.vue'))

// 平台连接验证完成后才挂载业务会话，避免旧实例令牌和领域请求抢先发往新服务。
const startup = useStartupConnection()
provide(startupConnectionKey, startup)
let stopDesktopCloseListener = () => {}

function beforeUnload(event: BeforeUnloadEvent) {
  if (!dirtyGuardRegistry.blocksUnload()) return
  event.preventDefault()
  event.returnValue = ''
}

onMounted(() => {
  window.addEventListener('beforeunload', beforeUnload)
  void desktopBridge()?.onWindowCloseRequested(() => dirtyGuardRegistry.confirmLeave('window-close')).then((stop) => {
    stopDesktopCloseListener = stop
  })
  void startup.start()
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', beforeUnload)
  stopDesktopCloseListener()
})
</script>
