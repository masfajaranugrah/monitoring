<script setup>
defineProps({
  status: { type: String, default: 'OFFLINE' }
})
</script>

<template>
  <span class="badge" :class="`badge--${(status || 'OFFLINE').toLowerCase()}`">
    <span class="badge__dot" />
    <span>{{ status }}</span>
  </span>
</template>