<script setup>
import { computed } from 'vue'

const props = defineProps({
  points: { type: Array, default: () => [] },
  height: { type: Number, default: 160 }
})

// points: [{ t: timestamp, v: latency_ms }]

const width = 600

const chartPoints = computed(() => {
  if (!props.points.length) return []
  const max = Math.max(...props.points.map((p) => p.v || 0), 10)
  const min = 0
  const range = max - min || 1
  const step = width / Math.max(props.points.length - 1, 1)
  return props.points.map((p, i) => ({
    x: i * step,
    y: props.height - 8 - ((p.v || 0) - min) / range * (props.height - 24),
    t: p.t,
    v: p.v
  }))
})

const pathD = computed(() => {
  if (!chartPoints.value.length) return ''
  return chartPoints.value.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x},${p.y}`).join(' ')
})
</script>

<template>
  <div class="latency-chart">
    <svg :viewBox="`0 0 ${width} ${height}`" preserveAspectRatio="none" class="latency-chart__svg">
      <polyline :points="chartPoints.map(p => `${p.x},${p.y}`).join(' ')" fill="none" class="latency-chart__line" />
      <g v-for="(p, i) in chartPoints" :key="i">
        <circle :cx="p.x" :cy="p.y" r="3" class="latency-chart__dot" />
        <title>{{ new Date(p.t).toLocaleTimeString('id-ID') }} - {{ p.v }} ms</title>
      </g>
    </svg>
    <div v-if="!chartPoints.length" class="latency-chart__empty">Belum ada data latency</div>
  </div>
</template>