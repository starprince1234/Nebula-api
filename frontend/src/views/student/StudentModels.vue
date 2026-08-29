<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { studentAPI } from '../../api/student'
import type { Model, ModelCategory, ModelStatus } from '../../api/types'
import StatusBadge from '../../components/StatusBadge.vue'
import EmptyState from '../../components/EmptyState.vue'
import AppDialog from '../../components/AppDialog.vue'
import LoadingRegion from '../../components/LoadingRegion.vue'
import { useLoadState } from '../../composables/useLoadState'
import { APIError } from '../../api/client'

const rows=ref<Model[]>([]),query=ref(''),category=ref<ModelCategory|''>(''),status=ref<ModelStatus|''>(''),expanded=ref(''),tipOpen=ref(false),error=ref(''),loadState=useLoadState()
const filtered=computed(()=>rows.value.filter(model=>(!query.value||`${model.display_name} ${model.model_id} ${model.capabilities.join(' ')}`.toLowerCase().includes(query.value.toLowerCase()))&&(!category.value||model.category===category.value)&&(!status.value||model.status===status.value)))
async function load(){await loadState.run(async()=>{try{rows.value=await studentAPI.models();error.value=''}catch(caught){error.value=caught instanceof APIError?caught.message:'加载模型失败'}})}
function refresh(){void load()}
onMounted(()=>{void load();window.addEventListener('nebula:refresh',refresh)})
onBeforeUnmount(()=>window.removeEventListener('nebula:refresh',refresh))
</script>

<template><section class="page"><div class="page-heading"><div><p class="eyebrow">STUDENT</p><h1>模型广场</h1><p class="muted">倍率表示每次计费操作消耗的 credits。</p></div><button class="button primary" @click="tipOpen=true">申请新模型贴士</button></div><p v-if="error" class="error banner">{{error}}</p><section class="panel"><div class="grid two"><label>搜索<input v-model="query" placeholder="名称、模型 ID 或能力"></label><div class="row"><label>类别<select v-model="category"><option value="">全部类别</option><option v-for="value in ['text','image','audio','video','multimodal','embedding','rerank']" :key="value">{{value}}</option></select></label><label>状态<select v-model="status"><option value="">全部状态</option><option value="active">已启用</option><option value="inactive">已停用</option><option value="pending_configuration">待配置</option></select></label></div></div></section><LoadingRegion :initial-loading="loadState.initialLoading.value" :refreshing="loadState.refreshing.value" variant="cards" label="正在加载模型" style="margin-top:20px"><div v-if="filtered.length" class="grid two"><article v-for="model in filtered" :key="model.id" class="panel is-interactive"><div class="row"><div><strong>{{model.display_name}}</strong><p class="muted">{{model.model_id}} · {{model.category}}</p></div><StatusBadge :status="model.status"/></div><div class="chip-list"><span class="chip">{{model.credit_multiplier??'未配置'}}x</span></div><p class="muted">{{model.description||'暂无模型说明'}}</p><button class="button ghost" @click="expanded=expanded===model.id?'':model.id">{{expanded===model.id?'收起':'查看详情'}}</button><Transition name="content-fade"><dl v-if="expanded===model.id"><dt>计费倍率</dt><dd>{{model.credit_multiplier??'未配置'}}x / 次</dd><dt>能力</dt><dd>{{model.capabilities.join('、')||'—'}}</dd><dt>输入 / 输出</dt><dd>{{model.input_modalities.join('、')}} / {{model.output_modalities.join('、')}}</dd><dt>上下文 / 最大输出</dt><dd>{{model.context_window??'未提供'}} / {{model.max_output_tokens??'未提供'}}</dd></dl></Transition></article></div><EmptyState v-else title="没有匹配的模型" description="调整搜索或筛选条件。"/></LoadingRegion><AppDialog :open="tipOpen" title="申请新模型贴士" @update:open="tipOpen=$event"><p>申请新模型应在申请密钥阶段添加，待老师配置倍率和路由后才能通过终审。</p></AppDialog></section></template>
