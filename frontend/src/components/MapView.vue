<script setup>
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import L from 'leaflet'
import 'leaflet.markercluster'
import { parseKmzFile } from '../services/kmz'
import api from '../api'

const props = defineProps({
  customers: { type: Array, default: () => [] },
  statusFilter: { type: String, default: 'ALL' },
  vpnFilter: { type: String, default: 'ALL' },
  clickToAdd: { type: Boolean, default: false },
  draftPoint: { type: Object, default: null },
  initialView: { type: Object, default: null },
  manageFeatures: { type: Boolean, default: false }
})

const emit = defineEmits(['open-detail', 'map-click', 'draw-complete', 'feature-manage'])

const mapEl = ref(null)
const kmzFileInput = ref(null)
let map = null
let markers = null
let draftMarker = null
let areaMarker = null
let selfMarker = null
let geoWatch = null
let locating = ref(false)
let kmzBusy = false
let kmzDelEl = null
let importedKmzIds = []
const kmzMeta = ref(null)
const markerMap = new Map()
const featureById = new Map()

const features = ref([])
let featureGroup = null

const drawMode = ref('')
const drawVerts = []
let drawPreview = null
let drawSupClicked = false
const drawHintText = computed(() => {
  if (drawMode.value === 'point') return 'Klik peta untuk menempatkan titik informasi'
  if (drawMode.value === 'line') return 'Klik peta untuk menambah titik jalur — klik 2x / klik kanan untuk selesai'
  if (drawMode.value === 'polygon') return 'Klik peta untuk menambah titik area — klik 2x / klik kanan untuk selesai'
  return ''
})

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

// ---------- Ikon per fitur (dot / customer / wifi / jb / router) ----------
const ICON_SVGS = {
  dot: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="6.2" fill="var(--fm-color)"/></svg>',
  customer:
    '<svg viewBox="0 0 24 24"><circle cx="12" cy="8" r="4" fill="var(--fm-color)"/><path d="M4 21c0-4.2 3.6-6.5 8-6.5s8 2.3 8 6.5" fill="var(--fm-color)"/></svg>',
  wifi:
    '<svg viewBox="0 0 24 24" fill="none" stroke="var(--fm-color)" stroke-width="2.2" stroke-linecap="round"><path d="M4.5 11.5a11 11 0 0 1 15 0"/><path d="M7.5 15.5a6.4 6.4 0 0 1 9 0"/><path d="M10.6 19.4a3 3 0 0 1 2.8 0"/></svg>',
  jb: '<svg viewBox="0 0 24 24"><path d="M6 2.5h12v18.5l-6-4.2-6 4.2z" fill="var(--fm-color)"/><circle cx="12" cy="9" r="2.4" fill="#fff"/></svg>',
  router:
    '<svg viewBox="0 0 24 24"><rect x="3" y="11" width="18" height="7.5" rx="2" fill="var(--fm-color)"/><rect x="5.5" y="7" width="13" height="4" rx="1" fill="var(--fm-color)" opacity=".55"/><path d="M7.5 15.2h.01M11 15.2h.01M16.6 7.4a6 6 0 0 1 0 3.4M19 6.5a9 9 0 0 1 0 5" stroke="var(--fm-color)" stroke-width="1.6" fill="none" stroke-linecap="round"/></svg>'
}

function featureDivIcon(f) {
  const color = f.color || '#3b82f6'
  const svg = ICON_SVGS[f.icon] || ICON_SVGS.dot
  const isDot = !f.icon || f.icon === 'dot'
  return L.divIcon({
    className: '',
    html: `<div class="fm-icon ${isDot ? 'fm-icon--dot' : ''}" style="--fm-color:${color}">${svg}</div>`,
    iconSize: [26, 26],
    iconAnchor: [13, 13],
    popupAnchor: [0, -14]
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

function featurePopup(f) {
  const manageBtn = props.manageFeatures
    ? `<button class="map-popup__btn" onclick="window.__fmFeature(${f.id})">Kelola Fitur</button>`
    : ''
  const typeLabel = { point: 'Titik', line: 'Jalur', polygon: 'Area' }[f.feature_type] || 'Fitur'
  return `
    <div class="map-popup">
      <div class="map-popup__head">
        <span class="map-popup__status" style="background:${escapeHtml(f.color || '#3b82f6')}"></span>
        <strong>${escapeHtml(f.name || typeLabel)}</strong>
      </div>
      ${f.description ? `<div class="map-popup__code">${escapeHtml(f.description)}</div>` : ''}
      <table class="map-popup__table">
        <tr><td>Jenis</td><td>${typeLabel}</td></tr>
        <tr><td>Ikon</td><td>${escapeHtml(f.icon || 'dot')}</td></tr>
      </table>
      ${manageBtn}
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

function renderFeatures() {
  if (!map) return
  if (featureGroup) {
    map.removeLayer(featureGroup)
  }
  featureGroup = L.layerGroup().addTo(map)
  featureById.clear()

  features.value.forEach((f) => {
    const gj = { type: 'Feature', properties: {}, geometry: f.geometry }
    if (!f.geometry || !f.geometry.type) return

    featureById.set(f.id, f)

    const isPoint =
      f.feature_type === 'point' || f.geometry.type === 'Point' || f.geometry.type === 'MultiPoint'

    if (isPoint) {
      const coords = f.geometry.type === 'MultiPoint' ? f.geometry.coordinates[0] : f.geometry.coordinates
      if (!coords || coords.length < 2) return
      const mk = L.marker([coords[1], coords[0]], { icon: featureDivIcon(f) })
      mk.bindPopup(featurePopup(f), { maxWidth: 280, autoPanPadding: [30, 30], className: 'map-popup-shell' })
      featureGroup.addLayer(mk)
      return
    }

    const layer = L.geoJSON(gj, {
      style: {
        color: f.color || '#3b82f6',
        weight: 2.5,
        opacity: 0.9,
        fillColor: f.color || '#3b82f6',
        fillOpacity: 0.12
      }
    })
    layer.eachLayer((l) => {
      l.bindPopup(featurePopup(f), { maxWidth: 280, autoPanPadding: [30, 30], className: 'map-popup-shell' })
    })
    featureGroup.addLayer(layer)
  })
}

async function loadFeatures() {
  try {
    const { data } = await api.get('/map/features')
    features.value = data.data || []
    renderFeatures()
  } catch (e) {
    // belum login / gagal: tampilan senyap, fitur kosong
  }
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

// ---------- Alat gambar (jalur / area / titik informasi) ----------
function clearDrawPreview() {
  if (drawPreview) {
    if (map) map.removeLayer(drawPreview)
    drawPreview = null
  }
  drawVerts.length = 0
}

function updateDrawPreview() {
  clearDrawPreview()
  if (drawVerts.length < 2) return
  drawPreview = L.polyline(drawVerts, {
    color: '#f43f5e',
    weight: 3,
    opacity: 0.95,
    dashArray: '6 6'
  }).addTo(map)
}

function startDraw(mode) {
  if (!map || drawMode.value === mode) return
  cancelDraw({ keepMode: false })
  if (mode !== 'line' && mode !== 'polygon' && mode !== 'point') return
  drawMode.value = mode
  drawVerts.length = 0
  if (mode !== 'point' && map.doubleClickZoom) map.doubleClickZoom.disable()
}

function cancelDraw(opts) {
  const keep = opts && opts.keepMode
  clearDrawPreview()
  drawMode.value = ''
  drawSupClicked = false
  if (map && map.doubleClickZoom) map.doubleClickZoom.enable()
  if (!keep) {
    drawVerts.length = 0
  }
}

function finishDraw() {
  if (drawMode.value === 'line' || drawMode.value === 'polygon') {
    if (drawVerts.length < 2) {
      cancelDraw()
      return
    }
    const latlngs = drawVerts.map((v) => ({ lat: v.lat, lng: v.lng }))
    emit('draw-complete', { type: drawMode.value, latlngs })
  }
  cancelDraw()
}

function onMapClick(e) {
  if (drawMode.value === 'point') {
    drawVerts.length = 0
    emit('draw-complete', { type: 'point', latlng: { lat: e.latlng.lat, lng: e.latlng.lng } })
    cancelDraw()
    return
  }
  if (drawMode.value === 'line' || drawMode.value === 'polygon') {
    if (drawSupClicked) return
    drawVerts.push([e.latlng.lat, e.latlng.lng])
    updateDrawPreview()
    return
  }
  if (!props.clickToAdd) return
  emit('map-click', { lat: e.latlng.lat, lng: e.latlng.lng })
}

function onMapDblClick() {
  if (drawMode.value === 'line' || drawMode.value === 'polygon') {
    drawSupClicked = true
    finishDraw()
  }
}

function onMapContextMenu(e) {
  if (drawMode.value === 'line' || drawMode.value === 'polygon') {
    e.originalEvent.preventDefault()
    finishDraw()
  }
}

function onDrawKeydown(e) {
  if (e.key === 'Escape' && drawMode.value) cancelDraw()
}

// ---------- Ikon lokasi saya ----------
function selfIcon() {
  return L.divIcon({
    className: '',
    html: '<div class="map-marker map-marker--self"><span class="map-marker__inner"></span></div>',
    iconSize: [28, 28],
    iconAnchor: [14, 14],
    popupAnchor: [0, -16]
  })
}

function placeSelf(lat, lng, accuracy, follow = false) {
  if (!map) return
  if (selfMarker) map.removeLayer(selfMarker)
  selfMarker = L.marker([lat, lng], { icon: selfIcon(), zIndexOffset: 3000 })
  const acc = accuracy != null ? `akurasi ±${Math.round(accuracy)} m` : ''
  selfMarker.bindPopup(
    `<div class="map-popup"><strong>Lokasi Saya</strong>${acc ? `<div class="map-popup__code">${escapeHtml(acc)}</div>` : ''}</div>`,
    { maxWidth: 260, className: 'map-popup-shell' }
  )
  selfMarker.addTo(map)
  if (!follow) {
    map.flyTo([lat, lng], 16, { duration: 1.2 })
  }
}

function showGeoError(msg) {
  if (!map) return
  map.closePopup()
  const popup = L.popup({ className: 'map-popup-shell', closeButton: false })
    .setLatLng(map.getCenter())
    .setContent(`<div class="map-popup"><strong>${escapeHtml(msg)}</strong></div>`)
    .openOn(map)
  setTimeout(() => popup.remove(), 3200)
}

const LocateControl = L.Control.extend({
  options: { position: 'topleft' },
  onAdd() {
    const btn = L.DomUtil.create('button', 'map-locate-btn leaflet-bar')
    btn.type = 'button'
    btn.title = 'Pusatkan ke lokasi saya'
    btn.setAttribute('aria-label', 'Pusatkan ke lokasi saya')
    btn.innerHTML =
      '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3.2"></circle><path d="M12 2v3.2M12 18.8V22M2 12h3.2M18.8 12H22"></path></svg>'
    btn.addEventListener('click', () => {
      if (!locating.value) locate()
    })
    L.DomEvent.disableClickPropagation(btn)
    return btn
  }
})

function locate() {
  if (!map || locating.value) return
  if (!('geolocation' in navigator)) {
    showGeoError('Geolokasi tidak didukung oleh browser')
    return
  }
  locating.value = true
  if (geoWatch != null) {
    navigator.geolocation.clearWatch(geoWatch)
    geoWatch = null
  }
  const opts = { enableHighAccuracy: true, timeout: 10000, maximumAge: 30000 }
  navigator.geolocation.getCurrentPosition(
    (pos) => {
      locating.value = false
      placeSelf(pos.coords.latitude, pos.coords.longitude, pos.coords.accuracy, false)
      geoWatch = navigator.geolocation.watchPosition(
        (p) => placeSelf(p.coords.latitude, p.coords.longitude, p.coords.accuracy, true),
        () => {},
        opts
      )
    },
    (err) => {
      locating.value = false
      const msg =
        err.code === 1
          ? 'Izin lokasi ditolak'
          : err.code === 2
          ? 'Lokasi tidak tersedia'
          : 'Waktu permintaan lokasi habis'
      showGeoError(msg)
    },
    opts
  )
}

function getView() {
  if (!map) return { lat: -2.5, lng: 118, zoom: 5 }
  const c = map.getCenter()
  return { lat: c.lat, lng: c.lng, zoom: map.getZoom() }
}

// ---------- Impor KMZ/KML (disimpan ke database) ----------
function kmzBoundsFor(features) {
  const latlngs = []
  const pushCoord = (c) => {
    if (c && typeof c[0] === 'number') latlngs.push([c[1], c[0]])
  }
  const walk = (geo) => {
    if (!geo || !geo.type) return
    if (geo.type === 'Point') pushCoord(geo.coordinates)
    else if (geo.type === 'MultiPoint' || geo.type === 'LineString') geo.coordinates.forEach(pushCoord)
    else if (geo.type === 'Polygon') geo.coordinates.forEach((r) => r.forEach(pushCoord))
    else if (geo.type === 'MultiLineString') geo.coordinates.forEach((l) => l.forEach(pushCoord))
    else if (geo.type === 'MultiPolygon') geo.coordinates.forEach((p) => p.forEach((r) => r.forEach(pushCoord)))
  }
  features.forEach((f) => walk(f.geometry))
  if (!latlngs.length) return null
  return L.latLngBounds(latlngs)
}

async function onKmzFile(e) {
  const file = e.target && e.target.files && e.target.files[0]
  if (e.target) e.target.value = ''
  if (!file || !map || kmzBusy) return
  kmzBusy = true
  try {
    const kmz = await parseKmzFile(file)
    const { data } = await api.post('/map/features/bulk', {
      source: 'kmz',
      features: kmz.features
    })
    const created = data.data || []
    importedKmzIds = created.map((f) => f.id)
    await loadFeatures()
    const bounds = kmzBoundsFor(created)
    if (bounds && bounds.isValid()) {
      map.fitBounds(bounds.pad(0.08), { maxZoom: 17 })
    }
    kmzMeta.value = { name: kmz.name, count: created.length }
    if (kmzDelEl) kmzDelEl.style.display = 'flex'
    if (data.skipped) {
      showGeoError(`${data.skipped} fitur tanpa geometri dilewati (folder kosong/ScreenOverlay)`)
    }
  } catch (err) {
    showGeoError(`Gagal impor ${file.name}: ${err && err.message ? err.message : 'file tidak valid'}`)
  } finally {
    kmzBusy = false
  }
}

async function clearKmz() {
  if (!importedKmzIds.length) return
  kmzBusy = true
  try {
    await Promise.all(importedKmzIds.map((id) => api.delete(`/map/features/${id}`).catch(() => {})))
    importedKmzIds = []
    kmzMeta.value = null
    if (kmzDelEl) kmzDelEl.style.display = 'none'
    await loadFeatures()
  } catch (err) {
    // diam saja
  } finally {
    kmzBusy = false
  }
}

const KmzControl = L.Control.extend({
  options: { position: 'topleft' },
  onAdd() {
    const container = L.DomUtil.create('div', 'leaflet-bar kmz-control')
    const btn = L.DomUtil.create('button', 'kmz-control__btn', container)
    btn.type = 'button'
    btn.title = 'Impor mapping KMZ/KML dari Google Earth (disimpan di database)'
    btn.setAttribute('aria-label', 'Impor KMZ/KML')
    btn.innerHTML =
      '<svg viewBox="0 0 24 24"><path d="M12 3v12m0 0 4-4m-4 4-4-4"/><path d="M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2"/></svg>'
    btn.addEventListener('click', () => {
      if (!kmzBusy && kmzFileInput.value) kmzFileInput.value.click()
    })

    const del = L.DomUtil.create('button', 'kmz-control__btn kmz-control__btn--del', container)
    del.type = 'button'
    del.title = 'Hapus lapisan KMZ yang diimpor'
    del.setAttribute('aria-label', 'Hapus lapisan KMZ')
    del.innerHTML = '✕'
    del.style.display = 'none'
    del.addEventListener('click', () => clearKmz())
    kmzDelEl = del

    L.DomEvent.disableClickPropagation(container)
    L.DomEvent.disableScrollPropagation(container)
    return container
  }
})

function openCustomerFromWindow(id) {
  const c = props.customers.find((x) => x.id === id)
  if (c) emit('open-detail', c)
}

function openFeatureFromWindow(id) {
  const f = featureById.get(id)
  if (f) emit('feature-manage', f)
}

let prevCustomerHandler = null
let prevFeatureHandler = null

onMounted(() => {
  prevCustomerHandler = window.__fmCustomer
  prevFeatureHandler = window.__fmFeature
  window.__fmCustomer = openCustomerFromWindow
  window.__fmFeature = openFeatureFromWindow

  const init = props.initialView
  map = L.map(mapEl.value, {
    center: init && init.lat != null && init.lng != null ? [init.lat, init.lng] : [-2.5, 118],
    zoom: init && init.zoom ? init.zoom : 5,
    zoomControl: true
  })

  L.control
    .layers(BASE_LAYERS, null, { position: 'bottomright', collapsed: false })
    .addTo(map)

  map.addControl(new LocateControl())
  if (props.manageFeatures) map.addControl(new KmzControl())

  BASE_LAYERS['OpenStreetMap'].addTo(map)

  markers = L.markerClusterGroup({ disableClusteringAtZoom: 10, chunkedLoading: true })
  map.addLayer(markers)
  map.on('click', onMapClick)
  map.on('dblclick', onMapDblClick)
  map.on('contextmenu', onMapContextMenu)
  window.addEventListener('keydown', onDrawKeydown)
  rebuildMarkers()
  renderDraft()
  loadFeatures()
})

onUnmounted(() => {
  if (geoWatch != null) {
    navigator.geolocation.clearWatch(geoWatch)
    geoWatch = null
  }
  if (window.__fmCustomer === openCustomerFromWindow) {
    window.__fmCustomer = prevCustomerHandler
  }
  if (window.__fmFeature === openFeatureFromWindow) {
    window.__fmFeature = prevFeatureHandler
  }
  window.removeEventListener('keydown', onDrawKeydown)
  cancelDraw()
  if (map) {
    map.off('click', onMapClick)
    map.off('dblclick', onMapDblClick)
    map.off('contextmenu', onMapContextMenu)
    map.remove()
    map = null
    markers = null
    draftMarker = null
    areaMarker = null
    selfMarker = null
    kmzMeta.value = null
    markerMap.clear()
    featureById.clear()
  }
})

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

watch(filtered, () => rebuildMarkers(), { deep: true })
watch(() => props.draftPoint, renderDraft)

defineExpose({
  focusCustomer,
  flyTo,
  getView,
  locate,
  startDraw,
  cancelDraw,
  refreshFeatures: loadFeatures
})
</script>

<template>
  <div class="monitor-map-wrap">
    <div
      ref="mapEl"
      class="monitor-map"
      :class="{
        'monitor-map--clickable': clickToAdd,
        'monitor-map--drawing': !!drawMode
      }"
    ></div>
    <input
      ref="kmzFileInput"
      class="kmz-file-input"
      type="file"
      accept=".kmz,.kml,.xml,application/vnd.google-earth.kmz"
      @change="onKmzFile"
    />
    <div v-if="kmzMeta" class="kmz-badge">
      <span>{{ kmzMeta.name }} · {{ kmzMeta.count }} fitur tersimpan di database</span>
    </div>
    <div v-if="drawMode" class="map-draw-hint">
      <span>{{ drawHintText }} · Esc = batal</span>
      <button class="map-draw-hint__cancel" type="button" @click="cancelDraw()">Batal</button>
    </div>
  </div>
</template>