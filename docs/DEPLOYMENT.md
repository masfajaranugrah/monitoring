# Deployment Guide — Debian 12 (Produksi)

Panduan lengkap men-deploy Fiber Monitor pada server Debian 12 untuk produksi.

---

## 1. Server Requirements

| Sumber daya | Rekomendasi |
|---|---|
| OS | Debian 12 (bookworm) |
| CPU | 2+ vCPU (10.000 pelanggan: 4+ vCPU) |
| RAM | 2 GB (base) – 8 GB (skala besar) |
| Disk | 40 GB SSD+ (utamakan untuk DB) |
| Network | Wajib akses ke jaringan VPN pelanggan |

Karena ICMP ping dan pembuatan interface VPN membutuhkan privilese tinggi,
service dijalankan sebagai **root** atau binary diberi capability.

---

## 2. Hardening Awal

```bash
# Update & upgrade
sudo apt update && sudo apt upgrade -y

# Firewall (ketat: hanya 22, 80, 443 — akses 8080 cukup via localhost/Nginx)
sudo apt install -y ufw
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
# Jika butuh akses langsung dari luar (tidak direkomendasikan):
# sudo ufw allow 8080/tcp
sudo ufw enable

# Auto security updates
sudo apt install -y unattended-upgrades
sudo dpkg-reconfigure unattended-upgrades

# Fail2ban (ssh)
sudo apt install -y fail2ban
```

---

## 3. Install Stack

```bash
sudo apt install -y \
  postgresql postgis postgresql-15-postgis-3 \
  nginx curl golang-go nodejs npm \
  xl2tpd sstp-client iputils-ping build-essential git
```

### Database

```bash
sudo -u postgres psql <<'SQL'
CREATE USER monitor WITH PASSWORD '<STRONG_DB_PASSWORD>';
CREATE DATABASE fiber_monitor OWNER monitor;
GRANT ALL PRIVILEGES ON DATABASE fiber_monitor TO monitor;
SQL
# Jalankan dua perintah berikut sebagai user postgres:
#   psql fiber_monitor -c 'CREATE EXTENSION IF NOT EXISTS postgis;'
sudo -u postgres psql fiber_monitor -c "CREATE EXTENSION IF NOT EXISTS postgis;"
```

Disarankan membuat user DB khusus (bukan root).

---

## 4. Deploy Aplikasi

```bash
sudo mkdir -p /opt/fiber-monitor
sudo chown $(whoami) /opt/fiber-monitor
cp -r backend frontend nginx deployment Makefile /opt/fiber-monitor/
cd /opt/fiber-monitor
cp .env.example .env
```

### Isi `.env` produksi

```ini
SERVER_PORT=8080
GIN_MODE=release

DATABASE_URL=postgres://monitor:<STRONG_DB_PASSWORD>@localhost:5432/fiber_monitor?sslmode=disable

JWT_SECRET=<openssl rand -base64 48>
JWT_EXPIRY_HOURS=24

ENCRYPTION_KEY=<openssl rand -base64 32>

ADMIN_INITIAL_USERNAME=admin
ADMIN_INITIAL_PASSWORD=<STRONG_INITIAL_ADMIN_PASSWORD>

PING_CONCURRENCY=30
PING_HISTORY_RETENTION_DAYS=7
```

Simpan file `.env` dengan permission ketat:

```bash
chmod 600 /opt/fiber-monitor/.env
```

### Build

```bash
cd backend && go mod download && go build -o ../fiber-monitor-server ./cmd/server && cd ..
cd frontend && npm ci && npm run build && cd ..
mkdir -p web && cp -r frontend/dist/* web/
```

---

## 5. Capability untuk Ping (lebih aman daripada root)

Opsional — supaya service tidak perlu `User=root`:

```bash
sudo setcap cap_net_raw+ep /opt/fiber-monitor/fiber-monitor-server
```

Namun untuk **VPN tunnel (interface up/down)** tetap wajib `NET_ADMIN`
(data kemampuan interface membutuhkan root). Untuk produksi penuh kombinasikan:
- `User=root` (simbol kesederhanaan) — dianggap wajar untuk NOC box, ATAU
- service terpisah: monitor ping berjalan user non-root via capability,
  dan VPN manager kecil berjalan root.

---

## 6. Systemd

```bash
sudo cp deployment/fiber-monitor.service /etc/systemd/system/fiber-monitor.service
sudo systemctl daemon-reload
sudo systemctl enable --now fiber-monitor
sudo journalctl -u fiber-monitor -f
```

Cek kesehatan:

```bash
curl http://127.0.0.1:8080/health
# {"status":"ok","monitor":true,...}
```

---

## 7. Nginx + HTTPS (Let's Encrypt)

```bash
sudo cp nginx/fiber-monitor.conf /etc/nginx/sites-available/fiber-monitor
sudo ln -s /etc/nginx/sites-available/fiber-monitor /etc/nginx/sites-enabled/
# Ganti server_name di konfigurasi
sudo sed -i 's/monitor.example.com/YOUR_DOMAIN/g' /etc/nginx/sites-available/fiber-monitor
sudo nginx -t && sudo systemctl reload nginx

# Let's Encrypt
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d YOUR_DOMAIN
```

Konfigurasi nginx sudah:
- Meneruskan `/api/*` dan `/api/events` (dengan `proxy_buffering off`).
- Menyajikan SPA build.
- `Cache-Control` di `/assets/`.

---

## 8. Backup & Maintenance

### Backup database harian (crontab)

```bash
# /etc/cron.d/fiber-monitor-backup
30 2 * * * root  pg_dump -Fc postgres://monitor:...@localhost/fiber_monitor \
  | gzip > /var/backups/fiber_monitor_$(date +\%F).gz && \
  find /var/backups -name 'fiber_monitor_*.gz' -mtime +14 -delete
```

### Monitoring service

```bash
# Status
sudo systemctl status fiber-monitor
# Relog
sudo journalctl -u fiber-monitor -f
```

---

## 9. Skala 10.000+ Pelanggan

Jika `total_checks` dan `ping_results` tumbuh pesat:

1. **Partisi waktu** pada `ping_results`:
   ```sql
   ALTER TABLE ping_results RENAME TO ping_results_old;
   CREATE TABLE ping_results (LIKE ping_results_old INCLUDING ALL)
     PARTITION BY RANGE (pinged_at);
   CREATE TABLE ping_results_2026_09 PARTITION OF ping_results
     FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
   -- dst, atau gunakan partitioner otomatis
   ```
2. **Retensi**: kecilkan `PING_HISTORY_RETENTION_DAYS` mis. 3–7 hari.
3. **Redis (optional)**: cache statistik/agregasi.
4. Naikkan `PING_CONCURRENCY` sesuai kapasitas server (awali 30–50).

---

## 10. Checklist Saat Go Live

- [ ] `JWT_SECRET` & `ENCRYPTION_KEY` diacak (bukan default).
- [ ] Password admin awal diubah.
- [ ] `.env` permission `600`.
- [ ] Firewall hanya buka 22/80/443.
- [ ] HTTPS aktif (certbot).
- [ ] Backup database terjadwal & teruji restore.
- [ ] Semua VPN dites (`/vpn` → Test).
- [ ] Monitoring berjalan: `curl /health` → `monitor:true`.
- [ ] Uji OFFINE simulan: status berubah ke OFFLINE dan alert muncul.