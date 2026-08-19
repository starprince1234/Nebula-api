import { describe, expect, it } from 'vitest'
import { normalizeModelSelection, validateModelCount } from './models'

describe('model selection', () => {
  it('deduplicates model IDs case-insensitively', () => {
    expect(normalizeModelSelection(['GPT-4.1'], [{ model_id: 'gpt-4.1', display_name: 'x', category: 'text', capabilities: ['chat'], input_modalities: ['text'], output_modalities: ['text'] }])).toEqual(['GPT-4.1'])
  })
  it('enforces 1 to 100 models', () => {
    expect(validateModelCount(0)).toBe(false); expect(validateModelCount(1)).toBe(true); expect(validateModelCount(100)).toBe(true); expect(validateModelCount(101)).toBe(false)
  })
})
