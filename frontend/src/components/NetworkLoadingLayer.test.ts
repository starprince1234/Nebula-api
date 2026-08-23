import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import NetworkLoadingLayer from './NetworkLoadingLayer.vue'
import { networkActivity } from '../api/client'

describe('NetworkLoadingLayer', () => {
  afterEach(() => {
    networkActivity.reads = 0
    networkActivity.writes = 0
  })

  it('keeps global feedback non-positional and leaves skeletons to local regions', async () => {
    networkActivity.reads = 1
    const wrapper = mount(NetworkLoadingLayer)

    expect(wrapper.find('.network-progress').exists()).toBe(true)
    expect(wrapper.find('.network-skeleton').exists()).toBe(false)
    expect(wrapper.find('.network-loading-layer').attributes('role')).toBe('status')
  })
})
