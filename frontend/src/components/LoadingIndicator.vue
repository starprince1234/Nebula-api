<script setup lang="ts">
withDefaults(defineProps<{ label?: string; size?: 'sm' | 'md' | 'lg'; inline?: boolean }>(), { label: '正在加载', size: 'md', inline: false })
</script>

<template>
  <span class="loading-indicator" :class="[`loading-indicator-${size}`,{inline}]" role="status" aria-live="polite">
    <span class="loading-spinner" aria-hidden="true" />
    <span class="loading-label">{{ label }}</span>
  </span>
</template>
