import { describe, expect, it } from 'vitest'
import { capabilityLabel, formatTokenK, parseTokenK } from './modelCatalog'

describe('model catalog presentation', () => {
  it('uses decimal K units without losing precision', () => {
    expect(formatTokenK(128000)).toBe('128K')
    expect(formatTokenK(131072)).toBe('131.072K')
    expect(parseTokenK('131.072')).toBe(131072)
  })

  it('labels fixed capabilities', () => {
    expect(capabilityLabel('reasoning')).toBe('推理')
    expect(capabilityLabel('tool_calling')).toBe('工具调用')
  })
})
