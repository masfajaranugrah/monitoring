<script setup>
import { ref, onMounted, computed } from 'vue'
import api from '../api'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()

const loading = ref(true)
const creating = ref(false)
const error = ref('')
const backups = ref([])
const info = ref(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const { data } = await api.get('/backups')
    backups.value = data.data || []
    info.value = data.info || null
  } catch (e) {
    error.value = e.response?.data?.error || 'Gagal memuat daftar backup'
  } finally {
    loading.value = false
  }
}

async function createBackup() {
  creating.value = true
  error.value = ''
  try {
    await api.post('/backups')
    await load()
  } catch (e) {
    error.value = e.response?.data?.error || 'Gagal membuat backup'
  } finally {
    creating.value = false
  }
}

function download(b) {
  const url = `/api/backups/${encodeURIComponent(b.file)}`
  const a = document.createElement('a')
  a.href = url
  a.download = b.file
  document.body.appendChild(a)
  a.click()
  a.remove()
}

async function removeBackup(b) {
  if (!window.confirm(`Hapus backup ${b.file}?`)) return
  error.value = ''
  try {
    await api.delete(`/backups/${encodeURIComponent(b.file)}`)
    await load()
  } catch (e) {
    error.value = e.response?.data?.error || 'Gagal menghapus backup'
  }
}

function fmtSize(bytes) {
  if (bytes == null) return '-'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  if (bytes < 1024 * 1024 * 1024) return (bytes / 1024 / 1024).toFixed(2) + ' MB'
  return (bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB'
}

function fmtTime(t) {
  if (!t) return '-'
  return new Date(t).toLocaleString('id-ID')
}

function fmtNext(t) {
  if (!t) return '-'
  return new Date(t).toLocaleString('id-ID')
}

const totalSize = computed(() => backups.value.reduce((s, b) => s + (b.size || 0), 0))
</script>

<template>
  <div class="backup-page">
    <div class="backup-page__head">
      <h2>Backup Database</h2>
      <span class="backup-page__hint">Cadangkan database PostgreSQL (pg_dump .sql) secara manual atau otomatis berkala</span>
      <template v-if="auth.isAdmin">
        <button v-if="info && !info.pg_dump_available" class="btn btn--dangerghost btn--sm" disabled title="pg_dump tidak ditemukan di server">
          pg_dump tidak tersedia
        </button>
      </template>
    </div>

    <div v-if="error" class="backup-page__error">{{ error }}</div>

    <div v-if="loading" class="panel loading-text">Memuat backup...</div>

    <template v-else>
      <div class="backup-stats">
        <div class="backup-stat panel">
          <span class="backup-stat__label">Total Backup</span>
          <span class="backup-stat__value">{{ info?.count ?? backups.length }}</span>
        </div>
        <div class="backup-stat panel">
          <span class="backup-stat__label">Total Ukuran</span>
          <span class="backup-stat__value">{{ fmtSize(totalSize) }}</span>
        </div>
        <div class="backup-stat panel">
          <span class="backup-stat__label">Backup Terakhir</span>
          <span class="backup-stat__value backup-stat__value--sm">{{ info?.last_backup ? fmtTime(info.last_backup) : 'Belum ada' }}</span>
        </div>
        <div class="backup-stat panel">
          <span class="backup-stat__label">Otomatis</span>
          <span class="backup-stat__value" :class="info?.enabled ? 'text-ok' : 'text-muted'">
            {{ info?.enabled ? 'AKTIF' : 'NONAKTIF' }}
          </span>
        </div>
      </div>

      <div class="panel backup-config">
        <div class="kv kv--tight">
          <dt>Interval otomatis</dt><dd>{{ info?.interval_hours ? 'Setiap ' + info?.interval_hours + ' jam' : '-' }}</dd>
          <dt>Retensi</dt><dd>{{ info?.retention ? 'Simpan ' + info?.retention + ' backup terakhir' : '-' }}</dd>
          <dt>Direktori</dt><dd class="mono">{{ info?.dir || '-' }}</dd>
          <dt>Jadwal berikutnya</dt><dd>{{ fmtNext(info?.next_scheduled) }}</dd>
        </div>
        <div class="backup-config__actions">
          <button v-if="auth.isAdmin" class="btn btn--primary btn--sm" :disabled="creating || (info && !info.pg_dump_available)" @click="createBackup">
            {{ creating ? 'Membuat...' : '+ Backup Sekarang' }}
          </button>
          <button class="btn btn--ghost btn--sm" @click="load">Muat Ulang</button>
        </div>
      </div>

      <div class="panel table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>File Backup</th>
              <th>Tanggal Dibuat</th>
              <th>Ukuran</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="b in backups" :key="b.file">
              <td class="mono">{{ b.file }}</td>
              <td>{{ fmtTime(b.created_at) }}</td>
              <td>{{ fmtSize(b.size) }}</td>
              <td class="backup-page__actions">
                <button class="btn btn--ghost btn--sm" @click="download(b)">Unduh</button>
                <button v-if="auth.isAdmin" class="btn btn--dangerghost btn--sm" @click="removeBackup(b)">Hapus</button>
              </td>
            </tr>
            <tr v-if="!backups.length">
              <td colspan="4" class="table__empty">Belum ada backup. Klik "+ Backup Sekarang" untuk membuat cadangan pertama.</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>

<style scoped>
.backup-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.backup-page__head {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.backup-page__head h2 {
  margin: 0;
  font-size: 18px;
}

.backup-page__hint {
  flex: 1;
  color: var(--text-dim);
  font-size: 12.5px;
}

.backup-page__error {
  padding: 10px 14px;
  border-radius: 8px;
  background: rgba(239, 68, 68, 0.12);
  color: #f87171;
  font-size: 12.5px;
  border: 1px solid rgba(248, 113, 113, 0.35);
}

.backup-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 12px;
}

.backup-stat {
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  border-top: 3px solid var(--primary);
}

.backup-stat__label {
  font-size: 11px;
  letter-spacing: 0.07em;
  color: var(--text-dim);
  text-transform: uppercase;
}

.backup-stat__value {
  font-size: 22px;
  font-weight: 800;
  line-height: 1;
}

.backup-stat__value--sm {
  font-size: 13px;
}

.text-ok { color: var(--green); }
.text-muted { color: var(--text-faint); }

.backup-config {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  padding: 14px 16px;
}

.backup-config .kv {
  grid-template-columns: auto auto;
  row-gap: 4px;
}

.backup-config__actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.backup-page__actions {
  display: flex;
  gap: 8px;
}
</style>