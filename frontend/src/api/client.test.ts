import { afterEach, describe, expect, it, vi } from 'vitest'
import { APIError, apiErrorMessage, networkActivity, request, token } from './client'
import { authAPI } from './auth'

describe('API client', () => {
  afterEach(() => { vi.unstubAllGlobals(); token.set(null); networkActivity.reads = 0; networkActivity.writes = 0 })

  it('parses structured errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: 'MODEL_NOT_READY', message: '模型尚未就绪', details: { model_ids: ['m1'] } } }), { status: 409, headers: { 'Content-Type': 'application/json' } })))
    await expect(request('/api/v1/teacher/api-key-reviews/id/approve', { method: 'POST' })).rejects.toMatchObject({ code: 'MODEL_NOT_READY', status: 409 })
  })

  it('turns approval and validation errors into actionable Chinese messages', () => {
    expect(apiErrorMessage(new APIError('mentor is no longer a member of the project organization', 409, 'MENTOR_NOT_IN_ORGANIZATION'))).toBe('该导师未加入项目所属组织，请先在“组织管理”中分配导师')
    expect(apiErrorMessage(new APIError('mentor already belongs to the project', 409, 'MENTOR_ALREADY_PROJECT_MEMBER'))).toBe('该导师已经加入此项目，无需重复审批')
    expect(apiErrorMessage(new APIError('rejection reason is required', 400, 'REJECTION_REASON_REQUIRED'))).toBe('驳回原因不能为空，请填写具体原因')
    expect(apiErrorMessage(new APIError('request validation failed', 400, 'VALIDATION_ERROR', [{ field: 'body', reason: 'unexpected end of JSON input' }]))).toBe('body：请求内容不完整')
    expect(apiErrorMessage(new APIError('one or more models are not ready', 409, 'MODEL_NOT_READY', { model_ids: ['gpt-4.1', 'embedding-3'] }))).toContain('gpt-4.1、embedding-3')
    expect(apiErrorMessage(new APIError('json: unknown field "confirm"', 400, 'VALIDATION_ERROR', [{ field: 'body', reason: 'json: unknown field "confirm"' }]))).toContain('confirm')
    expect(apiErrorMessage(new APIError('email is already registered', 409, 'EMAIL_ALREADY_REGISTERED'))).toBe('该邮箱已注册，请直接登录')
    expect(apiErrorMessage(new APIError('dependency failure', 503, 'DEPENDENCY_UNAVAILABLE', { operation: 'api_key_approval', state_changed: false }, 'request-123'))).toBe('终审未完成，额度和申请状态均未变更。请联系管理员并提供请求编号 request-123')
  })

  it('registers with only the fields accepted by the server DTO', async () => {
    let body = ''
    vi.stubGlobal('fetch', vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => { body = String(init?.body ?? ''); return new Response(JSON.stringify({ data: {} }), { status: 201, headers: { 'Content-Type': 'application/json' } }) }))
    await authAPI.register('student', { name: '学生', email: 'student@example.com', password: 'a-secure-password', verification_code: '123456' })
    expect(JSON.parse(body)).toEqual({ name: '学生', email: 'student@example.com', password: 'a-secure-password', verification_code: '123456' })
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
