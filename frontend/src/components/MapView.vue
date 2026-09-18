<script setup>
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import L from 'leaflet'
import 'leaflet.markercluster'

const props = defineProps({
  customers: { type: Array, default: () => [] },
  statusFilter: { type: String, default: 'ALL' },
  vpnFilter: { type: String, default: 'ALL' },
  clickToAdd: { type: Boolean, default: false },
  draftPoint: { type: Object, default: null },
  initialView: { type: Object, default: null }
})

const emit = defineEmits(['open-detail', 'map-click'])

const mapEl = ref(null)
let map = null
let markers = null
let draftMarker = null
let areaMarker = null
const markerMap = new Map()

const STATUS_COLORS = {
  ONLINE: '#22c55e',
  OFFLINE: '#ef4444',
  WARNING: '#eab308'
}

const GOOGLE_TILES_OPTS = {
  maxZoom: 20,
  maxNativeZoom: 20,
  subdomains: ['mt0', 'mt1', 'mt2', 'mt3']
}

const BASE_LAYERS = {
  'Google Streets': L.tileLayer('https://{s}.google.com/vt/lyrs=m&x={x}&y={y}&z={z}', {
    ...GOOGLE_TILES_OPTS,
    attribution: '&copy; Google'
  }),
  'Google Satelit': L.tileLayer('https://{s}.google.com/vt/lyrs=s&x={x}&y={y}&z={z}', {
    ...GOOGLE_TILES_OPTS,
    attribution: '&copy; Google'
  }),
  'Google Hybrid': L.tileLayer('https://{s}.google.com/vt/lyrs=s,h&x={x}&y={y}&z={z}', {
    ...GOOGLE_TILES_OPTS,
    attribution: '&copy; Google'
  }),
  'Google Terrain': L.tileLayer('https://{s}.google.com/vt/lyrs=p&x={x}&y={y}&z={z}', {
    ...GOOGLE_TILES_OPTS,
    attribution: '&copy; Google'
  }),
  'OpenStreetMap': L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19,
    attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
  })
}

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

function divIcon(customer) {
  const color = STATUS_COLORS[customer.status] || '#94a3b8'
  const pulse = customer.status === 'WARNING' ? ' map-marker--pulse' : ''
  return L.divIcon({
    className: '',
    html: `<div class="map-marker${pulse}" style="--marker-color:${color}"><span class="map-marker__inner"></span></div>`,
    iconSize: [22, 22],
    iconAnchor: [11, 11],
    popupAnchor: [0, -14]
  })
}

function draftIcon() {
  return L.divIcon({
    className: '',
    html: '<div class="map-marker map-marker--draft"><span class="map-marker__inner"></span></div>',
    iconSize: [22, 22],
    iconAnchor: [11, 11]
  })
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

function rebuildMarkers() {
  if (!map || !markers) return
  markers.clearLayers()
  markerMap.clear()

  filtered.value.forEach((c) => {
    if (c.latitude == null || c.longitude == null) return
    const m = L.marker([c.latitude, c.longitude], { icon: divIcon(c) })
    m.bindPopup(popupFor(c), { maxWidth: 260, autoPanPadding: [30, 30], className: 'map-popup-shell' })
    markers.addLayer(m)
    markerMap.set(c.id, m)
  })
}

function focusCustomer(customer) {
  if (!map) return
  map.flyTo([customer.latitude, customer.longitude], 14, { duration: 1.2 })
  const m = markerMap.get(customer.id)
  if (m) {
    setTimeout(() => {
      map.closePopup()
      m.openPopup()
    }, 1400)
  }
}

function flyTo(lat, lng, zoom = 12, label = '') {
  if (!map) return
  if (areaMarker) {
    map.removeLayer(areaMarker)
    areaMarker = null
  }
  map.flyTo([lat, lng], zoom, { duration: 1.2 })
  if (label) {
    areaMarker = L.marker([lat, lng], {
      icon: L.divIcon({
        className: '',
        html: '<div class="map-marker map-marker--area" style="--marker-color:var(--primary)"><span class="map-marker__inner"></span></div>',
        iconSize: [22, 22],
        iconAnchor: [11, 22]
      }),
      zIndexOffset: 2000
    })
    areaMarker.bindPopup(`<div class="map-popup"><strong>${escapeHtml(label)}</strong></div>`, {
      maxWidth: 260,
      className: 'map-popup-shell'
    })
    areaMarker.addTo(map)
    setTimeout(() => {
      map.closePopup()
      if (areaMarker) areaMarker.openPopup()
    }, 1400)
  }
}

function onMapClick(e) {
  if (!props.clickToAdd) return
  emit('map-click', { lat: e.latlng.lat, lng: e.latlng.lng })
}

function renderDraft() {
  if (!map) return
  if (draftMarker) {
    map.removeLayer(draftMarker)
    draftMarker = null
  }
  if (props.draftPoint) {
    draftMarker = L.marker([props.draftPoint.lat, props.draftPoint.lng], {
      icon: draftIcon(),
      interactive: false,
      zIndexOffset: 1000
    })
    map.addLayer(draftMarker)
  }
}

function openCustomerFromWindow(id) {
  const c = props.customers.find((x) => x.id === id)
  if (c) emit('open-detail', c)
}

function getView() {
  if (!map) return { lat: -2.5, lng: 118, zoom: 5 }
  const c = map.getCenter()
  return { lat: c.lat, lng: c.lng, zoom: map.getZoom() }
}

let prevCustomerHandler = null

onMounted(() => {
  prevCustomerHandler = window.__fmCustomer
  window.__fmCustomer = openCustomerFromWindow

  const init = props.initialView
  map = L.map(mapEl.value, {
    center: init && init.lat != null && init.lng != null ? [init.lat, init.lng] : [-2.5, 118],
    zoom: init && init.zoom ? init.zoom : 5,
    zoomControl: true
  })

  L.control
    .layers(BASE_LAYERS, null, { position: 'bottomright', collapsed: false })
    .addTo(map)

  BASE_LAYERS['OpenStreetMap'].addTo(map)

  markers = L.markerClusterGroup({ disableClusteringAtZoom: 10, chunkedLoading: true })
  map.addLayer(markers)
  map.on('click', onMapClick)
  rebuildMarkers()
  renderDraft()
})

onUnmounted(() => {
  if (window.__fmCustomer === openCustomerFromWindow) {
    window.__fmCustomer = prevCustomerHandler
  }
  if (map) {
    map.off('click', onMapClick)
    map.remove()
    map = null
    markers = null
    draftMarker = null
    areaMarker = null
    markerMap.clear()
  }
})

watch(filtered, () => rebuildMarkers(), { deep: true })
watch(() => props.draftPoint, renderDraft)

defineExpose({ focusCustomer, flyTo, getView })
</script>

<template>
  <div class="monitor-map-wrap">
    <div ref="mapEl" class="monitor-map" :class="{ 'monitor-map--clickable': clickToAdd }"></div>
  </div>
</template>
