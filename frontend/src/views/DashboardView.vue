<script setup>
import { ref, computed, onMounted, watch, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import MapView from '../components/MapView.vue'
import StatusBadge from '../components/StatusBadge.vue'
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
const addForm = ref({ customer_name: '', ip_address: '', vpn_id: '' })
const saving = ref(false)
const addError = ref('')

const mapExpanded = ref(false)
const fullStartView = ref(null)
const fullMapView = ref(null)
const fullSearch = ref('')
const fullSearchOpen = ref(false)
const fullAreaSearch = ref('')
const fullAreaOpen = ref(false)
const { areaResults: fullAreaResults, areaLoading: fullAreaLoading, clearArea: clearFullArea } = useAreaSearch(fullAreaSearch)

const fullResults = computed(() => {
  const q = fullSearch.value.toLowerCase().trim()
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

const fullList = computed(() => {
  const q = fullSearch.value.toLowerCase().trim()
  if (!q) return customers.value
  return customers.value.filter(
    (c) =>
      c.customer_code?.toLowerCase().includes(q) ||
      c.customer_name?.toLowerCase().includes(q) ||
      c.ip_address?.toLowerCase().includes(q)
  )
})

function openFullscreen() {
  fullStartView.value = mapView.value?.getView() || null
  fullSearch.value = ''
  fullSearchOpen.value = false
  fullAreaSearch.value = ''
  fullAreaOpen.value = false
  mapExpanded.value = true
}

function closeFullscreen() {
  const v = fullMapView.value?.getView()
  if (v && mapView.value) mapView.value.flyTo(v.lat, v.lng, v.zoom)
  mapExpanded.value = false
  fullSearch.value = ''
  fullSearchOpen.value = false
  fullAreaSearch.value = ''
  fullAreaOpen.value = false
}

function focusFromFull(c) {
  if (c.latitude == null || c.longitude == null) return
  fullMapView.value?.focusCustomer(c)
}

function pickFullFirst() {
  if (fullResults.value.length) focusFromFull(fullResults.value[0])
}

function focusFromFullArea(a) {
  fullMapView.value?.flyTo(Number(a.lat), Number(a.lon), areaZoom(a), a.display_name)
  clearFullArea()
  fullAreaSearch.value = ''
  fullAreaOpen.value = false
}

function pickFullAreaFirst() {
  if (fullAreaResults.value.length) focusFromFullArea(fullAreaResults.value[0])
  else if (fullResults.value.length) focusFromFull(fullResults.value[0])
}

function onFullAreaBlur() {
  setTimeout(() => {
    fullAreaOpen.value = false
  }, 150)
}

function onFullSearchBlur() {
  setTimeout(() => {
    fullSearchOpen.value = false
  }, 150)
}

function openFromFull(c) {
  mapExpanded.value = false
  openDetail(c)
}

function onKeydown(e) {
  if (e.key === 'Escape' && mapExpanded.value) closeFullscreen()
}

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
  addForm.value = { customer_name: '', ip_address: '', vpn_id: '' }
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
      vpn_id: addForm.value.vpn_id ? Number(addForm.value.vpn_id) : null,
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
  const { customer_id, status, latency_ms, last_check } = ev.data || {}
  const target = customers.value.find((c) => c.id === customer_id)
  if (!target) return
  if (status) target.status = status
  if (latency_ms !== undefined) target.latency_ms = latency_ms
  if (last_check) target.last_check = last_check
}

const FEATURE_ICONS = [
  { value: 'dot', label: 'Titik' },
  { value: 'customer', label: 'Pelanggan' },
  { value: 'wifi', label: 'WiFi' },
  { value: 'jb', label: 'Info (JB)' },
  { value: 'router', label: 'Router' }
]
const FEATURE_COLORS = [
  '#3b82f6', '#ef4444', '#22c55e', '#f59e0b', '#a855f7', '#06b6d4',
  '#f97316', '#84cc16', '#ec4899', '#6366f1', '#14b8a6', '#eab308'
]

const drawTool = ref('')
const featureDraft = ref(null) // { geometry, isPoint }
const featureEditId = ref(null)
const showFeatureModal = ref(false)
const featureForm = ref({ name: '', icon: 'dot', color: '#3b82f6', description: '' })
const featureSaving = ref(false)
const featureError = ref('')

function toggleDraw(mode) {
  if (drawTool.value === mode) {
    mapView.value?.cancelDraw()
    fullMapView.value?.cancelDraw()
    drawTool.value = ''
    return
  }
  featureDraft.value = null
  mapView.value?.startDraw(mode)
  fullMapView.value?.startDraw(mode)
  drawTool.value = mode
}

function openFeatureModal() {
  featureEditId.value = null
  featureEditSource.value = null
  featureForm.value = { name: '', icon: 'dot', color: FEATURE_COLORS[0], description: '' }
  featureError.value = ''
  showFeatureModal.value = true
}

function closeFeatureModal() {
  showFeatureModal.value = false
}

function onDrawComplete(payload) {
  drawTool.value = ''
  if (payload.type === 'point') {
    featureDraft.value = {
      isPoint: true,
      geometry: { type: 'Point', coordinates: [payload.latlng.lng, payload.latlng.lat] }
    }
  } else if (payload.type === 'line') {
    featureDraft.value = {
      isPoint: false,
      geometry: {
        type: 'LineString',
        coordinates: payload.latlngs.map((l) => [l.lng, l.lat])
      }
    }
  } else if (payload.type === 'polygon') {
    const ring = payload.latlngs.map((l) => [l.lng, l.lat])
    const first = ring[0]
    const last = ring[ring.length - 1]
    if (first && last && (first[0] !== last[0] || first[1] !== last[1])) ring.push([first[0], first[1]])
    featureDraft.value = {
      isPoint: false,
      geometry: { type: 'Polygon', coordinates: [ring] }
    }
  }
  openFeatureModal()
}

function onFeatureManage(feature) {
  featureEditId.value = feature.id
  featureDraft.value = {
    isPoint: feature.feature_type === 'point',
    geometry: feature.geometry
  }
  featureForm.value = {
    name: feature.name || '',
    icon: feature.icon || (feature.feature_type === 'point' ? 'dot' : 'dot'),
    color: feature.color || FEATURE_COLORS[0],
    description: feature.description || ''
  }
  featureError.value = ''
  showFeatureModal.value = true
}

async function saveFeature() {
  if (!featureForm.value.name.trim() || !featureDraft.value) return
  featureSaving.value = true
  featureError.value = ''
  const payload = {
    name: featureForm.value.name.trim(),
    feature_type: featureDraft.value.isPoint ? 'point' : featureDraft.value.geometry.type === 'LineString' ? 'line' : 'polygon',
    icon: featureForm.value.icon,
    color: featureForm.value.color,
    description: featureForm.value.description.trim(),
    geometry: featureDraft.value.geometry
  }
  try {
    if (featureEditId.value) {
      await api.patch(`/map/features/${featureEditId.value}`, payload)
    } else {
      await api.post('/map/features', payload)
    }
    closeFeatureModal()
    featureDraft.value = null
    mapView.value?.refreshFeatures()
    fullMapView.value?.refreshFeatures()
  } catch (e) {
    featureError.value = e.response?.data?.error || 'Gagal menyimpan fitur'
  } finally {
    featureSaving.value = false
  }
}

async function deleteFeature() {
  if (!featureEditId.value) return
  if (!window.confirm('Hapus fitur ini dari peta?')) return
  featureSaving.value = true
  featureError.value = ''
  try {
    await api.delete(`/map/features/${featureEditId.value}`)
    closeFeatureModal()
    featureDraft.value = null
    mapView.value?.refreshFeatures()
    fullMapView.value?.refreshFeatures()
  } catch (e) {
    featureError.value = e.response?.data?.error || 'Gagal menghapus fitur'
  } finally {
    featureSaving.value = false
  }
}

watch(() => monitor.lastEvent, applyRealtime)

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  load()
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
})
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

    <div class="map-tools">
      <button
        type="button"
        class="btn btn--ghost btn--sm"
        :class="{ 'map-tools__active': drawTool === 'line' }"
        @click="toggleDraw('line')"
      >
        <svg class="icon icon--xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 5 8 18M18 5h3v3M3 8l5-5 5 5m-5 8v5"/></svg>
        Buat Jalur
      </button>
      <button
        type="button"
        class="btn btn--ghost btn--sm"
        :class="{ 'map-tools__active': drawTool === 'polygon' }"
        @click="toggleDraw('polygon')"
      >
        <svg class="icon icon--xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m12 3 8 4.5v9L12 21l-8-4.5v-9z"/></svg>
        Buat Area
      </button>
      <button
        type="button"
        class="btn btn--ghost btn--sm"
        :class="{ 'map-tools__active': drawTool === 'point' }"
        @click="toggleDraw('point')"
      >
        <svg class="icon icon--xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 21s-7-5.6-7-11a7 7 0 0 1 14 0c0 5.4-7 11-7 11z"/><circle cx="12" cy="10" r="2.5"/></svg>
        Titik Info
      </button>
      <button
        v-if="drawTool"
        type="button"
        class="btn btn--ghost btn--sm map-tools__cancel"
        @click="toggleDraw(drawTool)"
      >
        Batal gambar
      </button>
    </div>

    <div class="dashboard__map">
      <MapView
        ref="mapView"
        :customers="customers"
        :status-filter="statusFilter"
        :vpn-filter="vpnFilter"
        :click-to-add="true"
        :draft-point="draft"
        :manage-features="true"
        @open-detail="openDetail"
        @map-click="onMapClick"
        @draw-complete="onDrawComplete"
        @feature-manage="onFeatureManage"
      />
      <div class="legend">
        <div class="legend__item"><span class="legend__dot legend__dot--online" /> ONLINE</div>
        <div class="legend__item"><span class="legend__dot legend__dot--offline" /> OFFLINE</div>
        <div class="legend__item"><span class="legend__dot legend__dot--warning" /> WARNING</div>
      </div>

      <button class="map-expand" title="Perbesar peta" @click="openFullscreen">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M3 16v3a2 2 0 0 0 2 2h3M16 21h3a2 2 0 0 0 2-2v-3"/></svg>
      </button>
    </div>

    <div v-if="mapExpanded" class="map-full" role="dialog" aria-modal="true">
      <MapView
        ref="fullMapView"
        :customers="customers"
        :status-filter="statusFilter"
        :vpn-filter="vpnFilter"
        :initial-view="fullStartView"
        :click-to-add="true"
        :draft-point="draft"
        :manage-features="true"
        @open-detail="openFromFull"
        @map-click="onMapClick"
        @draw-complete="onDrawComplete"
        @feature-manage="onFeatureManage"
      />

      <div class="map-full__top">
        <button class="map-full__back" @click="closeFullscreen">
          <svg class="icon icon--xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 18l-6-6 6-6"/></svg>
          Kembali
        </button>

        <div class="map-full__area">
          <div class="map-full__area-wrap">
            <div class="search">
              <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/></svg>
              <input
                v-model="fullAreaSearch"
                placeholder="Cari wilayah..."
                @focus="fullAreaOpen = true"
                @input="fullAreaOpen = !!fullAreaSearch.trim()"
                @blur="onFullAreaBlur"
                @keydown.enter="pickFullAreaFirst"
              />
            </div>
            <div v-if="fullAreaOpen && fullAreaSearch.trim()" class="search-drop">
              <div class="search-drop__head">Wilayah</div>
              <li v-if="fullAreaLoading" class="search-drop__empty">Mencari wilayah...</li>
              <template v-else>
                <li
                  v-for="a in fullAreaResults"
                  :key="'fa' + a.osm_id"
                  @mousedown.prevent="focusFromFullArea(a)"
                >
                  <strong>{{ a.name || a.display_name }}</strong>
                  <span class="search-drop__meta search-drop__meta--ellipsis">{{ a.display_name }}</span>
                </li>
                <li v-if="!fullAreaResults.length" class="search-drop__empty">
                  Wilayah tidak ditemukan
                </li>
              </template>
            </div>
          </div>
        </div>

        <div class="map-full__panel">
          <div class="map-full__search-wrap">
            <div class="search">
              <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/></svg>
              <input
                v-model="fullSearch"
                placeholder="Cari kode, nama, atau IP..."
                @focus="fullSearchOpen = true"
                @input="fullSearchOpen = !!fullSearch.trim()"
                @blur="onFullSearchBlur"
                @keydown.enter="pickFullFirst"
              />
            </div>
            <div v-if="fullSearchOpen && (fullSearch.trim() || fullResults.length)" class="search-drop">
              <div class="search-drop__head">Pelanggan</div>
              <template v-if="fullResults.length">
                <li
                  v-for="c in fullResults"
                  :key="'f' + c.id"
                  @mousedown.prevent="focusFromFull(c)"
                >
                  <strong>{{ c.customer_name }}</strong>
                  <span class="search-drop__meta">{{ c.customer_code }} · {{ c.ip_address }} · {{ c.status }}</span>
                </li>
              </template>
              <li v-else class="search-drop__empty">Tidak ada pelanggan cocok</li>
            </div>
          </div>

          <div class="map-full__list-head">
            <span>Daftar Pelanggan</span>
            <span class="map-full__count">{{ fullList.length }}</span>
          </div>
          <ul class="map-full__items">
            <li
              v-for="c in fullList"
              :key="c.id"
              class="map-full__item"
              @click="focusFromFull(c)"
            >
              <StatusBadge :status="c.status" />
              <div class="map-full__item-main">
                <strong>{{ c.customer_name }}</strong>
                <span>{{ c.customer_code }} · {{ c.ip_address }} · {{ c.vpn_name || '-' }}</span>
              </div>
              <span v-if="c.latency_ms != null" class="map-full__ms">{{ c.latency_ms }} ms</span>
            </li>
            <li v-if="!fullList.length" class="map-full__empty">Tidak ada pelanggan</li>
          </ul>
        </div>
      </div>

      <div class="map-full__legend legend">
        <div class="legend__item"><span class="legend__dot legend__dot--online" /> ONLINE</div>
        <div class="legend__item"><span class="legend__dot legend__dot--offline" /> OFFLINE</div>
        <div class="legend__item"><span class="legend__dot legend__dot--warning" /> WARNING</div>
      </div>
    </div>

    <div v-if="showAddModal" class="modal-mask" :class="{ 'modal-mask--top': mapExpanded }" @click.self="cancelAdd">
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
            <label class="field"><span>VPN (opsional)</span>
              <select v-model="addForm.vpn_id">
                <option value="">— Tanpa VPN (via default route) —</option>
                <option v-for="v in monitor.vpns" :key="v.id" :value="v.id">
                  {{ v.name }} <template v-if="v.interface_name">({{ v.interface_name }})</template>
                </option>
              </select>
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

    <div v-if="showFeatureModal" class="modal-mask" :class="{ 'modal-mask--top': mapExpanded }" @click.self="closeFeatureModal">
      <div class="modal modal--sm">
        <h3>{{ featureEditId ? 'Kelola Fitur' : 'Simpan Fitur Peta' }}</h3>
        <p v-if="featureDraft" class="modal__coord">
          {{ featureDraft.isPoint ? 'Titik informasi' : featureDraft.geometry.type === 'LineString' ? 'Jalur / routing' : 'Area / wilayah' }}
        </p>
        <form @submit.prevent="saveFeature">
          <div class="form-grid form-grid--single">
            <label class="field"><span>Nama *</span>
              <input v-model="featureForm.name" placeholder="mis. Jalur Lantai-3, Titik Info JB" required autofocus />
            </label>
            <div v-if="featureDraft && featureDraft.isPoint" class="field">
              <span>Ikon</span>
              <div class="icon-picker">
                <button
                  v-for="ic in FEATURE_ICONS"
                  :key="ic.value"
                  type="button"
                  class="icon-picker__item"
                  :class="{ 'icon-picker__item--active': featureForm.icon === ic.value }"
                  :style="`--fm-color:${featureForm.color}`"
                  :title="ic.label"
                  @click="featureForm.icon = ic.value"
                >
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path v-if="ic.value === 'dot'" d="M12 12m-5 0a5 5 0 1 0 10 0a5 5 0 1 0-10 0" fill="var(--fm-color)" stroke="none"/>
                    <g v-else-if="ic.value === 'customer'"><circle cx="12" cy="8" r="4" fill="var(--fm-color)" stroke="none"/><path d="M4 20c0-4 4-6 8-6s8 2 8 6" fill="var(--fm-color)" stroke="none"/></g>
                    <g v-else-if="ic.value === 'wifi'"><path d="M4.5 11.5a11 11 0 0 1 15 0"/><path d="M7.5 15.5a6.4 6.4 0 0 1 9 0"/><path d="M10.6 19.4a3 3 0 0 1 2.8 0"/></g>
                    <path v-else-if="ic.value === 'jb'" d="M6 2.5h12v18.5l-6-4.2-6 4.2z" fill="var(--fm-color)" stroke="none"/>
                    <g v-else-if="ic.value === 'router'"><rect x="3" y="11" width="18" height="7.5" rx="2" fill="var(--fm-color)" stroke="none"/><path d="M7.5 15.2h.01M11 15.2h.01M16.6 7.4a6 6 0 0 1 0 3.4"/></g>
                  </svg>
                  <span>{{ ic.label }}</span>
                </button>
              </div>
            </div>
            <div class="field">
              <span>Warna</span>
              <div class="color-picker">
                <button
                  v-for="c in FEATURE_COLORS"
                  :key="c"
                  type="button"
                  class="color-picker__swatch"
                  :class="{ 'color-picker__swatch--active': featureForm.color === c }"
                  :style="`background:${c}`"
                  :title="c"
                  @click="featureForm.color = c"
                ></button>
              </div>
            </div>
            <label class="field"><span>Deskripsi (opsional)</span>
              <textarea v-model="featureForm.description" rows="2" placeholder="Keterangan singkat"></textarea>
            </label>
          </div>
          <p v-if="featureError" class="login__error">{{ featureError }}</p>
          <div class="modal__actions">
            <button v-if="featureEditId" type="button" class="btn btn--danger" :disabled="featureSaving" @click="deleteFeature">
              {{ featureSaving ? '...' : 'Hapus' }}
            </button>
            <span class="spacer"></span>
            <button type="button" class="btn btn--ghost" @click="closeFeatureModal">Batal</button>
            <button type="submit" class="btn btn--primary" :disabled="featureSaving">
              {{ featureSaving ? 'Menyimpan...' : 'Simpan' }}
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

.map-expand {
  position: absolute;
  top: 10px;
  right: 10px;
  z-index: 900;
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  background: var(--bg-panel-2);
  border: 1px solid var(--border);
  border-radius: 8px;
  color: var(--text-dim);
  cursor: pointer;
  transition: color 0.15s, background 0.15s;
}

.modal-mask--top {
  z-index: 3100;
}

.map-expand:hover {
  color: var(--text);
  background: var(--bg-hover);
}

.map-expand svg {
  width: 16px;
  height: 16px;
}

.map-full {
  position: fixed;
  inset: 0;
  z-index: 3000;
  background: var(--bg);
}

.map-full__top {
  position: absolute;
  top: 12px;
  left: 12px;
  right: 12px;
  display: flex;
  align-items: flex-start;
  gap: 12px;
  z-index: 1500;
  pointer-events: none;
}

.map-full :deep(.leaflet-top.leaflet-left .leaflet-control-zoom) {
  margin-top: 56px;
}

.map-full__back {
  pointer-events: auto;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: rgba(22, 33, 58, 0.92);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 8px 14px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  flex-shrink: 0;
}

.map-full__back:hover {
  background: var(--bg-hover);
}

.map-full__panel {
  pointer-events: auto;
  width: 320px;
  max-width: calc(100vw - 24px);
  max-height: calc(100vh - 80px);
  background: rgba(22, 33, 58, 0.92);
  border: 1px solid var(--border);
  border-radius: 10px;
  backdrop-filter: blur(6px);
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.4);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.map-full__area {
  pointer-events: auto;
  flex: 1;
  min-width: 0;
  display: flex;
  justify-content: center;
}

.map-full__area-wrap {
  position: relative;
  width: 100%;
  max-width: 380px;
}

.map-full__search-wrap {
  position: relative;
  padding: 10px 10px 6px;
}

.map-full__list-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 14px 4px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-faint);
}

.map-full__count {
  background: var(--bg-panel-2);
  border-radius: 999px;
  padding: 1px 8px;
  color: var(--text-dim);
}

.map-full__items {
  list-style: none;
  margin: 0;
  padding: 4px 6px 10px;
  overflow-y: auto;
  flex: 1;
  min-height: 0;
}

.map-full__item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px;
  border-radius: 8px;
  cursor: pointer;
}

.map-full__item:hover {
  background: var(--bg-hover);
}

.map-full__item-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.map-full__item-main strong {
  font-size: 12.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.map-full__item-main span {
  font-size: 11px;
  color: var(--text-dim);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.map-full__ms {
  font-family: "SF Mono", Menlo, monospace;
  font-size: 11px;
  color: var(--text-dim);
}

.map-full__empty {
  padding: 18px;
  text-align: center;
  color: var(--text-faint);
  font-size: 13px;
}

.map-full__legend {
  position: absolute;
  bottom: 14px;
  left: 14px;
}

@media (max-width: 640px) {
  .map-full__top {
    flex-direction: column;
    align-items: stretch;
  }

  .map-full__back {
    align-self: flex-start;
  }

  .map-full__area {
    width: 100%;
  }

  .map-full__area-wrap {
    max-width: none;
  }

  .map-full__panel {
    width: 100%;
    max-width: none;
    max-height: 45vh;
  }
}
</style>
