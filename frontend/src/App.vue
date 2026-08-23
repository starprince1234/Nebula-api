<script setup lang="ts">
import { useAuthStore } from './store/auth'
import ToastViewport from './components/ToastViewport.vue'
import AppBootLoader from './components/AppBootLoader.vue'
import NetworkLoadingLayer from './components/NetworkLoadingLayer.vue'

const auth = useAuthStore()
</script>

<template><RouterView v-slot="{Component,route}"><Transition name="app-route" mode="out-in"><AppBootLoader v-if="!auth.ready" key="boot"/><component :is="Component" v-else :key="String(route.name||route.path)"/></Transition></RouterView><NetworkLoadingLayer/><ToastViewport/></template>
