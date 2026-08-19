<script setup lang="ts">
import { Check } from 'lucide-vue-next'
import type { ProgressCurrent, ProgressStep } from '../api/types'
const props = defineProps<{ current: ProgressCurrent; completed: ProgressStep[] }>()
const steps: Array<{ key: ProgressStep; label: string }> = [{ key: 'submitted', label: '已提交' }, { key: 'mentor_review', label: '导师审核' }, { key: 'teacher_review', label: '老师审核' }, { key: 'claimed', label: '领取生效' }]
const currentMap: Record<ProgressCurrent, ProgressStep> = { mentor_review: 'mentor_review', teacher_review: 'teacher_review', claim: 'claimed', active: 'claimed', rejected: 'mentor_review', rejected_teacher: 'teacher_review', revoked: 'claimed' }
const isDone = (key: ProgressStep) => props.completed.includes(key)
const isCurrent = (key: ProgressStep) => currentMap[props.current] === key && !isDone(key)
</script>
<template><ol class="status-progress" :aria-label="`当前进度：${current}`"><li v-for="step in steps" :key="step.key" :class="{done:isDone(step.key),current:isCurrent(step.key),terminal:['rejected','rejected_teacher','revoked'].includes(current)&&isCurrent(step.key)}"><span class="progress-dot"><Check v-if="isDone(step.key)" :size="13"/><i v-else/></span><small>{{step.label}}</small></li></ol></template>
