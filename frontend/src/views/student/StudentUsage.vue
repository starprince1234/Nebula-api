<script setup lang="ts">
import { ref } from 'vue'
import { studentAPI } from '../../api/student'
import type { StudentUsage } from '../../api/types'
import { APIError } from '../../api/client'
import CreditDonut from '../../components/CreditDonut.vue'
import EmptyState from '../../components/EmptyState.vue'
import { useVisibilityPolling } from '../../composables/useVisibilityPolling'

const usage = ref<StudentUsage | null>(null)
const month = ref(new Date().toISOString().slice(0, 7))
const error = ref('')
async function load() {
  try { usage.value = await studentAPI.usage(month.value); error.value = '' }
  catch (caught) { error.value = caught instanceof APIError ? caught.message : '加载个人用量失败' }
}
const { refresh } = useVisibilityPolling(load)
function percent(used: string, quota: string) { return Number(quota) === 0 ? null : Number(used) / Number(quota) * 100 }
function percentLabel(used: string, quota: string) { const value=percent(used,quota);return value===null?'N/A':`${value.toFixed(1)}%` }
function remaining(quota:string,used:string,pending:string){return Math.max(0,Number(quota)-Number(used)-Number(pending)).toFixed(3)}
</script>

<template>
  <section class="page">
    <div class="page-heading"><div><p class="eyebrow">STUDENT</p><h1>个人用量</h1><p class="muted">每张图对应一个 API Key，按模型展示本月已消耗 credits。</p></div><label>月份<input v-model="month" type="month" @change="refresh"></label></div>
    <p v-if="error" class="error banner">{{ error }}</p>
    <div v-if="usage?.keys.length" class="grid two">
      <article v-for="key in usage.keys" :key="key.id" class="panel">
        <div class="row"><div><h2>{{ key.name }}</h2><p class="muted">{{ key.status }} · 月额度 {{ key.quota }}</p></div><strong>{{ percentLabel(key.used,key.quota) }}</strong></div>
        <div class="usage-chart-card"><CreditDonut :segments="key.models.map(model=>({name:model.name,value:Number(model.credits)})).concat([{name:'剩余额度',value:Number(remaining(key.quota,key.used,key.pending)),color:'#dfe3ec'}])" :total="Number(key.quota)" :label="`${key.name} 按模型拆分的本月 credit 用量`"/></div>
        <div class="usage-summary"><div class="usage-stat">已用<strong>{{ key.used }}</strong></div><div class="usage-stat">在途<strong>{{ key.pending }}</strong></div><div class="usage-stat">剩余<strong>{{ remaining(key.quota,key.used,key.pending) }}</strong></div></div>
        <table class="usage-table"><thead><tr><th>模型</th><th>credits</th><th>调用次数</th></tr></thead><tbody><tr v-for="model in key.models" :key="model.id"><td>{{model.name}}</td><td>{{model.credits}}</td><td>{{model.calls ?? 0}}</td></tr><tr v-if="!key.models.length"><td colspan="3">本月尚无调用</td></tr></tbody></table>
        <p v-if="key.overage!=='0.000'" class="error">已超额 {{key.overage}} credits；图表填充已封顶，实际百分比保留。</p>
      </article>
    </div>
    <EmptyState v-else title="本月暂无可展示的 Key" description="已审批/生效 Key，以及本月产生过用量的已撤销 Key 会显示在这里。"/>
  </section>
</template>
