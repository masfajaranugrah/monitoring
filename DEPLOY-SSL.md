# Setup SSL (HTTPS) dengan Certbot

Panduan memasang/memasang ulang SSL untuk `monitoring.jernih.net.id`
dengan **certbot + nginx plugin**.

> Deploy berada di **`/var/www/monitoring`** dan nginx memakai
> `/etc/nginx/sites-available/monitoring` (lihat `nginx/monitoring.conf`).

---

## 1. Ganti konfigurasi nginx ke versi HTTP saja

File `nginx/monitoring.conf` di repo sudah versi HTTP-only (tanpa blok `:443`).
Salin ke server:

```bash
sudo cp nginx/monitoring.conf /etc/nginx/sites-available/monitoring
sudo ln -sf /etc/nginx/sites-available/monitoring /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

Cek bahwa nginx masih memuat dengan benar (sudah tidak ada blok `listen 443`):

```bash
sudo nginx -t
```

---

## 2. Prasyarat

- DNS `monitoring.jernih.net.id` harus mengarah ke IP publik server.
- Port **80** dan **443** terbuka dari internet ke server (port-forwarding di router).
- `location /.well-known/acme-challenge/` sudah ada (sudah di config HTTP di atas).

---

## 3. Install certbot + plugin nginx

```bash
sudo apt update
sudo apt install -y certbot python3-certbot-nginx
```

---

## 4. Pasang SSL

```bash
sudo certbot --nginx -d monitoring.jernih.net.id
```

Certbot akan:

1. Mengambil sertifikat via tantangan HTTP-01.
2. **Menyisipkan otomatis** blok `listen 443 ssl` pada
   `/etc/nginx/sites-available/monitoring`, lengkap dengan
   `ssl_certificate`, `ssl_certificate_key`, redirect HTTP→HTTPS, dst.
3. Menjalankan koneksi uji (`nginx -t`) lalu reload nginx.

Ikuti pertanyaan wizard:

| Pertanyaan | Pilihan disarankan |
|---|---|
| Redirect HTTP ke HTTPS? | `2` — **Redirect** (agar semua lalu lintas pakai HTTPS) |

---

## 5. Verifikasi

```bash
curl -I https://monitoring.jernih.net.id          # HTTP/2 200
sudo certbot certificates                         # status sertifikat
systemctl status certbot.timer                    # timer renewal aktif
```

---

## 6. Auto-renewal

Certbot Ubuntu/Debian memasang `systemd timer` (`certbot.timer`) otomatis.
Cek aktif:

```bash
sudo systemctl list-timers | grep certbot
```

Uji proses renewal tanpa mengubah sertifikat:

```bash
sudo certbot renew --dry-run
```

---

## 7. Memasang ulang SSL (jika sertifikat bermasalah)

Jika SSL perlu dimasang ulang dari nol:

```bash
# a) Turunkan dulu semua konfigurasi SSL — gunakan config HTTP saja:
sudo cp nginx/monitoring.conf /etc/nginx/sites-available/monitoring
sudo nginx -t && sudo systemctl reload nginx

# b) Hapus sertifikat lama (opsional, bila mau bersih dari nol)
sudo certbot delete --cert-name monitoring.jernih.net.id

# c) Pasang ulang
sudo certbot --nginx -d monitoring.jernih.net.id
sudo certbot certificates
```

---

## 8. Troubleshooting

| Masalah | Solusi |
|---|---|
| `certbot: command not found` | `sudo apt install -y certbot python3-certbot-nginx` |
| `Temporary failure in name resolution` | Pastikan DNS sudah mengarah ke server (`dig monitoring.jernih.net.id`) |
| `Invalid response from http://.../.well-known/acme-challenge/...` | Pastikan port 80 terbuka & `location /.well-known/acme-challenge/` ada |
| Sertifikat lama walau sudah pasang ulang | `sudo certbot delete --cert-name monitoring.jernih.net.id` lalu install lagi |
| Renewal gagal | `sudo certbot renew --dry-run` untuk cek penyebabnya |

---

## Catatan

- `make deploy` **tidak menyentuh nginx** — sertifikat tetap berlaku selama
  konfigurasi nginx tidak di-overwrite dengan versi HTTP-only.
- Jika ingin `client_max_body_size` lebih besar (upload firmware ZTE ±10–20MB),
  sesuaikan di config; nilai default di sini `50m`.