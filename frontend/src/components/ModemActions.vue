<script setup>
import { ref, computed } from 'vue'
import api from '../api'
import ModemData from './ModemData.vue'
import { useAuthStore } from '../stores/auth'

const props = defineProps({
  customer: { type: Object, required: true }
})
const emit = defineEmits(['changed'])

const auth = useAuthStore()
const canWrite = computed(() => auth.isAdmin)

const base = computed(() => `/modems/${props.customer.id}`)
const busy = ref('')
const feedback = ref(null)

const timeValue = ref(nowString())
const userIndex = ref(0)
const userName = ref('')
const userPass = ref('')
const ssidBand = ref('2.4g')
const ssidIndex = ref(0)
const ssidValue = ref('')
const wanTable = ref('')
const wanIndex = ref(0)
const wanField = ref('')
const wanValue = ref('')
const algProto = ref('')
const rawCommand = ref('sendcmd 1 DB get DeviceInfo')

function nowString() {
  const d = new Date()
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

async function run(key, fn) {
  if (!canWrite.value) return
  busy.value = key
  feedback.value = null
  try {
    const data = await fn()
    feedback.value = { ok: true, text: 'Berhasil', data }
    emit('changed')
  } catch (e) {
    const d = e.response?.data
    feedback.value = { ok: false, text: d?.error || e.message || 'Gagal' }
  } finally {
    busy.value = ''
  }
}

const post = (path, body) => api.post(base.value + path, body).then((r) => r.data)

function reboot() {
  if (!window.confirm('Reboot perangkat sekarang? Koneksi internet pelanggan akan terputus sementara.')) return
  run('reboot', () => post('/manage/reboot', {}))
}

function factoryReset() {
  const answer = window.prompt('Factory reset akan menghapus SELURUH konfigurasi. Ketik "RESET" untuk melanjutkan:')
  if (answer !== 'RESET') return
  run('factory', () => post('/manage/factory-reset', {}))
}

function setTime() {
  run('time', () => post('/manage/time', { value: timeValue.value.trim() }))
}

function setUser() {
  run('user', () =>
    post('/manage/users', {
      index: Number(userIndex.value) || 0,
      user: userName.value.trim(),
      password: userPass.value
    })
  )
}

function setSSID() {
  run('ssid', () =>
    post('/network/wlan/ssid', {
      band: ssidBand.value,
      index: Number(ssidIndex.value) || 0,
      ssid: ssidValue.value.trim()
    })
  )
}

function setFirewall(enable) {
  run('firewall', () => post('/security/firewall', { enable }))
}

function setUpnp(enable) {
  run('upnp', () => post('/application/upnp', { enable }))
}

function setDhcp(enable) {
  run('dhcp', () => post('/network/lan/dhcp', { enable }))
}

function setAlg(enable) {
  run('alg', () => post('/security/alg', { proto: algProto.value.trim().toLowerCase(), enable }))
}

function updateWan() {
  run('wan', () =>
    post('/network/wan', {
      table: wanTable.value.trim(),
      index: Number(wanIndex.value) || 0,
      field: wanField.value.trim(),
      value: wanValue.value
    })
  )
}

function runRaw() {
  run('raw', () => post('/raw', { command: rawCommand.value.trim() }))
}

async function downloadBackup() {
  await run('backup', async () => {
    const resp = await api.get(base.value + '/manage/config/backup', { responseType: 'blob' })
    const url = URL.createObjectURL(resp.data)
    const a = document.createElement('a')
    a.href = url
    a.download = 'config.bin'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    return 'Config perangkat diunduh'
  })
}

function upload(fileKey, path, file) {
  if (!file) return
  const fd = new FormData()
  fd.append(fileKey, file)
  run(fileKey, () => api.post(base.value + path, fd).then((r) => r.data))
}

function onRestoreFile(ev) {
  upload('config', '/manage/config/restore', ev.target.files?.[0])
  ev.target.value = ''
}

function onFirmwareFile(ev) {
  upload('firmware', '/manage/firmware', ev.target.files?.[0])
  ev.target.value = ''
}
</script>

<template>
  <div class="ma">
    <p v-if="!canWrite" class="ma__notice">Aksi tulis hanya tersedia untuk akun ADMIN.</p>

    <div class="ma__grid">
      <!-- Perangkat -->
      <section class="ma__card">
        <h4>Perangkat</h4>
        <div class="ma__row">
          <button class="btn btn--ghost btn--sm" :disabled="!canWrite || busy" @click="reboot">Reboot</button>
          <button class="btn btn--dangerghost btn--sm" :disabled="!canWrite || busy" @click="factoryReset">Factory Reset</button>
        </div>
      </section>

      <!-- Toggle -->
      <section class="ma__card">
        <h4>Toggle Layanan</h4>
        <div class="ma__toggle">
          <span>Firewall</span>
          <div>
            <button class="btn btn--sm" :disabled="!canWrite || busy" @click="setFirewall(true)">ON</button>
            <button class="btn btn--ghost btn--sm" :disabled="!canWrite || busy" @click="setFirewall(false)">OFF</button>
          </div>
        </div>
        <div class="ma__toggle">
          <span>UPnP</span>
          <div>
            <button class="btn btn--sm" :disabled="!canWrite || busy" @click="setUpnp(true)">ON</button>
            <button class="btn btn--ghost btn--sm" :disabled="!canWrite || busy" @click="setUpnp(false)">OFF</button>
          </div>
        </div>
        <div class="ma__toggle">
          <span>DHCP Server</span>
          <div>
            <button class="btn btn--sm" :disabled="!canWrite || busy" @click="setDhcp(true)">ON</button>
            <button class="btn btn--ghost btn--sm" :disabled="!canWrite || busy" @click="setDhcp(false)">OFF</button>
          </div>
        </div>
        <div class="ma__toggle">
          <span>ALG</span>
          <input v-model="algProto" class="ma__inline-input" placeholder="proto (sip/rtsp/...) " />
          <div>
            <button class="btn btn--sm" :disabled="!canWrite || busy" @click="setAlg(true)">ON</button>
            <button class="btn btn--ghost btn--sm" :disabled="!canWrite || busy" @click="setAlg(false)">OFF</button>
          </div>
        </div>
      </section>

      <!-- WiFi -->
      <section class="ma__card">
        <h4>Ubah SSID WiFi</h4>
        <label class="ma__field"><span>Band</span>
          <select v-model="ssidBand">
            <option value="2.4g">2.4G</option>
            <option value="5g">5G</option>
          </select>
        </label>
        <label class="ma__field"><span>Index</span>
          <input v-model.number="ssidIndex" type="number" min="0" />
        </label>
        <label class="ma__field ma__field--grow"><span>SSID</span>
          <input v-model="ssidValue" placeholder="Nama WiFi" />
        </label>
        <button class="btn btn--primary btn--sm" :disabled="!canWrite || busy || !ssidValue.trim()" @click="setSSID">Simpan SSID</button>
      </section>

      <!-- Waktu -->
      <section class="ma__card">
        <h4>Waktu Perangkat</h4>
        <label class="ma__field ma__field--grow"><span>Waktu (YYYY-MM-DD HH:MM:SS)</span>
          <input v-model="timeValue" />
        </label>
        <div class="ma__row">
          <button class="btn btn--ghost btn--sm" :disabled="busy" @click="timeValue = nowString()">Sekarang</button>
          <button class="btn btn--primary btn--sm" :disabled="!canWrite || busy" @click="setTime">Set Waktu</button>
        </div>
      </section>

      <!-- User perangkat -->
      <section class="ma__card">
        <h4>Ubah User Perangkat</h4>
        <div class="ma__row">
          <label class="ma__field"><span>Index</span>
            <input v-model.number="userIndex" type="number" min="0" />
          </label>
          <label class="ma__field ma__field--grow"><span>Username</span>
            <input v-model="userName" autocomplete="off" />
          </label>
        </div>
        <label class="ma__field ma__field--grow"><span>Password baru</span>
          <input v-model="userPass" type="password" autocomplete="new-password" />
        </label>
        <button class="btn btn--primary btn--sm" :disabled="!canWrite || busy || !userPass" @click="setUser">Simpan User</button>
      </section>

      <!-- WAN field -->
      <section class="ma__card">
        <h4>Ubah Field WAN</h4>
        <div class="ma__row">
          <label class="ma__field ma__field--grow"><span>Tabel</span>
            <input v-model="wanTable" placeholder="WANPPPConnection" />
          </label>
          <label class="ma__field"><span>Index</span>
            <input v-model.number="wanIndex" type="number" min="0" />
          </label>
        </div>
        <div class="ma__row">
          <label class="ma__field ma__field--grow"><span>Field</span>
            <input v-model="wanField" placeholder="Username" />
          </label>
          <label class="ma__field ma__field--grow"><span>Value</span>
            <input v-model="wanValue" />
          </label>
        </div>
        <button class="btn btn--primary btn--sm" :disabled="!canWrite || busy || !wanField.trim()" @click="updateWan">Update WAN</button>
      </section>

      <!-- Konfigurasi -->
      <section class="ma__card">
        <h4>Backup / Restore / Firmware</h4>
        <div class="ma__row">
          <button class="btn btn--ghost btn--sm" :disabled="busy" @click="downloadBackup">Unduh Config</button>
          <label class="ma__file" :class="{ 'ma__file--disabled': !canWrite || busy }">
            Restore Config
            <input type="file" :disabled="!canWrite || busy" @change="onRestoreFile" />
          </label>
          <label class="ma__file" :class="{ 'ma__file--disabled': !canWrite || busy }">
            Upgrade Firmware
            <input type="file" :disabled="!canWrite || busy" @change="onFirmwareFile" />
          </label>
        </div>
        <p class="ma__hint">Restore/firmware memakai file dari perangkat. Jangan matikan perangkat saat proses berlangsung.</p>
      </section>

      <!-- Raw -->
      <section class="ma__card ma__card--full">
        <h4>Perintah Telnet (advanced)</h4>
        <textarea v-model="rawCommand" rows="2" class="ma__textarea"></textarea>
        <button class="btn btn--dangerghost btn--sm" :disabled="!canWrite || busy || !rawCommand.trim()" @click="runRaw">Jalankan</button>
      </section>
    </div>

    <div v-if="busy" class="ma__status">Memproses {{ busy }}...</div>
    <div v-if="feedback" class="ma__feedback" :class="{ 'ma__feedback--err': !feedback.ok }">
      <strong>{{ feedback.text }}</strong>
      <ModemData v-if="feedback.ok && feedback.data" :data="feedback.data?.data ?? feedback.data" />
    </div>
  </div>
</template>

<style scoped>
.ma {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.ma__notice {
  margin: 0;
  color: #fbbf24;
  font-size: 12.5px;
}
.ma__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 14px;
}
.ma__card {
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  background: var(--bg-panel-2);
}
.ma__card--full {
  grid-column: 1 / -1;
}
.ma__card h4 {
  margin: 0;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-dim);
}
.ma__row {
  display: flex;
  gap: 8px;
  align-items: flex-end;
  flex-wrap: wrap;
}
.ma__field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  flex: 0 0 auto;
  min-width: 90px;
}
.ma__field--grow {
  flex: 1;
}
.ma__field span {
  font-size: 11px;
  color: var(--text-dim);
}
.ma__field input,
.ma__field select,
.ma__inline-input,
.ma__textarea {
  background: var(--bg-panel);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 13px;
  outline: none;
}
.ma__textarea {
  width: 100%;
  font-family: "SF Mono", Menlo, monospace;
  resize: vertical;
}
.ma__toggle {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ma__toggle > span {
  font-size: 12.5px;
  min-width: 96px;
}
.ma__toggle > div {
  display: flex;
  gap: 6px;
  margin-left: auto;
}
.ma__inline-input {
  flex: 1;
  min-width: 80px;
}
.ma__file {
  position: relative;
  overflow: hidden;
  display: inline-flex;
  align-items: center;
  padding: 6px 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--bg-panel);
  color: var(--text);
  font-size: 13px;
  cursor: pointer;
}
.ma__file--disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.ma__file input {
  position: absolute;
  inset: 0;
  opacity: 0;
  cursor: pointer;
}
.ma__hint {
  margin: 0;
  font-size: 11px;
  color: var(--text-faint);
}
.ma__status {
  font-size: 13px;
  color: var(--text-dim);
}
.ma__feedback {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--bg-panel-2);
}
.ma__feedback strong {
  color: #4ade80;
  font-size: 13px;
}
.ma__feedback--err strong {
  color: #f87171;
}
</style>
