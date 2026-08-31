<template>
  <ModulePageFrame :content="contentComponent" />
</template>

<script setup lang="ts">
import {computed} from 'vue'
import {useWorkspaceContext} from '../../composables/workspaceContext'
import ModulePageFrame from './module/ModulePageFrame.vue'
import {resolveModulePage} from './module/modulePageRegistry'

const {activeKey} = useWorkspaceContext()
const contentComponent = computed(() => resolveModulePage(activeKey.value))
</script>
