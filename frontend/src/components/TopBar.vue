<script setup>
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useMonitorStore } from '../stores/monitor'
import { toggleMenu } from '../composables/ui'

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
    <div class="topbar__left">
      <button class="topbar__burger" @click="toggleMenu" aria-label="Buka menu">
        <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="3" y1="6" x2="21" y2="6" /><line x1="3" y1="12" x2="21" y2="12" /><line x1="3" y1="18" x2="21" y2="18" />
        </svg>
      </button>
      <h1 class="topbar__title">{{ titles[route.path] || 'Monitoring' }}</h1>
    </div>

    <div class="topbar__right">
      <span class="topbar__status" :class="{ 'topbar__status--live': monitor.connected }">
        <span class="dot" /> <span class="topbar__status-txt">{{ monitor.connected ? 'LIVE' : 'CONNECTING' }}</span>
      </span>
      <button class="topbar__btn" @click="monitor.fetchVPNs"><span class="topbar__btn-txt">VPN</span> {{ monitor.stats.active_vpns }}/{{ monitor.stats.total_vpns }}</button>
      <button class="topbar__btn" @click="logout">Logout</button>
    </div>
  </header>
</template>