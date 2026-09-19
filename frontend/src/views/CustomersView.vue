<script setup>
import { ref, onMounted, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import StatusBadge from '../components/StatusBadge.vue'
import LatencyBadge from '../components/LatencyBadge.vue'
import { useMonitorStore } from '../stores/monitor'
import { useAuthStore } from '../stores/auth'
import { CUSTOMER_ICONS, customerIconSvg, statusColor } from '../services/customerIcons'

const router = useRouter()
const monitor = useMonitorStore()
const auth = useAuthStore()

const customers = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const totalPages = ref(1)

const search = ref('')
const statusFilter = ref('')
const vpnFilter = ref('')
const sortBy = ref('customer_code')
const sortOrder = ref('asc')

const loading = ref(true)
const showModal = ref(false)
const editing = ref(null)
const form = ref(emptyForm())
const saving = ref(false)
const errorMsg = ref('')
const deleting = ref(null)
const deleteError = ref('')

function emptyForm() {
  return {
    customer_code: '',
    customer_name: '',
    ip_address: '',
    latitude: '',
    longitude: '',
    vpn_id: '',
    icon: 'customer',
    description: '',
    monitoring_enabled: true,
    ping_interval: 10,
    timeout_ms: 2000,
    retry_count: 2
  }
}

async function load() {
  loading.value = true
  try {
    const params = {
      page: page.value,
      page_size: pageSize.value,
      sort_by: sortBy.value,
      sort_order: sortOrder.value
    }
    if (search.value) params.search = search.value
    if (statusFilter.value) params.status = statusFilter.value
    if (vpnFilter.value) params.vpn_id = vpnFilter.value

    const { data } = await api.get('/customers', { params })
    customers.value = data.data || []
    total.value = data.total || 0
    totalPages.value = data.pages || 1
  } catch (e) {
    customers.value = []
  } finally {
    loading.value = false
  }
}

const pages = computed(() => Array.from({ length: totalPages.value }, (_, i) => i + 1))

function applyFilters() {
  page.value = 1
  load()
}

function toggleSort(col) {
  if (sortBy.value === col) {
    sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortBy.value = col
    sortOrder.value = 'asc'
  }
  load()
}

function goPage(p) {
  if (p < 1 || p > totalPages.value) return
  page.value = p
  load()
}

function openCreate() {
  editing.value = null
  form.value = emptyForm()
  errorMsg.value = ''
  showModal.value = true
}

function openEdit(c) {
  editing.value = c
  form.value = {
    customer_code: c.customer_code,
    customer_name: c.customer_name,
    ip_address: c.ip_address,
    latitude: c.latitude,
    longitude: c.longitude,
    vpn_id: c.vpn_id || '',
    icon: c.icon || 'customer',
    description: c.description || '',
    monitoring_enabled: c.monitoring_enabled,
    ping_interval: c.ping_interval,
    timeout_ms: c.timeout_ms,
    retry_count: c.retry_count
  }
  errorMsg.value = ''
  showModal.value = true
}

async function save() {
  saving.value = true
  errorMsg.value = ''
  const payload = {
    ...form.value,
    latitude: parseFloat(form.value.latitude),
    longitude: parseFloat(form.value.longitude),
    vpn_id: form.value.vpn_id ? Number(form.value.vpn_id) : null,
    monitoring_enabled: !!form.value.monitoring_enabled
  }
  try {
    if (editing.value) {
      await api.put(`/customers/${editing.value.id}`, payload)
    } else {
      await api.post('/customers', payload)
    }
    showModal.value = false
    load()
  } catch (e) {
    errorMsg.value = e.response?.data?.error || 'Gagal menyimpan pelanggan'
  } finally {
    saving.value = false
  }
}

async function toggleMonitoring(c) {
  try {
    await api.patch(`/customers/${c.id}/monitoring`, { monitoring_enabled: !c.monitoring_enabled })
    c.monitoring_enabled = !c.monitoring_enabled
  } catch (e) {
    console.error(e)
  }
}

async function confirmDelete() {
  deleteError.value = ''
  try {
    await api.delete(`/customers/${deleting.value.id}`)
    deleting.value = null
    load()
  } catch (e) {
    deleteError.value = e.response?.data?.error || 'Gagal menghapus'
  }
}

function openDetail(c) {
  router.push(`/customers/${c.id}`)
}

function fmtTime(t) {
  return t ? new Date(t).toLocaleTimeString('id-ID') : '-'
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
}

watch(() => monitor.lastEvent, applyRealtime)

onMounted(() => {
  load()
  monitor.fetchVPNs()
})
</script>

<template>
  <div class="crud panel">
    <div class="crud__toolbar">
      <div class="search">
        <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/></svg>
        <input
          v-model="search"
          placeholder="Cari kode / nama / IP..."
          @keyup.enter="applyFilters"
        />
      </div>
      <button class="btn btn--ghost btn--sm" @click="applyFilters">Cari</button>

      <select v-model="statusFilter" class="select select--sm" @change="applyFilters">
        <option value="">Semua Status</option>
        <option value="ONLINE">ONLINE</option>
        <option value="OFFLINE">OFFLINE</option>
        <option value="WARNING">WARNING</option>
      </select>

      <select v-model="vpnFilter" class="select select--sm" @change="applyFilters">
        <option value="">Semua VPN</option>
        <option v-for="v in monitor.vpns" :key="v.id" :value="v.id">{{ v.name }}</option>
      </select>

      <span class="crud__count">{{ total }} pelanggan</span>

      <button v-if="auth.isAdmin" class="btn btn--primary btn--sm" @click="openCreate">+ Tambah Pelanggan</button>
    </div>

    <div class="table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th>Status</th>
            <th @click="toggleSort('customer_code')" class="sortable">
              Customer <span class="sort-hint">{{ sortBy === 'customer_code' ? (sortOrder === 'asc' ? '▲' : '▼') : '' }}</span>
            </th>
            <th>IP</th>
            <th>VPN</th>
            <th @click="toggleSort('latency')" class="sortable">
              Latency <span class="sort-hint">{{ sortBy === 'latency' ? (sortOrder === 'asc' ? '▲' : '▼') : '' }}</span>
            </th>
            <th @click="toggleSort('last_check')" class="sortable">
              Last Check <span class="sort-hint">{{ sortBy === 'last_check' ? (sortOrder === 'asc' ? '▲' : '▼') : '' }}</span>
            </th>
            <th>Monitoring</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in customers" :key="c.id" class="clk" @click="openDetail(c)">
            <td><StatusBadge :status="c.status" /></td>
            <td>
              <div class="crud__customer">
                <span class="cust-icon" :style="{ '--ic-color': statusColor(c.status) }" v-html="customerIconSvg(c.icon)"></span>
                <div class="crud__customer-main">
                  <strong>{{ c.customer_code }}</strong>
                  <span>{{ c.customer_name }}</span>
                </div>
              </div>
            </td>
            <td class="mono">{{ c.ip_address }}</td>
            <td>{{ c.vpn_name || '-' }}</td>
            <td><LatencyBadge :latency="c.latency_ms" /></td>
            <td>{{ fmtTime(c.last_check) }}</td>
            <td>
              <span
                class="toggle"
                :class="{ 'toggle--on': c.monitoring_enabled }"
                @click.stop="toggleMonitoring(c)"
              >
                {{ c.monitoring_enabled ? 'ON' : 'OFF' }}
              </span>
            </td>
            <td class="crud__actions">
              <button class="btn btn--ghost btn--sm" @click.stop="openEdit(c)">Edit</button>
              <button v-if="auth.isAdmin" class="btn btn--dangerghost btn--sm" @click.stop="deleting = c">Hapus</button>
            </td>
          </tr>
          <tr v-if="!customers.length">
            <td colspan="8" class="table__empty">{{ loading ? 'Memuat...' : 'Tidak ada data' }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="pagination">
      <button class="btn btn--ghost btn--sm" :disabled="page <= 1" @click="goPage(page - 1)">‹ Prev</button>
      <span class="pagination__info">Halaman {{ page }} dari {{ totalPages }}</span>
      <button class="btn btn--ghost btn--sm" :disabled="page >= totalPages" @click="goPage(page + 1)">Next ›</button>
    </div>

    <div v-if="showModal" class="modal-mask" @click.self="showModal = false">
      <div class="modal">
        <h3>{{ editing ? 'Edit Pelanggan' : 'Tambah Pelanggan' }}</h3>
        <form @submit.prevent="save">
          <div class="form-grid">
            <label class="field"><span>Kode Pelanggan *</span>
              <input v-model="form.customer_code" required />
            </label>
            <label class="field"><span>Nama *</span>
              <input v-model="form.customer_name" required />
            </label>
            <label class="field"><span>IP Address *</span>
              <input v-model="form.ip_address" placeholder="103.143.196.10" required />
            </label>
            <label class="field"><span>VPN</span>
              <select v-model="form.vpn_id">
                <option value="">- Tanpa VPN -</option>
                <option v-for="v in monitor.vpns" :key="v.id" :value="v.id">{{ v.name }}</option>
              </select>
            </label>
            <div class="field field--full">
              <span>Ikon / Logo</span>
              <div class="icon-picker">
                <button
                  v-for="ic in CUSTOMER_ICONS"
                  :key="ic.value"
                  type="button"
                  class="icon-picker__item"
                  :class="{ 'icon-picker__item--active': form.icon === ic.value }"
                  :style="'--ic-color:currentColor'"
                  :title="ic.label"
                  @click="form.icon = ic.value"
                >
                  <span v-html="customerIconSvg(ic.value)"></span>
                  <span>{{ ic.label }}</span>
                </button>
              </div>
            </div>
            <label class="field"><span>Latitude *</span>
              <input v-model="form.latitude" placeholder="-7.5231" required />
            </label>
            <label class="field"><span>Longitude *</span>
              <input v-model="form.longitude" placeholder="110.8123" required />
            </label>
            <label class="field"><span>Interval Ping (detik)</span>
              <input v-model.number="form.ping_interval" type="number" min="5" />
            </label>
            <label class="field"><span>Timeout (ms)</span>
              <input v-model.number="form.timeout_ms" type="number" min="500" />
            </label>
            <label class="field"><span>Retry</span>
              <input v-model.number="form.retry_count" type="number" min="1" />
            </label>
            <label class="field field--full"><span>Deskripsi</span>
              <input v-model="form.description" />
            </label>
            <label class="check"><input type="checkbox" v-model="form.monitoring_enabled" /> Monitoring aktif</label>
          </div>
          <p v-if="errorMsg" class="login__error">{{ errorMsg }}</p>
          <div class="modal__actions">
            <button type="button" class="btn btn--ghost" @click="showModal = false">Batal</button>
            <button type="submit" class="btn btn--primary" :disabled="saving">
              {{ saving ? 'Menyimpan...' : 'Simpan' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <div v-if="deleting" class="modal-mask" @click.self="deleting = null">
      <div class="modal modal--sm">
        <h3>Hapus Pelanggan?</h3>
        <p>Yakin ingin menghapus <strong>{{ deleting.customer_name }}</strong> ({{ deleting.customer_code }})?
        Riwayat ping akan ikut terhapus.</p>
        <p v-if="deleteError" class="login__error">{{ deleteError }}</p>
        <div class="modal__actions">
          <button class="btn btn--ghost" @click="deleting = null">Batal</button>
          <button class="btn btn--danger" @click="confirmDelete">Hapus</button>
        </div>
      </div>
    </div>
  </div>
</template>