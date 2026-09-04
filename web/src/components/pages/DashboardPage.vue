<template>
  <div class="dashboard">
    <div class="welcome-block">
      <div><h1>{{ greeting }}，{{ currentUser?.name || currentUser?.username }}</h1></div>
      <DesktopUpdatePanel v-if="desktopClient" compact />
    </div>
    <section v-if="quickActions.length" class="home-section" aria-labelledby="home-actions-title">
      <div class="home-section-title"><h2 id="home-actions-title">业务入口</h2></div>
      <div class="quick-grid">
        <button v-for="item in quickActions" :key="item.key" class="quick-card" type="button"
          :aria-label="`打开${item.title}`" @click="switchModule(item.key)">
          <span class="quick-icon" aria-hidden="true"><component :is="item.icon" /></span>
          <span class="quick-copy"><strong>{{ item.title }}</strong><small>{{ item.description }}</small></span>
          <el-icon class="quick-arrow" aria-hidden="true"><ArrowRight /></el-icon>
        </button>
      </div>
    </section>
    <PageState v-if="!businessItems.length" kind="permission" title="暂无可用业务"
      description="请联系管理员分配所需权限。" />
  </div>
</template>

<script setup lang="ts">
import DesktopUpdatePanel from '../DesktopUpdatePanel.vue'
import PageState from '../ui/PageState.vue'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import {ArrowRight} from '@element-plus/icons-vue'
const {desktopClient, currentUser, businessItems, greeting, quickActions, switchModule} = useWorkspaceContext()
</script>
