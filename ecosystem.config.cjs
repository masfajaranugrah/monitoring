// PM2 ecosystem untuk Monitoring (tanpa Docker).
//
// Pakai:
//   make build          # build backend (bin/monitoring-server) + frontend (web/)
//   pm2 start ecosystem.config.cjs
//   pm2 save            # simpan proses agar hidup lagi setelah reboot (setelah `pm2 startup`)
//
// Catatan:
// - Binary Go memuat file `.env` sendiri dari `cwd` (root repo ini).
// - Monitoring ICMP/VPN butuh hak akses jaringan, jalankan PM2 sebagai root
//   atau beri capability: sudo setcap cap_net_raw,cap_net_admin+eip bin/monitoring-server
module.exports = {
  apps: [
    {
      name: 'monitoring',
      script: './bin/monitoring-server',
      cwd: __dirname,
      interpreter: 'none',
      instances: 1,
      exec_mode: 'fork',
      autorestart: true,
      watch: false,
      max_memory_restart: '512M',
      kill_timeout: 5000,
      env: {
        GIN_MODE: 'release',
        SERVER_PORT: 8080
      },
      error_file: './.data/logs/pm2-error.log',
      out_file: './.data/logs/pm2-out.log',
      merge_logs: true,
      time: true
    }
  ]
}
