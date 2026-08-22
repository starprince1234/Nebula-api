import { computed, ref } from 'vue'

export function useLoadState() {
  const pendingCount = ref(0)
  const settled = ref(false)
  const loading = computed(() => pendingCount.value > 0)
  const initialLoading = computed(() => loading.value && !settled.value)
  const refreshing = computed(() => loading.value && settled.value)

  async function run<T>(operation: () => Promise<T>): Promise<T> {
    pendingCount.value += 1
    try {
      return await operation()
    } finally {
      pendingCount.value -= 1
      settled.value = true
    }
  }

  return { settled, loading, initialLoading, refreshing, run }
}
