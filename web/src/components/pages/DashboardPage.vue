<template>
        <div class="dashboard">
          <div class="welcome-block">
            <div>
              <p class="eyebrow">{{ greeting }}</p>
              <h1>{{ currentUser?.name || currentUser?.username }}，今天要处理什么？</h1>
              <p>从常用功能开始，快速完成手头的工作。</p>
            </div>
            <div class="service-status" :class="`is-${healthStatus}`" role="status" aria-live="polite">
              <span></span> {{ healthStatusLabel }}
            </div>
            <DesktopUpdatePanel v-if="desktopClient" compact />
          </div>

          <AnimatePresence :initial="false">
            <motion.section
              v-if="dashboardFocusItems.length"
              key="dashboard-focus"
              layout
              class="home-section"
              aria-labelledby="dashboard-focus-title"
              :initial="{opacity: 0, y: 6}"
              :animate="{opacity: 1, y: 0}"
              :exit="{opacity: 0, y: -6}"
              :transition="{duration: 0.18, ease: [0.2, 0, 0, 1]}"
            >
              <div class="home-section-title">
                <div>
                  <h2 id="dashboard-focus-title">今日关注</h2>
                  <p>按高频 ERP 场景快速核对异常、进度和保养事项</p>
                </div>
              </div>
              <motion.div layout class="dashboard-focus-grid">
                <AnimatePresence :initial="false">
                  <motion.button
                    v-for="item in dashboardFocusItems"
                    :key="item.key"
                    layout="position"
                    class="dashboard-focus-card"
                    type="button"
                    :initial="{opacity: 0, y: 6}"
                    :animate="{opacity: 1, y: 0}"
                    :exit="{opacity: 0, scale: 0.98}"
                    :transition="{duration: 0.16, ease: [0.2, 0, 0, 1], layout: {duration: 0.2, ease: [0.2, 0, 0, 1]}}"
                    @click="switchModule(item.key)"
                  >
                    <span class="dashboard-focus-card__heading">
                      <StatusTag :label="item.label" :tone="item.tone" />
                      <span aria-hidden="true">→</span>
                    </span>
                    <strong>{{ item.title }}</strong>
                    <small>{{ item.description }}</small>
                  </motion.button>
                </AnimatePresence>
              </motion.div>
            </motion.section>
          </AnimatePresence>

          <section class="home-section dashboard-overview" aria-labelledby="dashboard-overview-title">
            <div class="home-section-title">
              <div>
                <h2 id="dashboard-overview-title">工作概览</h2>
                <p>先确认服务与当前账号可用范围，再进入业务办理</p>
              </div>
            </div>
            <div class="dashboard-metrics">
              <MetricCard
                v-for="card in dashboardMetricCards"
                :key="card.label"
                :label="card.label"
                :value="card.value"
                :caption="card.caption"
                :tone="card.tone"
                :status-label="card.statusLabel"
                :status-tone="card.statusTone"
              />
            </div>
          </section>

          <section v-if="quickActions.length" class="home-section">
            <div class="home-section-title">
              <div>
                <h2>常用功能</h2>
                <p>一步直达最常办理的业务</p>
              </div>
            </div>
            <div class="quick-grid">
              <button
                  v-for="item in quickActions"
                  :key="item.key"
                  class="quick-card"
                  type="button"
                  :aria-label="`打开${item.title}：${item.description}`"
                  @click="switchModule(item.key)"
              >
                <span class="quick-icon" aria-hidden="true">{{ item.icon }}</span>
                <span class="quick-copy">
                  <strong>{{ item.title }}</strong>
                  <small>{{ item.description }}</small>
                </span>
                <span class="quick-arrow" aria-hidden="true">→</span>
              </button>
            </div>
          </section>

          <section v-if="businessGroups.length" class="home-section">
            <div class="home-section-title">
              <div>
                <h2>全部业务</h2>
                <p>按工作场景查找功能</p>
              </div>
            </div>
            <div class="business-grid">
              <el-card v-for="group in businessGroups" :key="group.title" class="business-card" shadow="never">
                <div class="business-card-heading">
                  <span aria-hidden="true">{{ group.icon }}</span>
                  <div>
                    <strong>{{ group.title }}</strong>
                    <small>{{ group.caption }}</small>
                  </div>
                </div>
                <el-button
                    v-for="item in group.items"
                    :key="item.key"
                    link
                    @click="switchModule(item.key)"
                >
                  {{ item.title }} <span>→</span>
                </el-button>
              </el-card>
            </div>
          </section>
          <PageState
            v-if="!businessItems.length"
            kind="permission"
            title="当前账号没有可用的日常业务"
            description="如需使用其他功能，请联系管理员为账号配置对应的查看权限。"
          />
        </div>

</template>

<script setup lang="ts">
import {AnimatePresence, motion} from 'motion-v'
import DesktopUpdatePanel from '../DesktopUpdatePanel.vue'
import MetricCard from '../ui/MetricCard.vue'
import PageState from '../ui/PageState.vue'
import StatusTag from '../ui/StatusTag.vue'
import {useWorkspaceContext} from '../../composables/workspaceContext'

const {
  desktopClient,
  currentUser,
  healthStatus,
  businessItems,
  greeting,
  quickActions,
  businessGroups,
  healthStatusLabel,
  dashboardMetricCards,
  dashboardFocusItems,
  switchModule,
} = useWorkspaceContext()
</script>
