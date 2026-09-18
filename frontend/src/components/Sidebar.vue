<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import api from '../api'

const route = useRoute()
const auth = useAuthStore()

const items = [
  { to: '/dashboard', icon: 'layout-dashboard', label: 'Dashboard' },
  { to: '/map', icon: 'map', label: 'Map Monitoring' },
  { to: '/customers', icon: 'users', label: 'Customers' },
  { to: '/vpn', icon: 'network', label: 'VPN Connections' },
  { to: '/history', icon: 'activity', label: 'Ping History' },
  { to: '/alerts', icon: 'bell', label: 'Alerts' },
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
  <aside class="sidebar">
    <div class="sidebar__brand">
      <div class="sidebar__logo">FM</div>
      <div class="sidebar__title">
        <strong>Monitoring</strong>
        <span>ISP NOC Dashboard</span>
      </div>
    </div>

    <nav class="sidebar__nav">
      <router-link
        v-for="item in items"
        v-show="!item.admin || auth.isAdmin"
        :key="item.to"
        :to="item.to"
        class="sidebar__link"
        :class="{ 'sidebar__link--active': route.path.startsWith(item.to) }"
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
</template>