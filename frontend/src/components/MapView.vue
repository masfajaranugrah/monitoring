<script setup>
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import L from 'leaflet'
import 'leaflet.markercluster'

const props = defineProps({
  customers: { type: Array, default: () => [] },
  statusFilter: { type: String, default: 'ALL' },
  vpnFilter: { type: String, default: 'ALL' }
})

const emit = defineEmits(['open-detail'])

const mapEl = ref(null)
let map = null
let markers = null
const markerMap = new Map()

const STATUS_COLORS = {
  ONLINE: '#22c55e',
  OFFLINE: '#ef4444',
  WARNING: '#eab308'
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

function popupFor(customer) {
  const when = customer.last_check
    ? new Date(customer.last_check).toLocaleTimeString('id-ID')
    : '-'
  return `
    <div class="map-popup">
      <div class="map-popup__head">
        <span class="map-popup__status" style="background:${STATUS_COLORS[customer.status] || '#94a3b8'}"></span>
        <strong>${customer.customer_name || customer.customer_code}</strong>
      </div>
      <div class="map-popup__code">${customer.customer_code}</div>
      <table class="map-popup__table">
        <tr><td>IP Address</td><td>${customer.ip_address || '-'}</td></tr>
        <tr><td>VPN</td><td>${customer.vpn_name || '-'}</td></tr>
        <tr><td>Status</td><td>${customer.status}</td></tr>
        <tr><td>Latency</td><td>${customer.latency_ms != null ? customer.latency_ms + ' ms' : '-'}</td></tr>
        <tr><td>Last Check</td><td>${when}</td></tr>
        <tr><td>Uptime</td><td>${customer.uptime_percentage ?? '-'}%</td></tr>
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
    m.bindPopup(popupFor(c), { maxWidth: 260, autoPanPadding: [30, 30] })
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

function openCustomerFromWindow(id) {
  const c = props.customers.find((x) => x.id === id)
  if (c) emit('open-detail', c)
}

onMounted(() => {
  window.__fmCustomer = openCustomerFromWindow

  map = L.map(mapEl.value, {
    center: [-2.5, 118],
    zoom: 5,
    zoomControl: true
  })

  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19,
    attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
  }).addTo(map)

  markers = L.markerClusterGroup({ disableClusteringAtZoom: 10, chunkedLoading: true })
  map.addLayer(markers)
  rebuildMarkers()
})

onUnmounted(() => {
  if (window.__fmCustomer === openCustomerFromWindow) delete window.__fmCustomer
  if (map) {
    map.remove()
    map = null
    markers = null
    markerMap.clear()
  }
})

watch(filtered, () => rebuildMarkers(), { deep: true })

defineExpose({ focusCustomer })
</script>

<template>
  <div ref="mapEl" class="monitor-map"></div>
</template>