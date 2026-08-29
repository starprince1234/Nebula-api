<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { studentAPI } from '../../api/student'
import type { StudentUsage } from '../../api/types'
import CreditDonut from '../../components/CreditDonut.vue'
const usage = ref<StudentUsage | null>(null)
const month = ref(new Date().toISOString().slice(0, 7))
let timer: number | undefined
async function load() { usage.value = await studentAPI.usage(month.value) }
onMounted(() => { void load(); timer = window.setInterval(() => { if (document.visibilityState === 'visible') void load() }, 30000) })
onBeforeUnmount(() => { if (timer) window.clearInterval(timer) })
</script>
<template><section class="page"><div class="page-heading"><div><p class="eyebrow">STUDENT</p><h1>个人用量</h1></div><label>月份<input v-model="month" type="month" @change="load"/></label></div><div class="grid two"><article v-for="key in usage?.keys" :key="key.id" class="panel"><h2>{{key.name}}</h2><CreditDonut :segments="key.models.map(model=>({name:model.name,value:Number(model.credits)})).concat([{name:'未消耗',value:Math.max(0,Number(key.quota)-Number(key.used))}])" :total="Number(key.quota)" :label="`${key.name} credit 用量`"/><p class="muted">额度 {{key.quota}} · 已用 {{key.used}}</p><div v-for="model in key.models" :key="model.id" class="list-row"><span>{{model.name}}</span><strong>{{model.credits}}</strong></div><p v-if="key.overage!=='0.000'" class="error">超额 {{key.overage}} credits</p></article></div></section></template>
