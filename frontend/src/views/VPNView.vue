<script setup>
import { ref, onMounted } from 'vue'
import api from '../api'
import { useMonitorStore } from '../stores/monitor'
import { useAuthStore } from '../stores/auth'

const monitor = useMonitorStore()
const auth = useAuthStore()

const loading = ref(true)
const testing = ref(null)
const testResult = ref(null)
const showModal = ref(false)
const editing = ref(null)
const errorMsg = ref('')
const form = ref(emptyForm())

const typeBadges = {
  L2TP: 'type-badge type-badge--l2tp',
  SSTP: 'type-badge type-badge--sstp',
  PPTP: 'type-badge type-badge--pptp',
  OPENVPN: 'type-badge type-badge--openvpn',
  WIREGUARD: 'type-badge type-badge--wg'
}

function emptyForm() {
  return {
    name: '',
    vpn_type: 'L2TP',
    server_address: '',
    username: '',
    password: '',
    local_ip: '',
    interface_name: '',
    is_active: true,
    description: ''
  }
}

async function load() {
  loading.value = true
  await monitor.fetchVPNs()
  loading.value = false
}

async function testVPN(v) {
  testing.value = v.id
  testResult.value = null
  try {
    testResult.value = await monitor.testVPN(v.id)
  } catch (e) {
    testResult.value = { message: e.response?.data?.error || 'Test gagal' }
  } finally {
    testing.value = null
  }
}

function openCreate() {
  editing.value = null
  form.value = emptyForm()
  errorMsg.value = ''
  showModal.value = true
}

function openEdit(v) {
  editing.value = v
  form.value = {
    name: v.name,
    vpn_type: v.vpn_type,
    server_address: v.server_address,
    username: v.username,
    password: '',
    local_ip: v.local_ip || '',
    interface_name: v.interface_name || '',
    is_active: v.is_active,
    description: v.description || ''
  }
  errorMsg.value = ''
  showModal.value = true
}

async function save() {
  errorMsg.value = ''
  try {
    if (editing.value) {
      await api.put(`/vpn/${editing.value.id}`, form.value)
    } else {
      await api.post('/vpn', form.value)
    }
    showModal.value = false
    load()
  } catch (e) {
    errorMsg.value = e.response?.data?.error || 'Gagal menyimpan VPN'
  }
}

async function setActive(v, active) {
  try {
    await api.patch(`/vpn/${v.id}/active`, { is_active: active })
    v.is_active = active
  } catch (e) {
    console.error(e)
  }
}

async function deleteVPN(v) {
  if (!confirm(`Hapus VPN ${v.name}?`)) return
  try {
    await api.delete(`/vpn/${v.id}`)
    load()
  } catch (e) {
    alert(e.response?.data?.error || 'Gagal menghapus')
  }
}

const connecting = ref(null)

function updateVpnFromResult(v, res) {
  if (res.status) v.status = res.status
  if (res.local_ip) v.local_ip = res.local_ip
  if (res.latency_ms != null) v.latency_ms = res.latency_ms
}

async function connectVPN(v) {
  connecting.value = v.id
  testResult.value = null
  try {
    const res = await api.post(`/vpn/${v.id}/connect`)
    if (res.data) updateVpnFromResult(v, res.data)
    testResult.value = {
      connected: !!(res.data && res.data.connected),
      message: res.data?.message || 'OK',
      local_ip: res.data?.local_ip
    }
    load()
  } catch (e) {
    testResult.value = {
      connected: false,
      message: e.response?.data?.error || 'Gagal menghubungkan VPN'
    }
  } finally {
    connecting.value = null
  }
}

async function disconnectVPN(v) {
  connecting.value = v.id
  testResult.value = null
  try {
    const res = await api.post(`/vpn/${v.id}/disconnect`)
    if (res.data) updateVpnFromResult(v, res.data)
    testResult.value = {
      connected: false,
      message: res.data?.message || 'VPN dihentikan'
    }
    load()
  } catch (e) {
    testResult.value = {
      connected: false,
      message: e.response?.data?.error || 'Gagal menghentikan VPN'
    }
  } finally {
    connecting.value = null
  }
}

onMounted(load)
</script>

<template>
  <div class="vpn-page">
    <div class="vpn-page__head">
      <h2>VPN Connections</h2>
      <span class="vpn-page__hint">Kelola VPN langsung dari web (L2TP/SSTP/PPTP/OpenVPN/WireGuard)</span>
      <button v-if="auth.isAdmin" class="btn btn--primary btn--sm" @click="openCreate">+ Tambah VPN</button>
    </div>

    <div v-if="loading" class="panel loading-text">Memuat VPN...</div>

    <div v-else class="vpn-grid">
      <div v-for="v in monitor.vpns" :key="v.id" class="vpn-card panel">
        <div class="vpn-card__head">
          <div>
            <h3>{{ v.name }}</h3>
            <span :class="typeBadges[v.vpn_type] || 'type-badge'">{{ v.vpn_type }}</span>
          </div>
          <span class="vpn-status" :class="`vpn-status--${(v.status || '').toLowerCase()}`">
            <span class="dot" /> {{ v.status }}
          </span>
        </div>

        <dl class="kv kv--tight">
          <dt>Server</dt><dd class="mono">{{ v.server_address }}</dd>
          <dt>Username</dt><dd class="mono">{{ v.username }}</dd>
          <dt>Local IP</dt><dd class="mono">{{ v.local_ip || '-' }}</dd>
          <dt>Interface</dt><dd class="mono">{{ v.interface_name || '-' }}</dd>
          <dt>Customers</dt><dd>{{ v.customer_count }}</dd>
          <dt>Latency</dt><dd>{{ v.latency_ms != null ? v.latency_ms + ' ms' : '-' }}</dd>
          <dt>Connected</dt><dd>{{ v.last_connected_at ? new Date(v.last_connected_at).toLocaleTimeString('id-ID') : '-' }}</dd>
        </dl>

        <p v-if="v.description" class="vpn-card__desc">{{ v.description }}</p>

        <div class="vpn-card__actions">
          <button class="btn btn--primary btn--sm" @click="connectVPN(v)"
                  :disabled="connecting === v.id || !v.is_active">
            {{ connecting === v.id ? 'Proses...' : (v.status === 'CONNECTED' ? 'Tersambung' : 'Connect') }}
          </button>
          <button class="btn btn--ghost btn--sm" @click="disconnectVPN(v)"
                  :disabled="connecting === v.id || v.status !== 'CONNECTED'">
            Disconnect
          </button>
          <button class="btn btn--ghost btn--sm" @click="testVPN(v)" :disabled="testing === v.id">
            {{ testing === v.id ? 'Testing...' : 'Test' }}
          </button>
          <button v-if="auth.isAdmin" class="btn btn--ghost btn--sm" @click="openEdit(v)">Edit</button>
          <button v-if="auth.isAdmin" class="btn btn--ghost btn--sm" @click="setActive(v, !v.is_active)">
            {{ v.is_active ? 'Nonaktifkan' : 'Aktifkan' }}
          </button>
          <button v-if="auth.isAdmin" class="btn btn--dangerghost btn--sm" @click="deleteVPN(v)">Hapus</button>
        </div>
      </div>

      <div v-if="!monitor.vpns.length" class="panel vpn-grid__empty">
        Belum ada VPN. Tambahkan VPN pertama Anda.
      </div>
    </div>

    <div v-if="testResult" class="test-result" :class="testResult.connected ? 'test-result--ok' : 'test-result--fail'">
      <strong>{{ testResult.connected ? 'KONEKSI OK' : 'KONEKSI GAGAL' }}</strong>
      <span>{{ testResult.message }}</span>
      <span v-if="testResult.local_ip"> · Local IP: {{ testResult.local_ip }}</span>
      <button class="btn btn--ghost btn--xs" @click="testResult = null">×</button>
    </div>

    <div v-if="showModal" class="modal-mask" @click.self="showModal = false">
      <div class="modal">
        <h3>{{ editing ? 'Edit VPN' : 'Tambah VPN' }}</h3>
        <form @submit.prevent="save">
          <div class="form-grid">
            <label class="field"><span>Nama VPN *</span>
              <input v-model="form.name" placeholder="VPN-01" required />
            </label>
            <label class="field"><span>Tipe *</span>
              <select v-model="form.vpn_type" required>
                <option>L2TP</option>
                <option>SSTP</option>
                <option>PPTP</option>
                <option>OPENVPN</option>
                <option>WIREGUARD</option>
              </select>
            </label>
            <label class="field"><span>Server Address *</span>
              <input v-model="form.server_address" placeholder="10.10.10.1" required />
            </label>
            <label class="field"><span>Username *</span>
              <input v-model="form.username" required />
            </label>
            <label class="field"><span>Password VPN {{ editing ? '(kosongkan jika tidak berubah)' : '*' }}</span>
              <input v-model="form.password" type="password" :required="!editing" />
            </label>
            <label class="field"><span>Local IP</span>
              <input v-model="form.local_ip" placeholder="10.10.10.2" />
            </label>
            <label class="field field--full"><span>Interface Name (opsional)</span>
              <input v-model="form.interface_name" placeholder="l2tp / sstp" />
            </label>
            <label class="field field--full"><span>Deskripsi</span>
              <input v-model="form.description" />
            </label>
            <label class="check"><input type="checkbox" v-model="form.is_active" /> Aktif</label>
          </div>
          <p v-if="errorMsg" class="login__error">{{ errorMsg }}</p>
          <div class="modal__actions">
            <button type="button" class="btn btn--ghost" @click="showModal = false">Batal</button>
            <button type="submit" class="btn btn--primary">Simpan</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>