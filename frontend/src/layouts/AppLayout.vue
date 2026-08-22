<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Menu, X, Wifi, WifiOff } from 'lucide-vue-next'
import { useAuthStore } from '../store/auth'
import { openEvents, type EventConnectionState } from '../api/client'
import { roleLabel } from '../utils/format'
import { studentAPI } from '../api/student'
import { mentorAPI } from '../api/mentor'
import { teacherAPI } from '../api/teacher'
import LoadingButton from '../components/LoadingButton.vue'
import { usePendingActions } from '../composables/usePendingActions'
const auth=useAuthStore(),route=useRoute(),router=useRouter();const open=ref(false),eventController=ref<AbortController|null>(null),connection=ref<EventConnectionState>('connecting'),failures=ref(0)
const actions=usePendingActions()
const pendingCount=ref(0)
const links=computed(()=>auth.user?.role==='student'?[{to:'/student/api-keys',label:'申请密钥',count:pendingCount.value},{to:'/student/models',label:'模型广场',count:0}]:auth.user?.role==='mentor'?[{to:'/mentor/reviews',label:'审核密钥',count:pendingCount.value},{to:'/mentor/projects',label:'项目管理',count:0}]:[{to:'/teacher/key-reviews',label:'审批密钥',count:pendingCount.value},{to:'/teacher/organizations',label:'组织管理',count:0},{to:'/teacher/projects',label:'项目管理',count:0},{to:'/teacher/providers',label:'供应商管理',count:0},{to:'/teacher/models',label:'模型管理',count:0}])
const workspace=computed(()=>auth.user?.role==='teacher'?'老师工作台':auth.user?.role==='mentor'?'导师工作台':'学生工作台')
async function refreshCount(){try{pendingCount.value=auth.user?.role==='student'?(await studentAPI.keys()).filter(k=>['pending_mentor','pending_teacher','approved'].includes(k.status)).length:auth.user?.role==='mentor'?(await mentorAPI.reviews()).length:(await teacherAPI.keyReviews()).length}catch{/* page surfaces request errors */}}
function focused(){void refreshCount();window.dispatchEvent(new CustomEvent('nebula:refresh',{detail:'window-focus'}))}
onMounted(()=>{void refreshCount();window.addEventListener('focus',focused);window.addEventListener('nebula:refresh',refreshCount);eventController.value=new AbortController();openEvents((event)=>{if(event==='api_key.status_changed'||event==='models.common_changed')window.dispatchEvent(new CustomEvent('nebula:refresh',{detail:event}))},(state,count)=>{connection.value=state;failures.value=count;if(state==='connected')window.dispatchEvent(new CustomEvent('nebula:refresh',{detail:'sse-restored'}))},eventController.value.signal)});onBeforeUnmount(()=>{eventController.value?.abort();window.removeEventListener('focus',focused);window.removeEventListener('nebula:refresh',refreshCount)})
async function logout(){await actions.run('logout',async()=>{await auth.logout();await router.replace('/login')})}
</script>
<template><div class="app-shell"><aside class="sidebar" :class="{open}"><div class="sidebar-head"><div class="brand-lockup"><span class="nebula-mark">N</span><span>Nebula</span></div><button class="icon-button mobile-only" aria-label="关闭导航" @click="open=false"><X :size="20"/></button></div><nav aria-label="主导航"><RouterLink v-for="link in links" :key="link.to" :to="link.to" @click="open=false"><span>{{link.label}}</span><strong v-if="link.count" class="nav-count">{{link.count}}</strong></RouterLink></nav><div class="sidebar-footer"><span class="connection"><Wifi v-if="connection==='connected'" :size="15"/><WifiOff v-else :size="15"/>{{connection==='connected'?'实时已连接':failures>=3?'实时连接中断':'正在连接'}}</span><span class="muted">{{roleLabel(auth.user?.role)}}</span></div></aside><Transition name="scrim"><div v-if="open" class="drawer-scrim mobile-only" @click="open=false"/></Transition><main class="app-main"><header class="app-header"><button class="icon-button mobile-only" aria-label="打开导航" @click="open=true"><Menu :size="21"/></button><div><p class="eyebrow">{{workspace}}</p><h1>{{String(route.meta.title||'Nebula')}}</h1></div><div class="account"><div><strong>{{auth.user?.name}}</strong><small>{{auth.user?.email}} · {{roleLabel(auth.user?.role)}}</small></div><LoadingButton class="button ghost" :pending="actions.pending('logout')" pending-label="退出中…" @click="logout">退出登录</LoadingButton></div></header><div class="content"><RouterView v-slot="{Component,route:childRoute}"><Transition name="page-route" mode="out-in"><component :is="Component" :key="String(childRoute.name||childRoute.path)"/></Transition></RouterView></div></main></div></template>
