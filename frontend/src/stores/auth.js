import { defineStore } from 'pinia'
import api from '../api'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('fm_token') || '',
    user: JSON.parse(localStorage.getItem('fm_user') || 'null')
  }),
  getters: {
    isAuthenticated: (s) => !!s.token,
    isAdmin: (s) => s.user?.role === 'ADMIN'
  },
  actions: {
    async login(username, password) {
      const { data } = await api.post('/auth/login', { username, password })
      this.token = data.token
      this.user = data.user
      localStorage.setItem('fm_token', data.token)
      localStorage.setItem('fm_user', JSON.stringify(data.user))
    },
    logout() {
      this.token = ''
      this.user = null
      localStorage.removeItem('fm_token')
      localStorage.removeItem('fm_user')
    },
    async changePassword(current, next) {
      await api.post('/auth/change-password', {
        current_password: current,
        new_password: next
      })
    }
  }
})