<script setup>
defineProps({
  latency: { type: Number, default: null }
})

function cls(ms) {
  if (ms === null || ms === undefined) return 'bg-badge--none'
  if (ms < 30) return 'bg-badge--good'
  if (ms <= 100) return 'bg-badge--warn'
  return 'bg-badge--high'
}

function label(ms) {
  if (ms === null || ms === undefined) return '-'
  return `${ms} ms`
}
</script>

<template>
  <span class="bg-badge" :class="cls(latency)">
    <svg class="icon icon--xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
      <path d="M13 2 3 14h7l-1 8 10-12h-7l1-8z" />
    </svg>
    {{ label(latency) }}
  </span>
</template>