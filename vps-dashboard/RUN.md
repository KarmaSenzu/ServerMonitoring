# Quickstart — Server DMR Monitoring

> Lihat juga: [`../ARCHITECTURE.md`](../ARCHITECTURE.md) untuk gambaran besar setup.

Ringkas. Detail lengkap ada di `DEPLOY_DOCKER.md` (operasional) dan `CLOUDFLARE_TUNNEL_SETUP.md` (tunnel + access).

## TL;DR

```bash
# Dari WSL2:
cd vps-dashboard
cp .env.docker.example .env
# Edit .env: isi TUNNEL_TOKEN, JWT_SECRET, BOOTSTRAP_ADMIN_PASSWORD
docker compose up -d --build
chmod +x verify.sh
./verify.sh
```

Lalu buka `https://server-dmr.devplay.online`.

---

## 5 Langkah Detail

### 1. Siapkan Cloudflare Tunnel (sekali setup)

Lihat `CLOUDFLARE_TUNNEL_SETUP.md` Bagian A–C. Hasil yang dibutuhkan:
- `TUNNEL_TOKEN` (string panjang `eyJ...`)
- DNS record `server-dmr.devplay.online` sudah otomatis terbuat oleh Cloudflare

### 2. Aktifkan Cloudflare Access (lapisan keamanan)

Lihat `CLOUDFLARE_TUNNEL_SETUP.md` Bagian D. Whitelist email kamu dengan One-time PIN.

### 3. Isi `.env`

Dari WSL2:

```bash
cd vps-dashboard
cp .env.docker.example .env
nano .env
```

Wajib isi:

| Variable | Cara dapat | Contoh |
|---|---|---|
| `TUNNEL_TOKEN` | Cloudflare dashboard (Bagian B.4) | `eyJhIjoi...` |
| `JWT_SECRET` | `openssl rand -hex 64` | `a1b2c3...` (128 char) |
| `BOOTSTRAP_ADMIN_PASSWORD` | Kamu pilih, ≥16 karakter, kuat | `Sup3rS3cr3tPass!2026` |

### 4. Build & Start Stack

```bash
docker compose up -d --build
```

Tunggu 2-5 menit untuk build pertama. Saat selesai:

```bash
docker compose ps
```

Semua 4 service harus `running`. `api` dan `web` harus `healthy`.

### 5. Verifikasi & Login Pertama

```bash
chmod +x verify.sh
./verify.sh
```

Buka browser: `https://server-dmr.devplay.online`
- Cloudflare Access prompt → email + OTP
- Halaman login dashboard → `admin` / password yang di-set di step 3
- Setelah login, **ganti password admin dari UI**, lalu kosongkan `BOOTSTRAP_ADMIN_PASSWORD` di `.env`, dan `docker compose up -d`

---

## Operasional Cepat

```bash
docker compose ps                            # status
docker compose logs -f api                   # live log backend
docker compose logs -f cloudflared           # live log tunnel
docker compose restart api                   # restart backend
docker compose down                          # stop semua
docker compose up -d                         # start lagi
docker compose pull && docker compose up -d  # update image upstream
docker compose build --no-cache api          # rebuild api dari awal
```

Atau pakai Makefile: `make ps`, `make logs-api`, `make restart-api`, dst.

## Troubleshooting Tercepat

| Gejala | Cek |
|---|---|
| Domain tidak bisa dibuka | `docker compose logs cloudflared` — cari `Registered tunnel connection` |
| 502 di domain | `docker compose logs web` + `docker compose logs api` |
| Login gagal | `docker compose logs api` — cari log `auth` |
| Metric host kelihatan kecil | `docker compose exec api ls /host/proc \| head` (harus banyak PID) |
| `/system/tunnels` cuma return 1 entry | Restart api: `docker compose restart api` (re-scan `/etc/cloudflared` subdirs) |
| Login fail / password tidak cocok | Lihat `.env` (`BOOTSTRAP_ADMIN_PASSWORD`) atau reset DB: stop api, hapus `data/vps-dashboard.db`, `docker compose up -d api` (re-bootstrap admin dari env) |

Lihat `DEPLOY_DOCKER.md` Bagian 14 untuk troubleshooting lengkap.

## File Penting

| File | Fungsi |
|---|---|
| `docker-compose.yml` | Definisi stack |
| `.env` | Secret + config (JANGAN commit) |
| `.env.docker.example` | Template env |
| `DEPLOY_DOCKER.md` | Dokumentasi lengkap deployment |
| `CLOUDFLARE_TUNNEL_SETUP.md` | Panduan tunnel + access |
| `RUN.md` | File ini (quickstart) |
| `verify.sh` | Health check semua service |
| `Makefile` | Operational commands (Linux/WSL) |
| `start.ps1` | Operational commands (Windows native) |
| `cloudflared-config/config.yml` | Konfigurasi tunnel `server-dmr` (ingress rules) |
| `cloudflared-config/credentials.json` | Credentials tunnel (JANGAN commit) |
| `nginx-dashboard.conf` | Legacy nginx config — tidak dipakai container, hanya reference |

## Naming Reference

| Komponen | Nama |
|---|---|
| Compose project | `server-dmr` |
| Network | `server-dmr-net` |
| Container API | `server-dmr-api` |
| Container Web | `server-dmr-web` |
| Container Docker Proxy | `server-dmr-docker-proxy` |
| Container Tunnel | `server-dmr-tunnel` |
| Image API | `server-dmr-api:latest` |
| Image Web | `server-dmr-web:latest` |
| Domain publik | `server-dmr.devplay.online` |
| Tunnel UUID | `f6eeebfd-8e8d-4021-8a3a-8b8a76a98182` |
