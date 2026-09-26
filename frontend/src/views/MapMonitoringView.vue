<script setup>
import { ref, onMounted, computed, onBeforeUnmount, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import MapView from '../components/MapView.vue'
import { useMonitorStore } from '../stores/monitor'
import { useAreaSearch } from '../composables/useAreaSearch'
import { customerIconSvg, statusColor } from '../services/customerIcons'
import { FEATURE_ICONS, FEATURE_COLORS } from '../services/featureIcons'
import { formatDistance } from '../services/geo'
import { mapsUrl, waShareUrl, downFor } from '../services/share'

const router = useRouter()
const monitor = useMonitorStore()

const customers = ref([])
const statusFilter = ref('ALL')
const vpnFilter = ref('ALL')
const search = ref('')
const searchOpen = ref(false)
const loading = ref(true)
const mapView = ref(null)
const { areaResults, areaLoading, clearArea, areaZoom } = useAreaSearch(search)

const drawOpen = ref(false)
const drawTool = ref('')
const mapWide = ref(false)

const featureEditId = ref(null)
const featureDraft = ref(null)
const showFeatureModal = ref(false)
const featureForm = ref({ name: '', icon: 'dot', color: '#3b82f6', description: '' })
const featureSaving = ref(false)
const featureError = ref('')

const measureTool = ref(false)
const measure = ref({ active: false, count: 0, totalM: 0, segments: [], points: [] })

function onMeasureChange(payload) {
  measure.value = payload || { active: false, count: 0, totalM: 0, segments: [], points: [] }
  measureTool.value = !!measure.value.active
}

function toggleMeasure() {
  if (measureTool.value) {
    measureTool.value = false
    mapView.value?.cancelMeasure({ keepResult: true })
    return
  }
  drawTool.value = ''
  measureTool.value = true
  drawOpen.value = true
  mapView.value?.startMeasure()
}

function clearMeasure() {
  mapView.value?.resetMeasure()
  measure.value = { active: false, count: 0, totalM: 0, segments: [], points: [] }
}

function saveMeasureAsRoute() {
  const pts = measure.value.points
  if (!pts || pts.length < 2) return
  const coords = pts.map((p) => [p.lng, p.lat])
  const name = `Jalur ${formatDistance(measure.value.totalM)}`
  measureTool.value = false
  mapView.value?.cancelMeasure()
  featureDraft.value = {
    isPoint: false,
    geometry: { type: 'LineString', coordinates: coords }
  }
  featureEditId.value = null
  featureForm.value = {
    name,
    icon: 'dot',
    color: FEATURE_COLORS[0],
    description: ''
  }
  featureError.value = ''
  showFeatureModal.value = true
}

function onFeatureManage(feature) {
  featureEditId.value = feature.id
  featureDraft.value = {
    isPoint: feature.feature_type === 'point',
    geometry: feature.geometry
  }
  featureForm.value = {
    name: feature.name || '',
    icon: feature.icon || 'dot',
    color: feature.color || FEATURE_COLORS[0],
    description: feature.description || ''
  }
  featureError.value = ''
  showFeatureModal.value = true
}

function closeFeatureModal() {
  showFeatureModal.value = false
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
    description: featureForm.value.description.trim()
  }
  try {
    if (featureEditId.value) {
      await api.patch(`/map/features/${featureEditId.value}`, payload)
    } else {
      await api.post('/map/features', { ...payload, geometry: featureDraft.value.geometry })
    }
    closeFeatureModal()
    featureDraft.value = null
    mapView.value?.refreshFeatures()
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
  } catch (e) {
    featureError.value = e.response?.data?.error || 'Gagal menghapus fitur'
  } finally {
    featureSaving.value = false
  }
}

function toggleDrawTools() {
  if (drawOpen.value) closeDrawTools()
  else {
    drawOpen.value = true
    mapWide.value = true
  }
}

function closeDrawTools() {
  drawTool.value = ''
  measureTool.value = false
  if (mapView.value) mapView.value.cancelDraw()
  mapView.value?.cancelMeasure({ keepResult: true })
  drawOpen.value = false
  mapWide.value = false
}

function toggleDraw(mode) {
  if (drawTool.value === mode) {
    closeDrawTools()
    return
  }
  if (measureTool.value) mapView.value?.cancelMeasure()
  if (drawTool.value) mapView.value?.cancelDraw()
  measureTool.value = false
  drawOpen.value = true
  drawTool.value = mode
  mapWide.value = true
  mapView.value?.startDraw(mode)
}

function finishDrawNow() {
  mapView.value?.finishDrawNow()
  drawTool.value = ''
  drawOpen.value = false
  mapWide.value = false
}

function onDrawComplete(payload) {
  drawTool.value = ''
  drawOpen.value = false
  mapWide.value = false
  if (payload.type === 'point') {
    featureDraft.value = {
      isPoint: true,
      geometry: { type: 'Point', coordinates: [payload.latlng.lng, payload.latlng.lat] }
    }
  } else if (payload.type === 'line') {
    featureDraft.value = {
      isPoint: false,
      geometry: { type: 'LineString', coordinates: payload.latlngs.map((l) => [l.lng, l.lat]) }
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
  featureEditId.value = null
  featureForm.value = { name: '', icon: 'dot', color: FEATURE_COLORS[0], description: '' }
  featureError.value = ''
  showFeatureModal.value = true
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

const STATUS_RANK = { OFFLINE: 0, WARNING: 1, ONLINE: 2 }

const filteredList = computed(() => {
  const q = search.value.toLowerCase().trim()
  let list = q
    ? customers.value.filter(
        (c) =>
          c.customer_code?.toLowerCase().includes(q) ||
          c.customer_name?.toLowerCase().includes(q) ||
          c.ip_address?.toLowerCase().includes(q)
      )
    : customers.value.slice()
  list.sort((a, b) => {
    const ra = STATUS_RANK[a.status] ?? 3
    const rb = STATUS_RANK[b.status] ?? 3
    if (ra !== rb) return ra - rb
    if (a.status === 'OFFLINE') {
      const ta = a.last_offline ? new Date(a.last_offline).getTime() : 0
      const tb = b.last_offline ? new Date(b.last_offline).getTime() : 0
      return ta - tb
    }
    return 0
  })
  return list
})

function openDetail(customer) {
  router.push(`/customers/${customer.id}`)
}

function focusOnList(customer) {
  mapView.value?.focusCustomer(customer)
}

function focusFromArea(a) {
  mapView.value?.flyTo(Number(a.lat), Number(a.lon), areaZoom(a), a.display_name)
  clearArea()
  search.value = ''
}

function onSearchBlur() {
  setTimeout(() => {
    searchOpen.value = false
  }, 150)
}

function pickAreaFirst() {
  if (filteredList.value.length === 1) {
    focusOnList(filteredList.value[0])
  } else if (areaResults.value.length) {
    focusFromArea(areaResults.value[0])
  }
}

watch(filteredList, (list) => {
  if (list.length !== 1 || !search.value.trim()) return
  const c = list[0]
  if (c.latitude != null && c.longitude != null) {
    mapView.value?.focusCustomer(c)
  }
})

onMounted(() => {
  load()
  monitor.fetchStats()
})

onBeforeUnmount(() => {
  mapView.value = null
})
</script>

<template>
  <div class="mapmon" :class="{ 'mapmon--wide': mapWide }">
    <div class="mapmon__toolbar panel">
      <div class="search-wrap">
        <div class="search">
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/></svg>
          <input
            v-model="search"
            placeholder="Cari kode, nama, IP, atau wilayah..."
            @focus="searchOpen = true"
            @input="searchOpen = true"
            @blur="onSearchBlur"
            @keydown.enter="pickAreaFirst"
          />
        </div>

        <div v-if="searchOpen && search.trim()" class="search-drop">
          <div class="search-drop__head">Wilayah</div>
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
          v-for="s in ['ALL', 'ONLINE', 'OFFLINE', 'WARNING']"
          :key="s"
          class="filter-chip"
          :class="{ 'filter-chip--active': statusFilter === s, [`filter-chip--${s.toLowerCase()}`]: statusFilter === s }"
          @click="statusFilter = s; load()"
        >
          {{ s }}
        </button>
      </div>

      <select v-model="vpnFilter" class="select" @change="load()">
        <option value="ALL">ALL VPN</option>
        <option v-for="v in monitor.vpns" :key="v.id" :value="v.id">{{ v.name }}</option>
      </select>
    </div>

    <div class="mapmon__body">
      <div class="mapmon__map">
        <MapView
          ref="mapView"
          :customers="customers"
          :status-filter="statusFilter"
          :vpn-filter="vpnFilter"
          :manage-features="true"
          @open-detail="openDetail"
          @feature-manage="onFeatureManage"
          @draw-complete="onDrawComplete"
          @measure-change="onMeasureChange"
        />

        <div class="map-draw">
          <button
            type="button"
            class="map-draw__main"
            :class="{ 'map-draw__main--open': drawOpen }"
            @click="toggleDrawTools"
          >
            <svg class="icon icon--xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 2 15.5 7.5M21 2l-5 5-4-2-2.5 2.5 4 4L8 18l-4 1 2 2 2-1 2.5-2.5 4 4L17 18l-2-4 5-5z"/><path d="M13 11l4-4"/></svg>
            Gambar
          </button>

          <div v-if="drawOpen" class="map-draw__tools">
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
              type="button"
              class="btn btn--ghost btn--sm"
              :class="{ 'map-tools__active': measureTool }"
              @click="toggleMeasure"
            >
              <svg class="icon icon--xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 12h18M3 12l4-4M3 12l4 4M21 12l-4-4M21 12l-4 4"/><circle cx="12" cy="12" r="9"/></svg>
              Cek Jarak
            </button>
            <button
              v-if="drawTool && drawTool !== 'point'"
              type="button"
              class="btn btn--primary btn--sm"
              @click="finishDrawNow"
            >
              Selesai / Simpan
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
        </div>

        <div v-if="measure.count >= 1" class="measure-panel">
          <div class="measure-panel__head">
            <span>Hasil Cek Jarak</span>
            <button type="button" class="measure-panel__close" title="Tutup" @click="clearMeasure">✕</button>
          </div>
          <div class="measure-panel__total">
            <strong>{{ formatDistance(measure.totalM) }}</strong>
            <span>{{ measure.count }} titik</span>
          </div>
          <ul v-if="measure.segments.length > 1" class="measure-panel__list">
            <li v-for="(seg, i) in measure.segments" :key="i">
              <span>Segmen {{ i + 1 }}</span>
              <strong>{{ formatDistance(seg) }}</strong>
            </li>
          </ul>
          <div class="measure-panel__actions">
            <button
              type="button"
              class="btn btn--ghost btn--sm"
              :disabled="measureTool"
              @click="toggleMeasure"
            >
              {{ measureTool ? 'Mode ukur aktif' : 'Lanjut ukur' }}
            </button>
            <button
              type="button"
              class="btn btn--primary btn--sm"
              :disabled="measure.count < 2"
              @click="saveMeasureAsRoute"
            >
              Simpan jadi Jalur
            </button>
          </div>
        </div>

        <div class="legend">
          <div class="legend__item"><span class="legend__dot legend__dot--online" /> ONLINE</div>
          <div class="legend__item"><span class="legend__dot legend__dot--offline" /> OFFLINE</div>
          <div class="legend__item"><span class="legend__dot legend__dot--warning" /> WARNING</div>
        </div>
      </div>

      <div class="mapmon__list panel">
        <h3 class="mapmon__list-title">Pelanggan <span>{{ filteredList.length }}</span></h3>
        <ul class="mapmon__list-items">
          <li
            v-for="c in filteredList"
            :key="c.id"
            class="mapmon__item"
            :class="{ 'mapmon__item--offline': c.status === 'OFFLINE' }"
            @click="focusOnList(c)"
          >
            <span
              class="cust-icon"
              :style="{ '--ic-color': statusColor(c.status) }"
              v-html="customerIconSvg(c.icon || 'customer')"
            ></span>
            <div class="mapmon__item-main">
              <strong>{{ c.customer_name }}</strong>
              <span>{{ c.customer_code }} · {{ c.ip_address }} · {{ c.vpn_name || '-' }}</span>
              <span v-if="c.status === 'OFFLINE'" class="mapmon__down">
                Down sejak {{ downFor(c.last_offline) }}
              </span>
            </div>
            <span v-if="c.latency_ms != null && c.status !== 'OFFLINE'" class="mapmon__item-ms">{{ c.latency_ms }} ms</span>
            <div class="mapmon__share">
              <a
                :href="mapsUrl(c.latitude, c.longitude)"
                target="_blank"
                rel="noopener"
                class="mapmon__share-btn"
                title="Buka / share lokasi di Google Maps"
                @click.stop
              >
                <svg class="icon icon--xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 21s-7-5.6-7-11a7 7 0 0 1 14 0c0 5.4-7 11-7 11z"/><circle cx="12" cy="10" r="2.5"/></svg>
              </a>
              <a
                :href="waShareUrl(c.latitude, c.longitude, c.customer_name)"
                target="_blank"
                rel="noopener"
                class="mapmon__share-btn"
                title="Kirim lokasi via WhatsApp"
                @click.stop
              >
                <svg class="icon icon--xs" viewBox="0 0 24 24" fill="currentColor"><path d="M17.5 14.4l-2-1c-.2-.1-.5-.1-.7.1l-.3.5c-.6 1-1.6.9-2.6.4-1-.6-2-1.8-2.2-2.9-.2-.9.2-1.6.8-2.2.2-.2-.1-.5-.2-1-.1-.6-.6-.8-1-.8-.6 0-1.1.2-1 1 .4 1.9 2.2 3.9 4.1 5.1 1 .7 1.8 1.1 2.6 1.2.8.1 1.2-.2 1.4-.7.2-.5.2-1 .1-1.1z"/><path d="M12 2a10 10 0 0 0-8.6 15L2 22l5.2-1.4A10 10 0 1 0 12 2zm0 18.2a8.2 8.2 0 0 1-4.2-1.2l-.3-.2-3.1.8.8-3-.2-.3A8.2 8.2 0 1 1 12 20.2z"/></svg>
              </a>
            </div>
          </li>
          <li v-if="!filteredList.length" class="mapmon__empty">Tidak ada pelanggan</li>
        </ul>
      </div>
    </div>

    <div v-if="showFeatureModal" class="modal-mask" @click.self="closeFeatureModal">
      <div class="modal modal--sm">
        <h3>Kelola Fitur</h3>
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
.search-wrap {
  position: relative;
  flex: 1;
  min-width: 220px;
}

.search-wrap .search {
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

.search-drop__head {
  padding: 8px 10px 4px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-faint);
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

.search-drop__empty {
  padding: 8px 10px;
  color: var(--text-faint);
  font-size: 13px;
}
</style>