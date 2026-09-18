import { ref, watch } from 'vue'

export function useAreaSearch(queryRef) {
  const areaResults = ref([])
  const areaLoading = ref(false)
  let timer = null
  let seq = 0

  async function fetchAreas(q) {
    const my = ++seq
    areaLoading.value = true
    try {
      const url =
        'https://nominatim.openstreetmap.org/search?format=jsonv2&limit=6&countrycodes=id&accept-language=id&q=' +
        encodeURIComponent(q)
      const res = await fetch(url)
      const data = await res.json()
      if (my !== seq) return
      areaResults.value = Array.isArray(data) ? data : []
    } catch (e) {
      if (my === seq) areaResults.value = []
    } finally {
      if (my === seq) areaLoading.value = false
    }
  }

  watch(queryRef, (q) => {
    clearTimeout(timer)
    const clean = (q || '').trim()
    if (!clean) {
      seq++
      areaResults.value = []
      areaLoading.value = false
      return
    }
    timer = setTimeout(() => fetchAreas(clean), 400)
  })

  function clearArea() {
    seq++
    areaResults.value = []
    areaLoading.value = false
  }

  function areaZoom(a) {
    const bb = a.boundingbox
    if (Array.isArray(bb) && bb.length >= 4) {
      const dLat = Math.abs(Number(bb[1]) - Number(bb[0]))
      const dLng = Math.abs(Number(bb[3]) - Number(bb[2]))
      const size = Math.max(dLat, dLng)
      if (size > 0) {
        const z = Math.round(18 - Math.log2(360 / size))
        return Math.max(10, Math.min(18, z))
      }
    }
    return 15
  }

  return { areaResults, areaLoading, clearArea, areaZoom }
}