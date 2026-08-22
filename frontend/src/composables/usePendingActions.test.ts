import { describe, expect, it, vi } from 'vitest'
import { usePendingActions } from './usePendingActions'

describe('usePendingActions', () => {
  it('deduplicates the same action while allowing different actions', async () => {
    const actions = usePendingActions()
    let release!: () => void
    const operation = vi.fn(() => new Promise<number>(resolve => { release = () => resolve(1) }))
    const first = actions.run('save', operation)
    const duplicate = actions.run('save', operation)
    expect(operation).toHaveBeenCalledTimes(1)
    expect(actions.pending('save')).toBe(true)
    release()
    await expect(Promise.all([first, duplicate])).resolves.toEqual([1, 1])
    expect(actions.pending('save')).toBe(false)
  })
})
