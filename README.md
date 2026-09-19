# Monitoring — ISP Customer Connection Monitoring

Aplikasi **production-ready** untuk memonitor koneksi pelanggan ISP dari sebuah VPS/Linux.
Server monitoring terhubung ke jaringan pelanggan melalui **VPN MikroTik L2TP/SSTP** (hingga 10 VPN aktif),
melakukan **ICMP ping** ke IP pelanggan secara paralel, dan menampilkan hasilnya secara **realtime** di
map dashboard bergaya NOC.

---

## Fitur

| Fitur | Keterangan |
|---|---|
| Monitoring paralel | Worker pool goroutine, bukan ping blocking per pelanggan |
| Status otomatis | `ONLINE` / `WARNING` / `OFFLINE` dengan retry state machine |
| Map realtime | Leaflet + OpenStreetMap, marker berwarna, clustering, zoom |
| Realtime update | SSE (Server-Sent Events) — tanpa reload halaman |
| VPN Manager | L2TP (xl2tpd), SSTP (sstpc), hingga 10 VPN, test connection |
| Customer CRUD | Search, filter status/VPN, sort, pagination, modal form |
| Detail pelanggan | Info lengkap + grafik latency + riwayat ping |
| Ping history | Riwayat status & latency untuk hitung uptime |
| Alert system | Event & log OFFLINE (siap di-extend ke Telegram/WhatsApp/Email) |
| **Modem API** | **Kontrol penuh ONT ZTE ZXHN F663NV9: Status, Network, Security, Application, Manage, Diagnosis, Help (web + telnet)** |
| Keamanan | JWT auth, hash bcrypt, password VPN terenkripsi AES-256-GCM |
| Target skala | 10 VPN, 10.000+ pelanggan |

---

## Arsitektur

```
VPS
│
├── VPN Manager
│   ├── VPN-01  ...  VPN-10        (xl2tpd / sstpc)
│
├── Monitoring Engine
│   ├── ICMP Ping (golang.org/x/net/icmp, fallback system ping)
│   ├── Status Detection          (retry state machine)
│   ├── Latency Measurement
│   └── Background Worker         (goroutine worker pool)
│
├── REST API                      (Gin Framework)
│
└── PostgreSQL + PostGIS
        │
        ▼
    Vue 3 + Leaflet  (dashboard web)
```

### Status Logic

```
Ping SUCCESS              → ONLINE  (consecutive failures reset ke 0)
Ping FAIL ke-1            → WARNING
Ping FAIL ke-2 (retry)    → WARNING
Ping FAIL ke-3 (> retry)  → OFFLINE
Kembali berhasil          → ONLINE
```

### Indikator Latency

| Latency | Indikator |
|---|---|
| < 30 ms | GOOD |
| 30 – 100 ms | WARNING |
| > 100 ms | HIGH |

---

## Struktur Folder

```
monitoring/
├── backend/
│   ├── cmd/server/main.go        # entry point, routing
│   ├── internal/
│   │   ├── config/               # konfigurasi env
│   │   ├── database/             # koneksi + migrasi + seed admin
│   │   ├── models/               # model & enum
│   │   ├── middleware/           # JWT auth + role
│   │   ├── handlers/             # REST API (auth, customer, vpn, dashboard, alerts)
│   │   ├── ping/                 # ICMP pinger + monitoring engine (worker)
│   │   ├── vpn/                  # VPN manager (xl2tpd / sstpc)
│   │   ├── sse/                  # realtime hub SSE
│   │   ├── store/                # persistence (state machine, history, retention)
│   │   └── crypto/               # enkripsi password VPN
│   └── go.mod
├── frontend/
│   ├── src/
│   │   ├── views/                # Dashboard, Map, Customers, Detail, VPN, History, Alerts, Settings, Login
│   │   ├── components/           # MapView (Leaflet), Sidebar, TopBar, badges, chart
│   │   ├── stores/               # Pinia (auth, monitor)
│   │   ├── api/                  # axios client
│   │   ├── services/             # SSE realtime client
│   │   └── router/
│   ├── package.json
│   └── vite.config.js
├── nginx/monitoring.conf
├── deployment/monitoring.service
├── ecosystem.config.cjs          # konfigurasi PM2
├── DEPLOY-PM2.md                 # panduan deploy tanpa Docker
├── Makefile
├── .env.example
└── README.md
```

---

## Prasyarat (Server)

```
Debian 12 / Ubuntu 20.04+, akses root, koneksi internet
PostgreSQL 15+ dengan PostGIS
Go 1.22+ (build), Node 20+ (build frontend), PM2, Nginx
Klien VPN: xl2tpd (L2TP), sstp-client (SSTP)
```

---

## Instalasi — PM2 (tanpa Docker)

Panduan lengkap: [`DEPLOY-PM2.md`](DEPLOY-PM2.md). Ringkasnya:

```bash
git clone <url-repo> monitoring && cd monitoring

# 1. Environment
cp .env.example .env
#   isi DATABASE_URL, JWT_SECRET, ENCRYPTION_KEY
#   openssl rand -base64 48   → JWT_SECRET / ENCRYPTION_KEY

# 2. Build backend + frontend, lalu salin SPA ke web/
make build
make sync-dist

# 3. Jalankan dengan PM2
pm2 start ecosystem.config.cjs
pm2 save
pm2 startup systemd   # agar otomatis start setelah reboot
```

Akses: `http://SERVER_IP:8080` — login `admin` / `admin123` (ubah setelah login).

> Peta memakai Leaflet + OpenStreetMap (gratis, tanpa API key).
> Tidak perlu konfigurasi tambahan saat build.

**Catatan penting:**
- Backend Go melayani REST API **dan** SPA, cukup satu proses PM2.
- Monitoring engine tetap berjalan walau browser ditutup.
- ICMP ping / VPN butuh root atau `cap_net_raw,cap_net_admin`; jalankan PM2 sebagai
  root atau `sudo setcap cap_net_raw,cap_net_admin+eip bin/monitoring-server`.
- Deploy ulang cukup: `make deploy`.

---

## Instalasi — Manual (Go native)

```bash
# 1. Prasyarat
sudo apt update
sudo apt install -y postgresql postgis golang nodejs npm nginx xl2tpd sstp-client iputils-ping

# 2. Database
sudo -u postgres psql <<'SQL'
CREATE USER monitor WITH PASSWORD 'monitor123';
CREATE DATABASE monitoring OWNER monitor;
CREATE EXTENSION IF NOT EXISTS postgis;
SQL

# 3. Build
cp .env.example .env
#   edit DATABASE_URL, JWT_SECRET, ENCRYPTION_KEY
make build
make sync-dist

# 4. Jalankan
./bin/monitoring-server
curl http://127.0.0.1:8080/health   # cek

# 5. Nginx reverse proxy
sudo cp nginx/monitoring.conf /etc/nginx/sites-available/monitoring
sudo ln -s /etc/nginx/sites-available/monitoring /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

**Catatan penting:**
- ICMP ping membutuhkan root / `cap_net_raw` (atau fallback ke binary `ping` sistem).
- Untuk VPN tunnel, proses perlu membuat interface (root) dan privilese `NET_ADMIN`.

---

## Penggunaan

1. Login → buat **VPN** (`/vpn`): Nama, Tipe (L2TP/SSTP), Server, Username, Password.
   Tekan **Test** untuk memverifikasi koneksi tunnel.
2. **Dashboard** `/dashboard`: peta (Leaflet + OpenStreetMap) penuh. **Klik titik di peta** → muncul modal,
   isi **Nama** + **IP** saja (koordinat terisi otomatis). Marker **hijau** online,
   **merah** offline, **kuning** warning. Klik marker → popup detail → halaman detail.
3. Tambah/kelola **Pelanggan** (`/customers`): CRUD lengkap, kode, nama, IP,
   latitude/longitude, VPN, interval ping. IP & koordinat divalidasi server.
   Kode pelanggan boleh dikosongkan → digenerate otomatis dari IP (mis. `CO-10-10-10-55`).
4. `/map`: peta + daftar pelanggan + search + filter status/VPN. Klik item daftar → peta fokus.
5. Halaman **Ping History**, **Alerts**, dan **Settings** tersedia di sidebar.

---

## REST API

Semua endpoint (kecuali `/auth/login`) wajib header:
`Authorization: Bearer <token>`

| Method | Endpoint | Keterangan | Role |
|---|---|---|---|
| POST | `/api/auth/login` | Login | publik |
| GET | `/api/events?token=` | Realtime WebSocket stream | Bearer* |
| GET | `/api/auth/me` | Info user | – |
| POST | `/api/auth/change-password` | Ganti password | – |
| GET | `/api/dashboard/stats` | Statistik dashboard | – |
| GET | `/api/map/customers?status=&vpn_id=` | Data marker peta | – |
| GET/POST | `/api/customers` | List (search/filter/sort/page) / create | create: ADMIN |
| GET/PUT/DELETE | `/api/customers/:id` | Detail / update / hapus | hapus: ADMIN |
| PATCH | `/api/customers/:id/monitoring` | Toggle monitoring | – |
| GET | `/api/customers/:customer_id/ping-history` | Riwayat ping | – |
| GET | `/api/customers/:customer_id/status-logs` | Log transisi status | – |
| GET/POST | `/api/vpn` | List / create | create: ADMIN |
| GET/PUT/DELETE | `/api/vpn/:id` | Detail / update / hapus | ADMIN |
| PATCH | `/api/vpn/:id/active` | Aktif/nonaktif | ADMIN |
| POST | `/api/vpn/:id/test` | Test koneksi VPN | – |
| POST | `/api/vpn/refresh` | Refresh status semua VPN | ADMIN |
| GET | `/api/alerts` | Daftar alert | – |
| PATCH | `/api/alerts/:id/read` | Tandai dibaca | – |

> \* Koneksi WebSocket browser tidak bisa set header `Authorization`, sehingga token JWT
> dikirim via query string (`/api/events?token=<jwt>`). Pastikan token selalu valid; koneksi
> akan ditutup server jika token invalid/expired.

Contoh query list customer:

```
GET /api/customers?search=budi&status=ONLINE&vpn_id=1&sort_by=latency&sort_order=desc&page=1&page_size=50
```

Return: `{ data, total, page, page_size, pages }`

> `POST /api/customers` — field `customer_code` **opsional**. Jika dikosongkan, server
> akan generate kode dari IP pelanggan (mis. `CO-10-10-10-55`) dan memastikan unik.

---

## Modem Management API — ZTE ZXHN F663NV9

Seluruh menu Web UI perangkat ONT/ONU ZTE (model acuan **ZXHN F663NV9**) diekspos
menjadi REST API. Client ada di `backend/internal/modem` dan mengakses perangkat
lewat **dua jalur**:

1. **Web management** (CGI/login token) — untuk aksi web (reboot, backup/restore,
   upgrade firmware) dan data halaman.
2. **Telnet + `sendcmd 1 DB`** — akses langsung database konfigurasi perangkat
   (paling lengkap & stabil) untuk Status/Network/Security/Application/Manage/Diagnosis.

Field database ZTE dinormalisasi: setiap tabel DB dipetakan menjadi
`{ "table": "WLANCfg", "rows": [ { "SSID1": "...", ... } ] }`, sehingga variasi
penamaan antar-firmware tidak mengubah bentuk response.

### Kredensial

Kredensial diambil berurutan dari:

1. Data pelanggan (`modem_web_user`, `modem_web_pass`, `modem_telnet_user`,
   `modem_telnet_pass`, `modem_web_port`, `modem_web_https`, `modem_telnet_port`)
   — password web/telnet dienkripsi AES-256-GCM sebelum disimpan.
2. Default environment `MODEM_WEB_USER`, `MODEM_WEB_PASS`, `MODEM_TELNET_USER`,
   `MODEM_TELNET_PASS`, `MODEM_WEB_PORT`, `MODEM_WEB_HTTPS`, `MODEM_TELNET_PORT`.

### Endpoint (prefix `/api/modems/<customer_id>`)

Semua endpoint butuh `Authorization: Bearer <token>` dan VPN/routing server harus
 bisa menjangkau IP pelanggan.

| Method | Endpoint | Keterangan | Role |
|---|---|---|---|
| GET | `/api/modems/:id/features` | Katalog endpoint modem | – |
| GET | `/api/modems/:id/probe` | Tes konektivitas web & telnet | – |
| GET | `/api/modems/:id/help` | Info bantuan/versi perangkat | – |
| GET | `/api/modems/:id/status/device` | Device Information | – |
| GET | `/api/modems/:id/status/network-info` | Network Information | – |
| GET | `/api/modems/:id/status/user-info` | User Information | – |
| GET | `/api/modems/:id/status/voice` | Voice Message | – |
| GET | `/api/modems/:id/status/remote-management` | Remote Management | – |
| GET | `/api/modems/:id/network/wan` | Konfigurasi WAN | – |
| POST | `/api/modems/:id/network/wan` | Ubah field WAN | ADMIN |
| GET | `/api/modems/:id/network/lan` | LAN / DHCP | – |
| POST | `/api/modems/:id/network/lan/dhcp` | Toggle DHCP server | ADMIN |
| GET | `/api/modems/:id/network/wlan` | Konfigurasi WiFi 2.4G/5G | – |
| POST | `/api/modems/:id/network/wlan/ssid` | Ubah SSID WiFi | ADMIN |
| GET | `/api/modems/:id/network/routing` | Tabel routing | – |
| GET | `/api/modems/:id/network/dns` | DNS / hosts | – |
| GET | `/api/modems/:id/network/port-binding` | Port binding | – |
| GET | `/api/modems/:id/security/firewall` | Firewall | – |
| POST | `/api/modems/:id/security/firewall` | Toggle firewall | ADMIN |
| GET | `/api/modems/:id/security/ip-filter` | IP filter | – |
| GET | `/api/modems/:id/security/mac-filter` | MAC filter | – |
| GET | `/api/modems/:id/security/url-filter` | URL filter | – |
| GET | `/api/modems/:id/security/alg` | ALG | – |
| POST | `/api/modems/:id/security/alg` | Toggle ALG | ADMIN |
| GET | `/api/modems/:id/application/upnp` | UPnP | – |
| POST | `/api/modems/:id/application/upnp` | Toggle UPnP | ADMIN |
| GET | `/api/modems/:id/application/ddns` | DDNS | – |
| GET | `/api/modems/:id/application/dmz` | DMZ host | – |
| GET | `/api/modems/:id/application/port-forwarding` | Port forwarding | – |
| GET | `/api/modems/:id/application/sntp` | SNTP / waktu | – |
| GET | `/api/modems/:id/application/multicast` | Multicast / IGMP | – |
| GET | `/api/modems/:id/application/usb` | USB storage | – |
| GET | `/api/modems/:id/application/voip` | VoIP | – |
| GET | `/api/modems/:id/manage/device` | Device management | – |
| GET | `/api/modems/:id/manage/users` | User perangkat | – |
| POST | `/api/modems/:id/manage/users` | Ubah user/password | ADMIN |
| POST | `/api/modems/:id/manage/reboot` | Reboot perangkat | ADMIN |
| POST | `/api/modems/:id/manage/factory-reset` | Factory reset | ADMIN |
| GET | `/api/modems/:id/manage/config/backup` | Unduh config.bin | – |
| POST | `/api/modems/:id/manage/config/restore` | Restore config (multipart `config`) | ADMIN |
| POST | `/api/modems/:id/manage/firmware` | Upgrade firmware (multipart `firmware`) | ADMIN |
| GET | `/api/modems/:id/manage/time` | Waktu perangkat | – |
| POST | `/api/modems/:id/manage/time` | Set waktu perangkat | ADMIN |
| GET | `/api/modems/:id/manage/log` | Log sistem | – |
| POST | `/api/modems/:id/diagnosis/ping` | Ping dari sisi modem | – |
| POST | `/api/modems/:id/diagnosis/traceroute` | Traceroute dari sisi modem | – |
| GET | `/api/modems/:id/diagnosis/arp` | Tabel ARP | – |
| GET | `/api/modems/:id/diagnosis/mac-table` | Tabel MAC/FDB | – |
| GET | `/api/modems/:id/diagnosis/optical` | Daya optik & status PON | – |
| GET | `/api/modems/:id/diagnosis/loopback` | Loopback detection | – |
| POST | `/api/modems/:id/raw` | Perintah shell mentah (debug) | ADMIN |

Contoh:

```bash
# Cek konektivitas perangkat pelanggan id=12
curl -H "Authorization: Bearer $TOKEN" \
  http://SERVER:8080/api/modems/12/probe

# Baca konfigurasi WiFi
curl -H "Authorization: Bearer $TOKEN" \
  http://SERVER:8080/api/modems/12/network/wlan

# Ping dari sisi modem
curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"host":"8.8.8.8","count":4}' \
  http://SERVER:8080/api/modems/12/diagnosis/ping

# Unduh config perangkat
curl -H "Authorization: Bearer $TOKEN" -OJ \
  http://SERVER:8080/api/modems/12/manage/config/backup
```

Bentuk response seragam:

```json
{
  "section": "status",
  "feature": "device",
  "title": "Device Information",
  "source": "telnet",
  "data": { "device_info": [ { "SerialNumber": "ZTEG...", "SoftwareVersion": "V2.2.0P1T8" } ] }
}
```

`source` bernilai `web`, `telnet`, atau `combined` menandakan jalur yang berhasil.

### Antarmuka (Frontend)

Di halaman **Pelanggan**:

- Tombol **Modem** — membuka proxy Web UI perangkat (iframe/tab baru, sesi login lama).
- Tombol **Data** — membuka viewer data modem bertab (Status, Network, Security,
  Application, Manage, Diagnosis, Help) dan tab **Aksi** untuk operasi tulis:
  reboot, factory reset, toggle firewall/UPnP/DHCP/ALG, ubah SSID, set waktu,
  ubah user perangkat, update field WAN, backup/restore config, upgrade firmware,
  dan perintah telnet mentah. Tab **Aksi** hanya aktif untuk role ADMIN.
- Kredensial modem per pelanggan diisi pada form Tambah/Edit Pelanggan bagian
  **Akses Perangkat (Modem)**. Kosongkan password saat edit bila tidak ingin
  mengubahnya.

### CLI pengujian

Untuk menguji langsung ke perangkat tanpa lewat API:

```bash
make build-modem
./bin/monitoring-modem -host 192.168.1.1 -user admin -pass admin probe
./bin/monitoring-modem -host 192.168.1.1 status device
./bin/monitoring-modem -host 192.168.1.1 network wlan
./bin/monitoring-modem -host 192.168.1.1 -target 8.8.8.8 diagnosis ping
./bin/monitoring-modem -host 192.168.1.1 -tuser root -tpass Zte521 raw "sendcmd 1 DB get DeviceInfo"
./bin/monitoring-modem features
```

### Catatan

- Nama tabel DB ZTE berbeda antar-firmware/operator. Client memakai daftar
  kandidat per fitur; bila firmware Anda memakai nama lain, sesuaikan daftar di
  `backend/internal/modem/features_*.go`.
- Aksi tulis (set/reboot/factory reset) memerlukan akun dengan hak admin dan
  disarankan diuji lebih dulu lewat `/raw` atau CLI.
- Jalur telnet butuh fitur telnet aktif di perangkat. Aktifkan lewat setelan
  perangkat atau kredensial `root`.

---

## Realtime (WebSocket)

Frontend membuka WebSocket `ws(s)://<host>/api/events?token=<jwt>`. Event dikirim sebagai
JSON `{ "event": ..., "data": ... }`:

| Event | Payload |
|---|---|
| `customer:update` | perubahan status/latency satu pelanggan |
| `stats:update` | statistik dashboard |
| `alert:new` | alert baru |

Fitur ini membuat marker peta dan statistik berubah **tanpa reload halaman** dan **tanpa polling
berat** — semua ping berjalan di server. Di balik reverse proxy (nginx) pastikan header
`Upgrade`/`Connection: upgrade` diteruskan ke `/api/events` (lihat `nginx/monitoring.conf`).

---

## Environment Variables

Lihat `.env.example`. Variabel utama:

| Variabel | Default | Deskripsi |
|---|---|---|
| `DATABASE_URL` | – | URL koneksi PostgreSQL |
| `JWT_SECRET` | – | Secret JWT (wajib ganti) |
| `ENCRYPTION_KEY` | dev | Kunci AES untuk password VPN |
| `PING_CONCURRENCY` | 30 | Worker ping paralel |
| `PING_HISTORY_RETENTION_DAYS` | 7 | Retensi riwayat ping |
| `ADMIN_INITIAL_USERNAME/PASSWORD` | admin/admin123 | Admin pertama |
| `VITE_GOOGLE_MAPS_API_KEY` | – | Opsional, hanya jika memakai varian peta Google Maps |

---

## Skala 10.000+ Pelanggan

- **Worker pool**: ping paralel 30 worker (configurable) — tidak ada blocking.
- **Queue**: channel berisi kerja; interval per pelanggan dijadwalkan scheduler.
- **Peta**: Leaflet marker clustering — tidak ribuan marker individual di zoom rendah.
- **History**: `ping_results` terindeks `(customer_id, pinged_at DESC)`; cleanup otomatis.
  Untuk skala besar gunakan partisi waktu:
  ```sql
  CREATE TABLE ping_results (...) PARTITION BY RANGE (pinged_at);
  ```
- Singleton app; nginx `/api/events` harus meneruskan `Upgrade`/`Connection: upgrade`
  (`proxy_set_header Upgrade $http_upgrade;`) agar WebSocket tidak tertahan proxy.
- Retensi: `PING_HISTORY_RETENTION_DAYS`.

---

## Roadmap / Phase

- **Phase 1 (selesai)**: VPN + ping + customer + map + ONLINE/OFFLINE.
- **Phase 2 (selesai)**: history + latency + dashboard stats + uptime.
- **Phase 3 (selesai)**: SSE realtime + alert event/log.
- **Phase 4 (optimasi)**:
  - Partisi `ping_results` per bulan.
  - Agregasi stats via materialized view / redis cache.
  - Alert channel adapter (Telegram/WhatsApp/Email) pada hook `PublishAlert`.
  - Redis opcional untuk distribusi event antar instance.

---

## Lisensi

MIT — silakan gunakan dan kembangkan.