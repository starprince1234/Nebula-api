<script setup lang="ts">
import { computed } from 'vue'
import { networkActivity } from '../api/client'

const reading = computed(() => networkActivity.reads > 0)
const writing = computed(() => networkActivity.writes > 0)
</script>

<template>
  <Transition name="network-loading">
    <div v-if="reading" class="network-loading-layer" role="status" aria-live="polite">
      <span class="sr-only">正在加载页面和表格数据</span>
      <span class="network-progress" aria-hidden="true" />
    </div>
  </Transition>
  <Transition name="content-fade">
    <div v-if="writing" class="network-write-indicator" role="status" aria-live="polite"><span class="loading-spinner loading-spinner-sm" aria-hidden="true"/>正在提交操作</div>
  </Transition>
</template>
