<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { mentorAPI } from '../../api/mentor'
import type { CallLog } from '../../api/types'
const rows = ref<CallLog[]>([])
async function load() { rows.value = (await mentorAPI.callLogs()).items }
onMounted(load)
</script>
<template><section class="page"><div class="page-heading"><h1>调用日志</h1><button class="button secondary" @click="load">刷新</button></div><section class="panel"><div v-for="row in rows" :key="row.id" class="list-row"><div><strong>{{row.user_name}} · {{row.api_key_name}}</strong><p class="muted">{{row.project_name}} · {{row.model_name}} · {{row.provider_name||'全部失败'}}</p></div><strong>{{row.credits}} credits</strong></div><div v-if="!rows.length" class="empty">暂无调用记录</div></section></section></template>
