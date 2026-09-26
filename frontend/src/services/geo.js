const EARTH_R = 6371008.8

const toRad = (deg) => (deg * Math.PI) / 180

function num(v) {
  const n = typeof v === 'number' ? v : parseFloat(v)
  return Number.isFinite(n) ? n : null
}

export function toLatLng(p) {
  if (!p) return null
  if (Array.isArray(p)) {
    const lat = num(p[0])
    const lng = num(p[1])
    return lat != null && lng != null ? { lat, lng } : null
  }
  const lat = num(p.lat)
  const lng = num(p.lng != null ? p.lng : p.lon)
  return lat != null && lng != null ? { lat, lng } : null
}

export function haversine(a, b) {
  const p = toLatLng(a)
  const q = toLatLng(b)
  if (!p || !q) return 0
  const dLat = toRad(q.lat - p.lat)
  const dLng = toRad(q.lng - p.lng)
  const s =
    Math.sin(dLat / 2) * Math.sin(dLat / 2) +
    Math.cos(toRad(p.lat)) * Math.cos(toRad(q.lat)) * Math.sin(dLng / 2) * Math.sin(dLng / 2)
  return 2 * EARTH_R * Math.asin(Math.min(1, Math.sqrt(s)))
}

export function cleanPoints(points) {
  return (points || []).map(toLatLng).filter(Boolean)
}

export function dedupePoints(points) {
  const out = []
  cleanPoints(points).forEach((p) => {
    const last = out[out.length - 1]
    if (!last || last.lat !== p.lat || last.lng !== p.lng) out.push(p)
  })
  return out
}

export function pathLength(points, close = false) {
  const pts = cleanPoints(points)
  if (pts.length < 2) return 0
  let total = 0
  for (let i = 1; i < pts.length; i++) total += haversine(pts[i - 1], pts[i])
  if (close) total += haversine(pts[pts.length - 1], pts[0])
  return total
}

export function segmentLengths(points) {
  const pts = cleanPoints(points)
  const out = []
  for (let i = 1; i < pts.length; i++) out.push(haversine(pts[i - 1], pts[i]))
  return out
}

export function ringArea(points) {
  const pts = cleanPoints(points)
  if (pts.length < 3) return 0
  let total = 0
  for (let i = 0; i < pts.length; i++) {
    const p1 = pts[i]
    const p2 = pts[(i + 1) % pts.length]
    total += toRad(p2.lng - p1.lng) * (2 + Math.sin(toRad(p1.lat)) + Math.sin(toRad(p2.lat)))
  }
  return Math.abs((total * EARTH_R * EARTH_R) / 2)
}

function outerRing(polygon) {
  const ring = cleanPoints(polygon && polygon[0])
  if (ring.length > 1) {
    const first = ring[0]
    const last = ring[ring.length - 1]
    if (first.lat === last.lat && first.lng === last.lng) ring.pop()
  }
  return ring
}

export function geometryMetrics(geometry) {
  if (!geometry || !geometry.type || !geometry.coordinates) return null
  const coords = geometry.coordinates

  if (geometry.type === 'LineString') {
    return { lengthM: pathLength(coords) }
  }
  if (geometry.type === 'MultiLineString') {
    return { lengthM: (coords || []).reduce((sum, line) => sum + pathLength(line), 0) }
  }
  if (geometry.type === 'Polygon') {
    const ring = outerRing(coords)
    return { areaM2: ringArea(ring), lengthM: pathLength(ring, true) }
  }
  if (geometry.type === 'MultiPolygon') {
    let areaM2 = 0
    let lengthM = 0
    ;(coords || []).forEach((polygon) => {
      const ring = outerRing(polygon)
      areaM2 += ringArea(ring)
      lengthM += pathLength(ring, true)
    })
    return { areaM2, lengthM }
  }
  return null
}

export function formatDistance(meters) {
  const m = num(meters)
  if (m == null) return '-'
  if (m < 1000) return `${Math.round(m).toLocaleString('id-ID')} m`
  return `${(m / 1000).toLocaleString('id-ID', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })} km`
}

export function formatArea(sqMeters) {
  const m2 = num(sqMeters)
  if (m2 == null || m2 <= 0) return '-'
  if (m2 < 10000) return `${Math.round(m2).toLocaleString('id-ID')} m²`
  return `${(m2 / 10000).toLocaleString('id-ID', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })} ha`
}

export function metricsSummary(geometry) {
  const m = geometryMetrics(geometry)
  if (!m) return null
  const rows = []
  if (m.lengthM != null) rows.push({ label: m.areaM2 ? 'Keliling' : 'Panjang', value: formatDistance(m.lengthM) })
  if (m.areaM2) rows.push({ label: 'Luas', value: formatArea(m.areaM2) })
  return rows.length ? rows : null
}
