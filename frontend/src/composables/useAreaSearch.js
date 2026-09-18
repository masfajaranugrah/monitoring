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

  return { areaResults, areaLoading, clearArea }
}