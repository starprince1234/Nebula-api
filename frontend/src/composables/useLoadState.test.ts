import { describe, expect, it } from 'vitest'
import { useLoadState } from './useLoadState'

describe('useLoadState', () => {
  it('separates initial loading from refresh loading', async () => {
    const state = useLoadState()
    let release!: () => void
    const first = state.run(() => new Promise<void>(resolve => { release = resolve }))
    expect(state.initialLoading.value).toBe(true)
    release()
    await first
    expect(state.settled.value).toBe(true)

    const refresh = state.run(async () => undefined)
    expect(state.refreshing.value).toBe(true)
    await refresh
    expect(state.loading.value).toBe(false)
  })

  it('keeps loading active until concurrent requests settle', async () => {
    const state = useLoadState()
    let releaseFirst!: () => void
    let releaseSecond!: () => void
    const first = state.run(() => new Promise<void>(resolve => { releaseFirst = resolve }))
    const second = state.run(() => new Promise<void>(resolve => { releaseSecond = resolve }))
    releaseFirst()
    await first
    expect(state.loading.value).toBe(true)
    releaseSecond()
    await second
    expect(state.loading.value).toBe(false)
  })
})
