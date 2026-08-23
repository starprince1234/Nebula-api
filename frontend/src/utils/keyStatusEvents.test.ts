import { describe, expect, it } from 'vitest'
import type { ApiKey } from '../api/types'
import { changedApiKeyID } from './keyStatusEvents'

const key = { id: 'key-1', status: 'pending_mentor' } as ApiKey

describe('API Key status events', () => {
  it('reveals an existing application only when its status changed', () => {
    expect(changedApiKeyID([key], { api_key_id: 'key-1', status: 'pending_teacher' })).toBe('key-1')
    expect(changedApiKeyID([key], { api_key_id: 'key-1', status: 'pending_mentor' })).toBeNull()
  })

  it('does not reveal details for initial data, new keys, or unrelated refreshes', () => {
    expect(changedApiKeyID([], { api_key_id: 'key-1', status: 'pending_teacher' })).toBeNull()
    expect(changedApiKeyID([key], { api_key_id: 'key-2', status: 'approved' })).toBeNull()
    expect(changedApiKeyID([key], 'sse-restored')).toBeNull()
  })
})
