<script setup>
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import { MarkerClusterer } from '@googlemaps/markerclusterer'

const props = defineProps({
  customers: { type: Array, default: () => [] },
  statusFilter: { type: String, default: 'ALL' },
  vpnFilter: { type: String, default: 'ALL' },
  clickToAdd: { type: Boolean, default: false },
  draftPoint: { type: Object, default: null }
})

const emit = defineEmits(['open-detail', 'map-click'])

const mapEl = ref(null)
const loadError = ref('')

let gmaps = null
let map = null
let clusterer = null
let infoWindow = null
let draftMarker = null
const markerMap = new Map()

const STATUS_COLORS = {
  ONLINE: '#22c55e',
  OFFLINE: '#ef4444',
  WARNING: '#eab308'
}

const DEFAULT_CENTER = { lat: -2.5, lng: 118 }
const DEFAULT_ZOOM = 5

const filtered = computed(() => {
  return props.customers.filter((c) => {
    if (props.statusFilter !== 'ALL' && c.status !== props.statusFilter) return false
    if (props.vpnFilter !== 'ALL') {
      const vpnName = c.vpn_name || ''
      const matchesId = String(c.vpn_id || '') === props.vpnFilter
      if (!matchesId && vpnName !== props.vpnFilter) return false
    }
    return true
  })
})

let loaderPromise = null
function loadGoogleMaps(apiKey) {
  if (window.google?.maps) return Promise.resolve(window.google.maps)
  if (!apiKey) return Promise.reject(new Error('VITE_GOOGLE_MAPS_API_KEY belum diisi'))
  if (loaderPromise) return loaderPromise
  loaderPromise = new Promise((resolve, reject) => {
    const cb = '__gmapsCallback'
    window[cb] = () => {
      delete window[cb]
      resolve(window.google.maps)
    }
    const script = document.createElement('script')
    script.src = `https://maps.googleapis.com/maps/api/js?key=${encodeURIComponent(apiKey)}&libraries=marker&callback=${cb}&loading=async&v=weekly`
    script.async = true
    script.onerror = () => reject(new Error('Gagal memuat Google Maps'))
    document.head.appendChild(script)
  })
  return loaderPromise
}

function statusIcon(status) {
  return {
    path: gmaps.SymbolPath.CIRCLE,
    fillColor: STATUS_COLORS[status] || '#94a3b8',
    fillOpacity: 1,
    strokeColor: '#ffffff',
    strokeWeight: 2,
    scale: status === 'WARNING' ? 9 : 7
  }
}

function draftIcon() {
  return {
    path: gmaps.SymbolPath.CIRCLE,
    fillColor: '#3b82f6',
    fillOpacity: 1,
    strokeColor: '#ffffff',
    strokeWeight: 2,
    scale: 10
  }
}

function escapeHtml(value) {
  return String(value ?? '').replace(/[&<>"']/g, (ch) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[ch]))
}

function popupFor(customer) {
  const when = customer.last_check
    ? new Date(customer.last_check).toLocaleTimeString('id-ID')
    : '-'
  return `
    <div class="map-popup">
      <div class="map-popup__head">
        <span class="map-popup__status" style="background:${STATUS_COLORS[customer.status] || '#94a3b8'}"></span>
        <strong>${escapeHtml(customer.customer_name || customer.customer_code)}</strong>
      </div>
      <div class="map-popup__code">${escapeHtml(customer.customer_code)}</div>
      <table class="map-popup__table">
        <tr><td>IP Address</td><td>${escapeHtml(customer.ip_address || '-')}</td></tr>
        <tr><td>VPN</td><td>${escapeHtml(customer.vpn_name || '-')}</td></tr>
        <tr><td>Status</td><td>${escapeHtml(customer.status)}</td></tr>
        <tr><td>Latency</td><td>${customer.latency_ms != null ? customer.latency_ms + ' ms' : '-'}</td></tr>
        <tr><td>Last Check</td><td>${when}</td></tr>
        <tr><td>Uptime</td><td>${escapeHtml(customer.uptime_percentage ?? '-')}%</td></tr>
      </table>
      <button class="map-popup__btn" onclick="window.__fmCustomer(${customer.id})">Lihat Detail</button>
    </div>`
}

function openInfo(customer, marker) {
  if (!infoWindow) return
  infoWindow.setContent(popupFor(customer))
  infoWindow.open({ map, anchor: marker })
}

function rebuildMarkers() {
  if (!map || !clusterer || !gmaps) return
  clusterer.clearMarkers()
  markerMap.forEach((m) => m.setMap(null))
  markerMap.clear()

  const markers = []
  filtered.value.forEach((c) => {
    if (c.latitude == null || c.longitude == null) return
    const marker = new gmaps.Marker({
      position: { lat: Number(c.latitude), lng: Number(c.longitude) },
      icon: statusIcon(c.status),
      title: c.customer_name || c.customer_code,
      optimized: true
    })
    marker.addListener('click', () => openInfo(c, marker))
    markers.push(marker)
    markerMap.set(c.id, marker)
  })
  clusterer.addMarkers(markers)
}

function focusCustomer(customer) {
  if (!map || customer.latitude == null || customer.longitude == null) return
  const position = { lat: Number(customer.latitude), lng: Number(customer.longitude) }
  map.panTo(position)
  map.setZoom(16)
  const marker = markerMap.get(customer.id)
  if (marker) {
    setTimeout(() => openInfo(customer, marker), 400)
  }
}

function renderDraft() {
  if (!map || !gmaps) return
  if (draftMarker) {
    draftMarker.setMap(null)
    draftMarker = null
  }
  if (props.draftPoint) {
    draftMarker = new gmaps.Marker({
      position: { lat: props.draftPoint.lat, lng: props.draftPoint.lng },
      icon: draftIcon(),
      zIndex: 100000
    })
    draftMarker.setMap(map)
  }
}

function openCustomerFromWindow(id) {
  const c = props.customers.find((x) => x.id === id)
  if (c) emit('open-detail', c)
}

function initMap() {
  map = new gmaps.Map(mapEl.value, {
    center: DEFAULT_CENTER,
    zoom: DEFAULT_ZOOM,
    mapTypeId: 'hybrid',
    mapTypeControl: true,
    mapTypeControlOptions: {
      style: gmaps.MapTypeControlStyle.HORIZONTAL_BAR,
      position: gmaps.ControlPosition.TOP_RIGHT,
      mapTypeIds: ['roadmap', 'hybrid', 'satellite', 'terrain']
    },
    fullscreenControl: true,
    streetViewControl: false,
    clickableIcons: false,
    gestureHandling: 'greedy'
  })

  infoWindow = new gmaps.InfoWindow()
  clusterer = new MarkerClusterer({ map, markers: [] })

  map.addListener('click', (e) => {
    if (!props.clickToAdd) return
    emit('map-click', { lat: e.latLng.lat(), lng: e.latLng.lng() })
  })

  rebuildMarkers()
  renderDraft()
}

onMounted(async () => {
  window.__fmCustomer = openCustomerFromWindow
  try {
    gmaps = await loadGoogleMaps(import.meta.env.VITE_GOOGLE_MAPS_API_KEY)
    initMap()
  } catch (e) {
    loadError.value = e.message || 'Gagal memuat peta'
  }
})

onUnmounted(() => {
  if (window.__fmCustomer === openCustomerFromWindow) delete window.__fmCustomer
  if (draftMarker) { draftMarker.setMap(null); draftMarker = null }
  if (clusterer) { clusterer.clearMarkers(true); clusterer = null }
  if (infoWindow) { infoWindow.close(); infoWindow = null }
  if (map) {
    gmaps?.event?.clearInstanceListeners(map)
    map = null
  }
  markerMap.clear()
  gmaps = null
})

watch(filtered, () => rebuildMarkers(), { deep: true })
watch(() => props.draftPoint, renderDraft)

defineExpose({ focusCustomer })
</script>

<template>
  <div class="monitor-map-wrap">
    <div ref="mapEl" class="monitor-map" :class="{ 'monitor-map--clickable': clickToAdd }"></div>
    <div v-if="loadError" class="monitor-map__error">
      <strong>Peta Google tidak dapat dimuat</strong>
      <span>{{ loadError }}</span>
      <span>Isi <code>VITE_GOOGLE_MAPS_API_KEY</code> pada file <code>.env</code> lalu rebuild frontend.</span>
    </div>
  </div>
</template>
