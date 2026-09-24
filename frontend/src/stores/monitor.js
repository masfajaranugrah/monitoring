import { defineStore } from 'pinia'
import api from '../api'

export const useMonitorStore = defineStore('monitor', {
  state: () => ({
    stats: {
      total_customers: 0,
      online_count: 0,
      offline_count: 0,
      warning_count: 0,
      active_vpns: 0,
      total_vpns: 0
    },
    vpns: [],
    connected: false,
    lastEvent: null
  }),
  actions: {
    async fetchStats() {
      try {
        const { data } = await api.get('/dashboard/stats')
        this.stats = data
      } catch (e) {
        console.error('stats fetch failed', e)
      }
    },
    async fetchVPNs() {
      try {
        const { data } = await api.get('/vpn')
        this.vpns = data.data || []
      } catch (e) {
        console.error('vpn fetch failed', e)
      }
    },
    async testVPN(id) {
      const { data } = await api.post(`/vpn/${id}/test`)
      await this.fetchVPNs()
      return data
    }
  }
})