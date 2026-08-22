<script setup lang="ts">
withDefaults(defineProps<{
  initialLoading: boolean
  refreshing?: boolean
  label?: string
  variant?: 'list' | 'cards' | 'split' | 'form' | 'detail' | 'page'
  rows?: number
}>(), { refreshing: false, label: '正在加载数据', variant: 'list', rows: 4 })
</script>

<template>
  <section class="async-region" :class="`async-region-${variant}`" :aria-busy="initialLoading||refreshing">
    <div v-if="refreshing" class="refresh-progress" role="status" aria-live="polite"><span class="sr-only">正在刷新数据</span></div>
    <Transition name="content-fade" mode="out-in">
      <div v-if="initialLoading" key="loading" class="loading-region" role="status" aria-live="polite">
        <span class="sr-only">{{ label }}</span>
        <div class="skeleton-layout" :class="`skeleton-${variant}`" aria-hidden="true">
          <div v-for="index in rows" :key="index" class="skeleton-item">
            <span class="skeleton-line skeleton-line-title" />
            <span class="skeleton-line" />
            <span class="skeleton-line skeleton-line-short" />
          </div>
        </div>
      </div>
      <div v-else key="content" class="async-content"><slot /></div>
    </Transition>
  </section>
</template>
