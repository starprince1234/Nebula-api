import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  test: { environment: 'jsdom' },
  server: { port: 5173, proxy: { '/api': 'http://127.0.0.1:8080', '/v1': 'http://127.0.0.1:8080' } },
  build: { sourcemap: false }
})
