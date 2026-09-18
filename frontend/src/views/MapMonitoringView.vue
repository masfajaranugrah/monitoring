<script setup>
import { ref, onMounted, computed, onBeforeUnmount, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import MapView from '../components/MapView.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { useMonitorStore } from '../stores/monitor'
import { useAreaSearch } from '../composables/useAreaSearch'

const router = useRouter()
const monitor = useMonitorStore()

const customers = ref([])
const statusFilter = ref('ALL')
const vpnFilter = ref('ALL')
const search = ref('')
const searchOpen = ref(false)
const loading = ref(true)
let mapView = null
const { areaResults, areaLoading, clearArea, areaZoom } = useAreaSearch(search)

async function load() {
  try {
    const params = {}
    if (statusFilter.value !== 'ALL') params.status = statusFilter.value
    if (vpnFilter.value !== 'ALL') params.vpn_id = vpnFilter.value
    const { data } = await api.get('/map/customers', { params })
    customers.value = data.data || []
    loading.value = false
  } catch (e) {
    loading.value = false
  }
}

function applyRealtime() {
  const ev = monitor.lastEvent
  if (!ev || ev.event !== 'customer:update') return
  const { customer_id, status, latency_ms, last_check } = ev.data || {}
  const target = customers.value.find((c) => c.id === customer_id)
  if (!target) return
  if (status) target.status = status
  if (latency_ms !== undefined) target.latency_ms = latency_ms
  if (last_check) target.last_check = last_check
  monitor.fetchStats()
}

watch(() => monitor.lastEvent, applyRealtime)

const filteredList = computed(() => {
  const q = search.value.toLowerCase().trim()
  if (!q) return customers.value
  return customers.value.filter(
    (c) =>
      c.customer_code?.toLowerCase().includes(q) ||
      c.customer_name?.toLowerCase().includes(q) ||
      c.ip_address?.toLowerCase().includes(q)
  )
})

function openDetail(customer) {
  router.push(`/customers/${customer.id}`)
}

function focusOnList(customer) {
  mapView?.focusCustomer(customer)
}

function focusFromArea(a) {
  mapView?.flyTo(Number(a.lat), Number(a.lon), areaZoom(a), a.display_name)
  clearArea()
  search.value = ''
}

function onSearchBlur() {
  setTimeout(() => {
    searchOpen.value = false
  }, 150)
}

function pickAreaFirst() {
  if (filteredList.value.length === 1) {
    focusOnList(filteredList.value[0])
  } else if (areaResults.value.length) {
    focusFromArea(areaResults.value[0])
  }
}

watch(filteredList, (list) => {
  if (list.length !== 1 || !search.value.trim()) return
  const c = list[0]
  if (c.latitude != null && c.longitude != null) {
    mapView?.focusCustomer(c)
  }
})

onMounted(() => {
  load()
  monitor.fetchStats()
})

onBeforeUnmount(() => {
  mapView = null
})
</script>

<template>
  <div class="mapmon">
    <div class="mapmon__toolbar panel">
      <div class="search-wrap">
        <div class="search">
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/></svg>
          <input
            v-model="search"
            placeholder="Cari kode, nama, IP, atau wilayah..."
            @focus="searchOpen = true"
            @input="searchOpen = true"
            @blur="onSearchBlur"
            @keydown.enter="pickAreaFirst"
          />
        </div>

        <div v-if="searchOpen && search.trim()" class="search-drop">
          <div class="search-drop__head">Wilayah</div>
          <li v-if="areaLoading" class="search-drop__empty">Mencari wilayah...</li>
          <template v-else>
            <li
              v-for="a in areaResults"
              :key="'a' + a.osm_id"
              @mousedown.prevent="focusFromArea(a)"
            >
              <strong>{{ a.name || a.display_name }}</strong>
              <span class="search-drop__meta search-drop__meta--ellipsis">{{ a.display_name }}</span>
            </li>
            <li v-if="!areaResults.length" class="search-drop__empty">
              Wilayah tidak ditemukan
            </li>
</template>

<style scoped>
.search-wrap {
  position: relative;
  flex: 1;
  min-width: 220px;
}

.search-wrap .search {
  flex: 1;
}

.search-drop {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  right: 0;
  z-index: 1200;
  margin: 0;
  padding: 4px;
  list-style: none;
  background: var(--bg-panel-2);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
  overflow: hidden;
}

.search-drop__head {
  padding: 8px 10px 4px;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-faint);
}

.search-drop li {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  font-size: 13px;
}

.search-drop li:hover {
  background: var(--bg-hover);
}

.search-drop__meta {
  font-size: 12px;
  color: var(--text-faint);
}

.search-drop__meta--ellipsis {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.search-drop__empty {
  padding: 8px 10px;
  color: var(--text-faint);
  font-size: 13px;
}
</style>
        </div>
      </div>

      <div class="filter-group">
        <span class="filter-group__label">Status:</span>
        <button
          v-for="s in ['ALL', 'ONLINE', 'OFFLINE', 'WARNING']"
          :key="s"
          class="filter-chip"
          :class="{ 'filter-chip--active': statusFilter === s, [`filter-chip--${s.toLowerCase()}`]: statusFilter === s }"
          @click="statusFilter = s; load()"
        >
          {{ s }}
        </button>
      </div>

      <select v-model="vpnFilter" class="select" @change="load()">
        <option value="ALL">ALL VPN</option>
        <option v-for="v in monitor.vpns" :key="v.id" :value="v.id">{{ v.name }}</option>
      </select>
    </div>

    <div class="mapmon__body">
      <div class="mapmon__map">
        <MapView
          ref="mapView"
          :customers="customers"
          :status-filter="statusFilter"
          :vpn-filter="vpnFilter"
          @open-detail="openDetail"
        />
        <div class="legend">
          <div class="legend__item"><span class="legend__dot legend__dot--online" /> ONLINE</div>
          <div class="legend__item"><span class="legend__dot legend__dot--offline" /> OFFLINE</div>
          <div class="legend__item"><span class="legend__dot legend__dot--warning" /> WARNING</div>
        </div>
      </div>

      <div class="mapmon__list panel">
        <h3 class="mapmon__list-title">Pelanggan <span>{{ filteredList.length }}</span></h3>
        <ul class="mapmon__list-items">
          <li
            v-for="c in filteredList"
            :key="c.id"
            class="mapmon__item"
            @click="focusOnList(c)"
          >
            <StatusBadge :status="c.status" />
            <div class="mapmon__item-main">
              <strong>{{ c.customer_name }}</strong>
              <span>{{ c.customer_code }} · {{ c.ip_address }} · {{ c.vpn_name || '-' }}</span>
            </div>
            <span v-if="c.latency_ms != null" class="mapmon__item-ms">{{ c.latency_ms }} ms</span>
          </li>
          <li v-if="!filteredList.length" class="mapmon__empty">Tidak ada pelanggan</li>
        </ul>
      </div>
    </div>
  </div>
</template>