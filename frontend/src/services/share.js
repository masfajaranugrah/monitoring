export function mapsUrl(lat, lng) {
  if (lat == null || lng == null) return ''
  return `https://www.google.com/maps/search/?api=1&query=${Number(lat).toFixed(6)},${Number(lng).toFixed(6)}`
}

export function waShareUrl(lat, lng, label = '') {
  const base = mapsUrl(lat, lng)
  if (!base) return ''
  const text = [label, `Lokasi: ${base}`].filter(Boolean).join('\n')
  return `https://wa.me/?text=${encodeURIComponent(text)}`
}

export function downFor(iso) {
  if (!iso) return ''
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return ''
  let s = Math.floor((Date.now() - t) / 1000)
  if (s <= 0) return 'baru saja'
  const d = Math.floor(s / 86400)
  s -= d * 86400
  const h = Math.floor(s / 3600)
  s -= h * 3600
  const m = Math.floor(s / 60)
  if (d > 0) return `${d} hari${h ? ` ${h} jam` : ''}`
  if (h > 0) return `${h} jam${m ? ` ${m} mnt` : ''}`
  if (m > 0) return `${m} mnt`
  return '< 1 mnt'
}