<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts/core'
import { PieChart } from 'echarts/charts'
import { TooltipComponent, LegendComponent, AriaComponent } from 'echarts/components'
import { SVGRenderer } from 'echarts/renderers'
echarts.use([PieChart, TooltipComponent, LegendComponent, AriaComponent, SVGRenderer])
const props = defineProps<{ segments: Array<{ name: string; value: number; color?: string }>; total: number; label?: string }>()
const root = ref<HTMLElement | null>(null)
let chart: echarts.ECharts | undefined
let resizeHandler: (() => void) | undefined
function render() { chart?.setOption({ aria: { enabled: true }, tooltip: { trigger: 'item', valueFormatter: (value: number) => `${value.toFixed(3)} credits` }, legend: { show: false }, series: [{ type: 'pie', radius: ['58%', '82%'], label: { show: false }, data: props.segments.map(item => ({ name: item.name, value: item.value, itemStyle: { color: item.color } })) }] }) }
onMounted(() => { if (root.value) { chart = echarts.init(root.value, undefined, { renderer: 'svg' }); render(); resizeHandler = () => chart?.resize(); window.addEventListener('resize', resizeHandler) } })
watch(() => props.segments, render, { deep: true })
onBeforeUnmount(() => { if (resizeHandler) window.removeEventListener('resize', resizeHandler); chart?.dispose(); chart = undefined })
</script>
<template><div ref="root" class="credit-donut" :aria-label="props.label || 'credit usage chart'" role="img"></div></template>
