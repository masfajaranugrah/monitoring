import JSZip from 'jszip'
import * as tg from '@tmcw/togeojson'

const STORAGE_KEY = 'fm_kmz_kml'
const MAX_STORED = 4 * 1024 * 1024

function kmlToGeoJSON(kmlText) {
  const doc = new DOMParser().parseFromString(kmlText, 'application/xml')
  if (doc.querySelector('parsererror')) {
    throw new Error('KML tidak valid')
  }
  return tg.kml(doc)
}

// Read a .kmz (zip) or .kml file into GeoJSON.
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
  return { name, geojson, kmlText }
}

export function saveKmz(kmlText) {
  try {
    if (!kmlText || kmlText.length > MAX_STORED) return
    localStorage.setItem(STORAGE_KEY, kmlText)
  } catch (e) {
    /* storage full or unavailable */
  }
}

export function loadSavedKmz() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const geojson = kmlToGeoJSON(raw)
    return { name: 'Google Earth (tersimpan)', geojson, kmlText: raw }
  } catch (e) {
    return null
  }
}

export function clearSavedKmz() {
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch (e) {
    /* noop */
  }
}