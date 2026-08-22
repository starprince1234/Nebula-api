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
  }
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
