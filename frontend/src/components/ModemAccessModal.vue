<script setup>
import { ref, computed, onMounted } from 'vue'
import api from '../api'

const props = defineProps({
  customer: { type: Object, required: true }
})
const emit = defineEmits(['close'])

const scheme = ref('http')
const port = ref(80)
const frameKey = ref(0)
const loaded = ref(false)
const warming = ref(false)

const ip = computed(() => (props.customer?.ip_address || '').trim())
const id = computed(() => props.customer?.id)

const targetUrl = computed(() => {
  const base = ip.value
  if (!base) return ''
  const s = scheme.value
  const p = Number(port.value) || 80
  const defaultPort = s === 'https' ? 443 : 80
  return p === defaultPort ? `${s}://${base}/` : `${s}://${base}:${p}/`
})

const proxyUrl = computed(() => {
  if (!id.value) return ''
  const p = Number(port.value) || 80
  return `/api/modem/proxy/${id.value}/?scheme=${scheme.value}&port=${p}`
})

// "Pemanasan" dulu via axios (token dikirim lewat header Authorization interceptor,
// tidak pernah tampil di URL) supaya cookie sesi modem terpasang. Iframe & tab
// berikutnya memakai URL bersih yang terautentikasi oleh cookie tersebut.
async function warm() {
  if (!id.value) return
  warming.value = true
  try {
    await api.get(`/modem/proxy/${id.value}/?scheme=${scheme.value}&port=${Number(port.value) || 80}`, {
      withCredentials: true,
      maxRedirects: 0,
      proxy: false
    })
  } catch (e) {
    // Tanggapan 401/4xx/5xx dari modem tetap sudah memasang cookie sebelum body.
  } finally {
    warming.value = false
  }
}

async function bump() {
  loaded.value = false
  await warm()
  frameKey.value++
}

function useScheme(s) {
  scheme.value = s
  port.value = s === 'https' ? 443 : 80
  bump()
}

onMounted(bump)
</script>

<template>
  <div class="modal-mask modem-mask" @click.self="emit('close')">
    <div class="modal modal--wide">
      <div class="modem-head">
        <h3 class="modem-head__title">Akses Modem</h3>
        <span class="mono modem-head__ip">{{ targetUrl }}</span>
        <button type="button" class="modem-head__close" aria-label="Tutup" @click="emit('close')">✕</button>
      </div>

      <div class="modem-bar">
        <div class="modem-bar__scheme">
          <button
            type="button"
            class="modem-scheme"
            :class="{ 'modem-scheme--active': scheme === 'http' }"
            @click="useScheme('http')"
          >http://</button>
          <button
            type="button"
            class="modem-scheme"
            :class="{ 'modem-scheme--active': scheme === 'https' }"
            @click="useScheme('https')"
          >https://</button>
        </div>

        <label class="modem-bar__port">
          <span>Port</span>
          <input v-model.number="port" type="number" min="1" max="65535" @change="bump" />
        </label>

        <span class="modem-bar__url mono">{{ proxyUrl }}</span>

        <div class="modem-bar__actions">
          <button type="button" class="btn btn--ghost btn--sm" @click="bump">Muat ulang</button>
        </div>
      </div>

      <div class="modal__body">
        <div v-if="warming || !loaded" class="modem-loading">{{ warming ? 'Menghubungkan...' : 'Memuat halaman login modem...' }}</div>
        <iframe :key="frameKey" :src="proxyUrl" class="modem-frame" @load="loaded = true"></iframe>
      </div>

      <p class="modem-note">
        Halaman disajikan lewat server (same-origin) sehingga tidak ada masalah mixed-content maupun
        X-Frame-Options. Jika tampak kosong, coba ganti scheme/port lalu <strong>Muat ulang</strong>.
      </p>
    </div>
  </div>
</template>

<style scoped>
.modem-mask {
  z-index: 2100;
}

.modal--wide {
  width: min(96vw, 1100px);
  max-width: none;
  max-height: 92vh;
  display: flex;
  flex-direction: column;
  padding: 0;
  overflow: hidden;
}

.modem-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 18px;
  border-bottom: 1px solid var(--border);
}

.modem-head__title {
  margin: 0;
  font-size: 15px;
}

.modem-head__ip {
  color: var(--text-dim);
  font-size: 12px;
}

.modem-head__close {
  margin-left: auto;
  background: none;
  border: none;
  color: var(--text-faint);
  font-size: 16px;
  cursor: pointer;
  line-height: 1;
}

.modem-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 18px;
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
}

.modem-bar__scheme {
  display: flex;
  gap: 4px;
}

.modem-scheme {
  padding: 5px 10px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--bg-panel-2);
  color: var(--text-faint);
  cursor: pointer;
  font-size: 12px;
}

.modem-scheme--active {
  border-color: var(--accent, #22d3ee);
  color: var(--accent, #22d3ee);
}

.modem-bar__port {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-dim);
}

.modem-bar__port input {
  width: 74px;
  padding: 5px 8px;
  background: var(--bg-panel-2);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 6px;
  font-size: 12px;
}

.modem-bar__url {
  flex: 1;
  min-width: 220px;
  font-size: 12px;
  color: var(--text-faint);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.modem-bar__actions {
  display: flex;
  gap: 6px;
  margin-left: auto;
}

.modal__body {
  position: relative;
  flex: 1;
  min-height: 0;
  height: 64vh;
  background: #fff;
}

.modem-frame {
  width: 100%;
  height: 100%;
  border: none;
  display: block;
  background: #fff;
}

.modem-loading {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-dim);
  background: var(--bg-panel);
}

.modem-note {
  margin: 0;
  padding: 8px 18px 12px;
  font-size: 11px;
  color: var(--text-dim);
}
</style>