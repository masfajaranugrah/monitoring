<script setup>
import { ref, onMounted } from 'vue'
import api from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import { useMonitorStore } from '../stores/monitor'

const monitor = useMonitorStore()
const history = ref([])
const selectedCustomer = ref('')
const customerOptions = ref([])
const loading = ref(true)

async function loadCustomers() {
  const { data } = await api.get('/customers', { params: { page_size: 500 } })
  customerOptions.value = data.data || []
}

async function loadHistory() {
  if (!selectedCustomer.value) {
    history.value = []
    loading.value = false
    return
  }
  loading.value = true
  try {
    const { data } = await api.get(`/customers/${selectedCustomer.value}/ping-history`, { params: { limit: 300 } })
    history.value = (data.data || []).slice().reverse()
  } catch (e) {
    history.value = []
  } finally {
    loading.value = false
  }
}

function fmtTime(t) {
  return t ? new Date(t).toLocaleString('id-ID') : '-'
}

onMounted(async () => {
  await monitor.fetchVPNs()
  await loadCustomers()
})
</script>

<template>
  <div class="panel">
    <div class="crud__toolbar">
      <h2 style="margin-right: 12px">Ping History</h2>
      <select v-model="selectedCustomer" class="select" @change="loadHistory">
        <option value="">Pilih pelanggan...</option>
        <option v-for="c in customerOptions" :key="c.id" :value="c.id">
          {{ c.customer_code }} - {{ c.customer_name }}
        </option>
      </select>
      <button class="btn btn--ghost btn--sm" @click="loadHistory">Muat</button>
    </div>

    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr><th>Waktu</th><th>Status</th><th>Latency</th><th>Indikator</th></tr>
        </thead>
        <tbody>
          <tr v-for="(r, i) in history" :key="r.id">
            <td class="mono">{{ fmtTime(r.pinged_at) }}</td>
            <td><StatusBadge :status="r.status" /></td>
            <td>{{ r.latency_ms != null ? r.latency_ms + ' ms' : '-' }}</td>
            <td>
              <span v-if="r.latency_ms != null" :class="[
                r.latency_ms < 30 ? 'lat-hint lat-hint--good' :
                r.latency_ms <= 100 ? 'lat-hint lat-hint--warn' : 'lat-hint lat-hint--high'
              ]">
                {{ r.latency_ms < 30 ? 'GOOD' : r.latency_ms <= 100 ? 'WARNING' : 'HIGH' }}
              </span>
              <span v-else-if="r.status !== 'ONLINE'" class="lat-hint lat-hint--fail">—</span>
            </td>
          </tr>
          <tr v-if="!selectedCustomer"><td colspan="4" class="table__empty">Pilih pelanggan untuk melihat riwayat ping</td></tr>
          <tr v-else-if="!history.length && !loading"><td colspan="4" class="table__empty">Tidak ada data</td></tr>
        </tbody>
      </table>
    </div>
  </div>
</template>