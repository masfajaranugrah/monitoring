<script setup>
import { useMonitorStore } from '../stores/monitor'

const monitor = useMonitorStore()
</script>

<template>
  <div class="stats-grid">
    <div class="stat-card stat-card--total">
      <div class="stat-card__label">TOTAL PELANGGAN</div>
      <div class="stat-card__value">{{ monitor.stats.total_customers.toLocaleString('id-ID') }}</div>
    </div>
    <div class="stat-card stat-card--online">
      <div class="stat-card__label">ONLINE</div>
      <div class="stat-card__value">{{ monitor.stats.online_count.toLocaleString('id-ID') }}</div>
    </div>
    <div class="stat-card stat-card--offline">
      <div class="stat-card__label">OFFLINE</div>
      <div class="stat-card__value">{{ monitor.stats.offline_count.toLocaleString('id-ID') }}</div>
    </div>
    <div class="stat-card stat-card--warning">
      <div class="stat-card__label">WARNING</div>
      <div class="stat-card__value">{{ monitor.stats.warning_count.toLocaleString('id-ID') }}</div>
    </div>
    <div class="stat-card stat-card--vpn">
      <div class="stat-card__label">VPN AKTIF</div>
      <div class="stat-card__value">{{ monitor.stats.active_vpns }}<span class="stat-card__sub">/{{ monitor.stats.total_vpns }}</span></div>
    </div>
  </div>
</template>