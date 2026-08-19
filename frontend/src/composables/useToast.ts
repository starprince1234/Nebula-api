import { reactive } from 'vue'
export type ToastItem = { id: number; message: string; tone: 'success' | 'error' }
const items = reactive<ToastItem[]>([]); let sequence = 0
function push(message: string, tone: ToastItem['tone']) { const id = ++sequence; items.push({ id, message, tone }); window.setTimeout(() => dismiss(id), tone === 'success' ? 3000 : 6000) }
function dismiss(id: number) { const index = items.findIndex(item => item.id === id); if (index >= 0) items.splice(index, 1) }
export function useToast() { return { items, success: (message: string) => push(message, 'success'), error: (message: string) => push(message, 'error'), dismiss } }
