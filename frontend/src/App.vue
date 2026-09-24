<script setup>
import { onMounted, onUnmounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import Sidebar from './components/Sidebar.vue'
import TopBar from './components/TopBar.vue'
import { useAuthStore } from './stores/auth'
import { useMonitorStore } from './stores/monitor'
import { startRealtime, stopRealtime } from './services/realtime'

const route = useRoute()
const auth = useAuthStore()
const monitor = useMonitorStore()

// Buka/tutup koneksi WebSocket mengikuti status login, sehingga begitu user
// login koneksi langsung tersambung dan saat logout ikut terputus.
watch(
  () => auth.token,
  (token) => {
    if (token) startRealtime()
    else stopRealtime()
  },
  { immediate: true }
)

onMounted(() => {
  monitor.fetchStats()
  monitor.fetchVPNs()
})
onUnmounted(() => stopRealtime())
</script>

<template>
  <div class="app-shell">
    <Sidebar v-if="!route.meta.public" />
    <div class="app-main" :class="{ 'app-main--auth': route.meta.public }">
      <TopBar v-if="!route.meta.public" />
      <main class="app-content">
        <router-view />
      </main>
    </div>
  </div>
</template>