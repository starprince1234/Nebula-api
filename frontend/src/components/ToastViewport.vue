<script setup lang="ts">
import { CheckCircle2, CircleAlert, X } from 'lucide-vue-next'
import { useToast } from '../composables/useToast'
const toast = useToast()
</script>
<template><div class="toast-viewport" aria-live="polite"><TransitionGroup name="toast"><article v-for="item in toast.items" :key="item.id" class="toast-item" :class="item.tone"><CheckCircle2 v-if="item.tone==='success'" :size="18"/><CircleAlert v-else :size="18"/><span>{{item.message}}</span><button class="icon-button" aria-label="关闭提示" @click="toast.dismiss(item.id)"><X :size="16"/></button></article></TransitionGroup></div></template>
