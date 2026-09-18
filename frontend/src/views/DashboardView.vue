<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import MapView from '../components/MapView.vue'
import { useMonitorStore } from '../stores/monitor'
import { useAreaSearch } from '../composables/useAreaSearch'

const router = useRouter()
const monitor = useMonitorStore()

const customers = ref([])
const statusFilter = ref('ALL')
const vpnFilter = ref('ALL')
const loading = ref(true)
const mapView = ref(null)
const search = ref('')
const searchOpen = ref(false)
const { areaResults, areaLoading, clearArea, areaZoom } = useAreaSearch(search)

const searchResults = computed(() => {
  const q = search.value.toLowerCase().trim()
  if (!q) return []
  return customers.value
    .filter(
      (c) =>
        c.customer_code?.toLowerCase().includes(q) ||
        c.customer_name?.toLowerCase().includes(q) ||
        c.ip_address?.toLowerCase().includes(q)
    )
    .slice(0, 8)
})

function focusFromSearch(c) {
  if (c.latitude == null || c.longitude == null) {
    search.value = ''
    searchOpen.value = false
    return
  }
  mapView.value?.focusCustomer(c)
  search.value = ''
  searchOpen.value = false
}

function pickFirst() {
  if (searchResults.value.length) focusFromSearch(searchResults.value[0])
  else if (areaResults.value.length) focusFromArea(areaResults.value[0])
}

function focusFromArea(a) {
  mapView.value?.flyTo(Number(a.lat), Number(a.lon), areaZoom(a), a.display_name)
  clearArea()
  search.value = ''
  searchOpen.value = false
}

function onSearchBlur() {
  setTimeout(() => {
    searchOpen.value = false
  }, 150)
}

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
      <div class="dash-search-wrap">
        <div class="search">
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/></svg>
          <input
            v-model="search"
            placeholder="Cari kode, nama, atau IP..."
            @focus="searchOpen = true"
            @input="searchOpen = !!search.trim()"
            @blur="onSearchBlur"
            @keydown.enter="pickFirst"
          />
        </div>
        <div v-if="searchOpen && (search.trim() || searchResults.length)" class="search-drop">
          <template v-if="searchResults.length">
            <div class="search-drop__head">Pelanggan</div>
            <li
              v-for="c in searchResults"
              :key="'c' + c.id"
              @mousedown.prevent="focusFromSearch(c)"
            >
              <strong>{{ c.customer_name }}</strong>
              <span class="search-drop__meta">{{ c.customer_code }} · {{ c.ip_address }} · {{ c.status }}</span>
            </li>
          </template>
          <template v-else-if="search.trim()">
            <div class="search-drop__head">Pelanggan</div>
            <li class="search-drop__empty">Tidak ada pelanggan cocok</li>
          </template>

          <div v-if="search.trim()" class="search-drop__head">Wilayah</div>
          <li v-if="areaLoading" class="search-drop__empty">Mencari wilayah...</li>
          <template v-else>
            <li
              v-for="a in areaResults"
              :key="'a' + a.osm_id"
              @mousedown.prevent="focusFromArea(a)"
            >
              <strong>{{ a.name || a.display_name }}</strong>
              <span class="search-drop__meta search-drop__meta--ellipsis">{{ a.display_name }}</span>
            </li>
            <li v-if="!areaResults.length" class="search-drop__empty">
              Wilayah tidak ditemukan
            </li>
          </template>
        </div>
      </div>

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
        ref="mapView"
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

<style scoped>
.dash-search-wrap {
  position: relative;
  flex: 1;
  min-width: 220px;
  max-width: 360px;
}

.dash-search-wrap .search {
  flex: 1;
}

.search-drop {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  z-index: 1200;
  margin: 0;
  padding: 4px;
  list-style: none;
  background: var(--bg-panel-2);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
  overflow: hidden;
}

.search-drop li {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}

.search-drop__head {
  padding: 8px 10px 4px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-faint);
}

.search-drop li:hover {
  background: var(--bg-hover);
}

.search-drop__meta {
  font-size: 12px;
  color: var(--text-faint);
}

.search-drop__meta--ellipsis {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.search-drop--empty {
  padding: 10px;
  color: var(--text-faint);
  font-size: 13px;
}

.search-drop__empty {
  padding: 8px 10px;
  color: var(--text-faint);
  font-size: 13px;
}
</style>
