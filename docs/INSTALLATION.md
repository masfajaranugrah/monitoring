# Panduan Instalasi Fiber Monitor

Deployment **tanpa Docker**, memakai **PM2** sebagai process manager.
Panduan produksi lengkap: [`../DEPLOY-PM2.md`](../DEPLOY-PM2.md).

---

## Persiapan

Clone repository:

```bash
git clone <url-repo> fiber-monitor && cd fiber-monitor
cp .env.example .env
```

Edit `.env` — **wajib** mengganti kunci berikut sebelum production:

```bash
openssl rand -base64 48      # untuk JWT_SECRET
openssl rand -base64 48      # untuk ENCRYPTION_KEY
```

Contoh:

```ini
DATABASE_URL=postgres://fiber_monitor:PASSWORD@localhost:5432/fiber_monitor?sslmode=disable
JWT_SECRET=k0TnRj... (hasil openssl)
ENCRYPTION_KEY=x7Q1Pm... (hasil openssl)
```

---

## Opsi A — PM2 (direkomendasikan)

### 1. Install dependensi sistem

```bash
sudo apt update
sudo apt install -y \
  postgresql postgresql-contrib postgis \
  nginx curl \
  golang-go nodejs npm \
  xl2tpd sstp-client iputils-ping \
  build-essential

sudo npm install -g pm2
```

Versi Go/Node yang disarankan: Go ≥ 1.22, Node ≥ 20.
Jika repo Debian terlalu lama, gunakan:
- Go: unduh dari https://go.dev/dl → tar ke `/usr/local`
- Node: `curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -`

### 2. Setup database PostgreSQL + PostGIS

```bash
sudo -u postgres psql <<'SQL'
CREATE USER fiber_monitor WITH PASSWORD 'GANTI_PASSWORD_KUAT';
CREATE DATABASE fiber_monitor OWNER fiber_monitor;
\c fiber_monitor
CREATE EXTENSION IF NOT EXISTS postgis;
SQL
```

### 3. Build

```bash
make build          # backend -> bin/, frontend -> frontend/dist
make sync-dist      # SPA -> web/ (diserve oleh backend Go)
```

### 4. Jalankan dengan PM2

```bash
pm2 start ecosystem.config.cjs
pm2 save
pm2 startup systemd   # ikuti perintah sudo yang dicetak
```

### 5. Nginx reverse proxy

```bash
sudo cp nginx/fiber-monitor.conf /etc/nginx/sites-available/fiber-monitor
sudo ln -s /etc/nginx/sites-available/fiber-monitor /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

Akses `http://SERVER_IP` (port 80) atau `http://SERVER_IP:8080` langsung.

---

## Opsi B — Manual (Go native)

Sama seperti Opsi A tetapi menjalankan binary langsung tanpa PM2:

```bash
make build && make sync-dist
./bin/fiber-monitor-server
```

Untuk produksi tetap disarankan PM2/systemd agar otomatis restart.

---

## Verifikasi

1. `curl http://127.0.0.1:8080/health` → `{"status":"ok","monitor":true}`
2. Login web `admin` / password dari `.env`.
3. Menu **VPN Connections** → Tambah VPN → Test.
4. Dashboard → **klik peta** untuk menambah pelanggan (nama + IP) atau lewat menu **Customers**.
5. Matikan salah satu perangkat pelanggan → dalam beberapa interval status berubah
   ONLINE → WARNING → OFFLINE secara otomatis dan muncul alert.

## (Opsional) Data contoh

```bash
psql "$DATABASE_URL" -f backend/migrations/sample_data.sql
```

---

## Troubleshooting

| Gejala | Solusi |
|---|---|
| Peta tidak tampil | Pastikan akses ke `tile.openstreetmap.org` tidak diblokir firewall server |
| `ping: socket: Operation not permitted` | Jalankan PM2 sebagai root / tambahkan `cap_net_raw` pada binary |
| VPN tidak konek | Pastikan `xl2tpd`/`sstp-client` terinstal; cek `pm2 logs fiber-monitor` |
| Login 401 setelah upgrade | Token lama kadaluarsa; `JWT_SECRET` berubah → logout semua |
| SSE terputus | Pastikan nginx `proxy_buffering off` pada `/api/events` |
| Koneksi DB ditolak | Sesuaikan `DATABASE_URL`; pastikan `pg_hba.conf` mengizinkan host |
