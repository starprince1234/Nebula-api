<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { mentorAPI } from '../api/mentor'
import type { Organization, Project, ProjectUsage } from '../api/types'
import { APIError } from '../api/client'

export type MentorResourceFilter = {
  project_id: string
  user_id: string
  api_key_id: string
  model_id: string
  status: string
}

const props = defineProps<{
  filter: MentorResourceFilter
  statusOptions: Array<{ value: string; label: string }>
}>()
const emit = defineEmits<{ error: [message: string] }>()
const organizations=ref<Organization[]>([]),projects=ref<Project[]>([]),usage=ref<ProjectUsage|null>(null),loading=ref(false)
const members=computed(()=>usage.value?.members??[])
const keys=computed(()=>members.value.filter(member=>!props.filter.user_id||member.user_id===props.filter.user_id).flatMap(member=>member.keys.map(key=>({...key,user_name:member.user_name}))))
const models=computed(()=>{const unique=new Map<string,string>();for(const key of keys.value){if(props.filter.api_key_id&&key.id!==props.filter.api_key_id)continue;for(const model of key.models)unique.set(model.id,model.name)}return[...unique].map(([id,name])=>({id,name})).sort((left,right)=>left.name.localeCompare(right.name))})

async function loadProjects(){try{organizations.value=await mentorAPI.organizations();const groups=await Promise.all(organizations.value.map(organization=>mentorAPI.projects(organization.id)));projects.value=groups.flat().filter(project=>project.is_responsible).sort((left,right)=>left.name.localeCompare(right.name))}catch(caught){emit('error',caught instanceof APIError?caught.message:'加载项目筛选项失败')}}
async function changeProject(){props.filter.user_id='';props.filter.api_key_id='';props.filter.model_id='';usage.value=null;if(!props.filter.project_id)return;loading.value=true;try{usage.value=await mentorAPI.usage(props.filter.project_id)}catch(caught){emit('error',caught instanceof APIError?caught.message:'加载项目筛选范围失败')}finally{loading.value=false}}
function changeMember(){props.filter.api_key_id='';props.filter.model_id=''}
function changeKey(){props.filter.model_id=''}
onMounted(loadProjects)
</script>

<template><div class="cascade-filter-grid"><label>项目<select v-model="filter.project_id" @change="changeProject"><option value="">全部负责项目</option><option v-for="project in projects" :key="project.id" :value="project.id">{{project.name}}</option></select></label><label>成员<select v-model="filter.user_id" :disabled="!filter.project_id||loading" @change="changeMember"><option value="">{{filter.project_id?'全部成员':'请先选择项目'}}</option><option v-for="member in members" :key="member.user_id" :value="member.user_id">{{member.user_name}}</option></select></label><label>API Key<select v-model="filter.api_key_id" :disabled="!filter.project_id||loading" @change="changeKey"><option value="">{{filter.project_id?'全部 Key':'请先选择项目'}}</option><option v-for="key in keys" :key="key.id" :value="key.id">{{key.name}} · {{key.user_name}}</option></select></label><label>模型<select v-model="filter.model_id" :disabled="!filter.project_id||loading"><option value="">{{filter.project_id?'全部模型':'请先选择项目'}}</option><option v-for="model in models" :key="model.id" :value="model.id">{{model.name}}</option></select></label><label>结果<select v-model="filter.status"><option value="">全部结果</option><option v-for="option in statusOptions" :key="option.value" :value="option.value">{{option.label}}</option></select></label></div></template>
