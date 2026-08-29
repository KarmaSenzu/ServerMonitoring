# Deployment Monitoring Dashboard — Docker + Cloudflare Tunnel + Cloudflare Access

Panduan lengkap deployment dashboard monitoring di PC pribadi (Windows + WSL2 + Docker Desktop) dengan Cloudflare Tunnel sebagai gateway publik dan Cloudflare Access sebagai lapisan otentikasi tambahan.

Domain target dokumen ini: `server-dmr.devplay.online`.

---

## Daftar Isi

1. [Ringkasan Arsitektur](#1-ringkasan-arsitektur)
2. [Prasyarat](#2-prasyarat)
3. [Setup WSL2 + Docker Desktop](#3-setup-wsl2--docker-desktop)
4. [Setup Cloudflare Tunnel](#4-setup-cloudflare-tunnel)
5. [Setup Cloudflare Access](#5-setup-cloudflare-access-wajib-untuk-keamanan)
6. [First-time Setup di WSL2](#6-first-time-setup-di-wsl2)
7. [Build & Run](#7-build--run)
8. [Login Pertama](#8-login-pertama)
9. [Operasional Sehari-hari](#9-operasional-sehari-hari)
10. [Akses ke Container Docker Host](#10-akses-ke-container-docker-host)
11. [Multi-Tunnel Listing di Dashboard](#11-multi-tunnel-listing-di-dashboard)
12. [Akses ke Cloudflare Tunnel (Status Per-Tunnel)](#12-akses-ke-cloudflare-tunnel-status-per-tunnel)
13. [Security Checklist](#13-security-checklist)
14. [Troubleshooting](#14-troubleshooting)
15. [Update & Rollback](#15-update--rollback)
16. [Pertanyaan yang Sering Muncul](#16-pertanyaan-yang-sering-muncul)

---

## 1. Ringkasan Arsitektur

Stack ini terdiri dari 4 container yang berjalan di dalam network bridge `server-dmr-net`. Semua trafik publik masuk lewat Cloudflare Tunnel — tidak ada port yang di-expose ke internet langsung.

### Diagram Alir

```
                      INTERNET
                         │
                         ▼
               ┌──────────────────────┐
               │  Cloudflare Edge     │
               │  (server-dmr.        │
               │   devplay.online)    │
               └──────────┬───────────┘
                          │  HTTPS
                          ▼
               ┌──────────────────────┐
               │  Cloudflare Access   │
               │  (Email OTP gate —   │
               │   BELUM aktif)       │
               └──────────┬───────────┘
                          │
       ╔═════════════════▼════════════════════════════╗
       ║   WSL2 Host (PC pribadi, Windows)            ║
       ║   network: server-dmr-net                    ║
       ║                                              ║
       ║   ┌──────────────────────────────────────┐   ║
       ║   │  server-dmr-tunnel  (cloudflared)    │   ║
       ║   │  outbound-only, UUID f6eeebfd...     │   ║
       ║   └─────────────┬────────────────────────┘   ║
       ║                 │ HTTP                        ║
       ║                 ▼                            ║
       ║   ┌──────────────────────────────────────┐   ║
       ║   │  server-dmr-web  (nginx)             │   ║
       ║   │  serve SPA + reverse-proxy /api      │   ║
       ║   └─────────────┬────────────────────────┘   ║
       ║                 │ HTTP                        ║
       ║                 ▼                            ║
       ║   ┌──────────────────────────────────────┐   ║
       ║   │  server-dmr-api  (Go, user 1000)     │   ║
       ║   │  auth, metrics, alerts, tunnels      │   ║
       ║   └──┬──────────────┬──────────────┬─────┘   ║
       ║      │              │              │         ║
       ║      │              ▼              │         ║
       ║      │   ┌──────────────────┐      │         ║
       ║      │   │ server-dmr-      │      │         ║
       ║      │   │ docker-proxy     │      │         ║
       ║      │   │ (read-only)      │      │         ║
       ║      │   └────────┬─────────┘      │         ║
       ║      │            │                │         ║
       ║      ▼            ▼                ▼         ║
       ║   ┌──────────────────────────────────────┐   ║
       ║   │  Host mounts (read-only kecuali pm2) │   ║
       ║   │  /proc      → /host/proc  (ro)       │   ║
       ║   │  /sys       → /host/sys   (ro)       │   ║
       ║   │  /etc       → /host/etc   (ro)       │   ║
       ║   │  /          → /host       (ro, disk) │   ║
       ║   │  /root/.pm2 → /home/app/.pm2 (rw)    │   ║
       ║   │  /etc/cloudflared/<project>/  (ro)   │   ║
       ║   │  /var/run/docker.sock (via proxy)    │   ║
       ║   └──────────────────────────────────────┘   ║
       ╚══════════════════════════════════════════════╝
```

### Daftar Container

| Container               | Image                                    | Fungsi                                                                 |
|-------------------------|------------------------------------------|------------------------------------------------------------------------|
| `server-dmr-api`        | `server-dmr-api:latest` (build lokal)    | Backend Go: auth JWT, kolektor metrik, alerts, notifier, scheduler, tunnel listing, PM2 hybrid. Image ~260 MB karena include `docker-cli`, `nodejs`, `npm`, `pm2`, dan binary `cloudflared`. |
| `server-dmr-web`        | `server-dmr-web:latest` (build lokal)    | Nginx: serve frontend SPA dan reverse-proxy `/api` ke container `api`. |
| `server-dmr-docker-proxy` | `tecnativa/docker-socket-proxy:0.3.0`  | Proxy Docker socket ter-restriksi (read-only by default).              |
| `server-dmr-tunnel`     | `cloudflare/cloudflared:latest`          | Cloudflare Tunnel — outbound-only, gateway publik ke `web`.            |

---

## 2. Prasyarat

- **Windows 10 versi 2004+** atau **Windows 11**.
- **WSL2** aktif dengan distro Ubuntu (atau distro berbasis Debian).
- **Docker Desktop** dengan integrasi WSL2 enabled.
- Akun **Cloudflare** (free tier cukup) dengan domain sudah ditambahkan dan nameserver pointing ke Cloudflare.
- **PowerShell 5+** atau **pwsh** untuk command-command di sisi Windows.

### Verifikasi Cepat

```powershell
# Versi WSL — pastikan default version 2
wsl --status

# Docker engine
docker --version

# Docker Compose v2 (bukan docker-compose lama)
docker compose version
```

Output yang diharapkan untuk WSL kurang lebih:

```
Default Version: 2
```

Untuk Docker:

```
Docker version 24.x.x atau lebih baru
Docker Compose version v2.x.x atau lebih baru
```

---

## 3. Setup WSL2 + Docker Desktop

### Aktifkan WSL2

Buka **PowerShell sebagai Administrator** lalu jalankan:

```powershell
wsl --install
```

Restart Windows kalau diminta. Setelah restart, buka aplikasi **Ubuntu** dari Start Menu untuk inisialisasi (set username + password Linux).

### Set Ubuntu Sebagai Default Distro

```powershell
wsl --set-default Ubuntu
```

### Aktifkan Integrasi Docker Desktop ↔ WSL2

1. Buka **Docker Desktop**.
2. **Settings → Resources → WSL Integration**.
3. Aktifkan toggle **Enable integration with my default WSL distro**.
4. Aktifkan toggle untuk **Ubuntu** di daftar di bawahnya.
5. Klik **Apply & Restart**.

### Test Integrasi

```powershell
wsl docker run hello-world
```

Kalau muncul tulisan `Hello from Docker!`, integrasi sudah jalan.

### Catatan Penting: Lokasi Project

WSL2 jauh lebih cepat membaca file dari filesystem Linux native (`/home/...`) dibanding dari Windows mount (`/mnt/c/...`). Untuk monitoring stack ini, ada dua opsi:

- **Opsi A — recommended untuk production / 24/7**
  Clone atau copy project ke `~/vps-dashboard` di dalam WSL:

  ```bash
  # Dari WSL terminal
  cp -r "/mnt/c/PRIVAT SERVER PROJECT WEB AUTO/MONITORING SERVER/vps-dashboard" ~/vps-dashboard
  cd ~/vps-dashboard
  ```

- **Opsi B — cepat untuk test**
  Jalankan langsung dari path Windows. Lebih lambat (terutama saat build dan saat backend baca `/data`), tapi works tanpa migrasi:

  ```bash
  cd "/mnt/c/PRIVAT SERVER PROJECT WEB AUTO/MONITORING SERVER/vps-dashboard"
  ```

Sisa dokumen ini menggunakan `~/vps-dashboard` sebagai default. Sesuaikan kalau pakai Opsi B.

---

## 4. Setup Cloudflare Tunnel

1. Login ke **Cloudflare Zero Trust dashboard**: `https://one.dash.cloudflare.com`.
2. Sidebar kiri: **Networks → Tunnels**.
3. Klik **Create a tunnel** → pilih **Cloudflared** → **Next**.
4. Beri nama tunnel: `server-dmr` → **Save tunnel**.
5. Di tab **Install connector**, ada baris perintah berisi `--token eyJ...`. **Copy seluruh string token** (mulai `eyJ...` sampai akhir, tanpa `--token` dan tanpa command lain). Simpan; nanti dipakai di `.env` sebagai `TUNNEL_TOKEN`.
6. Klik **Next** untuk lanjut ke konfigurasi hostname.
7. Tab **Public Hostnames** → **Add a public hostname**:
   - **Subdomain**: `server-dmr`
   - **Domain**: pilih `devplay.online` dari dropdown
   - **Path**: kosongkan
   - **Type**: `HTTP`
   - **URL**: `web:80`
   - Klik **Save hostname**.
8. Cloudflare akan otomatis membuat DNS record `CNAME server-dmr → <tunnel-id>.cfargotunnel.com`. Tidak perlu setup DNS manual.

> Catatan: `web:80` adalah nama service di `docker-compose.yml`. Cloudflared di dalam network Docker bisa resolve nama service ini langsung.

---

## 5. Setup Cloudflare Access (Wajib untuk Keamanan)

> ⚠️ Cloudflare Access **belum aktif** untuk dashboard ini. Saat ini `https://server-dmr.devplay.online` publik di internet — siapa pun yang tahu URL bisa sampai ke halaman login dashboard. Aktifkan Access sebelum dianggap production-ready.

Tanpa Access, siapa pun yang tahu URL `https://server-dmr.devplay.online` bisa langsung sampai ke halaman login dashboard. Cloudflare Access menambah lapisan email-OTP **sebelum** request sampai ke aplikasi.

1. Di **Zero Trust dashboard**: **Access → Applications → Add an application**.
2. Pilih **Self-hosted**.
3. Form aplikasi:
   - **Application name**: `VPS Monitor Dashboard`
   - **Session duration**: `24h`
   - **Application domain**:
     - Subdomain: `server-dmr`
     - Domain: `devplay.online`
     - Path: kosongkan
4. **Identity providers**: pakai **One-time PIN** (default, tidak perlu setup IdP eksternal).
5. Klik **Next** untuk masuk ke konfigurasi policy.
6. Tambahkan policy:
   - **Policy name**: `Owner only`
   - **Action**: `Allow`
   - **Configure rules → Include**:
     - Selector: `Emails`
     - Value: email pribadi kamu (yang akan dipakai login)
7. Klik **Next → Add application**.

Hasilnya: setiap akses ke `https://server-dmr.devplay.online` akan ditampilkan halaman Cloudflare Access yang minta email + OTP. Hanya email yang masuk whitelist yang lolos ke dashboard.

---

## 6. First-time Setup di WSL2

```bash
# Masuk ke WSL dari PowerShell
wsl

# Pindah ke project (path Windows mounted ke WSL)
cd "/mnt/c/PRIVAT SERVER PROJECT WEB AUTO/MONITORING SERVER/vps-dashboard"
# Atau kalau sudah di-copy ke filesystem Linux native (lebih cepat):
# cd ~/vps-dashboard

# Buat .env dari template
cp .env.docker.example .env

# Generate JWT secret yang kuat (128 char hex)
openssl rand -hex 64
# Output ~128 karakter hex. Copy ke baris JWT_SECRET= di .env

# Edit .env dengan editor pilihan
nano .env
```

Yang **wajib** diisi di `.env`:

| Variabel                     | Nilai                                                          |
|------------------------------|----------------------------------------------------------------|
| `TUNNEL_TOKEN`               | Token dari step Cloudflare Tunnel (mulai `eyJ...`).            |
| `JWT_SECRET`                 | Hasil `openssl rand -hex 64` (64+ byte random).                |
| `BOOTSTRAP_ADMIN_PASSWORD`   | Password admin pertama. Min 16 karakter, kombinasi acak.       |

Yang opsional (sudah ada default yang masuk akal):

| Variabel             | Default                                  |
|----------------------|------------------------------------------|
| `JWT_TTL`            | `2h`                                     |
| `BOOTSTRAP_ADMIN_USERNAME` | `admin`                            |
| `CORS_ORIGINS`       | `https://server-dmr.devplay.online`      |
| `TZ`                 | `Asia/Jakarta`                           |
| `DOCKERPROXY_POST`   | `0` (read-only, paling aman)             |

Simpan file (Ctrl+O, Enter, Ctrl+X di nano).

---

## 7. Build & Run

```bash
# Build image api dan web (pertama kali butuh 2-5 menit)
docker compose build --pull

# Start seluruh stack di background
docker compose up -d

# Cek status
docker compose ps

# Pantau log realtime (Ctrl+C untuk keluar)
docker compose logs -f --tail=100
```

### Indikator Sukses

- `docker compose ps` menampilkan keempat container dengan status `Up`. Container yang punya healthcheck akan muncul `healthy`. Output yang diharapkan:

  ```
  NAME                       STATUS
  server-dmr-api             Up X seconds (healthy)
  server-dmr-web             Up X seconds (healthy)
  server-dmr-docker-proxy    Up X seconds
  server-dmr-tunnel          Up X seconds
  ```

- Log `cloudflared` ada baris semacam:

  ```
  Registered tunnel connection
  ```

- Log `api` ada baris:

  ```
  vps-dashboard-api starting
  ```

- Log `web` (nginx) tidak ada error 500 saat startup.

---

## 8. Login Pertama

1. Buka browser: `https://server-dmr.devplay.online`.
2. Halaman **Cloudflare Access** muncul: masukkan email yang sudah di-whitelist.
3. Cek inbox email, ambil **OTP code**, paste ke form. Klik submit.
4. Setelah lolos Access, halaman **login dashboard** muncul.
5. Kredensial:
   - Username: `admin` (atau yang di-set di `BOOTSTRAP_ADMIN_USERNAME`)
   - Password: yang di-set di `BOOTSTRAP_ADMIN_PASSWORD`

### Penting: Setelah Login Berhasil

1. Buka menu **Settings / Profile** di dashboard, ganti password admin via UI ke password baru.
2. Edit `.env`, **kosongkan** `BOOTSTRAP_ADMIN_PASSWORD`:

   ```bash
   nano .env
   # Ubah:
   #   BOOTSTRAP_ADMIN_PASSWORD=passwordlama
   # Menjadi:
   #   BOOTSTRAP_ADMIN_PASSWORD=
   ```

3. Restart stack supaya container tidak lagi punya password bootstrap di environment:

   ```bash
   docker compose up -d
   ```

   Container `api` akan di-recreate dengan env baru.

---

## 9. Operasional Sehari-hari

Ada dua jalur perintah: dari WSL (Linux, pakai `make` atau `docker compose` langsung) dan dari Windows (PowerShell, pakai script wrapper).

| Make (WSL)        | PowerShell (Windows)             | Fungsi                          |
|-------------------|----------------------------------|---------------------------------|
| `make up`         | `.\start.ps1 up`                 | Start stack                     |
| `make down`       | `.\start.ps1 down`               | Stop stack                      |
| `make ps`         | `docker compose ps`              | Lihat status container          |
| `make logs`       | `docker compose logs -f`         | Tail semua log                  |
| `make logs-api`   | `docker compose logs -f api`     | Tail log backend                |
| `make logs-web`   | `docker compose logs -f web`     | Tail log nginx                  |
| `make backup`     | (lihat dokumen scripts/)         | Snapshot DB manual              |
| `make update`     | `.\start.ps1 update`             | Pull + rebuild + run            |
| `make restart`    | `docker compose restart`         | Restart semua container         |

Kalau tidak ada Makefile / start.ps1, perintah ekuivalen `docker compose ...` langsung selalu jalan.

### Lokasi Data Penting

| Path                                           | Isi                                       |
|------------------------------------------------|-------------------------------------------|
| `~/vps-dashboard/data/vps-dashboard.db`        | SQLite database (users, events, metrics). |
| `~/vps-dashboard/data/backups/`                | Backup harian otomatis.                   |
| `~/vps-dashboard/.env`                         | Secrets — JANGAN commit, JANGAN share.    |

---

## 10. Akses ke Container Docker Host

Backend `api` tidak mount Docker socket langsung. Akses ke Docker host disalurkan lewat container `dockerproxy` yang membatasi endpoint apa saja yang boleh dipanggil.

### Default — Read-only

Backend bisa:

- List container, image, network, volume
- Inspect container (cek status, port, health)
- Baca info Docker engine

Backend **tidak bisa**:

- Start / stop / restart container
- Build / pull / push image
- Buat / hapus container, network, volume

### Mengaktifkan Kontrol (Hati-hati)

Kalau dashboard butuh tombol start/stop container:

```bash
nano .env
# Ubah:
#   DOCKERPROXY_POST=0
# Menjadi:
#   DOCKERPROXY_POST=1

docker compose up -d
```

> **Risiko nyata**: kalau dashboard tertembus (bug auth, XSS lewat session aktif, dll), attacker bisa membuat container dengan privilege root host. Aktifkan hanya kalau memang butuh, dan pasangan dengan Cloudflare Access yang ketat plus user dashboard yang minimal.

---

## 11. Multi-Tunnel Listing di Dashboard

Backend Go scan folder `/etc/cloudflared/` untuk daftar tunnel yang ditampilkan di halaman Tunnels dashboard. Layout yang didukung:

- **Flat** — file `config.yml` + `credentials.json` langsung di `/etc/cloudflared/`.
- **Per-project subdirs** — tiap project punya subfolder sendiri, mis. `/etc/cloudflared/server-dmr/config.yml`, `/etc/cloudflared/dmrxai/config.yml`. Backend (`gatherConfigEntries` di `internal/tunnel`) otomatis scan kedua layout dan gabungkan hasilnya.

Mount per project ditambahkan di `docker-compose.yml` service `api`:

```yaml
volumes:
  - ./cloudflared-config:/etc/cloudflared/server-dmr:ro
  - "../../Dmr x - Ai/cloudflared-config:/etc/cloudflared/dmrxai:ro"
```

Untuk menambahkan project baru ke listing tunnel:

1. Project lain harus punya folder `cloudflared-config/` dengan `config.yml` (dan `credentials.json` kalau ada).
2. Tambah baris mount baru di `vps-dashboard/docker-compose.yml`:

   ```yaml
   - "../<path-relatif>/<project>/cloudflared-config:/etc/cloudflared/<nama>:ro"
   ```

3. Restart api supaya re-scan:

   ```bash
   docker compose up -d api
   # atau
   docker compose restart api
   ```

### Verifikasi Mount

```bash
docker exec server-dmr-api ls -la /etc/cloudflared/
# Harus menampilkan subfolder per project: server-dmr/, dmrxai/, dst.
```

### Catatan

Tunnel yang dikelola di server lain (tidak punya folder lokal di mesin ini) **tidak akan tampil** di listing dashboard. Itu termasuk tunnel `budi-ai` dan `mysql-database` di akun Cloudflare yang di-deploy di host berbeda. Untuk listing menyeluruh berbasis akun Cloudflare, perlu integrasi Cloudflare API (token + account ID) — belum diimplementasi.

---

## 12. Akses ke Cloudflare Tunnel (Status Per-Tunnel)

Halaman **Tunnels** di dashboard ini awalnya dirancang untuk backend Node lama yang menjalankan `systemctl status cloudflared` di host. Pada deployment containerized + backend Go, fitur ini perlu di-rewrite supaya:

- Memanggil HTTP API `cloudflared` (`http://cloudflared:2000/metrics`) — perlu menambah flag `--metrics 0.0.0.0:2000` di service `cloudflared`.
- **Atau** memanggil Cloudflare API dari backend (perlu API token + tunnel ID disimpan di `.env`).

### Workaround Sementara

Pantau status tunnel langsung dari Cloudflare Zero Trust dashboard:

`https://one.dash.cloudflare.com → Networks → Tunnels → server-dmr`

Di sana terlihat: status koneksi (`HEALTHY` / `DEGRADED` / `DOWN`), connector ID, region, dan throughput.

### Roadmap

Tambahkan ke service `cloudflared` di `docker-compose.yml`:

```yaml
command: tunnel --no-autoupdate --metrics 0.0.0.0:2000 run
```

Lalu update handler `/system/tunnel` di backend Go untuk fetch dari `http://cloudflared:2000/metrics` dan parse format Prometheus.

---

## 13. Security Checklist

### Tier 1 — Status Sekarang

- [x] `.env` sudah di-`.gitignore`.
- [x] `JWT_SECRET` 128 karakter random hex (output `openssl rand -hex 64`).
- [x] `JWT_TTL` = `2h`.
- [ ] `BOOTSTRAP_ADMIN_PASSWORD` masih di `.env` — kosongkan setelah login pertama dan password admin diganti via UI.
- [ ] Cloudflare Access **BELUM aktif** — aktifkan untuk produksi (lihat Bagian 5).
- [x] Backend Node lama sudah dipindah ke `legacy/` dan tidak ikut di-deploy.

### Tier 2 — Container Hardening

- [x] Container `api` jalan sebagai user 1000, bukan root.
- [x] `cap_drop: ALL` aktif + `cap_add` minimal sesuai kebutuhan tiap service.
- [x] `security_opt: no-new-privileges:true` aktif.
- [x] Mount host `/proc`, `/sys`, `/etc` semuanya `:ro`.
- [x] Mount tambahan `/:/host:ro` untuk disk usage scan.
- [x] Docker socket TIDAK di-mount langsung ke `api`, tapi lewat `dockerproxy` (read-only).
- [ ] `DOCKERPROXY_POST=0` kecuali memang butuh fitur kontrol.

### Tier 3 — Operasional

- [ ] Backup harian otomatis aktif (sudah ada di backend Go, jadwal jam `BACKUP_HOUR_LOCAL` waktu lokal, default 03:00).
- [ ] Test restore dari backup minimal 1x — sebelum lupa caranya, dan sebelum benar-benar butuh.
- [ ] Telegram notifier dikonfigurasi untuk alert kritis (down, high CPU, high RAM).
- [ ] Review log `events` di dashboard mingguan untuk cek anomali login / aksi.
- [ ] Update Docker Desktop, WSL kernel, dan image upstream berkala (`make update` atau `docker compose pull && docker compose up -d`).

### Tier 4 — Host (PC Pribadi)

- [ ] Stack dijalankan di WSL2, bukan langsung di Windows host.
- [ ] **Windows Defender Firewall**: block inbound dari semua kecuali yang dibutuhkan Docker.
- [ ] Pertimbangkan user Windows terpisah khusus untuk akses Docker Desktop (mengurangi blast radius kalau akun utama tertembus).
- [ ] Auto-update Windows aktif.
- [ ] **UPS** untuk PC kalau service dianggap penting — listrik mati = dashboard mati.

---

## 14. Troubleshooting

### Tunnel error: `context deadline exceeded` atau `connection refused`

- Cek `TUNNEL_TOKEN` di `.env` benar dan tidak expired.
- Cek koneksi internet dari WSL:

  ```bash
  curl https://1.1.1.1
  ```

- Restart cloudflared:

  ```bash
  docker compose restart cloudflared
  docker compose logs -f cloudflared
  ```

- Cek di Cloudflare Zero Trust dashboard apakah tunnel `server-dmr` muncul `HEALTHY`.

### Frontend `502 Bad Gateway`

Backend `api` crash atau belum siap menerima request. Cek log:

```bash
docker compose logs api
```

Penyebab paling umum:

- `JWT_SECRET` kosong → container exit dengan error `JWT_SECRET wajib di-set di .env`.
- `BOOTSTRAP_ADMIN_PASSWORD` kosong saat first run dan tabel users masih kosong.
- Filesystem `./data` tidak writable. Cek permission:

  ```bash
  ls -la data/
  # owner harus uid 1000 supaya container api (user 1000) bisa write
  sudo chown -R 1000:1000 data/
  ```

### Metrik Host (CPU/RAM) Angkanya Kelihatan Kecil

Container baca metrik dari `/proc` di dalam container, bukan dari host.

```bash
# Cek host /proc termount dengan benar
docker compose exec api ls /host/proc | head
# Output harus berisi banyak PID (angka-angka) + cpuinfo, meminfo, dll.

# Cek env HOST_PROC ter-set
docker compose exec api env | grep HOST_
# Harus muncul: HOST_PROC=/host/proc, HOST_SYS=/host/sys, HOST_ETC=/host/etc
```

Kalau semua benar tapi angka tetap aneh, ingat bahwa di WSL2 yang dibaca adalah resource VM WSL (yang dialokasikan ke Linux kernel), bukan resource Windows host secara keseluruhan.

### Login Berhasil tapi Endpoint Lain Error 401

JWT clock skew. Cek waktu container vs host:

```bash
docker compose exec api date
date
```

Selisih > 1 menit sudah cukup bikin token invalid. Pastikan `TZ=Asia/Jakarta` di `.env` dan WSL2 clock sync. Kalau perlu paksa sync:

```bash
sudo hwclock -s
```

### Cloudflare Access Tidak Nge-block Akses Publik

- Pastikan policy `Allow` untuk email kamu **paling atas** di daftar policy aplikasi.
- Tambah policy default `Block` untuk `Everyone` di urutan paling bawah kalau Access masih meloloskan request.
- Cek **Application domain** di Access cocok persis dengan hostname tunnel (`server-dmr.devplay.online`).

### Port Bentrok di WSL2

Kalau `docker compose up` gagal karena port bentrok (misal 80 sudah dipakai aplikasi lain di WSL), edit `docker-compose.yml`. Tapi karena stack ini tidak expose port ke host (semua trafik lewat tunnel), kasus ini jarang muncul.

### Endpoint `/system/tunnels` Return Tunnel Kurang dari yang Diharapkan

Backend scan `/etc/cloudflared/` dengan layout flat + per-project subdirs. Cek isi mount:

```bash
docker exec server-dmr-api ls -la /etc/cloudflared/
# Harus tampil subfolder per project: server-dmr/, dmrxai/, dst.
```

Restart api setelah update mount di `docker-compose.yml`:

```bash
docker compose restart api
```

Tunnel yang dikelola di server lain (tidak punya folder lokal) tidak akan tampil di listing — itu by design.

### PM2 Endpoint Return 503 EACCES

Permission folder `/root/.pm2` di host belum bisa dibaca container `api` (user 1000). Run di WSL:

```bash
sudo chgrp -R 1000 /root/.pm2
sudo chmod -R g+rwX /root/.pm2
```

Lalu restart api:

```bash
docker compose restart api
```

### Disk Metric Kelihatan Kecil (Cuma Container Rootfs)

Container baca disk dari path yang dikonfigurasi `SYSTEM_ROOT_PATH`. Pastikan:

1. Mount `/:/host:ro` ada di `docker-compose.yml` service `api`.
2. Env `SYSTEM_ROOT_PATH=/host` ter-set di `.env` (atau langsung di compose `environment:`).

Verifikasi:

```bash
docker compose exec api df -h /host
# Harus menampilkan total disk host, bukan rootfs container
docker compose exec api env | grep SYSTEM_ROOT
```

### Network Metric Error karena Symlink `/host/proc/net`

`/proc/net` adalah symlink ke `self/net` yang berbeda namespace di dalam container. Sysinfo collector punya fallback otomatis ke `/host/proc/1/net/dev` (lihat `internal/sysinfo`). Kalau masih error:

```bash
docker compose logs api | grep -i "net dev\|sysinfo"
```

Snapshot system info juga membawa field `Errors []string` per-metric, jadi kegagalan satu metrik tidak mematikan endpoint — hanya memunculkan entry di array errors.

---

## 15. Update & Rollback

### Update Aplikasi (kode berubah)

```bash
cd ~/vps-dashboard

# Pull update terbaru kalau pakai git
git pull

# Build ulang image yang berubah
docker compose build --pull api web

# Recreate container yang berubah saja
docker compose up -d api web

# Verifikasi
docker compose ps
docker compose logs -f --tail=50
```

### Update Image Upstream (cloudflared, dockerproxy)

```bash
docker compose pull cloudflared dockerproxy
docker compose up -d cloudflared dockerproxy
```

### Tag Image Sebelum Update (recommended)

Sebelum build baru, tag image lama supaya bisa di-rollback:

```bash
docker tag server-dmr-api:latest server-dmr-api:backup-$(date +%Y%m%d)
docker tag server-dmr-web:latest server-dmr-web:backup-$(date +%Y%m%d)
```

### Rollback Kalau Update Bermasalah

```bash
# Lihat tag backup yang ada
docker images | grep server-dmr

# Misalnya ada server-dmr-api:backup-20260101
docker tag server-dmr-api:backup-20260101 server-dmr-api:latest
docker tag server-dmr-web:backup-20260101 server-dmr-web:latest

# Recreate container pakai image yang sudah di-retag
docker compose up -d api web
```

### Rollback Database

Database SQLite ada di `data/vps-dashboard.db`. Backup harian disimpan di `data/backups/`. Untuk restore:

```bash
docker compose down
cp data/backups/vps-dashboard-YYYYMMDD-HHMMSS.db data/vps-dashboard.db
docker compose up -d
```

---

## 16. Pertanyaan yang Sering Muncul

**Q: Apakah saya bisa pakai domain selain `server-dmr.devplay.online`?**

A: Bisa. Update di 3 tempat:

1. Hostname di Cloudflare Tunnel (Public Hostnames).
2. `CORS_ORIGINS` di `.env`.
3. Application domain di Cloudflare Access.

Sesuaikan ketiganya, lalu `docker compose up -d` untuk reload env baru.

**Q: Bisa banyak project di satu tunnel?**

A: Bisa. Tambahkan hostname baru di Cloudflare Tunnel dengan service yang berbeda. Contoh:

- `monitor.devplay.online` → `web:80` (dashboard ini)
- `api.devplay.online` → `api:3001` (kalau mau expose API langsung)
- `lain.devplay.online` → service container lain di network yang sama

Asalkan service-nya ada di network `server-dmr-net`, cloudflared bisa resolve langsung pakai nama service.

**Q: Kalau PC mati, dashboard mati?**

A: Ya. Tunnel pun ikut mati karena cloudflared jalan di PC. Untuk uptime tinggi, pertimbangkan:

- VPS murah (DigitalOcean / Vultr / Linode ~$5/bulan), lebih reliable dan punya alamat publik sendiri.
- Raspberry Pi 4/5 yang nyala 24/7 dengan storage SSD eksternal — lebih hemat listrik dari PC.

**Q: Bisa diakses dari LAN tanpa lewat internet?**

A: Bisa. Tambah port mapping di `docker-compose.yml` pada service `web`:

```yaml
web:
  # ... yang sudah ada ...
  ports:
    - "8080:80"
```

Lalu `docker compose up -d web`. Akses dari LAN: `http://<ip-wsl>:8080`.

> **Hati-hati**: jalur ini **bypass Cloudflare Access**. Pastikan Windows Defender Firewall membatasi akses ke port 8080 hanya dari subnet LAN-mu, dan jangan port-forward ke router publik.

**Q: Apakah aman menyimpan `JWT_SECRET` dan `BOOTSTRAP_ADMIN_PASSWORD` di `.env`?**

A: Aman selama:

1. `.env` tidak ke-commit ke git (sudah di `.gitignore`).
2. PC kamu tidak shared dengan user lain yang punya akses ke filesystem WSL.
3. Setelah admin pertama terbuat, `BOOTSTRAP_ADMIN_PASSWORD` dikosongkan.

Untuk hardening lebih jauh, pakai Docker secrets atau vault eksternal — tapi untuk skala PC pribadi, `.env` sudah cukup.

**Q: Bagaimana cara restart backend api saja tanpa nge-stop semuanya?**

A:

```bash
docker compose restart api
# atau
docker compose up -d --force-recreate api
```

**Q: Berapa banyak resource yang dibutuhkan?**

A: Estimasi konservatif untuk WSL2:

- RAM: 1.5–2 GB total untuk semua container saat idle.
- CPU: < 5% di mesin modern saat idle, spike pendek saat scrape metrik.
- Disk: ~500 MB untuk image + database tumbuh ~10 MB per bulan untuk 1 host.

Atur batas resource WSL di `%USERPROFILE%\.wslconfig` kalau perlu cap memory:

```ini
[wsl2]
memory=4GB
processors=4
```

> Catatan: WSL2 di mesin ini saat ini di-cap 4 GB RAM via `.wslconfig`. Pada beban scrape metrik 4 container, idle pemakaian sekitar 1.5–2 GB.

**Q: Kenapa tidak pakai HTTPS langsung di nginx?**

A: Cloudflare Tunnel sudah terminate TLS di edge Cloudflare. Trafik dari `cloudflared` ke `web` tetap di dalam network Docker (tidak keluar host), jadi tidak perlu TLS internal. Kalau dipaksakan, hanya menambah CPU overhead tanpa benefit keamanan riil.

---

Selesai. Stack siap dipantau lewat `https://server-dmr.devplay.online` dengan email-OTP gate dari Cloudflare Access.
