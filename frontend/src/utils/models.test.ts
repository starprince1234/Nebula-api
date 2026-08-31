import { describe, expect, it } from 'vitest'
import { modelCardStatus, normalizeModelSelection, validateModelCount } from './models'

describe('model selection', () => {
  it('deduplicates model IDs case-insensitively', () => {
    expect(normalizeModelSelection(['GPT-4.1','gpt-4.1'])).toEqual(['GPT-4.1'])
  })
  it('enforces 1 to 100 models', () => {
    expect(validateModelCount(0)).toBe(false); expect(validateModelCount(1)).toBe(true); expect(validateModelCount(100)).toBe(true); expect(validateModelCount(101)).toBe(false)
  })
  it('presents a configured pending model as ready without duplicating route readiness', () => {
    expect(modelCardStatus({ status: 'pending_configuration', route_ready: true })).toBe('ready')
    expect(modelCardStatus({ status: 'pending_configuration', route_ready: false })).toBe('pending_configuration')
    expect(modelCardStatus({ status: 'inactive', route_ready: true })).toBe('inactive')
  })
})
