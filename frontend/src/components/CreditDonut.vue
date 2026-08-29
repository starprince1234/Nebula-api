<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { PieChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent, AriaComponent } from 'echarts/components'
import { SVGRenderer } from 'echarts/renderers'

echarts.use([PieChart, TooltipComponent, LegendComponent, AriaComponent, SVGRenderer])

export type CreditSegment = { name: string; value: number; color?: string }

const props = withDefaults(defineProps<{
  segments: CreditSegment[]
  total: number
  label?: string
  mini?: boolean
}>(), { label: 'credit 用量图', mini: false })

const root = ref<HTMLElement | null>(null)
const palette = ['#7657e8', '#2fa8c4', '#e5a93f', '#e16d8b', '#45a779', '#8b79cb', '#64748b']
const positiveSegments = computed(() => props.segments
  .filter(item => Number.isFinite(item.value) && item.value > 0)
  .map((item, index) => ({ ...item, color: item.color ?? palette[index % palette.length] })))
const chartSegments = computed(() => {
  const totalUsed = positiveSegments.value.reduce((sum, item) => sum + item.value, 0)
  if (props.total <= 0 || totalUsed <= props.total) return positiveSegments.value
  const ratio = props.total / totalUsed
  return positiveSegments.value.map(item => ({ ...item, value: item.value * ratio }))
})
let chart: echarts.ECharts | undefined
let resizeObserver: ResizeObserver | undefined

function render() {
  chart?.setOption({
    animation: !window.matchMedia('(prefers-reduced-motion: reduce)').matches,
    aria: { enabled: true, description: props.label },
    tooltip: { trigger: 'item', confine: true, valueFormatter: (value: number) => `${value.toFixed(3)} credits` },
    legend: { show: false },
    series: [{ type: 'pie', radius: props.mini ? ['62%', '88%'] : ['58%', '82%'], silent: props.mini, label: { show: false }, data: chartSegments.value.map(item => ({ name: item.name, value: item.value, itemStyle: { color: item.color } })) }],
  }, true)
}

onMounted(() => {
  if (!root.value) return
  chart = echarts.init(root.value, undefined, { renderer: 'svg' })
  render()
  resizeObserver = new ResizeObserver(() => chart?.resize())
  resizeObserver.observe(root.value)
})
watch([chartSegments, () => props.label, () => props.mini], render, { deep: true })
onBeforeUnmount(() => { resizeObserver?.disconnect(); chart?.dispose(); chart = undefined })
</script>

<template>
  <div class="credit-chart" :class="{ mini }">
    <div ref="root" class="credit-donut" :aria-label="label" role="img" />
    <ul v-if="!mini" class="credit-legend" aria-label="图表图例">
      <li v-for="segment in positiveSegments" :key="segment.name">
        <span class="credit-legend-swatch" :style="{ backgroundColor: segment.color }" />
        <span class="credit-legend-name">{{ segment.name }}</span>
        <strong>{{ segment.value.toFixed(3) }}</strong>
      </li>
    </ul>
    <table v-if="!mini" class="sr-only">
      <caption>{{ label }}</caption>
      <thead><tr><th>分项</th><th>credits</th></tr></thead>
      <tbody><tr v-for="segment in positiveSegments" :key="segment.name"><td>{{ segment.name }}</td><td>{{ segment.value.toFixed(3) }}</td></tr></tbody>
    </table>
  </div>
</template>
