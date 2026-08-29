<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { mentorAPI } from '../../api/mentor'
import type { InputMonitorItem } from '../../api/types'
const rows = ref<InputMonitorItem[]>([])
const selected = ref<InputMonitorItem | null>(null)
async function load() { rows.value = (await mentorAPI.inputs()).items }
async function detail(id: string) { selected.value = await mentorAPI.input(id) }
onMounted(load)
</script>
<template><section class="page"><div class="page-heading"><h1>输入监控</h1><button class="button secondary" @click="load">刷新</button></div><section class="panel"><div v-for="row in rows" :key="row.call_id" class="list-row"><div><strong>{{row.user_name}} · {{row.project_name}}</strong><p class="muted">{{row.preview}}</p></div><button class="button ghost" @click="detail(row.call_id)">查看完整输入</button></div></section><div v-if="selected" class="panel"><h2>{{selected.model_name}} · {{selected.user_name}}</h2><pre>{{selected.content}}</pre><button class="button secondary" @click="selected=null">关闭</button></div></section></template>
