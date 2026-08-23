import type { ApiKey, ApiKeyStatus } from '../api/types'

export type ApiKeyStatusEvent = {
  api_key_id: string
  status: ApiKeyStatus
}

const apiKeyStatuses = new Set<ApiKeyStatus>([
  'pending_mentor',
  'pending_teacher',
  'approved',
  'active',
  'rejected',
  'revoked',
])

export function parseApiKeyStatusEvent(value: unknown): ApiKeyStatusEvent | null {
  if (!value || typeof value !== 'object') return null
  const event = value as Record<string, unknown>
  if (typeof event.api_key_id !== 'string' || typeof event.status !== 'string') return null
  if (!apiKeyStatuses.has(event.status as ApiKeyStatus)) return null
  return { api_key_id: event.api_key_id, status: event.status as ApiKeyStatus }
}

export function changedApiKeyID(keys: ApiKey[], value: unknown): string | null {
  const event = parseApiKeyStatusEvent(value)
  if (!event) return null
  const current = keys.find(key => key.id === event.api_key_id)
  return current && current.status !== event.status ? current.id : null
}
