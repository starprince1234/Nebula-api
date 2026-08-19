import type { RequestedModel } from '../api/types'

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
