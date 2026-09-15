<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import { useMonitorStore } from '../stores/monitor'

const router = useRouter()
const monitor = useMonitorStore()
const alerts = ref([])
const loading = ref(true)

async function load() {
  loading.value = true
  try {
    const { data } = await api.get('/alerts', { params: { limit: 100 } })
    alerts.value = data.data || []
  } catch (e) {
    alerts.value = []
  } finally {
    loading.value = false
  }
}

async function markRead(a) {
  try {
    await api.patch(`/alerts/${a.id}/read`)
    a.is_read = true
  } catch (e) { /* ignore */ }
}

function openCustomer(a) {
  if (a.customer_id) router.push(`/customers/${a.customer_id}`)
}

watch(() => monitor.lastEvent, (ev) => {
  if (ev?.event === 'alert:new') load()
})

let timer = null
onMounted(() => {
  load()
  timer = setInterval(load, 20000)
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div class="panel">
    <div class="crud__toolbar">
      <h2>Alerts</h2>
      <span class="crud__count">{{ alerts.filter((a) => !a.is_read).length }} belum dibaca</span>
      <button class="btn btn--ghost btn--sm" @click="load">Refresh</button>
    </div>

    <ul class="alerts">
      <li
        v-for="a in alerts"
        :key="a.id"
        class="alert-item"
        :class="{ 'alert-item--unread': !a.is_read }"
      >
        <div class="alert-item__icon" :class="`alert-item__icon--${a.severity.toLowerCase()}`">
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
            <path d="M13.73 21a2 2 0 0 1-3.46 0" />
          </svg>
        </div>
        <div class="alert-item__body">
          <div class="alert-item__head">
            <strong>{{ a.title }}</strong>
            <span class="alert-item__time">{{ new Date(a.created_at).toLocaleString('id-ID') }}</span>
          </div>
          <p>{{ a.message }}</p>
          <div class="alert-item__meta">
            <span v-if="a.customer_code" class="mono">{{ a.customer_code }}</span>
            <span>{{ a.alert_type }} · {{ a.severity }}</span>
          </div>
        </div>
        <div class="alert-item__actions">
          <button class="btn btn--ghost btn--xs" @click="openCustomer(a)">Detail</button>
          <button v-if="!a.is_read" class="btn btn--ghost btn--xs" @click="markRead(a)">Tandai dibaca</button>
        </div>
      </li>
      <li v-if="!alerts.length && !loading" class="alert-item alert-item--empty">Tidak ada alert</li>
    </ul>
  </div>
</template>