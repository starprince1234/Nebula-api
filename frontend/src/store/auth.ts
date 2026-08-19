import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { authAPI } from '../api/auth'
import { token } from '../api/client'
import type { Role, User } from '../api/types'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const ready = ref(false)
  const loading = ref(false)
  const isAuthenticated = computed(() => Boolean(user.value && token.get()))
  async function bootstrap() {
    if (ready.value) return
    try { const session = await authAPI.refresh(); token.set(session.access_token); user.value = session.user } catch { token.set(null); user.value = null } finally { ready.value = true }
  }
  async function login(email: string, password: string) { loading.value = true; try { const session = await authAPI.login({ email, password }); token.set(session.access_token); user.value = session.user } finally { loading.value = false } }
  async function logout() { try { await authAPI.logout() } finally { token.set(null); user.value = null } }
  async function loadMe() { user.value = await authAPI.me() }
  function hasRole(role?: Role | Role[]) { if (!role || !user.value) return true; return Array.isArray(role) ? role.includes(user.value.role) : user.value.role === role }
  return { user, ready, loading, isAuthenticated, bootstrap, login, logout, loadMe, hasRole }
})
