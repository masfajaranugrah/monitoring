<script setup>
import { useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const auth = useAuthStore()

const items = [
  { to: '/dashboard', icon: 'layout-dashboard', label: 'Dashboard' },
  { to: '/map', icon: 'map', label: 'Map Monitoring' },
  { to: '/customers', icon: 'users', label: 'Customers' },
  { to: '/vpn', icon: 'network', label: 'VPN Connections' },
  { to: '/history', icon: 'activity', label: 'Ping History' },
  { to: '/alerts', icon: 'bell', label: 'Alerts' },
  { to: '/settings', icon: 'settings', label: 'Settings' }
]
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
      <div class="sidebar__user">
        <div class="sidebar__avatar">{{ (auth.user?.username || 'A').slice(0, 1).toUpperCase() }}</div>
        <div>
          <strong>{{ auth.user?.username }}</strong>
          <span>{{ auth.user?.role }}</span>
        </div>
      </div>
    </div>
  </aside>
</template>