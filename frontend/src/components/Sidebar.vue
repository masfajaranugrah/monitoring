<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import api from '../api'
import { menuOpen, closeMenu } from '../composables/ui'

const route = useRoute()
const auth = useAuthStore()

const items = [
  { to: '/dashboard', icon: 'layout-dashboard', label: 'Dashboard' },
  { to: '/map', icon: 'map', label: 'Map Monitoring' },
  { to: '/customers', icon: 'users', label: 'Customers' },
  { to: '/vpn', icon: 'network', label: 'VPN Connections' },
  { to: '/history', icon: 'activity', label: 'Ping History' },
  { to: '/alerts', icon: 'bell', label: 'Alerts' },
  { to: '/backup', icon: 'archive', label: 'Backup', admin: true },
  { to: '/terminal', icon: 'terminal', label: 'Terminal', admin: true },
  { to: '/settings', icon: 'settings', label: 'Settings' }
]

const sys = ref(null)
let timer = null

async function loadSys() {
  try {
    const { data } = await api.get('/system/stats')
    sys.value = data
  } catch (e) {
    sys.value = null
  }
}

function formatBytes(n) {
  if (n == null) return '-'
  const gb = n / 1024 / 1024 / 1024
  if (gb >= 100) return gb.toFixed(0) + ' GB'
  if (gb >= 1) return gb.toFixed(1) + ' GB'
  return Math.round(n / 1024 / 1024) + ' MB'
}

const stats = computed(() => {
  const s = sys.value
  if (!s) return null
  return {
    cpu: s.cpu_percent,
    ramTotal: formatBytes(s.mem_total),
    ramUsed: formatBytes(s.mem_used),
    ramPct: Math.round(s.mem_percent || 0),
    diskTotal: formatBytes(s.disk_total),
    diskUsed: formatBytes(s.disk_used),
    diskPct: Math.round(s.disk_percent || 0),
    host: s.hostname || '-'
  }
})

function barColor(pct) {
  if (pct == null) return 'var(--text-faint)'
  if (pct >= 90) return 'var(--red)'
  if (pct >= 70) return 'var(--yellow)'
  return 'var(--green)'
}

onMounted(() => {
  loadSys()
  timer = setInterval(loadSys, 10000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <aside class="sidebar" :class="{ 'sidebar--open': menuOpen }">
    <svg width="0" height="0" style="position:absolute" aria-hidden="true" focusable="false">
      <defs>
        <symbol id="icon-layout-dashboard" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="3" width="7" height="9" rx="1.2" /><rect x="14" y="3" width="7" height="5" rx="1.2" />
          <rect x="14" y="12" width="7" height="9" rx="1.2" /><rect x="3" y="16" width="7" height="5" rx="1.2" />
        </symbol>
        <symbol id="icon-map" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polygon points="1 6 1 22 8 18 16 22 23 18 23 2 16 6 8 2 1 6" /><line x1="8" y1="2" x2="8" y2="18" />
          <line x1="16" y1="6" x2="16" y2="22" />
        </symbol>
        <symbol id="icon-users" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" />
          <path d="M23 21v-2a4 4 0 0 0-3-3.87" /><path d="M16 3.13a4 4 0 0 1 0 7.75" />
        </symbol>
        <symbol id="icon-network" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="2" y="2" width="8" height="8" rx="1.5" /><rect x="14" y="2" width="8" height="8" rx="1.5" />
          <rect x="2" y="14" width="8" height="8" rx="1.5" /><rect x="14" y="14" width="8" height="8" rx="1.5" />
          <path d="M10 6h4" /><path d="M6 10v4" /><path d="M18 10v4" /><path d="M10 18h4" />
        </symbol>
        <symbol id="icon-activity" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
        </symbol>
        <symbol id="icon-bell" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" /><path d="M13.73 21a2 2 0 0 1-3.46 0" />
        </symbol>
        <symbol id="icon-terminal" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="4 17 10 11 4 5" /><line x1="12" y1="19" x2="20" y2="19" />
        </symbol>
        <symbol id="icon-archive" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="3" width="18" height="4" rx="1" /><path d="M5 7v12a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V7" /><path d="M10 12h4" />
        </symbol>
        <symbol id="icon-settings" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="3" />
          <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
        </symbol>
      </defs>
    </svg>

    <div class="sidebar__brand">
      <div class="sidebar__logo">FM</div>
      <div class="sidebar__title">
        <strong>Monitoring</strong>
        <span>ISP NOC Dashboard</span>
      </div>
      <button class="sidebar__close" @click="closeMenu" aria-label="Tutup menu">
        <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
      </button>
    </div>

    <nav class="sidebar__nav">
      <router-link
        v-for="item in items"
        v-show="!item.admin || auth.isAdmin"
        :key="item.to"
        :to="item.to"
        class="sidebar__link"
        :class="{ 'sidebar__link--active': route.path.startsWith(item.to) }"
        @click="closeMenu"
      >
        <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <use :href="`#icon-${item.icon}`" />
        </svg>
        <span>{{ item.label }}</span>
      </router-link>
    </nav>

    <div class="sidebar__footer">
      <div v-if="stats" class="sys-stats">
        <div class="sys-stats__title">
          <strong>{{ stats.host }}</strong>
          <span>Server</span>
        </div>
        <div class="sys-stat">
          <div class="sys-stat__head">
            <span>CPU</span>
            <span>{{ stats.cpu >= 0 ? stats.cpu + '%' : '-' }}</span>
          </div>
          <div class="sys-stat__bar">
            <div class="sys-stat__fill" :style="{ width: Math.min(stats.cpu, 100) + '%', background: barColor(stats.cpu) }" />
          </div>
        </div>
        <div class="sys-stat">
          <div class="sys-stat__head">
            <span>RAM <em>{{ stats.ramUsed }} / {{ stats.ramTotal }}</em></span>
            <span>{{ stats.ramPct }}%</span>
          </div>
          <div class="sys-stat__bar">
            <div class="sys-stat__fill" :style="{ width: Math.min(stats.ramPct, 100) + '%', background: barColor(stats.ramPct) }" />
          </div>
        </div>
        <div class="sys-stat">
          <div class="sys-stat__head">
            <span>Disk <em>{{ stats.diskUsed }} / {{ stats.diskTotal }}</em></span>
            <span>{{ stats.diskPct }}%</span>
          </div>
          <div class="sys-stat__bar">
            <div class="sys-stat__fill" :style="{ width: Math.min(stats.diskPct, 100) + '%', background: barColor(stats.diskPct) }" />
          </div>
        </div>
      </div>

      <div class="sys-stats sys-stats--empty" v-else>
        <span class="sys-stats__hint">Memuat server...</span>
      </div>

      <div class="sidebar__user">
        <div class="sidebar__avatar">{{ (auth.user?.username || 'A').slice(0, 1).toUpperCase() }}</div>
        <div>
          <strong>{{ auth.user?.username }}</strong>
          <span class="sidebar__user-role">
            <span class="sidebar__status-dot" />{{ auth.user?.role }}
          </span>
        </div>
      </div>
    </div>
  </aside>
  <div v-if="menuOpen" class="sidebar__backdrop" @click="closeMenu" />
</template>