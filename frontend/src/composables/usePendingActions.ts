import { computed, reactive } from 'vue'

export function usePendingActions() {
  const flights = new Map<string, Promise<unknown>>()
  const keys = reactive(new Set<string>())
  const anyPending = computed(() => keys.size > 0)

  function pending(key: string) { return keys.has(key) }

  function run<T>(key: string, operation: () => Promise<T>): Promise<T> {
    const existing = flights.get(key) as Promise<T> | undefined
    if (existing) return existing

    keys.add(key)
    const flight = operation().finally(() => {
      flights.delete(key)
      keys.delete(key)
    })
    flights.set(key, flight)
    return flight
  }

  return { anyPending, pending, run }
}
