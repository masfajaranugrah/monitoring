<script setup>
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useMonitorStore } from '../stores/monitor'

const auth = useAuthStore()
const monitor = useMonitorStore()
const router = useRouter()
const route = useRoute()

const titles = {
  '/dashboard': 'Dashboard',
  '/map': 'Map Monitoring',
  '/customers': 'Customers',
  '/vpn': 'VPN Connections',
  '/history': 'Ping History',
  '/alerts': 'Alerts',
  '/settings': 'Settings'
}

function logout() {
  auth.logout()
  router.push('/login')
}
</script>

<template>
  <header class="topbar">
    <h1 class="topbar__title">{{ titles[route.path] || 'Monitoring' }}</h1>

    <div class="topbar__right">
      <span class="topbar__status" :class="{ 'topbar__status--live': monitor.connected }">
        <span class="dot" /> {{ monitor.connected ? 'LIVE' : 'CONNECTING' }}
      </span>
      <button class="topbar__btn" @click="monitor.fetchVPNs">VPN {{ monitor.stats.active_vpns }}/{{ monitor.stats.total_vpns }}</button>
      <button class="topbar__btn" @click="logout">Logout</button>
    </div>
  </header>
</template>