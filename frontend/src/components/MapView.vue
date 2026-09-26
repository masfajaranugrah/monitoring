<script setup>
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import L from 'leaflet'
import 'leaflet.markercluster'
import { parseKmzFile } from '../services/kmz'
import { customerIconSvg, statusColor } from '../services/customerIcons'
import { mapsUrl } from '../services/share'
import {
  haversine,
  pathLength,
  segmentLengths,
  ringArea,
  dedupePoints,
  formatDistance,
  formatArea,
  metricsSummary
} from '../services/geo'
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

const emit = defineEmits(['open-detail', 'map-click', 'draw-complete', 'feature-manage', 'measure-change'])

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
let resizeObserver = null

function resolveMapSize() {
  if (map) map.invalidateSize({ animate: false })
}

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
let suppressClicksUntil = 0
const drawHintText = computed(() => {
  if (drawMode.value === 'point') return 'Klik peta untuk menempatkan titik informasi'
  if (drawMode.value === 'line') return 'Klik peta untuk menambah titik jalur — klik 2x / klik kanan untuk selesai'
  if (drawMode.value === 'polygon') return 'Klik peta untuk menambah titik area — klik 2x / klik kanan untuk selesai'
  return ''
})

const drawMetricsText = computed(() => {
  const pts = dedupePoints(drawVerts)
  if (!pts.length) return ''
  if (drawMode.value === 'line') return formatDistance(pathLength(pts))
  if (drawMode.value === 'polygon') {
    const total = formatDistance(pathLength(pts))
    if (pts.length < 3) return total
    return `${total} · ${formatArea(ringArea(pts))}`
  }
  return ''
})

// ---------- Mode ukur jarak (cek jarak sederhana) ----------
const measureActive = ref(false)
const measurePts = ref([])
const measureHover = ref(null)
let measureLayer = null
let measureRaf = 0

const measurePoints = computed(() => dedupePoints(measurePts.value))
const measureTotal = computed(() => pathLength(measurePoints.value))

const hintText = computed(() => {
  if (measureActive.value) return 'Klik peta untuk menandai titik — klik kanan = hapus titik terakhir, Esc = selesai'
  return drawHintText.value
})

const hintMetrics = computed(() => {
  if (measureActive.value) {
    const n = measurePoints.value.length
    if (!n) return ''
    if (n === 1) return '1 titik · klik titik berikutnya'
    return `${n} titik · total ${formatDistance(measureTotal.value)}`
  }
  return drawMetricsText.value
})

function measureIcon(text, extra = '') {
  return L.divIcon({
    className: '',
    html: `<div class="map-measure-label ${extra}">${escapeHtml(text)}</div>`,
    iconSize: [0, 0],
    iconAnchor: [0, 0]
  })
}

function clearMeasureLayer() {
  if (measureLayer && map) map.removeLayer(measureLayer)
  measureLayer = null
}

function midpoint(a, b) {
  return [(a.lat + b.lat) / 2, (a.lng + b.lng) / 2]
}

function renderMeasure() {
  if (!map) return
  clearMeasureLayer()
  const pts = measurePoints.value
  const hover = measureHover.value
  const group = L.layerGroup()
  const shape = hover ? [...pts, hover] : pts

  if (shape.length >= 2) {
    group.addLayer(
      L.polyline(shape, {
        color: '#f59e0b',
        weight: 3,
        opacity: 0.95,
        dashArray: hover ? '4 6' : null
      })
    )
  }

  pts.forEach((p, i) => {
    group.addLayer(
      L.circleMarker([p.lat, p.lng], {
        radius: i === 0 ? 6 : 4.5,
        color: '#fff',
        weight: 2,
        fillColor: i === 0 ? '#22c55e' : '#f59e0b',
        fillOpacity: 1
      })
    )
    if (i === 0) return
    const [mlat, mlng] = midpoint(pts[i - 1], p)
    group.addLayer(
      L.marker([mlat, mlng], {
        icon: measureIcon(formatDistance(haversine(pts[i - 1], p))),
        interactive: false,
        zIndexOffset: 1200
      })
    )
  })

  if (hover && pts.length) {
    const last = pts[pts.length - 1]
    const [mlat, mlng] = midpoint(last, hover)
    group.addLayer(
      L.marker([mlat, mlng], {
        icon: measureIcon(formatDistance(haversine(last, hover)), 'map-measure-label--live'),
        interactive: false,
        zIndexOffset: 1200
      })
    )
  }

  if (pts.length >= 2) {
    const last = pts[pts.length - 1]
    group.addLayer(
      L.marker([last.lat, last.lng], {
        icon: measureIcon(`Total ${formatDistance(measureTotal.value)}`, 'map-measure-label--total'),
        interactive: false,
        zIndexOffset: 1400
      })
    )
  }

  measureLayer = group
  if (group.getLayers().length) group.addTo(map)
}

function emitMeasure() {
  emit('measure-change', {
    active: measureActive.value,
    count: measurePoints.value.length,
    totalM: measureTotal.value,
    segments: segmentLengths(measurePoints.value),
    points: measurePoints.value.map((p) => ({ lat: p.lat, lng: p.lng }))
  })
}

function startMeasure() {
  if (!map) return
  cancelDraw()
  measureActive.value = true
  measurePts.value = []
  measureHover.value = null
  if (map.doubleClickZoom) map.doubleClickZoom.disable()
  renderMeasure()
  emitMeasure()
}

function cancelMeasure(opts) {
  const keepResult = opts && opts.keepResult
  measureActive.value = false
  measureHover.value = null
  if (map && map.doubleClickZoom) map.doubleClickZoom.enable()
  if (keepResult) renderMeasure()
  else {
    measurePts.value = []
    clearMeasureLayer()
  }
  emitMeasure()
}

function resetMeasure() {
  measurePts.value = []
  measureHover.value = null
  renderMeasure()
  emitMeasure()
}

function undoMeasurePoint() {
  if (!measurePts.value.length) return
  measurePts.value = measurePts.value.slice(0, -1)
  renderMeasure()
  emitMeasure()
}

function cancelActiveTool() {
  if (measureActive.value) cancelMeasure({ keepResult: true })
  else cancelDraw()
}

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
    iconSize: [34, 34],
    iconAnchor: [17, 17],
    popupAnchor: [0, -18]
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
  const color = statusColor(customer.status)
  const pulse = customer.status === 'WARNING' ? ' map-marker--pulse' : ''
  const svg = customerIconSvg(customer.icon || 'customer')
  return L.divIcon({
    className: '',
    html: `<div class="map-marker map-marker--customer${pulse}" style="--marker-color:${color};--ic-color:${color}">${svg}</div>`,
    iconSize: [30, 30],
    iconAnchor: [15, 15],
    popupAnchor: [0, -18]
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
      <div class="map-popup__share">
        <a class="map-popup__btn" href="${mapsUrl(customer.latitude, customer.longitude)}" target="_blank" rel="noopener">Share Lokasi</a>
        <button
          type="button"
          class="map-popup__btn map-popup__btn--ghost"
          onclick="window.__fmShare(${Number(customer.latitude)},${Number(customer.longitude)})"
        >Salin Link</button>
      </div>
      <button class="map-popup__btn" onclick="window.__fmCustomer(${customer.id})">Lihat Detail</button>
    </div>`
}

function featurePopup(f) {
  const manageBtn = props.manageFeatures
    ? `<button class="map-popup__btn" onclick="window.__fmFeature(${f.id})">Kelola Fitur</button>`
    : ''
  const typeLabel = { point: 'Titik', line: 'Jalur', polygon: 'Area' }[f.feature_type] || 'Fitur'
  const metrics = metricsSummary(f.geometry) || []
  const metricRows = metrics
    .map((r) => `<tr><td>${escapeHtml(r.label)}</td><td>${escapeHtml(r.value)}</td></tr>`)
    .join('')
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
        ${metricRows}
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
    const tip = metricsSummary(f.geometry)
    layer.eachLayer((l) => {
      l.bindPopup(featurePopup(f), { maxWidth: 280, autoPanPadding: [30, 30], className: 'map-popup-shell' })
      if (tip) {
        l.bindTooltip(tip.map((r) => `${r.label} ${r.value}`).join(' · '), {
          sticky: true,
          direction: 'top',
          className: 'map-feature-tip'
        })
      }
    })
    featureGroup.addLayer(layer)
  })
}

async function loadFeatures() {
  try {
    const { data } = await api.get('/map/features')
    features.value = data.data || []
    if (kmzDelEl) {
      kmzDelEl.style.display = features.value.some((f) => f.source === 'kmz') ? 'flex' : 'none'
    }
    renderFeatures()
    // Auto-zoom ke area fitur bila peta masih di tampilan awal (zoom kecil),
    // supaya hasil impor langsung terlihat.
    if (features.value.length && map && map.getZoom() <= 6) {
      const b = kmzBoundsFor(features.value)
      if (b && b.isValid()) {
        map.fitBounds(b.pad(0.08), { maxZoom: 16 })
      }
    }
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
function removeDrawPreviewLayer() {
  if (drawPreview) {
    if (map) map.removeLayer(drawPreview)
    drawPreview = null
  }
}

function clearDrawPreview() {
  removeDrawPreviewLayer()
  drawVerts.length = 0
}

function updateDrawPreview() {
  removeDrawPreviewLayer()
  if (!drawVerts.length) return
  const first = drawVerts[0]
  const last = drawVerts[drawVerts.length - 1]
  const closing = drawMode.value === 'polygon'
  const shape = closing ? [...drawVerts, first] : drawVerts
  const group = L.layerGroup()
  if (shape.length >= 2) {
    group.addLayer(
      L.polyline(shape, {
        color: '#f43f5e',
        weight: 3,
        opacity: 0.95,
        dashArray: '6 6'
      })
    )
  }
  drawVerts.forEach((v, i) => {
    group.addLayer(
      L.circleMarker([v[0], v[1]], {
        radius: i === 0 && closing ? 6 : 5,
        color: '#fff',
        weight: 2,
        fillColor: '#f43f5e',
        fillOpacity: 1
      })
    )
  })
  const lastMark = L.circleMarker([last[0], last[1]], {
    radius: 7,
    color: '#f43f5e',
    weight: 2,
    fillColor: '#fff',
    fillOpacity: 1
  })
  group.addLayer(lastMark)
  if (shape.length >= 2) {
    group.addLayer(
      L.marker([last[0], last[1]], {
        icon: measureIcon(
          `${formatDistance(pathLength(shape))}${drawMode.value === 'polygon' ? ` · ${formatArea(ringArea(shape))}` : ''}`,
          'map-measure-label--total map-measure-label--rose'
        ),
        interactive: false,
        zIndexOffset: 1400
      })
    )
  }
  drawPreview = group
  if (map) group.addTo(map)
}

function startDraw(mode) {
  if (!map || drawMode.value === mode) return
  if (measureActive.value) cancelMeasure()
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
    const distinct = []
    for (const v of drawVerts) {
      const last = distinct[distinct.length - 1]
      if (!last || last[0] !== v[0] || last[1] !== v[1]) distinct.push(v)
    }
    if (distinct.length < 2) {
      cancelDraw()
      return
    }
    const latlngs = distinct.map((v) => ({ lat: v[0], lng: v[1] }))
    emit('draw-complete', { type: drawMode.value, latlngs })
  }
  cancelDraw()
}

function finishDrawGuarded() {
  if (drawMode.value !== 'line' && drawMode.value !== 'polygon') return
  // Klik yang menyusul double-click / klik-kanan tidak boleh menambah vertex
  // lagi atau memicu "tambah pelanggan".
  suppressClicksUntil = Date.now() + 400
  drawSupClicked = true
  finishDraw()
}

function onMapClick(e) {
  if (Date.now() < suppressClicksUntil) return
  if (measureActive.value) {
    measurePts.value = [...measurePts.value, { lat: e.latlng.lat, lng: e.latlng.lng }]
    measureHover.value = null
    renderMeasure()
    emitMeasure()
    return
  }
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

function onMapMouseMove(e) {
  if (!measureActive.value) return
  const next = { lat: e.latlng.lat, lng: e.latlng.lng }
  const hover = measureHover.value
  if (hover && hover.lat === next.lat && hover.lng === next.lng) return
  measureHover.value = next
  if (measureRaf) return
  measureRaf = requestAnimationFrame(() => {
    measureRaf = 0
    renderMeasure()
  })
}

function onMapMouseOut() {
  if (!measureActive.value || !measureHover.value) return
  measureHover.value = null
  renderMeasure()
}

function onMapDblClick(e) {
  if (measureActive.value) {
    if (e && e.originalEvent) e.originalEvent.preventDefault()
    return
  }
  if (drawMode.value === 'line' || drawMode.value === 'polygon') {
    if (e && e.originalEvent) e.originalEvent.preventDefault()
    finishDrawGuarded()
  }
}

function onMapContextMenu(e) {
  if (measureActive.value) {
    e.originalEvent.preventDefault()
    undoMeasurePoint()
    return
  }
  if (drawMode.value === 'line' || drawMode.value === 'polygon') {
    e.originalEvent.preventDefault()
    finishDrawGuarded()
  }
}

function onDrawKeydown(e) {
  if (e.key !== 'Escape') return
  if (drawMode.value) cancelDraw()
  else if (measureActive.value) cancelMeasure({ keepResult: true })
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

async function shareLocationFromWindow(lat, lng) {
  const url = mapsUrl(Number(lat), Number(lng))
  if (!url) return
  try {
    await navigator.clipboard.writeText(url)
    showGeoError('Link lokasi disalin!')
  } catch (e) {
    window.prompt('Salin link lokasi:', url)
  }
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

function kmzErrMsg(fileName, err) {
  const detail = err?.response?.data?.error
  if (detail && typeof detail === 'string') return detail
  return err && err.message ? err.message : 'file tidak valid'
}

async function onKmzFile(e) {
  const file = e.target && e.target.files && e.target.files[0]
  if (e.target) e.target.value = ''
  if (!file || !map || kmzBusy) return
  kmzBusy = true
  try {
    const kmz = await parseKmzFile(file)
    const { data } = await api.post(
      '/map/features/bulk',
      {
        source: 'kmz',
        features: kmz.features
      },
      { timeout: 300000 }
    )
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
    showGeoError(`Gagal impor ${file.name}: ${kmzErrMsg(file.name, err)}`)
  } finally {
    kmzBusy = false
  }
}

async function clearKmz() {
  if (kmzBusy) return
  kmzBusy = true
  try {
    let targets = features.value.filter((f) => f.source === 'kmz').map((f) => f.id)
    if (!targets.length) {
      const { data } = await api.get('/map/features')
      targets = (data.data || []).filter((f) => f.source === 'kmz').map((f) => f.id)
    }
    if (!targets.length) {
      importedKmzIds = []
      kmzMeta.value = null
      if (kmzDelEl) kmzDelEl.style.display = 'none'
      return
    }
    if (!window.confirm(`Hapus ${targets.length} fitur lapisan KMZ yang diimpor?`)) return
    await Promise.all(targets.map((id) => api.delete(`/map/features/${id}`).catch(() => {})))
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
let prevShareHandler = null

onMounted(() => {
  prevCustomerHandler = window.__fmCustomer
  prevFeatureHandler = window.__fmFeature
  prevShareHandler = window.__fmShare
  window.__fmCustomer = openCustomerFromWindow
  window.__fmFeature = openFeatureFromWindow
  window.__fmShare = shareLocationFromWindow

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
  map.on('mousemove', onMapMouseMove)
  map.on('mouseout', onMapMouseOut)
  window.addEventListener('keydown', onDrawKeydown)
  rebuildMarkers()
  renderDraft()
  loadFeatures()

  // Pastikan ukuran peta sudah benar setelah layout selesai. Tanpa ini peta
  // bisa ter-render kosong di mobile saat container dihitung ukurannya
  // setelah init (baru muncul setelah map diperbesar).
  requestAnimationFrame(() => requestAnimationFrame(resolveMapSize))
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => resolveMapSize())
    resizeObserver.observe(mapEl.value)
  } else {
    window.addEventListener('resize', resolveMapSize)
  }
})

onUnmounted(() => {
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  window.removeEventListener('resize', resolveMapSize)
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
  if (window.__fmShare === shareLocationFromWindow) {
    window.__fmShare = prevShareHandler
  }
  window.removeEventListener('keydown', onDrawKeydown)
  if (measureRaf) {
    cancelAnimationFrame(measureRaf)
    measureRaf = 0
  }
  cancelDraw()
  cancelMeasure()
  if (map) {
    map.off('click', onMapClick)
    map.off('dblclick', onMapDblClick)
    map.off('contextmenu', onMapContextMenu)
    map.off('mousemove', onMapMouseMove)
    map.off('mouseout', onMapMouseOut)
    map.remove()
    map = null
    markers = null
    draftMarker = null
    areaMarker = null
    selfMarker = null
    measureLayer = null
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
  finishDrawNow: finishDrawGuarded,
  refreshFeatures: loadFeatures,
  startMeasure,
  cancelMeasure,
  resetMeasure,
  measureSnapshot: () => ({
    active: measureActive.value,
    count: measurePoints.value.length,
    totalM: measureTotal.value,
    segments: segmentLengths(measurePoints.value),
    points: measurePoints.value.map((p) => ({ lat: p.lat, lng: p.lng }))
  })
})
</script>

<template>
  <div class="monitor-map-wrap">
    <div
      ref="mapEl"
      class="monitor-map"
      :class="{
        'monitor-map--clickable': clickToAdd,
        'monitor-map--drawing': !!drawMode || measureActive
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
    <div v-if="drawMode || measureActive" class="map-draw-hint">
      <span>{{ hintText }}</span>
      <span v-if="hintMetrics" class="map-draw-hint__metric">{{ hintMetrics }}</span>
      <template v-if="measureActive">
        <button
          v-if="measurePoints.length"
          type="button"
          class="map-draw-hint__act"
          @click="undoMeasurePoint"
        >
          Hapus titik
        </button>
        <button
          v-if="measurePoints.length"
          type="button"
          class="map-draw-hint__act"
          @click="resetMeasure"
        >
          Reset
        </button>
      </template>
      <button class="map-draw-hint__cancel" type="button" @click="cancelActiveTool">Batal</button>
    </div>
  </div>
</template>