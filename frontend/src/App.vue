<script setup>
import { onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import Sidebar from './components/Sidebar.vue'
import TopBar from './components/TopBar.vue'
import { useMonitorStore } from './stores/monitor'
import { startRealtime, stopRealtime } from './services/realtime'

const route = useRoute()
const monitor = useMonitorStore()

onMounted(() => {
  startRealtime()
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