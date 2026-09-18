# Setup Database Fiber Monitor

PostgreSQL 16 + PostGIS. Migrasi dan seed admin otomatis saat backend pertama kali start.
Deployment memakai **PM2 tanpa Docker** — lihat [`DEPLOY-PM2.md`](DEPLOY-PM2.md).

---

## Install PostgreSQL + PostGIS

```bash
sudo apt update
sudo apt install -y postgresql postgresql-contrib postgis

# Cek versi
psql --version
```

### Buat user & database

```bash
sudo -u postgres psql <<'SQL'
CREATE USER fiber_monitor WITH PASSWORD 'GANTI_PASSWORD_KUAT';
CREATE DATABASE fiber_monitor OWNER fiber_monitor;
\c fiber_monitor
CREATE EXTENSION IF NOT EXISTS postgis;
GRANT ALL PRIVILEGES ON DATABASE fiber_monitor TO fiber_monitor;
SQL
```

### Set `.env`

```ini
DATABASE_URL=postgres://fiber_monitor:GANTI_PASSWORD_KUAT@localhost:5432/fiber_monitor?sslmode=disable
```

Saat backend start, otomatis:

1. Konek ke database `fiber_monitor`
2. Jalankan migrasi (tabel, enum, index)
3. Buat admin user dari `.env`

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
# Service aktif?
systemctl status postgresql

# Koneksi siap?
pg_isready -h localhost -p 5432

# Lihat tabel
psql "$DATABASE_URL" -c "\dt"

# Lihat jumlah data
psql "$DATABASE_URL" -c "
SELECT 'users' as tbl, count(*) FROM users
UNION ALL SELECT 'vpn_connections', count(*) FROM vpn_connections
UNION ALL SELECT 'customers', count(*) FROM customers
UNION ALL SELECT 'ping_results', count(*) FROM ping_results
UNION ALL SELECT 'alerts', count(*) FROM alerts;
"
```

---

## Load sample data (opsional)

Untuk testing, ada file sample data berisi VPN dummy dan 10 pelanggan contoh:

```bash
psql "$DATABASE_URL" -f backend/migrations/sample_data.sql
```

Data yang diinsert:
- 3 VPN (L2TP, SSTP)
- 10 pelanggan di Jawa Tengah/Jawa Barat

---

## Backup & Restore

### Backup

```bash
pg_dump "$DATABASE_URL" > backup_$(date +%Y%m%d_%H%M%S).sql
```

### Backup compressed

```bash
pg_dump "$DATABASE_URL" | gzip > backup_$(date +%Y%m%d_%H%M%S).sql.gz
```

### Restore

```bash
psql "$DATABASE_URL" < backup_20260915.sql
```

### Restore compressed

```bash
gunzip -c backup_20260915.sql.gz | psql "$DATABASE_URL"
```

### Auto backup (cron)

```bash
sudo crontab -e
```

Tambah baris ini (backup setiap jam 2 malam):

```
0 2 * * * pg_dump "postgres://fiber_monitor:PASSWORD@localhost:5432/fiber_monitor" | gzip > /var/backups/fiber-monitor_$(date +\%Y\%m\%d).sql.gz 2>/dev/null
```

Buat folder backup:

```bash
sudo mkdir -p /var/backups
```

---

## Troubleshooting

| Masalah | Solusi |
|---|---|
| `FATAL: password authentication failed` | Cek `DATABASE_URL` di `.env` cocok dengan user/password PostgreSQL |
| `database "fiber_monitor" does not exist` | Buat ulang database (lihat atas) lalu restart: `pm2 restart fiber-monitor` |
| `connection refused` | Pastikan `systemctl start postgresql` berjalan |
| `relation "users" does not exist` | Migrasi belum jalan, cek log: `pm2 logs fiber-monitor` |
| `permission denied for table` | `sudo -u postgres psql -c "GRANT ALL ON ALL TABLES IN SCHEMA public TO fiber_monitor;"` |
| `PostGIS extension not found` | `sudo -u postgres psql -d fiber_monitor -c "CREATE EXTENSION postgis;"` |

### Reset database (fresh start)

**HATI-HATI: hapus semua data!**

```bash
sudo -u postgres psql <<'SQL'
DROP DATABASE fiber_monitor;
CREATE DATABASE fiber_monitor OWNER fiber_monitor;
\c fiber_monitor
CREATE EXTENSION IF NOT EXISTS postgis;
SQL
pm2 restart fiber-monitor
```
