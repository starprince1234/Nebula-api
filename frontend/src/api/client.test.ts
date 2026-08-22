import { afterEach, describe, expect, it, vi } from 'vitest'
import { APIError, networkActivity, request, token } from './client'

describe('API client', () => {
  afterEach(() => { vi.unstubAllGlobals(); token.set(null); networkActivity.reads = 0; networkActivity.writes = 0 })

  it('parses structured errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: 'MODEL_NOT_READY', message: '模型尚未就绪', details: { model_ids: ['m1'] } } }), { status: 409, headers: { 'Content-Type': 'application/json' } })))
    await expect(request('/api/v1/teacher/api-key-reviews/id/approve', { method: 'POST' })).rejects.toMatchObject({ code: 'MODEL_NOT_READY', status: 409 })
  })

  it('shares one refresh request across concurrent 401 responses', async () => {
    token.set('expired')
    let refreshes = 0
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.includes('/auth/refresh')) { refreshes++; await Promise.resolve(); return new Response(JSON.stringify({ data: { access_token: 'fresh', user: {} } }), { status: 200, headers: { 'Content-Type': 'application/json' } }) }
      const auth = token.get()
      return auth === 'fresh' ? new Response(JSON.stringify({ data: { ok: true } }), { status: 200, headers: { 'Content-Type': 'application/json' } }) : new Response('{}', { status: 401, headers: { 'Content-Type': 'application/json' } })
    })
    vi.stubGlobal('fetch', fetchMock)
    await Promise.all([request<{ok:boolean}>('/api/v1/me'), request<{ok:boolean}>('/api/v1/me')])
    expect(refreshes).toBe(1)
  })

  it('tracks reads and always clears activity after failures', async () => {
    let release!: () => void
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(resolve => {
      release = () => resolve(new Response(JSON.stringify({ error: { code: 'FAILED', message: '失败' } }), { status: 500, headers: { 'Content-Type': 'application/json' } }))
    })))
    const pending = request('/api/v1/models')
    expect(networkActivity.reads).toBe(1)
    release()
    await expect(pending).rejects.toBeInstanceOf(APIError)
    expect(networkActivity.reads).toBe(0)
    expect(networkActivity.writes).toBe(0)
  })
})
