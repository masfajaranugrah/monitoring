# Panduan Instalasi Fiber Monitor

Terdapat dua cara instalasi:

1. **Docker Compose** (cepat, direkomendasikan untuk pengujian)
2. **Debian 12 native** (produksi, VPN tunnel langsung di host)

---

## Persiapan

Clone repository:

```bash
git clone <url-repo> fiber-monitor && cd fiber-monitor
cp .env.example .env
```

Edit `.env` — **wajib** mengganti dua kunci berikut sebelum production:

```bash
# Generate secret:
openssl rand -base64 48      # untuk JWT_SECRET
openssl rand -base64 32      # untuk ENCRYPTION_KEY
```

Contoh:

```ini
JWT_SECRET=k0TnRj... (hasil openssl)
ENCRYPTION_KEY=x7Q1Pm... (hasil openssl)
```

---

## Opsi A — Docker Compose

Prasyarat: Docker Engine + Docker Compose plugin.

```bash
docker compose up -d --build
docker compose ps          # cek status
docker compose logs -f backend
```

Selesai — akses `http://SERVER_IP:8080`.

**Keunggulan**:
- PostgreSQL + PostGIS, backend, dan frontend otomatis.
- Database persist di volume `pgdata`.

**Catatan VPN**: container diberi `NET_ADMIN` + `NET_RAW` sehingga ping & tunnel berjalan.

---

## Opsi B — Debian 12 native

### 1. Install dependensi sistem

```bash
sudo apt update
sudo apt install -y \
  postgresql postgis postgresql-15-postgis-3 \
  nginx curl \
  golang-go nodejs npm \
  xl2tpd sstp-client iputils-ping \
  build-essential
```

Versi Go/Node yang disarankan: Go ≥ 1.22, Node ≥ 20.
Jika repo Debian terlalu lama, gunakan:
- Go: unduh dari https://go.dev/dl → tar ke `/usr/local`
- Node: `curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -`

### 2. Setup database PostgreSQL + PostGIS

```bash
sudo -u postgres psql <<'SQL'
CREATE USER monitor WITH PASSWORD 'monitor123';
CREATE DATABASE fiber_monitor OWNER monitor;
\c fiber_monitor
CREATE EXTENSION IF NOT EXISTS postgis;
SQL
```

### 3. Letakkan source & build

```bash
sudo mkdir -p /opt/fiber-monitor
sudo chown $(whoami) /opt/fiber-monitor
cp -r backend frontend .env.example Makefile /opt/fiber-monitor/
cd /opt/fiber-monitor
cp .env.example .env

# build backend
cd backend
go mod download
go build -o ../fiber-monitor-server ./cmd/server
cd ..

# build frontend
cd frontend
npm ci
npm run build
cd ..
mkdir -p web && cp -r frontend/dist/* web/
```

### 4. Konfigurasi `.env`

```ini
DATABASE_URL=postgres://monitor:monitor123@localhost:5432/fiber_monitor?sslmode=disable
JWT_SECRET=<acak48>
ENCRYPTION_KEY=<acak32>
ADMIN_INITIAL_USERNAME=admin
ADMIN_INITIAL_PASSWORD=<password-admin-yang-kuat>
```

### 5. Jalankan sebagai systemd service

```bash
sudo cp /opt/fiber-monitor/deployment/fiber-monitor.service /etc/systemd/system/fiber-monitor.service
sudo systemctl daemon-reload
sudo systemctl enable --now fiber-monitor
sudo systemctl status fiber-monitor
curl http://127.0.0.1:8080/health
```

### 6. Nginx reverse proxy

```bash
sudo cp /opt/fiber-monitor/nginx/fiber-monitor.conf /etc/nginx/sites-available/fiber-monitor
sudo ln -s /etc/nginx/sites-available/fiber-monitor /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

Akses `http://SERVER_IP` (port 80).

---

## Verifikasi

1. `curl http://127.0.0.1:8080/health` → `{"status":"ok","monitor":true}`
2. Login web `admin` / password dari `.env`.
3. Menu **VPN Connections** → Tambah VPN → Test.
4. Menu **Customers** → Tambah pelanggan → cek marker di map.
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
| `ping: socket: Operation not permitted` | Jalankan service sebagai root / tambahkan `cap_net_raw=+ep` pada binary |
| VPN tidak konek | Pastikan `xl2tpd`/`sstp-client` terinstal; cek log `journalctl -u fiber-monitor` |
| Login 401 setelah upgrade | Token lama kadaluarsa; `JWT_SECRET` berubah → logout semua |
| SSE terputus | Pastikan nginx `proxy_buffering off` pada `/api/events` |
| Koneksi DB ditolak | Sesuaikan `DATABASE_URL`; pastikan `pg_hba.conf` mengizinkan host |