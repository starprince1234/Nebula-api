import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import LoadingRegion from './LoadingRegion.vue'

describe('LoadingRegion', () => {
  it('shows an accessible skeleton before rendering content', async () => {
    const wrapper = mount(LoadingRegion, { props: { initialLoading: true, label: '正在加载项目' }, slots: { default: '<p>项目数据</p>' } })
    expect(wrapper.get('[role="status"]').text()).toContain('正在加载项目')
    expect(wrapper.text()).not.toContain('项目数据')
    await wrapper.setProps({ initialLoading: false })
    expect(wrapper.text()).toContain('项目数据')
  })
})
