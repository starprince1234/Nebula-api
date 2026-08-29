<script setup lang="ts">
import { ref } from 'vue'
import type { ModelCategory, ModelStatus } from '../api/types'

export type ModelFormValue = {
  model_id: string
  display_name: string
  description: string | null
  category: ModelCategory
  capabilities: string[]
  input_modalities: string[]
  output_modalities: string[]
  context_window: number | null
  max_output_tokens: number | null
  is_common?: boolean
  status?: ModelStatus
  credit_multiplier?: string | null
}

const props = withDefaults(defineProps<{ model: ModelFormValue; readonlyModelID?: boolean; showCommon?: boolean; submitLabel?: string }>(), {
  readonlyModelID: false,
  showCommon: true,
  submitLabel: '保存基础配置',
})
defineEmits<{ submit: [] }>()
const capability = ref('')

function addCapability() {
  const value = capability.value.trim()
  if (value && !props.model.capabilities.some(item => item.toLowerCase() === value.toLowerCase())) props.model.capabilities.push(value)
  capability.value = ''
}
</script>

<template>
  <form class="model-form" @submit.prevent="$emit('submit')">
    <label>模型 ID<input v-model="model.model_id" :readonly="readonlyModelID" required></label>
    <div class="grid two"><label>显示名称<input v-model="model.display_name" required></label><label>类别<select v-model="model.category"><option v-for="value in ['text','image','audio','video','multimodal','embedding','rerank']" :key="value">{{ value }}</option></select></label></div>
    <label>描述<textarea v-model="model.description" rows="3" /></label>
    <label>能力<div class="row"><input v-model="capability" @keydown.enter.prevent="addCapability"><button type="button" class="button secondary" @click="addCapability">添加</button></div></label>
    <div class="chip-list"><span v-for="value in model.capabilities" :key="value" class="chip">{{ value }}</span></div>
    <div class="grid two"><fieldset class="modality-fieldset"><legend>输入模态</legend><div class="checkbox-list"><label v-for="value in ['text','image','audio','video']" :key="value" class="checkbox-option"><input v-model="model.input_modalities" type="checkbox" :value="value"><span>{{ value }}</span></label></div></fieldset><fieldset class="modality-fieldset"><legend>输出模态</legend><div class="checkbox-list"><label v-for="value in ['text','image','audio','video']" :key="value" class="checkbox-option"><input v-model="model.output_modalities" type="checkbox" :value="value"><span>{{ value }}</span></label></div></fieldset></div>
    <div class="grid two"><label>上下文长度<input v-model.number="model.context_window" type="number" min="1"></label><label>最大输出 Token<input v-model.number="model.max_output_tokens" type="number" min="1"></label></div>
    <label v-if="showCommon" class="checkbox-option common-model-option"><input v-model="model.is_common" type="checkbox"><span>全局常用模型</span></label>
    <slot name="before-submit" />
    <div class="actions"><button class="button primary">{{ submitLabel }}</button></div>
  </form>
</template>
