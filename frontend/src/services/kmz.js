import JSZip from 'jszip'
import * as tg from '@tmcw/togeojson'

const POINT_TYPES = new Set(['Point', 'MultiPoint'])
const LINE_TYPES = new Set(['LineString', 'MultiLineString'])
const POLY_TYPES = new Set(['Polygon', 'MultiPolygon'])

function classifyGeometry(type) {
  if (POINT_TYPES.has(type)) return 'point'
  if (LINE_TYPES.has(type)) return 'line'
  if (POLY_TYPES.has(type)) return 'polygon'
  return 'line'
}

function extractFeature(feature, index) {
  const p = feature.properties || {}
  const type = feature.geometry && feature.geometry.type
  const name = p.name || p.Name || p.title || p.Title || ''
  const desc = p.description || p.Description || p.desc || ''
  const fallback = classifyGeometry(type) === 'point' ? 'Titik ' : 'Jalur '

  if (type && type === 'GeometryCollection') {
    const cols = []
    ;(feature.geometry.geometries || []).forEach((g, gi) => {
      if (!g || !g.type) return
      cols.push({
        name: name || `${fallback}${index + 1}-${gi + 1}`,
        feature_type: classifyGeometry(g.type),
        geometry: g,
        description: desc || ''
      })
    })
    return cols
  }

  return [
    {
      name: name || `${fallback}${index + 1}`,
      feature_type: classifyGeometry(type),
      geometry: feature.geometry,
      description: desc || ''
    }
  ]
}

function kmlToGeoJSON(kmlText) {
  const doc = new DOMParser().parseFromString(kmlText, 'application/xml')
  if (doc.querySelector('parsererror')) {
    throw new Error('KML tidak valid')
  }
  return tg.kml(doc)
}

// Baca file .kmz/.kml menjadi daftar fitur peta (geometri GeoJSON) untuk
// disimpan ke database via API. Tidak lagi menyimpan apa pun di browser.
export async function parseKmzFile(file) {
  const buf = await file.arrayBuffer()
  const bytes = new Uint8Array(buf)
  const isZip =
    bytes.length > 3 && bytes[0] === 0x50 && bytes[1] === 0x4b && (bytes[2] === 0x03 || bytes[2] === 0x05 || bytes[2] === 0x07)

  let kmlText
  if (isZip) {
    const zip = await JSZip.loadAsync(buf)
    const entry = Object.values(zip.files).find((f) => !f.dir && /\.kml$/i.test(f.name))
    if (!entry) throw new Error('Tidak ada file .kml di dalam .kmz')
    kmlText = await entry.async('string')
  } else {
    kmlText = new TextDecoder('utf-8').decode(bytes)
  }

  const geojson = kmlToGeoJSON(kmlText)
  const name = file.name.replace(/\.(kmz|kml)$/i, '')
  const features = []
  ;(geojson.features || []).forEach((feat, i) => {
    features.push(...extractFeature(feat, i))
  })
  if (!features.length) {
    throw new Error('KMZ/KML tidak mengandung fitur yang bisa dipetakan')
  }
  return { name, features }
}