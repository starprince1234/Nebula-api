import { onBeforeUnmount, onMounted } from 'vue'

export function useVisibilityPolling(load: () => Promise<void>, interval = 30_000) {
  let timer: number | undefined
  let running = false
  async function run() {
    if (running || document.visibilityState !== 'visible') return
    running = true
    try { await load() } finally { running = false }
  }
  function restart() {
    if (timer !== undefined) window.clearInterval(timer)
    if (document.visibilityState === 'visible') {
      void run()
      timer = window.setInterval(run, interval)
    } else timer = undefined
  }
  onMounted(() => {
    restart()
    document.addEventListener('visibilitychange', restart)
    window.addEventListener('focus', restart)
  })
  onBeforeUnmount(() => {
    if (timer !== undefined) window.clearInterval(timer)
    document.removeEventListener('visibilitychange', restart)
    window.removeEventListener('focus', restart)
  })
  return { refresh: run }
}
