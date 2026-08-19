import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import StatusProgress from './StatusProgress.vue'

describe('StatusProgress', () => {
  it('places teacher rejection at the teacher review step', () => {
    const wrapper = mount(StatusProgress, { props: { current: 'rejected_teacher', completed: ['submitted', 'mentor_review'] } })
    expect(wrapper.findAll('li')[2].classes()).toContain('terminal')
  })
})
