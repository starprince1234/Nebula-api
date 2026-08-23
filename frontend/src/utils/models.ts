import type { ModelStatus, RequestedModel } from '../api/types'

export type ModelCardStatus = ModelStatus | 'ready'

export function modelCardStatus(model: Pick<{ status: ModelStatus; route_ready: boolean }, 'status' | 'route_ready'>): ModelCardStatus {
  return model.status === 'pending_configuration' && model.route_ready ? 'ready' : model.status
}

export function normalizeModelSelection(existing: string[], requested: RequestedModel[]): string[] {
  const values = [...existing, ...requested.map(model => model.model_id)]
  const seen = new Set<string>()
  return values.filter(value => {
    const key = value.trim().toLowerCase()
    if (!key || seen.has(key)) return false
    seen.add(key)
    return true
  })
}

export function validateModelCount(count: number): boolean { return count >= 1 && count <= 100 }
