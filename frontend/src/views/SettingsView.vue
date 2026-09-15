<script setup>
import { ref } from 'vue'
import { useAuthStore } from '../stores/auth'
import { useMonitorStore } from '../stores/monitor'

const auth = useAuthStore()
const monitor = useMonitorStore()

const current = ref('')
const next = ref('')
const confirm = ref('')
const message = ref('')
const error = ref('')
const saving = ref(false)

async function changePassword() {
  error.value = ''
  message.value = ''
  if (next.value !== confirm.value) {
    error.value = 'Konfirmasi password tidak sama'
    return
  }
  saving.value = true
  try {
    await auth.changePassword(current.value, next.value)
    message.value = 'Password berhasil diubah'
    current.value = next.value = confirm.value = ''
  } catch (e) {
    error.value = e.response?.data?.error || 'Gagal mengubah password'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="settings-grid">
    <div class="panel detail__card">
      <h3>Account</h3>
      <dl class="kv">
        <dt>Username</dt><dd>{{ auth.user?.username }}</dd>
        <dt>Role</dt><dd>{{ auth.user?.role }}</dd>
        <dt>Nama</dt><dd>{{ auth.user?.full_name || '-' }}</dd>
      </dl>
    </div>

    <div class="panel detail__card">
      <h3>Ubah Password</h3>
      <form @submit.prevent="changePassword" class="settings-form">
        <label class="field"><span>Password saat ini</span>
          <input v-model="current" type="password" required />
        </label>
        <label class="field"><span>Password baru</span>
          <input v-model="next" type="password" required minlength="8" />
        </label>
        <label class="field"><span>Konfirmasi password baru</span>
          <input v-model="confirm" type="password" required minlength="8" />
        </label>
        <p v-if="error" class="login__error">{{ error }}</p>
        <p v-if="message" class="ok-message">{{ message }}</p>
        <button class="btn btn--primary" type="submit" :disabled="saving">
          {{ saving ? 'Menyimpan...' : 'Ubah Password' }}
        </button>
      </form>
    </div>

    <div class="panel detail__card">
      <h3>Sistem</h3>
      <dl class="kv">
        <dt>Realtime</dt><dd>{{ monitor.connected ? 'SSE Connected' : 'Disconnected' }}</dd>
        <dt>Monitoring Engine</dt><dd>Background worker (goroutine)</dd>
        <dt>Database</dt><dd>PostgreSQL + PostGIS</dd>
      </dl>
    </div>
  </div>
</template>