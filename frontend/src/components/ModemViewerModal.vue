<script setup>
import { ref, computed, onMounted } from 'vue'
import api from '../api'
import ModemData from './ModemData.vue'
import ModemActions from './ModemActions.vue'

const props = defineProps({
  customer: { type: Object, required: true }
})
const emit = defineEmits(['close'])

const pingHost = ref('8.8.8.8')
const pingCount = ref(4)
const traceHost = ref('8.8.8.8')
const traceHops = ref(15)

const TABS = [
  {
    key: 'status',
    label: 'Status',
    features: [
      { key: 'device', label: 'Device Info', path: '/status/device' },
      { key: 'network-info', label: 'Network Info', path: '/status/network-info' },
      { key: 'user-info', label: 'User Info', path: '/status/user-info' },
      { key: 'voice', label: 'Voice', path: '/status/voice' },
      { key: 'remote-management', label: 'Remote Mgmt', path: '/status/remote-management' }
    ]
  },
  {
    key: 'network',
    label: 'Network',
    features: [
      { key: 'wan', label: 'WAN', path: '/network/wan' },
      { key: 'lan', label: 'LAN / DHCP', path: '/network/lan' },
      { key: 'wlan', label: 'WLAN', path: '/network/wlan' },
      { key: 'routing', label: 'Routing', path: '/network/routing' },
      { key: 'dns', label: 'DNS', path: '/network/dns' },
      { key: 'port-binding', label: 'Port Binding', path: '/network/port-binding' }
    ]
  },
  {
    key: 'security',
    label: 'Security',
    features: [
      { key: 'firewall', label: 'Firewall', path: '/security/firewall' },
      { key: 'ip-filter', label: 'IP Filter', path: '/security/ip-filter' },
      { key: 'mac-filter', label: 'MAC Filter', path: '/security/mac-filter' },
      { key: 'url-filter', label: 'URL Filter', path: '/security/url-filter' },
      { key: 'alg', label: 'ALG', path: '/security/alg' }
    ]
  },
  {
    key: 'application',
    label: 'Application',
    features: [
      { key: 'upnp', label: 'UPnP', path: '/application/upnp' },
      { key: 'ddns', label: 'DDNS', path: '/application/ddns' },
      { key: 'dmz', label: 'DMZ', path: '/application/dmz' },
      { key: 'port-forwarding', label: 'Port Forwarding', path: '/application/port-forwarding' },
      { key: 'sntp', label: 'SNTP', path: '/application/sntp' },
      { key: 'multicast', label: 'Multicast', path: '/application/multicast' },
      { key: 'usb', label: 'USB', path: '/application/usb' },
      { key: 'voip', label: 'VoIP', path: '/application/voip' }
    ]
  },
  {
    key: 'manage',
    label: 'Manage',
    features: [
      { key: 'device', label: 'Device', path: '/manage/device' },
      { key: 'users', label: 'Users', path: '/manage/users' },
      { key: 'time', label: 'Time', path: '/manage/time' },
      { key: 'log', label: 'Log', path: '/manage/log' }
    ]
  },
  {
    key: 'diagnosis',
    label: 'Diagnosis',
    features: [
      {
        key: 'ping',
        label: 'Ping',
        path: '/diagnosis/ping',
        method: 'post',
        form: 'ping',
        body: () => ({ host: pingHost.value.trim(), count: Number(pingCount.value) || 4 })
      },
      {
        key: 'traceroute',
        label: 'Traceroute',
        path: '/diagnosis/traceroute',
        method: 'post',
        form: 'trace',
        body: () => ({ host: traceHost.value.trim(), max_hops: Number(traceHops.value) || 15 })
      },
      { key: 'arp', label: 'ARP', path: '/diagnosis/arp' },
      { key: 'mac-table', label: 'MAC Table', path: '/diagnosis/mac-table' },
      { key: 'optical', label: 'Optical', path: '/diagnosis/optical' },
      { key: 'loopback', label: 'Loopback', path: '/diagnosis/loopback' }
    ]
  },
  {
    key: 'help',
    label: 'Help',
    features: [
      { key: 'help', label: 'Perangkat', path: '/help' },
      { key: 'probe', label: 'Probe', path: '/probe' },
      { key: 'features', label: 'Katalog', path: '/features' }
    ]
  },
  { key: 'actions', label: 'Aksi', features: [] }
]

const activeTab = ref('status')
const activeFeature = ref('device')
const loading = ref(false)
const error = ref('')
const result = ref(null)

const base = computed(() => `/modems/${props.customer.id}`)
const tab = computed(() => TABS.find((t) => t.key === activeTab.value))
const feature = computed(() => tab.value?.features.find((f) => f.key === activeFeature.value))

function selectTab(key) {
  activeTab.value = key
  const first = TABS.find((t) => t.key === key).features[0]
  activeFeature.value = first ? first.key : ''
  load()
}

function selectFeature(key) {
  activeFeature.value = key
  load()
}

async function load() {
  const f = feature.value
  if (!f) return
  loading.value = true
  error.value = ''
  result.value = null
  try {
    const resp =
      f.method === 'post'
        ? await api.post(base.value + f.path, f.body ? f.body() : {})
        : await api.get(base.value + f.path)
    result.value = resp.data
  } catch (e) {
    const d = e.response?.data
    error.value = d?.error || e.message || 'Gagal mengambil data modem'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="modal-mask modem-viewer-mask" @click.self="emit('close')">
    <div class="modal modal--viewer">
      <div class="mv__head">
        <div>
          <h3 class="mv__title">Data Modem</h3>
          <span class="mv__sub mono">{{ customer.customer_code }} · {{ customer.ip_address }}</span>
        </div>
        <button type="button" class="mv__close" aria-label="Tutup" @click="emit('close')">✕</button>
      </div>

      <div class="mv__tabs">
        <button
          v-for="t in TABS"
          :key="t.key"
          type="button"
          class="mv__tab"
          :class="{ 'mv__tab--active': activeTab === t.key }"
          @click="selectTab(t.key)"
        >{{ t.label }}</button>
      </div>

      <div v-if="tab.features.length" class="mv__chips">
        <button
          v-for="f in tab.features"
          :key="f.key"
          type="button"
          class="mv__chip"
          :class="{ 'mv__chip--active': activeFeature === f.key }"
          @click="selectFeature(f.key)"
        >{{ f.label }}</button>
      </div>

      <div class="mv__body">
        <ModemActions v-if="activeTab === 'actions'" :customer="customer" @changed="load" />

        <template v-else>
          <div v-if="feature && feature.form === 'ping'" class="mv__form">
            <label class="mv__field"><span>Host</span>
              <input v-model="pingHost" placeholder="8.8.8.8" @keyup.enter="load" />
            </label>
            <label class="mv__field mv__field--sm"><span>Jumlah</span>
              <input v-model.number="pingCount" type="number" min="1" max="20" />
            </label>
            <button type="button" class="btn btn--primary btn--sm" :disabled="loading" @click="load">Ping</button>
          </div>

          <div v-else-if="feature && feature.form === 'trace'" class="mv__form">
            <label class="mv__field"><span>Host</span>
              <input v-model="traceHost" placeholder="8.8.8.8" @keyup.enter="load" />
            </label>
            <label class="mv__field mv__field--sm"><span>Max Hops</span>
              <input v-model.number="traceHops" type="number" min="1" max="30" />
            </label>
            <button type="button" class="btn btn--primary btn--sm" :disabled="loading" @click="load">Traceroute</button>
          </div>

          <div class="mv__meta">
            <span v-if="result?.title" class="mv__meta-title">{{ result.title }}</span>
            <span v-if="result?.source" class="mv__badge">{{ result.source }}</span>
            <button type="button" class="btn btn--ghost btn--sm" :disabled="loading" @click="load">Muat ulang</button>
          </div>

          <p v-if="result?.warning" class="mv__warning">{{ result.warning }}</p>
          <p v-if="loading" class="mv__status">Menghubungi perangkat...</p>
          <p v-else-if="error" class="login__error">{{ error }}</p>
          <template v-else-if="result?.error">
            <p class="login__error">{{ result.error }}</p>
          </template>
          <template v-else-if="result">
            <ModemData :data="result.data" />
            <pre v-if="result.raw" class="mv__raw">{{ result.raw }}</pre>
          </template>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modem-viewer-mask {
  z-index: 2000;
}
.modal--viewer {
  width: min(96vw, 980px);
  max-width: none;
  height: min(92vh, 820px);
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}
.mv__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 18px;
  border-bottom: 1px solid var(--border);
}
.mv__title {
  margin: 0;
  font-size: 15px;
}
.mv__sub {
  font-size: 12px;
  color: var(--text-dim);
}
.mv__close {
  background: none;
  border: none;
  color: var(--text-faint);
  font-size: 16px;
  cursor: pointer;
  line-height: 1;
}
.mv__tabs {
  display: flex;
  gap: 4px;
  padding: 10px 18px 0;
  overflow-x: auto;
  border-bottom: 1px solid var(--border);
}
.mv__tab {
  padding: 8px 12px;
  border: none;
  border-bottom: 2px solid transparent;
  background: none;
  color: var(--text-dim);
  cursor: pointer;
  font-size: 13px;
  white-space: nowrap;
}
.mv__tab--active {
  color: var(--text);
  border-bottom-color: var(--primary);
}
.mv__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  padding: 12px 18px;
}
.mv__chip {
  padding: 5px 10px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--bg-panel-2);
  color: var(--text-dim);
  cursor: pointer;
  font-size: 12px;
}
.mv__chip--active {
  border-color: var(--primary);
  color: var(--primary);
}
.mv__body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 0 18px 18px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.mv__form {
  display: flex;
  align-items: flex-end;
  gap: 10px;
  flex-wrap: wrap;
  padding: 12px;
  background: var(--bg-panel-2);
  border: 1px solid var(--border);
  border-radius: 8px;
}
.mv__field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  flex: 1;
  min-width: 160px;
}
.mv__field--sm {
  flex: 0 0 90px;
  min-width: 90px;
}
.mv__field span {
  font-size: 11px;
  color: var(--text-dim);
}
.mv__field input {
  background: var(--bg-panel);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 8px 10px;
  font-size: 13px;
  outline: none;
}
.mv__meta {
  display: flex;
  align-items: center;
  gap: 10px;
}
.mv__meta-title {
  font-size: 13px;
  font-weight: 600;
}
.mv__meta .btn {
  margin-left: auto;
}
.mv__badge {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--accent, #22d3ee);
  border: 1px solid var(--border);
  border-radius: 999px;
  padding: 2px 8px;
}
.mv__status {
  color: var(--text-dim);
  font-size: 13px;
}
.mv__warning {
  margin: 0;
  color: #fbbf24;
  font-size: 12.5px;
}
.mv__raw {
  margin: 0;
  padding: 12px;
  background: var(--bg-panel-2);
  border: 1px solid var(--border);
  border-radius: 8px;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
