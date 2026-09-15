# Setup Database Fiber Monitor

PostgreSQL 16 + PostGIS 3.4 via Docker. Migrasi dan seed admin otomatis saat backend pertama kali start.

---

## Cepat (Docker — recommended)

```bash
cd /opt/fiber-monitor
cp .env.example .env   # edit .env kalau belum
docker compose up -d --build
```

Itu saja. Database akan:
1. Start PostgreSQL + PostGIS container
2. Buat database `fiber_monitor`
3. Backend otomatis jalankan migrasi (buat tabel, enum, index)
4. Backend otomatis buat admin user dari `.env`

---

## Apa yang dibuat otomatis

### Tabel

| Tabel | Fungsi |
|---|---|
| `users` | Akun admin/operator |
| `vpn_connections` | Konfigurasi VPN tunnel |
| `customers` | Data pelanggan + koordinat GPS |
| `ping_results` | Riwayat ping per pelanggan |
| `customer_status_logs` | Log perubahan status |
| `alerts` | Notifikasi offline/warning |

### Enum types

- `customer_status`: ONLINE, OFFLINE, WARNING
- `vpn_status`: CONNECTED, DISCONNECTED, ERROR, DISABLED
- `vpn_type`: L2TP, SSTP, PPTP, OPENVPN, WIREGUARD
- `role_type`: ADMIN, OPERATOR

### Index

- `idx_customers_vpn_id` — index by VPN
- `idx_customers_status` — index by status
- `idx_customers_location` — spatial GIST index (PostGIS)
- `idx_customers_code` — unique search by customer code
- `idx_ping_results_customer_time` — riwayat ping per customer
- `idx_ping_results_time` — query by waktu
- `idx_status_logs_customer` — log per customer
- `idx_alerts_created` — alert by waktu
- `idx_alerts_read` — filter belum dibaca

### Admin user pertama

Dari `.env`:

```
ADMIN_INITIAL_USERNAME=admin
ADMIN_INITIAL_PASSWORD=admin123
```

**Ganti password setelah login pertama!**

---

## Cek status database

```bash
# Cek container
docker compose ps db

# Cek koneksi
docker compose exec db pg_isready -U monitor

# Lihat tabel
docker compose exec db psql -U monitor -d fiber_monitor -c "\dt"

# Lihat jumlah data
docker compose exec db psql -U monitor -d fiber_monitor -c "
SELECT 'users' as tbl, count(*) FROM users
UNION ALL SELECT 'vpn_connections', count(*) FROM vpn_connections
UNION ALL SELECT 'customers', count(*) FROM customers
UNION ALL SELECT 'ping_results', count(*) FROM ping_results
UNION ALL SELECT 'alerts', count(*) FROM alerts;
"
```

---

## Load sample data (opsional)

Untuk testing, ada file sample data yang berisi VPN dummy dan 10 pelanggan contoh:

```bash
docker compose exec -T db psql -U monitor -d fiber_monitor \
  < backend/migrations/sample_data.sql
```

Data yang diinsert:
- 3 VPN (L2TP, SSTP)
- 10 pelanggan di Jawa Tengah/Jawa Barat

---

## Manual setup (tanpa Docker)

Kalau mau install PostgreSQL langsung di server:

```bash
# Install PostgreSQL 15 + PostGIS
sudo apt install -y postgresql postgresql-contrib postgis postgresql-15-postgis-3

# Buat user & database
sudo -u postgres psql -c "CREATE USER monitor WITH PASSWORD 'monitor123';"
sudo -u postgres psql -c "CREATE DATABASE fiber_monitor OWNER monitor;"
sudo -u postgres psql -d fiber_monitor -c "CREATE EXTENSION postgis;"
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE fiber_monitor TO monitor;"
```

Setelah itu edit `.env`:

```
DATABASE_URL=postgres://monitor:monitor123@localhost:5432/fiber_monitor?sslmode=disable
```

Migrasi tetap otomatis jalan saat backend start.

---

## Backup & Restore

### Backup

```bash
docker compose exec -T db pg_dump -U monitor fiber_monitor > backup_$(date +%Y%m%d_%H%M%S).sql
```

### Backup compressed

```bash
docker compose exec -T db pg_dump -U monitor fiber_monitor | gzip > backup_$(date +%Y%m%d_%H%M%S).sql.gz
```

### Restore

```bash
cat backup_20260915.sql | docker compose exec -T db psql -U monitor -d fiber_monitor
```

### Restore compressed

```bash
gunzip -c backup_20260915.sql.gz | docker compose exec -T db psql -U monitor -d fiber_monitor
```

### Auto backup (cron)

```bash
sudo crontab -e
```

Tambah baris ini (backup setiap jam 2 malam):

```
0 2 * * * cd /opt/fiber-monitor && docker compose exec -T db pg_dump -U monitor fiber_monitor | gzip > /var/backups/fiber-monitor_$(date +\%Y\%m\%d).sql.gz 2>/dev/null
```

Buat folder backup:

```bash
sudo mkdir -p /var/backups
```

---

## Troubleshooting

| Masalah | Solusi |
|---|---|
| `FATAL: password authentication failed` | Cek `POSTGRES_PASSWORD` di `.env` cocok |
| `database "fiber_monitor" does not exist` | Restart backend: `docker compose restart backend` |
| `connection refused` | DB belum ready, tunggu atau cek: `docker compose ps` |
| `relation "users" does not exist` | Migrasi belum jalan, cek log backend: `docker compose logs backend` |
| `permission denied for table` | Run: `sudo -u postgres psql -c "GRANT ALL ON ALL TABLES IN SCHEMA public TO monitor;"` |
| `PostGIS extension not found` | Pastikan image `postgis/postgis:16-3.4`, bukan `postgres:16` |
| Container DB restart loop | Cek log: `docker compose logs db` — biasanya disk full atau corrupt volume |

### Reset database (fresh start)

**HATI-HATI: hapus semua data!**

```bash
docker compose down
docker volume rm monitoring_pgdata
docker compose up -d --build
```
