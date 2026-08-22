<script setup lang="ts">
defineOptions({ inheritAttrs: false })
withDefaults(defineProps<{ pending: boolean; pendingLabel?: string; disabled?: boolean; type?: 'button' | 'submit' | 'reset' }>(), { pendingLabel: '处理中…', disabled: false, type: 'button' })
</script>

<template>
  <button v-bind="$attrs" :type="type" :disabled="disabled||pending" :aria-busy="pending">
    <span class="loading-button-content" :class="{pending}">
      <span v-if="pending" class="loading-spinner loading-spinner-sm" aria-hidden="true" />
      <span>{{ pending ? pendingLabel : '' }}</span>
      <span v-if="!pending"><slot /></span>
    </span>
  </button>
</template>
