<script setup lang="ts">
import { onMounted } from 'vue'
import { useAuthStore } from './store/auth'
import ToastViewport from './components/ToastViewport.vue'

const auth = useAuthStore()
onMounted(() => auth.bootstrap())
</script>

<template><router-view /><ToastViewport/></template>
