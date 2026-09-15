# Deploy Fiber Monitor ke Ubuntu 20.04

Panduan lengkap deploy dari GitHub ke server Ubuntu 20.04.

---

## 1. Persiapan Server

### Update system

```bash
sudo apt update && sudo apt upgrade -y
```

### Install dependencies dasar

```bash
sudo apt install -y curl wget git ufw fail2ban
```

### Buka port firewall

```bash
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

---

## 2. Install Docker & Docker Compose

Ubuntu 20.04 tidak punya Docker Compose v2 default, jadi install manual.

```bash
# Install Docker
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER

# Install Docker Compose plugin
sudo mkdir -p /usr/local/lib/docker/cli-plugins
sudo curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-$(uname -m) \
  -o /usr/local/lib/docker/cli-plugins/docker-compose
sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-compose

# Verifikasi
docker --version
docker compose version
```

> **Logout & login lagi** agar group `docker` aktif, atau jalankan `newgrp docker`.

---

## 3. Clone & Konfigurasi

```bash
# Clone repo
cd /opt
sudo git clone https://github.com/YOUR-ORG/fiber-monitor.git
cd fiber-monitor

# Buat file .env
cp .env.example .env
```

### Edit `.env` — WAJIB diubah

```bash
nano .env
```

Nilai yang **harus diganti** untuk production:

| Variabel | Keterangan |
|---|---|
| `JWT_SECRET` | String acak minimal 32 karakter. Generate: `openssl rand -base64 48` |
| `ENCRYPTION_KEY` | String acak untuk enkripsi password VPN. Generate: `openssl rand -base64 48` |
| `POSTGRES_PASSWORD` | Password database, ganti dari default `monitor123` |
| `ADMIN_INITIAL_PASSWORD` | Password admin pertama, ganti dari default `admin123` |

---

## 4. Build & Jalankan (Docker)

```bash
# Build & start semua service
docker compose up -d --build

# Cek status
docker compose ps

# Lihat log
docker compose logs -f backend
```

### Cek health

```bash
curl http://localhost:8080/health
```

Harus返回 `{"status":"ok"}`.

---

## 5. Install Nginx (Reverse Proxy)

```bash
sudo apt install -y nginx
```

### Copy config Nginx

```bash
sudo cp nginx/fiber-monitor.conf /etc/nginx/sites-available/fiber-monitor
```

### Edit config — ganti `server_name`

```bash
sudo nano /etc/nginx/sites-available/fiber-monitor
```

Ganti `monitor.example.com` dengan domain/IP server kamu.

### Aktifkan site

```bash
sudo ln -sf /etc/nginx/sites-available/fiber-monitor /etc/nginx/sites-enabled/
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t
sudo systemctl reload nginx
```

---

## 6. Aktifkan HTTPS (Let's Encrypt)

```bash
sudo apt install -y certbot python3-certbot-nginx

# Pastikan domain sudah DNS指向 ke IP server, lalu:
sudo certbot --nginx -d monitor.example.com

# Auto-renew sudah diatur certbot via systemd timer
sudo systemctl status certbot.timer
```

---

## 7. Setup SSL VPN (opsional)

Jika server ini juga jadi VPN endpoint:

```bash
sudo apt install -y xl2tpd ppp pptp-linux
```

Untuk SSTP client sudah termasuk di Docker image.

---

## 8. Update / Deploy Ulang

```bash
cd /opt/fiber-monitor

# Pull perubahan terbaru
git pull origin main

# Rebuild & restart
docker compose up -d --build
```

---

## 9. Backup Database

```bash
# Manual backup
docker compose exec db pg_dump -U monitor fiber_monitor > backup_$(date +%Y%m%d).sql

# Restore
cat backup_20260915.sql | docker compose exec -T db psql -U monitor fiber_monitor
```

### Auto backup (cron)

```bash
sudo crontab -e
```

Tambahkan:

```
0 2 * * * cd /opt/fiber-monitor && docker compose exec -T db pg_dump -U monitor fiber_monitor | gzip > /var/backups/fiber-monitor_$(date +\%Y\%m\%d).sql.gz
```

---

## 10. Monitoring & Troubleshooting

### Cek semua service

```bash
docker compose ps
docker compose logs --tail=50 backend
docker compose logs --tail=50 db
```

### Restart service

```bash
docker compose restart backend
```

### Masuk ke container (debug)

```bash
docker compose exec backend sh
```

### Cek koneksi database

```bash
docker compose exec db psql -U monitor -d fiber_monitor -c "\dt"
```

### Common issues

| Masalah | Solusi |
|---|---|
| `permission denied` di ping | Pastikan container punya `NET_RAW` capability (sudah di docker-compose.yml) |
| Database tidak connect | Tunggu beberapa detik, DB butuh waktu warm-up. Cek: `docker compose ps` |
| Port 5432 already in use | Matikan PostgreSQL host: `sudo systemctl stop postgresql` |
| Frontend 404 | Pastikan build frontend sudah jalan: `docker compose up -d --build` |

---

## Ringkasan Service

| Service | Port | Keterangan |
|---|---|---|
| **Nginx** | 80, 443 | Reverse proxy + HTTPS |
| **Fiber Monitor Backend** | 8080 (internal) | Go API server |
| **PostgreSQL + PostGIS** | 5432 (internal) | Database |

Akses app: `https://monitor.example.com`

---

## Struktur Deployment

```
/opt/fiber-monitor/
├── .env                    # Konfigurasi (jangan commit)
├── docker-compose.yml
├── Dockerfile
├── backend/                # Go source
├── frontend/               # Vue source
├── nginx/                  # Nginx config
├── deployment/             # systemd unit (native install)
└── bin/                    # Build output (native install)
```
