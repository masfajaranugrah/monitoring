import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/login', name: 'login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
  { path: '/', redirect: '/dashboard' },
  { path: '/dashboard', name: 'dashboard', component: () => import('../views/DashboardView.vue') },
  { path: '/map', name: 'map', component: () => import('../views/MapMonitoringView.vue') },
  { path: '/customers', name: 'customers', component: () => import('../views/CustomersView.vue') },
  { path: '/customers/:id', name: 'customer-detail', component: () => import('../views/CustomerDetailView.vue'), props: true },
  { path: '/vpn', name: 'vpn', component: () => import('../views/VPNView.vue') },
  { path: '/history', name: 'history', component: () => import('../views/PingHistoryView.vue') },
  { path: '/alerts', name: 'alerts', component: () => import('../views/AlertsView.vue') },
  { path: '/backup', name: 'backup', component: () => import('../views/BackupView.vue') },
  { path: '/terminal', name: 'terminal', component: () => import('../views/TerminalView.vue') },
  { path: '/settings', name: 'settings', component: () => import('../views/SettingsView.vue') }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to) => {
  const token = localStorage.getItem('fm_token')
  if (!to.meta.public && !token) {
    return { name: 'login' }
  }
  if (to.meta.public && token && to.name === 'login') {
    return { name: 'dashboard' }
  }
  return true
})

export default router