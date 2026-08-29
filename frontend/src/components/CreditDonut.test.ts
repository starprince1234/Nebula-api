import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const chart = {
  dispose: vi.fn(),
  resize: vi.fn(),
  setOption: vi.fn(),
}

vi.mock('echarts/core', () => ({
  init: vi.fn(() => chart),
  use: vi.fn(),
}))
vi.mock('echarts/charts', () => ({ PieChart: {} }))
vi.mock('echarts/components', () => ({ TooltipComponent: {}, LegendComponent: {}, AriaComponent: {} }))
vi.mock('echarts/renderers', () => ({ SVGRenderer: {} }))

import CreditDonut from './CreditDonut.vue'

describe('CreditDonut', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', class {
      disconnect() {}
      observe() {}
    })
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false })))
  })

  afterEach(() => {
    chart.dispose.mockClear()
    chart.resize.mockClear()
    chart.setOption.mockClear()
    vi.unstubAllGlobals()
  })

  it('confines item tooltips to the donut viewport', () => {
    mount(CreditDonut, {
      props: {
        segments: [{ name: '用户已分配额度', value: 100 }],
        total: 100,
      },
    })

    expect(chart.setOption).toHaveBeenCalledWith(expect.objectContaining({
      tooltip: expect.objectContaining({ confine: true }),
    }), true)
  })
})
