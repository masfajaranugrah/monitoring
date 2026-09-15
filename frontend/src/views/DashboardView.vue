<script setup>
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import StatsCards from '../components/StatsCards.vue'
import MapView from '../components/MapView.vue'
import { useMonitorStore } from '../stores/monitor'

const router = useRouter()
const monitor = useMonitorStore()

const customers = ref([])
const statusFilter = ref('ALL')
const vpnFilter = ref('ALL')
const loading = ref(true)

const statuses = ['ALL', 'ONLINE', 'OFFLINE', 'WARNING']

async function load() {
  try {
    const params = {}
    if (statusFilter.value !== 'ALL') params.status = statusFilter.value
    if (vpnFilter.value !== 'ALL') params.vpn_id = vpnFilter.value
    const { data } = await api.get('/map/customers', { params })
    customers.value = data.data || []
    loading.value = false
  } catch (e) {
    loading.value = false
  }
}

function openDetail(customer) {
  router.push(`/customers/${customer.id}`)
}

function applyRealtime() {
  const ev = monitor.lastEvent
  if (!ev || ev.event !== 'customer:update') return
  const { customer_id, status, latency_ms, last_check } = ev.data || {}
  const target = customers.value.find((c) => c.id === customer_id)
  if (!target) return
  if (status) target.status = status
  if (latency_ms !== undefined) target.latency_ms = latency_ms
  if (last_check) target.last_check = last_check
  monitor.fetchStats()
}

watch(() => monitor.lastEvent, applyRealtime)
onMounted(load)
</script>

<template>
  <div class="dashboard">
    <StatsCards />

    <div class="dashboard__controls">
      <div class="filter-group">
        <span class="filter-group__label">Status:</span>
        <button
          v-for="s in statuses"
          :key="s"
          class="filter-chip"
          :class="{ 'filter-chip--active': statusFilter === s, [`filter-chip--${s.toLowerCase()}`]: true }"
          @click="statusFilter = s; load()"
        >
          {{ s }}
        </button>
      </div>

      <div class="filter-group">
        <span class="filter-group__label">VPN:</span>
        <select v-model="vpnFilter" class="select" @change="load()">
          <option value="ALL">ALL VPN</option>
          <option v-for="v in monitor.vpns" :key="v.id" :value="v.id">{{ v.name }}</option>
        </select>
      </div>

      <button class="btn btn--ghost btn--sm" @click="load()">Refresh</button>
      <span class="dashboard__count">{{ customers.length }} titik</span>
    </div>

    <div class="dashboard__map">
      <MapView
        :customers="customers"
        :status-filter="statusFilter"
        :vpn-filter="vpnFilter"
        @open-detail="openDetail"
      />
      <div class="legend">
        <div class="legend__item"><span class="legend__dot legend__dot--online" /> ONLINE</div>
        <div class="legend__item"><span class="legend__dot legend__dot--offline" /> OFFLINE</div>
        <div class="legend__item"><span class="legend__dot legend__dot--warning" /> WARNING</div>
      </div>
    </div>
  </div>
</template>