# Deploy Fiber Monitor dengan PM2 (tanpa Docker)

Panduan deploy ke server Linux (Ubuntu/Debian) menggunakan **PM2** sebagai process manager.
Backend Go melayani REST API **dan** SPA hasil build frontend, jadi cukup satu proses.

> Butuh hak akses jaringan (ICMP/VPN). Jalankan PM2 sebagai **root** atau beri capability
> pada binary (lihat bagian 6).

---

## 1. Prasyarat

```bash
sudo apt update
sudo apt install -y curl git build-essential \
  postgresql postgresql-contrib postgis \
  nginx xl2tpd ppp pptp-linux sstp-client iputils-ping
```

Install Node.js (untuk build frontend) dan PM2 (global):

```bash
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs
sudo npm install -g pm2
```

Install Go 1.22+ (jika build di server):

```bash
curl -fsSL https://go.dev/dl/go1.22.6.linux-amd64.tar.gz -o /tmp/go.tgz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tgz
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh
source /etc/profile.d/go.sh
```

---

## 2. Database PostgreSQL + PostGIS

```bash
sudo -u postgres psql <<'SQL'
CREATE USER fiber_monitor WITH PASSWORD 'GANTI_PASSWORD_KUAT';
CREATE DATABASE fiber_monitor OWNER fiber_monitor;
\c fiber_monitor
CREATE EXTENSION IF NOT EXISTS postgis;
SQL
```

---

## 3. Ambil source & konfigurasi

```bash
sudo mkdir -p /opt/fiber-monitor
sudo chown $USER /opt/fiber-monitor
git clone <url-repo> /opt/fiber-monitor
cd /opt/fiber-monitor

cp .env.example .env
nano .env
```

Isi yang **wajib**:

| Variabel | Keterangan |
|---|---|
| `DATABASE_URL` | `postgres://fiber_monitor:PASSWORD@localhost:5432/fiber_monitor?sslmode=disable` |
| `JWT_SECRET` | acak, generate: `openssl rand -base64 48` |
| `ENCRYPTION_KEY` | acak, generate: `openssl rand -base64 48` |
| `ADMIN_INITIAL_PASSWORD` | password admin pertama |

> Peta memakai Leaflet + OpenStreetMap — tidak butuh API key. `VITE_GOOGLE_MAPS_API_KEY`
> hanya diperlukan jika Anda beralih ke varian peta Google Maps (lihat catatan di akhir).

---

## 4. Build

```bash
cd /opt/fiber-monitor
make build          # bin/fiber-monitor-server + frontend/dist
make sync-dist      # salin SPA ke web/ (diserve oleh Go)
```

Jika `npm ci` gagal karena lock, jalankan `cd frontend && npm install`.

---

## 5. Jalankan dengan PM2

```bash
cd /opt/fiber-monitor
pm2 start ecosystem.config.cjs
pm2 status
pm2 logs fiber-monitor --lines 50
```

Cek health:

```bash
curl http://127.0.0.1:8080/health     # {"status":"ok"}
```

Agar otomatis hidup setelah reboot:

```bash
pm2 save
pm2 startup systemd
# jalankan perintah `sudo env PATH=... pm2 startup ...` yang dicetak PM2
```

---

## 6. Hak akses jaringan (ICMP & VPN)

Opsi A — jalankan PM2 sebagai root (paling sederhana, VPN butuh root):

```bash
sudo env PATH=$PATH:/usr/bin pm2 start ecosystem.config.cjs
sudo pm2 save
sudo env PATH=$PATH:/usr/bin pm2 startup systemd -u root --hp /root
```

Opsi B — jalankan sebagai user biasa + capability pada binary:

```bash
sudo setcap cap_net_raw,cap_net_admin+eip /opt/fiber-monitor/bin/fiber-monitor-server
```

> VPN tunnel (membuat interface) tetap memerlukan `NET_ADMIN`/root.

---

## 7. Nginx reverse proxy

```bash
sudo cp nginx/fiber-monitor.conf /etc/nginx/sites-available/fiber-monitor
sudo nano /etc/nginx/sites-available/fiber-monitor   # ganti server_name
sudo ln -sf /etc/nginx/sites-available/fiber-monitor /etc/nginx/sites-enabled/
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t && sudo systemctl reload nginx
```

HTTPS:

```bash
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d monitor.example.com
```

---

## 8. Update / deploy ulang

```bash
cd /opt/fiber-monitor
git pull origin main
make deploy          # build + sync + pm2 startOrReload + pm2 save
```

Atau manual:

```bash
make build && make sync-dist
pm2 reload ecosystem.config.cjs --update-env
```

---

## 9. Log & troubleshooting

```bash
pm2 logs fiber-monitor            # log realtime
pm2 monit                         # resource monitor
pm2 restart fiber-monitor
tail -f .data/logs/pm2-error.log  # log error
```

| Masalah | Solusi |
|---|---|
| Peta tidak muncul | Pastikan akses ke `tile.openstreetmap.org` tidak diblokir firewall server |
| `permission denied` saat ping | Jalankan PM2 sebagai root atau `setcap` binary (bagian 6) |
| Database tidak connect | Cek `DATABASE_URL` & `systemctl status postgresql` |
| VPN gagal connect | Butuh root/NET_ADMIN; cek `xl2tpd`/`sstpc` terinstall |
| Port 8080 dipakai proses lain | Ubah `SERVER_PORT` di `.env` + `env` pada `ecosystem.config.cjs` |

---

## 10. Backup database

```bash
pg_dump -U fiber_monitor fiber_monitor | gzip > /var/backups/fiber-monitor_$(date +%F).sql.gz
```

Cron harian:

```cron
0 2 * * * pg_dump -U fiber_monitor fiber_monitor | gzip > /var/backups/fiber-monitor_$(date +\%F).sql.gz
```

---

## Ringkasan

| Komponen | Cara jalan |
|---|---|
| Backend Go + SPA | PM2 (`fiber-monitor`), port 8080 |
| Database | PostgreSQL + PostGIS (systemd) |
| Reverse proxy + HTTPS | Nginx + certbot |
| Peta | Leaflet + OpenStreetMap (tanpa API key) |

### Opsional: beralih ke peta Google Maps

Implementasi Google Maps disimpan sebagai `frontend/src/components/MapView.google.vue`
(tidak dipakai saat ini). Untuk mengaktifkannya:

1. Isi `VITE_GOOGLE_MAPS_API_KEY` di `.env`.
2. Ganti `frontend/src/components/MapView.vue` dengan isi `MapView.google.vue`
   (atau import `MapView.google.vue` di tempat yang memakai `MapView.vue`).
3. `make build && make sync-dist`, lalu `pm2 reload fiber-monitor`.

Dependensi `@googlemaps/markerclusterer` sudah terpasang, jadi tidak perlu install ulang.
