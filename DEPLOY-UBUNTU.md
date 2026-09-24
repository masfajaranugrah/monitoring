# Deploy Monitoring ke Ubuntu/Debian

> **Catatan:** deployment sistem ini **tidak memakai Docker**. Panduan resmi ada di
> **[`DEPLOY-PM2.md`](DEPLOY-PM2.md)** — build native + PM2 sebagai process manager.

Ringkasan singkat:

```bash
# 1. Prasyarat
sudo apt update
sudo apt install -y curl git build-essential postgresql postgis \
  nginx xl2tpd ppp pptp-linux sstp-client iputils-ping
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs
sudo npm install -g pm2

# 2. Clone & konfigurasi
cd /opt
sudo git clone <url-repo> monitoring && cd monitoring
cp .env.example .env
nano .env   # DATABASE_URL, JWT_SECRET, ENCRYPTION_KEY

# 3. Build & jalankan
make build
make sync-dist
pm2 start ecosystem.config.cjs
pm2 save && pm2 startup systemd

# 4. Nginx + HTTPS
sudo cp nginx/monitoring.conf /etc/nginx/sites-available/monitoring
sudo ln -sf /etc/nginx/sites-available/monitoring /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d monitor.example.com
```

Lihat [`DEPLOY-PM2.md`](DEPLOY-PM2.md) untuk langkah lengkap, hak akses
ICMP/VPN, update, backup, dan troubleshooting.
