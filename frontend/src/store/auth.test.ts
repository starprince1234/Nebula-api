import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAuthStore } from './auth'
import { authAPI } from '../api/auth'
import { token } from '../api/client'

vi.mock('../api/auth', () => ({
  authAPI: {
    refresh: vi.fn(),
  },
}))

describe('auth store bootstrap', () => {
  afterEach(() => {
    token.set(null)
    vi.clearAllMocks()
  })

  it('shares one refresh rotation across concurrent bootstrap callers', async () => {
    setActivePinia(createPinia())
    const auth = useAuthStore()
    let release!: () => void
    vi.mocked(authAPI.refresh).mockReturnValue(new Promise(resolve => {
      release = () => resolve({ access_token: 'fresh', token_type: 'Bearer', expires_in: 3600, user: { id: 'u1', name: 'Teacher', email: 'teacher@example.com', role: 'teacher', status: 'active', created_at: '2026-01-01T00:00:00Z' } })
    }))

    const first = auth.bootstrap()
    const second = auth.bootstrap()

    expect(authAPI.refresh).toHaveBeenCalledTimes(1)
    release()
    await Promise.all([first, second])
    expect(auth.isAuthenticated).toBe(true)
    expect(auth.ready).toBe(true)
  })
})
