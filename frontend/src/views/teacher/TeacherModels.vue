<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { teacherAPI } from '../../api/teacher'
import type { Binding, BindingAdapter, Model, ModelCategory, ModelStatus, Provider } from '../../api/types'
import { APIError } from '../../api/client'
import StatusBadge from '../../components/StatusBadge.vue'
import AppDialog from '../../components/AppDialog.vue'
import ModelForm from '../../components/ModelForm.vue'
import LoadingRegion from '../../components/LoadingRegion.vue'
import { useLoadState } from '../../composables/useLoadState'
import { useToast } from '../../composables/useToast'
import { modelCardStatus } from '../../utils/models'

const adapterOptions: Array<{ value: BindingAdapter; label: string; description: string }> = [
  { value: 'openai_compatible', label: 'OpenAI Chat / Completions', description: '用于 /v1/chat/completions 与 /v1/completions' },
  { value: 'openai_responses', label: 'OpenAI Responses', description: '用于 HTTP/WebSocket /v1/responses 与 /v1/responses/compact，支持 Codex CLI' },
  { value: 'openai_embeddings', label: 'OpenAI Embeddings', description: '用于 /v1/embeddings' },
  { value: 'openai_images', label: 'OpenAI Images', description: '用于 /v1/images/generations、edits 与 variations' },
  { value: 'openai_audio', label: 'OpenAI Audio', description: '用于 /v1/audio/transcriptions、translations 与 speech' },
  { value: 'openai_video', label: 'OpenAI Videos', description: '用于 /v1/videos 的创建、查询、内容下载与 remix' },
  { value: 'openai_realtime', label: 'OpenAI Realtime', description: '用于 /v1/realtime WebSocket' },
  { value: 'openai_moderations', label: 'OpenAI Moderations', description: '用于 /v1/moderations' },
  { value: 'anthropic', label: 'Anthropic Messages', description: '用于 /v1/messages' },
  { value: 'cohere_rerank_v2', label: 'Cohere Rerank v2', description: '用于官方 /v2/rerank' },
  { value: 'google_gemini_v1beta', label: 'Google Gemini v1beta', description: '用于原生 generateContent、streamGenerateContent、embedContent 与 batchEmbedContents' },
]

const rows = ref<Model[]>([])
const providers = ref<Provider[]>([])
const tab = ref<'common' | 'all'>('common')
const query = ref('')
const category = ref('')
const status = ref('')
const selected = ref<{ model: Model; bindings: Binding[] } | null>(null)
const createOpen = ref(false)
const bindingOpen = ref(false)
const editingBinding = ref<Binding | null>(null)
const error = ref('')
const routingModels = ref<string[]>([])
const toast = useToast()
const route = useRoute()
const router = useRouter()
const loadState = useLoadState()

const filtered = computed(() => rows.value
  .filter(model => (tab.value === 'all' || model.is_common)
    && (!query.value || `${model.display_name} ${model.model_id}`.toLowerCase().includes(query.value.toLowerCase()))
    && (!category.value || model.category === category.value)
    && (!status.value || model.status === status.value))
  .sort((left, right) => left.display_name.localeCompare(right.display_name)))
const pending = computed(() => rows.value.filter(model => modelCardStatus(model) === 'pending_configuration').length)
const modelForm = reactive({ model_id: '', display_name: '', description: '', category: 'text' as ModelCategory, capabilities: [] as string[], input_modalities: [] as string[], output_modalities: [] as string[], context_window: null as number | null, max_output_tokens: null as number | null, is_common: false, status: 'pending_configuration' as ModelStatus, credit_multiplier: null as string | null })
const multiplierReason = ref('')
const bindingForm = reactive({ provider_id: '', upstream_model_name: '', adapter: 'openai_compatible' as BindingAdapter, priority: 100, status: 'active' as 'active' | 'inactive' })
const adapterDescription = computed(() => adapterOptions.find(option => option.value === bindingForm.adapter)?.description ?? '')

async function load() {
  await loadState.run(async () => {
    try {
      [rows.value, providers.value] = await Promise.all([teacherAPI.models(), teacherAPI.providers()])
      const target = String(route.query.model || '')
      if (target) {
        const row = rows.value.find(model => model.id === target)
        if (row) await open(row)
      }
    } catch (caught) { error.value = msg(caught) }
  })
}

async function open(model: Model) {
  try {
    selected.value = await teacherAPI.model(model.id)
    Object.assign(modelForm, selected.value.model)
    multiplierReason.value = ''
    await router.replace({ query: { model: model.id } })
  } catch (caught) { error.value = msg(caught) }
}

function close() { selected.value = null; void router.replace({ query: {} }) }
async function saveModel() {
  try {
    if (selected.value) {
      const { model_id, ...body } = modelForm
      void model_id
      const changed = body.credit_multiplier !== selected.value.model.credit_multiplier
      if (changed && !multiplierReason.value.trim()) { error.value = '修改倍率必须填写变更原因'; return }
      const updateBody = { ...body, ...(changed ? { multiplier_change_reason: multiplierReason.value.trim() } : {}) }
      if (updateBody.credit_multiplier === null) delete updateBody.credit_multiplier
      selected.value.model = await teacherAPI.updateModel(selected.value.model.id, updateBody)
      toast.success('模型配置已更新')
    } else {
      await teacherAPI.createModel({ model_id: modelForm.model_id, display_name: modelForm.display_name, description: modelForm.description, category: modelForm.category, capabilities: modelForm.capabilities, input_modalities: modelForm.input_modalities, output_modalities: modelForm.output_modalities, is_common: modelForm.is_common, status: 'pending_configuration', ...(modelForm.credit_multiplier === null ? {} : { credit_multiplier: modelForm.credit_multiplier }), ...(modelForm.context_window === null ? {} : { context_window: modelForm.context_window }), ...(modelForm.max_output_tokens === null ? {} : { max_output_tokens: modelForm.max_output_tokens }) })
      createOpen.value = false
      toast.success('模型已创建，状态为待配置')
    }
    await load()
  } catch (caught) { handle(caught) }
}

function openBinding(binding?: Binding) {
  editingBinding.value = binding ?? null
  Object.assign(bindingForm, { provider_id: binding?.provider_id ?? '', upstream_model_name: binding?.upstream_model_name ?? '', adapter: binding?.adapter ?? 'openai_compatible', priority: binding?.priority ?? 100, status: binding?.status ?? 'active' })
  bindingOpen.value = true
}

async function saveBinding() {
  if (!selected.value) return
  try {
    if (editingBinding.value) await teacherAPI.updateBinding(editingBinding.value.id, { upstream_model_name: bindingForm.upstream_model_name, adapter: bindingForm.adapter, priority: bindingForm.priority, status: bindingForm.status })
    else await teacherAPI.createBinding(selected.value.model.id, bindingForm)
    selected.value = await teacherAPI.model(selected.value.model.id)
    bindingOpen.value = false
    toast.success('Binding 已保存')
    await load()
  } catch (caught) { handle(caught) }
}

async function toggle(model: Model) {
  if (model.status === 'active' && !confirm('停用模型会影响使用该模型的 Key，是否继续？')) return
  try { await teacherAPI.updateModel(model.id, { status: model.status === 'active' ? 'inactive' : 'active' }); await load() } catch (caught) { handle(caught) }
}

function handle(caught: unknown) {
  if (caught instanceof APIError && caught.code === 'MODEL_ROUTING_REQUIRED' && !Array.isArray(caught.details)) {
    const details = caught.details as { model_ids?: string[] } | undefined
    routingModels.value = details?.model_ids ?? []
  } else error.value = msg(caught)
}
function msg(caught: unknown) { return caught instanceof APIError ? caught.message : '请求失败' }
onMounted(load)
</script>

<template>
  <section class="page">
    <div class="page-heading"><div><p class="eyebrow">TEACHER</p><h1>模型管理</h1><p class="muted">配置模型元数据、计费倍率、常用标记和供应商 Binding。</p></div><button class="button primary" @click="createOpen=true;Object.assign(modelForm,{model_id:'',display_name:'',description:'',category:'text',capabilities:[],input_modalities:[],output_modalities:[],context_window:null,max_output_tokens:null,is_common:false,status:'pending_configuration',credit_multiplier:null});multiplierReason=''">新增模型</button></div>
    <p v-if="error" class="error banner">{{ error }}</p>
    <section class="panel"><div class="segmented"><button :class="{active:tab==='common'}" @click="tab='common'">常用模型</button><button :class="{active:tab==='all'}" @click="tab='all'">所有模型 <span v-if="pending">{{ pending }}</span></button></div><div class="grid two"><label>搜索<input v-model="query" placeholder="名称或 model_id"></label><div class="row"><label>类别<select v-model="category"><option value="">全部</option><option v-for="value in ['text','image','audio','video','multimodal','embedding','rerank']" :key="value">{{ value }}</option></select></label><label>状态<select v-model="status"><option value="">全部</option><option value="pending_configuration">待配置</option><option value="active">启用</option><option value="inactive">停用</option></select></label></div></div></section>
    <LoadingRegion :initial-loading="loadState.initialLoading.value" :refreshing="loadState.refreshing.value" variant="cards" label="正在加载模型"><div class="grid two" style="margin-top:20px"><article v-for="model in filtered" :key="model.id" class="panel"><div class="row"><div><strong>{{ model.display_name }}</strong><p class="muted">{{ model.model_id }} · {{ model.category }}</p></div><StatusBadge :status="modelCardStatus(model)" /></div><p class="muted">{{ model.description || '暂无说明' }}</p><div class="chip-list"><span class="chip">{{ model.credit_multiplier ?? '未配置' }}x</span><span v-if="model.is_common" class="chip">常用</span></div><div class="actions"><button class="button ghost" @click="open(model)">配置</button><button class="button" :class="model.status==='active'?'danger':'secondary'" :disabled="model.status!=='active'&&!model.route_ready" @click="toggle(model)">{{ model.status === 'active' ? '停用' : '启用' }}</button></div></article></div></LoadingRegion>
    <AppDialog :open="Boolean(selected)||createOpen" :title="selected?selected.model.display_name:'新增模型'" wide @update:open="$event?null:(createOpen=false,close())"><ModelForm :model="modelForm" :readonly-model-id="Boolean(selected)" @submit="saveModel"><template #before-submit><label>计费倍率（每次调用 credits）<input v-model="modelForm.credit_multiplier" inputmode="decimal" :required="modelForm.status==='active'"></label><label v-if="selected&&modelForm.credit_multiplier!==selected.model.credit_multiplier">倍率变更原因<textarea v-model="multiplierReason" rows="3" required></textarea></label></template></ModelForm><template v-if="selected"><hr><div class="row"><h2>Bindings</h2><button class="button secondary" @click="openBinding()">新增 Binding</button></div><article v-for="binding in selected.bindings" :key="binding.id" class="list-row"><div><strong>{{ binding.upstream_model_name }}</strong><p class="muted">{{ adapterOptions.find(option=>option.value===binding.adapter)?.label || binding.adapter }} · 优先级 {{ binding.priority }}</p></div><div class="actions"><StatusBadge :status="binding.status" /><button class="button ghost" @click="openBinding(binding)">编辑</button></div></article></template></AppDialog>
    <AppDialog :open="bindingOpen" :title="editingBinding?'编辑 Binding':'新增 Binding'" @update:open="bindingOpen=$event"><form @submit.prevent="saveBinding"><label v-if="!editingBinding">供应商<select v-model="bindingForm.provider_id" required><option value="">请选择</option><option v-for="provider in providers" :key="provider.id" :value="provider.id">{{ provider.name }}</option></select></label><label>上游模型名<input v-model="bindingForm.upstream_model_name" required></label><label>Adapter<select v-model="bindingForm.adapter"><option v-for="option in adapterOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select><small class="muted">{{ adapterDescription }}</small></label><label>优先级<input v-model.number="bindingForm.priority" type="number" min="0" required></label><label>状态<select v-model="bindingForm.status"><option value="active">启用</option><option value="inactive">停用</option></select></label><div class="actions"><button class="button primary">保存</button></div></form></AppDialog>
    <AppDialog :open="Boolean(routingModels.length)" title="路由配置不足" @update:open="routingModels=$event?routingModels:[]"><p>以下 ACTIVE 模型会失去最后一条可用路由：</p><div class="chip-list"><span v-for="id in routingModels" :key="id" class="chip">{{ id }}</span></div></AppDialog>
  </section>
</template>
