<script setup>
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import MapView from '../components/MapView.vue'
import { useMonitorStore } from '../stores/monitor'

const router = useRouter()
const monitor = useMonitorStore()

const customers = ref([])
const statusFilter = ref('ALL')
const vpnFilter = ref('ALL')
const loading = ref(true)

const statuses = ['ALL', 'ONLINE', 'OFFLINE', 'WARNING']

const draft = ref(null)
const showAddModal = ref(false)
const addForm = ref({ customer_name: '', ip_address: '' })
const saving = ref(false)
const addError = ref('')

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

function onMapClick(pt) {
  draft.value = { lat: pt.lat, lng: pt.lng }
  addForm.value = { customer_name: '', ip_address: '' }
  addError.value = ''
  showAddModal.value = true
}

function cancelAdd() {
  showAddModal.value = false
  draft.value = null
}

async function saveCustomer() {
  if (!draft.value) return
  saving.value = true
  addError.value = ''
  try {
    await api.post('/customers', {
      customer_name: addForm.value.customer_name,
      ip_address: addForm.value.ip_address,
      latitude: draft.value.lat,
      longitude: draft.value.lng,
      monitoring_enabled: true,
      ping_interval: 10,
      timeout_ms: 2000,
      retry_count: 2
    })
    showAddModal.value = false
    draft.value = null
    await load()
    monitor.fetchStats()
  } catch (e) {
    addError.value = e.response?.data?.error || 'Gagal menyimpan pelanggan'
  } finally {
    saving.value = false
  }
}

function applyRealtime() {
  const ev = monitor.lastEvent
  if (!ev || ev.event !== 'customer:update') return
  const { customer_id, status, latency_ms } = ev.data || {}
  const target = customers.value.find((c) => c.id === customer_id)
  if (!target) return
  if (status) target.status = status
  if (latency_ms !== undefined) target.latency_ms = latency_ms
}

watch(() => monitor.lastEvent, applyRealtime)
onMounted(load)
</script>

<template>
  <div class="dashboard">
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
      <span class="dashboard__hint">Klik pada peta untuk menambah pelanggan</span>
      <span class="dashboard__count">{{ customers.length }} titik</span>
    </div>

    <div class="dashboard__map">
      <MapView
        :customers="customers"
        :status-filter="statusFilter"
        :vpn-filter="vpnFilter"
        :click-to-add="true"
        :draft-point="draft"
        @open-detail="openDetail"
        @map-click="onMapClick"
      />
      <div class="legend">
        <div class="legend__item"><span class="legend__dot legend__dot--online" /> ONLINE</div>
        <div class="legend__item"><span class="legend__dot legend__dot--offline" /> OFFLINE</div>
        <div class="legend__item"><span class="legend__dot legend__dot--warning" /> WARNING</div>
      </div>
    </div>

    <div v-if="showAddModal" class="modal-mask" @click.self="cancelAdd">
      <div class="modal modal--sm">
        <h3>Tambah Pelanggan</h3>
        <p v-if="draft" class="modal__coord">
          Koordinat: {{ draft.lat.toFixed(6) }}, {{ draft.lng.toFixed(6) }}
        </p>
        <form @submit.prevent="saveCustomer">
          <div class="form-grid form-grid--single">
            <label class="field"><span>Nama Pelanggan *</span>
              <input v-model="addForm.customer_name" required autofocus />
            </label>
            <label class="field"><span>IP Address *</span>
              <input v-model="addForm.ip_address" placeholder="103.143.196.10" required />
            </label>
          </div>
          <p v-if="addError" class="login__error">{{ addError }}</p>
          <div class="modal__actions">
            <button type="button" class="btn btn--ghost" @click="cancelAdd">Batal</button>
            <button type="submit" class="btn btn--primary" :disabled="saving">
              {{ saving ? 'Menyimpan...' : 'Simpan' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>
