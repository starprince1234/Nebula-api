<script setup lang="ts">
import { DialogClose, DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { X } from 'lucide-vue-next'
defineProps<{ open: boolean; title: string; wide?: boolean }>()
const emit = defineEmits<{ 'update:open': [value: boolean] }>()
</script>
<template><DialogRoot :open="open" @update:open="emit('update:open',$event)"><DialogPortal><DialogOverlay class="dialog-overlay"/><DialogContent class="dialog-content" :class="{wide}"><DialogTitle class="dialog-title">{{title}}</DialogTitle><DialogClose class="icon-button dialog-close" aria-label="关闭"><X :size="20"/></DialogClose><slot/></DialogContent></DialogPortal></DialogRoot></template>
