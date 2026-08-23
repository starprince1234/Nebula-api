import type { ApiError, ApiErrorDetails, Envelope, PageMeta } from './types'
import { reactive } from 'vue'

const base = import.meta.env.VITE_API_BASE_URL ?? ''
let accessToken: string | null = null
let refreshFlight: Promise<boolean> | null = null

export const networkActivity = reactive({ reads: 0, writes: 0 })

export const token = { get: () => accessToken, set: (value: string | null) => { accessToken = value } }

export class APIError extends Error {
  code: string
  details: ApiErrorDetails | undefined
  status: number
  requestId?: string
  constructor(message: string, status: number, code = 'REQUEST_FAILED', details?: ApiErrorDetails, requestId?: string) {
    super(message); this.status = status; this.code = code; this.details = details; this.requestId = requestId
    this.message = detailsMessage(code, details) ?? translateServerMessage(message) ?? errorMessages[code] ?? message
  }
}

const errorMessages: Record<string, string> = {
  REQUEST_FAILED: '请求失败，请稍后重试',
  VALIDATION_ERROR: '请求参数不正确，请检查填写内容后重试',
  REJECTION_REASON_REQUIRED: '驳回原因不能为空，请填写具体原因',
  AUTHENTICATION_REQUIRED: '登录状态已失效，请重新登录',
  INVALID_CREDENTIALS: '邮箱或密码不正确',
  TOKEN_EXPIRED: '登录状态已过期，请重新登录',
  FORBIDDEN: '当前账号没有执行此操作的权限',
  ACCOUNT_DISABLED: '账号已被停用，请联系老师',
  RESOURCE_NOT_FOUND: '请求的资源不存在，可能已被停用或删除',
  EMAIL_ALREADY_REGISTERED: '该邮箱已注册，请直接登录',
  RESOURCE_NAME_CONFLICT: '名称已存在，请更换后重试',
  INVALID_STATE_TRANSITION: '申请状态已发生变化，请刷新页面后重试',
  PROJECT_HAS_NO_MENTOR: '该项目尚未分配有效导师，暂时不能提交申请',
  MENTOR_NOT_IN_ORGANIZATION: '该导师未加入项目所属组织，请先在“组织管理”中分配导师',
  MENTOR_ALREADY_PROJECT_MEMBER: '该导师已经加入此项目，无需重复审批',
  MODEL_NOT_READY: '所选模型尚未完成可用配置',
  MODEL_ROUTING_REQUIRED: '操作会导致模型失去可用路由，请先保留至少一个有效 binding',
  KEY_ALREADY_CLAIMED: '该 API Key 已领取，不能重复领取',
  VERIFICATION_CODE_INVALID: '验证码不正确，请检查后重试',
  VERIFICATION_CODE_EXPIRED: '验证码已过期，请重新获取',
  INVITATION_INVALID: '邀请链接无效或已过期',
  RATE_LIMITED: '操作过于频繁，请稍后再试',
  VERIFICATION_LOCKED: '验证码失败次数过多，请稍后再试',
  DEPENDENCY_UNAVAILABLE: '服务暂时不可用，请稍后重试',
  INTERNAL_ERROR: '服务器内部错误，请稍后重试',
}

function validationReason(reason: string): string {
  const value = reason.trim()
  const unknownField = value.match(/unknown field ["']?([^"']+)["']?/i)
  if (unknownField) return `请求包含不支持的字段“${unknownField[1]}”，请刷新页面后重试`
  if (/required/i.test(value)) return '必填项不能为空'
  if (/invalid.*(email|format)|must be a valid email/i.test(value)) return '格式不正确，请按要求填写后重试'
  if (/password.*(length|between)|at least .*characters/i.test(value)) return '密码长度不符合要求，请使用 12–128 个字符'
  if (/invalid.*(code|verification)|verification.*invalid/i.test(value)) return '验证码不正确或已失效，请重新获取验证码'
  if (/unexpected end|unexpected eof|eof/i.test(value)) return '请求内容不完整'
  return value
}

function translateServerMessage(message: string): string | undefined {
  const value = message.trim().toLowerCase()
  const translations: Array<[RegExp, string]> = [
    [/invalid verification purpose/, '验证码用途不正确，请重新选择注册角色'],
    [/verification code was sent recently/, '验证码已发送，请稍后再试'],
    [/email is already registered/, '该邮箱已注册，请直接登录'],
    [/invalid email/, '邮箱格式不正确，请检查后重试'],
    [/invalid name/, '姓名不能为空且长度不能超过 64 个字符'],
    [/invalid or expired verification code/, '验证码不正确或已过期，请重新获取'],
    [/invalid verification code/, '验证码不正确，请检查后重试'],
    [/verification code expired/, '验证码已过期，请重新获取'],
    [/verification attempts are locked/, '验证码错误次数过多，请稍后再试'],
    [/invalid email or password/, '邮箱或密码不正确'],
    [/account is not active/, '账号当前不可用，请联系平台管理员'],
    [/refresh token is required|invalid refresh token/, '登录状态已失效，请重新登录'],
    [/invalid or expired invitation/, '邀请链接无效或已过期，请联系邀请人重新发送'],
    [/comment is too long/, '审核意见不能超过 1000 个字符'],
    [/invalid resource status/, '状态值不正确，请重新选择'],
    [/invalid model category/, '模型类别不正确，请重新选择'],
    [/invalid model status/, '模型状态不正确，请重新选择'],
    [/invalid binding status/, 'Binding 状态不正确，请重新选择'],
    [/priority must be non-negative/, '优先级不能为负数'],
    [/provider_id is required/, '请选择供应商'],
    [/invalid upstream_model_name/, '请填写有效的上游模型名称'],
    [/model description is too long/, '模型描述不能超过 2048 个字符'],
    [/requested model capabilities and modalities are required/, '请至少填写一项能力、输入模态和输出模态'],
    [/context_window must be positive/, '上下文长度必须为正整数'],
    [/max_output_tokens must be positive/, '最大输出 Token 必须为正整数'],
    [/new models require complete requested_models metadata/, '新模型信息不完整，请补齐模型卡片后再提交'],
    [/API key name is already in use/, 'Key 名称已存在，请更换名称'],
    [/project has no active mentor/, '该项目暂无有效导师，请等待导师加入并由老师批准'],
    [/organization not found/, '组织不存在或已停用'],
    [/project not found/, '项目不存在或已停用'],
    [/mentor does not belong to the project organization/, '该导师尚未加入项目所属组织，请先在“组织管理”中分配导师'],
    [/mentor already manages this project/, '该导师已经负责此项目，无需重复审批'],
    [/project application is already pending/, '该项目已有待审核申请，请勿重复提交'],
    [/project application is no longer pending|project application was reviewed concurrently/, '该申请已被其他操作处理，请刷新页面查看最新状态'],
    [/project not found/, '项目不存在或已停用'],
    [/organization not found/, '组织不存在或已停用'],
    [/invalid binding adapter/, '模型 Binding 类型不受支持'],
    [/model binding already exists/, '该模型已经配置了相同 Binding'],
    [/model ID already exists/, '该模型 ID 已存在，请直接配置已有模型'],
    [/provider name already exists/, '供应商名称已存在，请更换名称'],
    [/organization name already exists/, '组织名称已存在，请更换名称'],
    [/project name already exists in this organization/, '该组织下已存在同名项目，请更换名称'],
  ]
  return translations.find(([pattern]) => pattern.test(value))?.[1]
}

function detailsMessage(code: string, details: ApiErrorDetails | undefined): string | undefined {
  if (!details) return undefined
  if ((code === 'MODEL_NOT_READY' || code === 'MODEL_ROUTING_REQUIRED') && !Array.isArray(details) && Array.isArray(details.model_ids)) {
    const modelIds = details.model_ids.filter((value): value is string => typeof value === 'string')
    if (modelIds.length) return `${errorMessages[code]}：${modelIds.join('、')}`
  }
  if (code === 'VALIDATION_ERROR' && Array.isArray(details)) {
    const reasons = details.map(item => `${item.field || '请求'}：${validationReason(item.reason || '参数不合法')}`)
    if (reasons.length) return reasons.join('；')
  }
  return undefined
}

export function apiErrorMessage(error: unknown, fallback = '请求失败，请稍后重试'): string {
  if (!(error instanceof APIError)) return fallback
  return detailsMessage(error.code, error.details) ?? translateServerMessage(error.message) ?? errorMessages[error.code] ?? (error.message || fallback)
}

async function parseEnvelope<T>(response: Response): Promise<Envelope<T>> {
  if (response.status === 204) return { data: undefined as T }
  const body = await response.json().catch(() => ({} as ApiError)) as Envelope<T> & ApiError
  if (!response.ok) throw new APIError(body.error?.message ?? `请求失败 (${response.status})`, response.status, body.error?.code, body.error?.details, body.request_id)
  return body && typeof body === 'object' && 'data' in body ? body : { data: body as T }
}

async function refresh(): Promise<boolean> {
  if (!refreshFlight) {
    refreshFlight = fetch(`${base}/api/v1/auth/refresh`, { method: 'POST', credentials: 'include' })
      .then(async response => { if (!response.ok) return false; const envelope = await parseEnvelope<{ access_token: string }>(response); token.set(envelope.data.access_token); return true })
      .catch(() => false).finally(() => { refreshFlight = null })
  }
  return refreshFlight
}

async function rawRequest<T>(path: string, init: RequestInit = {}, retry = true): Promise<Envelope<T>> {
  const bucket = (init.method ?? 'GET').toUpperCase() === 'GET' ? 'reads' : 'writes'
  networkActivity[bucket] += 1
  try {
    const headers = new Headers(init.headers)
    if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
    if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`)
    const response = await fetch(`${base}${path}`, { ...init, headers, credentials: 'include' })
    if (response.status === 401 && retry && !path.includes('/auth/refresh') && await refresh()) return rawRequest<T>(path, init, false)
    return parseEnvelope<T>(response)
  } finally {
    networkActivity[bucket] -= 1
  }
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> { return (await rawRequest<T>(path, init)).data }
export async function page<T>(path: string): Promise<{ data: T; meta: PageMeta }> { const value = await rawRequest<T>(path); return { data: value.data, meta: value.meta ?? {} } }
export const get = <T>(path: string) => request<T>(path)
export const post = <T>(path: string, body?: unknown) => request<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) })
export const patch = <T>(path: string, body: unknown) => request<T>(path, { method: 'PATCH', body: JSON.stringify(body) })

export type EventConnectionState = 'connecting' | 'connected' | 'disconnected'
export const EVENT_RETRY_DELAYS = [1000, 2000, 5000, 10000, 30000] as const
const delay = (milliseconds: number, signal: AbortSignal) => new Promise<void>((resolve) => {
  const timer = window.setTimeout(resolve, milliseconds)
  signal.addEventListener('abort', () => { window.clearTimeout(timer); resolve() }, { once: true })
})

export function openEvents(
  onEvent: (event: string, data: unknown) => void,
  onState: (state: EventConnectionState, failures: number) => void,
  signal: AbortSignal,
): void {
  let lastEventId = ''
  const run = async () => {
    let failures = 0
    while (!signal.aborted) {
      onState('connecting', failures)
      try {
        const headers: Record<string, string> = {}
        if (accessToken) headers.Authorization = `Bearer ${accessToken}`
        if (lastEventId) headers['Last-Event-ID'] = lastEventId
        const response = await fetch(`${base}/api/v1/events`, { headers, credentials: 'include', signal })
        if (!response.ok || !response.body) throw new Error('event stream unavailable')
        failures = 0; onState('connected', failures)
        const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = ''
        while (!signal.aborted) {
          const part = await reader.read(); if (part.done) break
          buffer += decoder.decode(part.value, { stream: true })
          const chunks = buffer.split('\n\n'); buffer = chunks.pop() ?? ''
          for (const chunk of chunks) {
            const id = chunk.match(/^id: (.+)$/m)?.[1]; const event = chunk.match(/^event: (.+)$/m)?.[1]; const raw = chunk.match(/^data: (.+)$/m)?.[1]
            if (id) lastEventId = id
            if (event && raw) { try { onEvent(event, JSON.parse(raw)) } catch { /* events are only invalidation hints */ } }
          }
        }
        if (!signal.aborted) throw new Error('event stream ended')
      } catch {
        if (signal.aborted) break
        failures += 1; onState('disconnected', failures)
        await delay(EVENT_RETRY_DELAYS[Math.min(failures - 1, EVENT_RETRY_DELAYS.length - 1)], signal)
      }
    }
  }
  void run()
}
