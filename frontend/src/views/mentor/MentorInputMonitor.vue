<script setup lang="ts">
import { reactive, ref } from 'vue'
import { mentorAPI } from '../../api/mentor'
import type { InputMonitorItem } from '../../api/types'
import { APIError } from '../../api/client'
import AppDialog from '../../components/AppDialog.vue'
import EmptyState from '../../components/EmptyState.vue'
import MentorUsageFilters, { type MentorResourceFilter } from '../../components/MentorUsageFilters.vue'
import { useVisibilityPolling } from '../../composables/useVisibilityPolling'

const rows=ref<InputMonitorItem[]>([]),selected=ref<InputMonitorItem|null>(null),nextCursor=ref<string|null>(null),error=ref('')
const filter=reactive<MentorResourceFilter&{q:string;start:string;end:string}>({q:'',project_id:'',user_id:'',api_key_id:'',model_id:'',status:'',start:'',end:''})
const statusOptions=[{value:'succeeded',label:'成功'},{value:'outcome_unknown',label:'结果未知'}]
function query(cursor=''){const entries:Array<[string,string]>=Object.entries(filter).filter(([,value])=>value).map(([key,value])=>[key,(key==='start'||key==='end')?new Date(value).toISOString():value]);const params=new URLSearchParams(entries);if(cursor)params.set('cursor',cursor);return params.toString()}
async function load(append=false){try{const page=await mentorAPI.inputs(query(append?nextCursor.value??'':''));rows.value=append?rows.value.concat(page.items):page.items;nextCursor.value=page.next_cursor??null;error.value=''}catch(caught){error.value=caught instanceof APIError?caught.message:'加载输入监控失败'}}
async function detail(id:string){try{selected.value=await mentorAPI.input(id)}catch(caught){error.value=caught instanceof APIError?caught.message:'加载完整输入失败'}}
useVisibilityPolling(()=>load())
</script>

<template><section class="page"><div class="page-heading"><div><p class="eyebrow">MENTOR · SENSITIVE</p><h1>输入监控</h1><p class="muted">按项目、成员、Key、模型从左到右缩小范围；列表与全文访问都会写入不可变审计。</p></div><button class="button secondary" @click="load()">刷新</button></div><p v-if="error" class="error banner">{{error}}</p><section class="panel"><MentorUsageFilters :filter="filter" :status-options="statusOptions" @error="error=$event"/><div class="filter-grid"><label>关键词（至少 3 字符）<input v-model="filter.q" minlength="3"></label><label>开始时间<input v-model="filter.start" type="datetime-local"></label><label>结束时间<input v-model="filter.end" type="datetime-local"></label></div><button class="button primary" @click="load()">查询</button></section><section class="panel"><table v-if="rows.length" class="usage-table"><thead><tr><th>时间</th><th>用户</th><th>项目 / Key</th><th>模型 / 供应商</th><th>输入预览（Sensitive）</th><th></th></tr></thead><tbody><tr v-for="row in rows" :key="row.call_id"><td>{{new Date(row.created_at).toLocaleString()}}</td><td>{{row.user_name}}</td><td>{{row.project_name}}<br><span class="muted">{{row.api_key_name}}</span></td><td>{{row.model_name}}<br><span class="muted">{{row.provider_name||'未确定'}}</span></td><td>{{row.preview}}{{row.truncated?'…':''}}<br><span class="muted">{{row.content_bytes}} bytes · {{row.credits}} credits</span></td><td><button class="button ghost" @click="detail(row.call_id)">查看全文</button></td></tr></tbody></table><EmptyState v-else title="暂无可见文本输入" description="失败请求不会保留提示词；不支持文本提取的协议也不会生成监控记录。"/><button v-if="nextCursor" class="button secondary wide" @click="load(true)">加载更多</button></section><AppDialog :open="Boolean(selected)" title="完整输入 · Sensitive" wide @update:open="selected=$event?selected:null"><template v-if="selected"><p>{{selected.user_name}} · {{selected.project_name}} · {{selected.model_name}}</p><pre class="sensitive-text">{{selected.content}}</pre><p class="muted">{{selected.content_bytes}} bytes · {{new Date(selected.created_at).toLocaleString()}}</p></template></AppDialog></section></template>
