<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { teacherAPI } from '../../api/teacher'
import type { ProjectUsage } from '../../api/types'
import CreditDonut from '../../components/CreditDonut.vue'
const rows = ref<ProjectUsage[]>([])
const month = ref(new Date().toISOString().slice(0, 7))
async function load() { rows.value = await teacherAPI.projectSpend(`month=${month.value}`) }
onMounted(load)
</script>
<template><section class="page"><div class="page-heading"><h1>项目花费</h1><label>月份<input v-model="month" type="month" @change="load"/></label></div><section class="panel"><div v-for="row in rows" :key="row.project_id" class="list-row"><div><strong>{{row.project_name}}</strong><p class="muted">额度 {{row.quota}} · 已用 {{row.charged}} credits</p></div><CreditDonut :segments="row.models.map(model=>({name:model.name,value:Number(model.credits)})).concat([{name:'未消耗',value:Math.max(0,Number(row.quota)-Number(row.charged))}])" :total="Number(row.quota)" :label="`${row.project_name} actual spend`"/><div><span v-for="model in row.models" :key="model.id">{{model.name}} {{model.credits}}；</span><span v-for="model in row.free_models" :key="`free-${model.id}`">{{model.name}} 免费 {{model.calls}} 次</span></div></div></section></section></template>
