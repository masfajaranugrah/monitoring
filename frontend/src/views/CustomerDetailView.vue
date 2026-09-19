<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import LatencyBadge from '../components/LatencyBadge.vue'
import LatencyChart from '../components/LatencyChart.vue'
import { customerIconSvg, statusColor } from '../services/customerIcons'
import ModemAccessModal from '../components/ModemAccessModal.vue'

const props = defineProps({ id: { type: [String, Number], required: true } })
const router = useRouter()

const customer = ref(null)
const history = ref([])
const loading = ref(true)
const historyLimit = ref(200)
const showModemModal = ref(false)

async function load() {
  loading.value = true
  try {
    const [cRes, hRes] = await Promise.all([
      api.get(`/customers/${props.id}`),
      api.get(`/customers/${props.id}/ping-history`, { params: { limit: historyLimit.value } })
    ])
    customer.value = cRes.data
    history.value = hRes.data.data || []
  } catch (e) {
    customer.value = null
  } finally {
    loading.value = false
  }
}

const chartPoints = ref([])
watch(history, (h) => {
  chartPoints.value = h
    .slice()
    .reverse()
    .filter((r) => r.latency_ms != null)
    .map((r) => ({ t: r.pinged_at, v: r.latency_ms }))
})

const uptimeText = () => (customer.value?.uptime_percentage ?? 0) + '%'

function fmtTime(t) {
  return t ? new Date(t).toLocaleString('id-ID') : '-'
}

let timer = null
onMounted(() => {
  load()
  timer = setInterval(load, 15000)
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div v-if="loading" class="panel loading-text">Memuat detail pelanggan...</div>

  <div v-else-if="!customer" class="panel">
    <p>Pelanggan tidak ditemukan.</p>
    <button class="btn btn--ghost" @click="router.push('/customers')">Kembali</button>
  </div>

  <div v-else class="detail">
    <div class="detail__head panel">
      <div class="detail__head-ident">
        <span
          class="cust-icon cust-icon--lg"
          :style="{ '--ic-color': statusColor(customer.status) }"
          v-html="customerIconSvg(customer.icon || 'customer')"
        ></span>
        <div>
          <h2>{{ customer.customer_name }}</h2>
          <p class="mono">{{ customer.customer_code }} · {{ customer.ip_address }}</p>
        </div>
      </div>
      <div class="detail__head-right">
        <StatusBadge :status="customer.status" />
        <LatencyBadge :latency="customer.latency_ms" />
        <button class="btn btn--primary btn--sm" @click="showModemModal = true">
          <svg class="icon icon--xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 2 15.5 7.5M21 2l-5 5-4-2-2.5 2.5 4 4L8 18l-4 1 2 2 2-1 2.5-2.5 4 4L17 18l-2-4 5-5z"/><path d="M13 11l4-4"/></svg>
          Akses Modem
        </button>
        <button class="btn btn--ghost btn--sm" @click="router.push('/customers')">Kembali</button>
      </div>
    </div>

    <div class="detail__grid">
      <div class="panel detail__card">
        <h3>Customer Information</h3>
        <dl class="kv">
          <dt>Kode</dt><dd>{{ customer.customer_code }}</dd>
          <dt>Nama</dt><dd>{{ customer.customer_name }}</dd>
          <dt>Deskripsi</dt><dd>{{ customer.description || '-' }}</dd>
          <dt>Dibuat</dt><dd>{{ fmtTime(customer.created_at) }}</dd>
        </dl>
      </div>

      <div class="panel detail__card">
        <h3>Network Information</h3>
        <dl class="kv">
          <dt>IP Address</dt><dd class="mono">{{ customer.ip_address }}</dd>
          <dt>Latitude</dt><dd>{{ customer.latitude }}</dd>
          <dt>Longitude</dt><dd>{{ customer.longitude }}</dd>
        </dl>
      </div>

      <div class="panel detail__card">
        <h3>VPN & Monitoring</h3>
        <dl class="kv">
          <dt>VPN</dt><dd>{{ customer.vpn_name || '-' }}</dd>
          <dt>Monitoring</dt><dd>{{ customer.monitoring_enabled ? 'Enabled' : 'Disabled' }}</dd>
          <dt>Interval</dt><dd>{{ customer.ping_interval }} detik</dd>
          <dt>Timeout</dt><dd>{{ customer.timeout_ms }} ms</dd>
          <dt>Retry</dt><dd>{{ customer.retry_count }}x</dd>
          <dt>Total Check</dt><dd>{{ customer.total_checks }}</dd>
          <dt>Uptime (24 jam)</dt><dd>{{ uptimeText() }}</dd>
        </dl>
      </div>

      <div class="panel detail__card">
        <h3>Status History</h3>
        <dl class="kv">
          <dt>Status</dt><dd><StatusBadge :status="customer.status" /></dd>
          <dt>Latency</dt><dd><LatencyBadge :latency="customer.latency_ms" /></dd>
          <dt>Last Check</dt><dd>{{ fmtTime(customer.last_check) }}</dd>
          <dt>Last Online</dt><dd>{{ fmtTime(customer.last_online) }}</dd>
          <dt>Last Offline</dt><dd>{{ fmtTime(customer.last_offline) }}</dd>
        </dl>
      </div>
    </div>

    <div class="panel detail__chart">
      <h3>Grafik Latency</h3>
      <LatencyChart :points="chartPoints" :height="180" />
    </div>

    <div class="panel detail__history">
      <h3>Ping History <button class="btn btn--ghost btn--sm" @click="load">Refresh</button></h3>
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr><th>Waktu</th><th>Status</th><th>Latency</th><th>VPN</th></tr>
          </thead>
          <tbody>
            <tr v-for="r in history" :key="r.id">
              <td class="mono">{{ new Date(r.pinged_at).toLocaleTimeString('id-ID') }}</td>
              <td><StatusBadge :status="r.status" /></td>
              <td>{{ r.latency_ms != null ? r.latency_ms + ' ms' : '-' }}</td>
              <td>{{ r.vpn_id || '-' }}</td>
            </tr>
            <tr v-if="!history.length"><td colspan="4" class="table__empty">Belum ada riwayat</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <ModemAccessModal v-if="showModemModal" :customer="customer" @close="showModemModal = false" />
</template>