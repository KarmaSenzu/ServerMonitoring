# Cloudflare Tunnel + Cloudflare Access — Panduan Setup Super Detail

Panduan ini memandu kamu dari nol sampai punya domain `server-dmr.devplay.online` yang aman: traffic publik masuk lewat **Cloudflare Tunnel** (tanpa buka port di VPS / router), dan akses dashboard dilindungi **Cloudflare Access** (login pakai email OTP sebelum sampai ke aplikasi). Hasil akhir: dua lapis auth — Cloudflare Access (email OTP) → JWT login dashboard kamu sendiri. Total estimasi waktu kalau lancar: **30–35 menit**.

Yang harus kamu siapkan sebelum mulai:
- Akun Cloudflare aktif, domain `devplay.online` sudah ditambahkan dan status **Active**.
- Docker + Docker Compose sudah jalan (di WSL2 / Linux).
- File `vps-dashboard/docker-compose.yml` sudah ada dengan service `web` (port 80), `api` (port 3001), dan `cloudflared` (container name `server-dmr-tunnel`).
- Email yang bisa kamu akses untuk terima OTP.

---

## Bagian A — Persiapan (5 menit)

### A.1. Pastikan domain `devplay.online` sudah di Cloudflare

1. Login ke `https://dash.cloudflare.com`.
2. Di sidebar kiri (atau halaman utama "Websites"), klik domain **`devplay.online`**.
3. Lihat status di kanan atas / di bawah nama domain — harus tertulis **Active** (background hijau).
   - Kalau **Pending Nameserver Update**: berarti nameserver di registrar (tempat kamu beli domain) belum diarahkan ke Cloudflare. Tunggu propagasi (5 menit – 24 jam). Verifikasi nameserver di tab **DNS** → bagian "Cloudflare Nameservers".
   - Kalau **Moved**: domain sudah pindah dari Cloudflare. Stop, harus re-add domain dulu.

### A.2. Buka Zero Trust dashboard

1. Buka tab baru: `https://one.dash.cloudflare.com`.
2. Kalau ini pertama kali kamu pakai Zero Trust, akan muncul wizard:
   - **Choose a team name**: ketik `dmr-server` (atau bebas, ini hanya subdomain login Cloudflare). Team name tidak bisa diubah dengan mudah, jadi pilih yang netral.
   - **Choose a plan**: pilih **Free** (sampai 50 user gratis selamanya, lebih dari cukup).
   - Klik **Purchase** (gratis, tidak akan ditagih kalau pilih Free).
3. **Catat URL team kamu**: `https://dmr-server.cloudflareaccess.com` (ganti `dmr-server` dengan team name yang kamu pilih). URL ini yang akan jadi halaman login Cloudflare Access nanti.

> Tip: kalau kamu lupa team name, bisa lihat di Zero Trust → **Settings** → **Custom Pages** → "Team domain".

---

## Bagian B — Buat Tunnel (10 menit)

### B.1. Navigasi ke Tunnels

1. Masih di `https://one.dash.cloudflare.com`.
2. Sidebar kiri → klik **Networks** → klik **Tunnels**.
3. Di kanan atas, klik tombol biru **Create a tunnel**.

### B.2. Pilih konektor

1. Akan muncul pilihan dua kartu: **Cloudflared** dan **WARP Connector**.
2. Klik kartu **Cloudflared** (yang kiri).
3. Klik **Next** di kanan bawah.

### B.3. Beri nama tunnel

1. Field **Name your tunnel**: ketik `server-dmr`.
   - Nama ini hanya label internal di dashboard, tidak terlihat user.
2. Klik **Save tunnel** (kanan bawah).

### B.4. Copy TUNNEL_TOKEN

Setelah save, halaman pindah ke "Install and run a connector".

1. Di tab **Choose your environment**, pilih ikon **Docker**.
2. Akan muncul box berisi command, formatnya seperti ini:
   ```bash
   docker run cloudflare/cloudflared:latest tunnel --no-autoupdate run --token eyJhIjoiXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX...
   ```
3. **Yang perlu kamu copy hanyalah string token-nya**, yaitu bagian setelah `--token ` sampai akhir baris.
   - Token dimulai dengan `eyJ` (ini base64 dari JSON header).
   - Panjangnya **ratusan karakter** (biasanya 200–300 karakter). Pastikan kamu copy **utuh sampai karakter terakhir** — kalau kurang satu huruf saja, token invalid.
   - Cara aman: triple-click di token untuk select semua, lalu Ctrl+C.
4. **JANGAN klik Next sekarang.** Biarkan tab ini terbuka. Kita akan kembali setelah container cloudflared running.

### B.5. Paste token ke `.env`

1. Buka file `vps-dashboard/.env` di editor (VS Code, nano, dll).
   - Kalau file `.env` belum ada, copy dari template:
     ```bash
     cd vps-dashboard
     cp .env.docker.example .env
     ```
2. Cari (atau tambahkan) baris:
   ```env
   TUNNEL_TOKEN=
   ```
3. Paste token setelah tanda `=` (tanpa spasi, tanpa tanda kutip):
   ```env
   TUNNEL_TOKEN=eyJhIjoiXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX...
   ```
4. Save file (Ctrl+S).

> Verifikasi: jalankan `cat vps-dashboard/.env | grep TUNNEL_TOKEN` — harusnya muncul satu baris token utuh.

### B.6. Start container cloudflared (dari WSL2 / Linux)

```bash
cd vps-dashboard
docker compose up -d cloudflared
```

Output yang diharapkan:
```
[+] Running 2/2
 ✔ Network server-dmr-net  Created
 ✔ Container server-dmr-tunnel  Started
```

> Catatan: kalau container `web` dan `api` belum running, tunnel akan tetap start dan terhubung ke Cloudflare, tapi tidak ada upstream untuk dilayani. Itu **normal** untuk sekarang — kita akan setup hostname dulu, baru start service lain.

### B.7. Kembali ke browser tab Cloudflare

1. Pindah ke tab Cloudflare yang masih terbuka di halaman "Install and run a connector".
2. Klik **Next** di kanan bawah.
3. Cloudflare akan otomatis mendeteksi connector kamu. Ini biasanya butuh **30–60 detik**.
4. Setelah konek, status berubah jadi **Connected** dengan icon hijau dan akan terlihat 1 connector aktif.
   - Kalau setelah 2 menit masih "No connector", buka terminal:
     ```bash
     docker compose logs cloudflared --tail=50
     ```
     Cari baris error. Yang paling sering: token salah copy (kurang karakter), atau koneksi internet WSL bermasalah.

---

## Bagian C — Tambahkan Public Hostname (5 menit)

Setelah connector terdeteksi, halaman pindah ke "Route traffic" / "Public Hostnames".

### C.1. Halaman "Route traffic"

Isi form dengan **value persis seperti ini**:

| Field | Value |
|---|---|
| **Subdomain** | `server-dmr` |
| **Domain** | pilih `devplay.online` dari dropdown |
| **Path** | (kosongkan) |
| **Type** | `HTTP` |
| **URL** | `web:80` |

> **PENTING — kenapa `web:80` bukan `localhost:80`?**
> Container `cloudflared` ada di Docker network yang sama (`server-dmr-net`) dengan container `web`. Docker DNS otomatis resolve nama service (`web`) ke IP container-nya. Kalau kamu pakai `localhost` atau `127.0.0.1`, itu akan merujuk ke dalam container `cloudflared` sendiri (yang tidak ada nginx-nya), dan akan dapat connection refused.

Klik **Save tunnel** di kanan bawah.

### C.2. Verifikasi DNS auto-created

Cloudflare otomatis bikin DNS CNAME record. Verifikasi:

1. Buka tab baru: `https://dash.cloudflare.com`.
2. Klik domain `devplay.online`.
3. Sidebar kiri → klik **DNS** → **Records**.
4. Cari record dengan:
   - **Name**: `server-dmr`
   - **Type**: `CNAME`
   - **Content**: `<tunnel-id>.cfargotunnel.com` (UUID + `.cfargotunnel.com`)
   - **Proxy status**: **Proxied** (icon awan oranye, BUKAN abu-abu)

Kalau proxy status abu-abu (DNS only), klik record tersebut → toggle **Proxy status** ke ON → Save. Tanpa proxied, traffic tidak akan lewat tunnel.

### C.3. Test akses (masih tanpa Access layer)

Buka browser ke `https://server-dmr.devplay.online`.

Hasil yang mungkin:
- **Halaman frontend muncul**: container `web` sudah running, semua jalan.
- **502 Bad Gateway / 503 Service Unavailable**: tunnel jalan tapi container `web` belum start. Jalankan `docker compose up -d web api` untuk start service lain. Itu wajar untuk sekarang, kita lanjut setup Access dulu.
- **DNS_PROBE_FINISHED_NXDOMAIN**: DNS belum propagasi. Tunggu 1–2 menit, atau flush DNS lokal (`ipconfig /flushdns` di Windows).

---

## Bagian D — Cloudflare Access (Login Layer) — 10 menit

Sekarang domain bisa diakses publik tanpa auth. Kita tambahkan layer Cloudflare Access supaya hanya email yang di-whitelist yang bisa lewat.

### D.1. Buat Application

1. Kembali ke Zero Trust dashboard: `https://one.dash.cloudflare.com`.
2. Sidebar kiri → **Access** → **Applications**.
3. Klik **Add an application** (tombol biru kanan atas).
4. Pilih kartu **Self-hosted** (yang pertama).
5. Klik **Select**.

### D.2. Configuration

Halaman "Configure your application". Isi:

| Field | Value |
|---|---|
| **Application name** | `Server DMR Dashboard` |
| **Session Duration** | `24 hours` (atau `8 hours` kalau mau lebih ketat) |
| **Subdomain** | `server-dmr` |
| **Domain** | pilih `devplay.online` dari dropdown |
| **Path** | (kosongkan) |

Scroll ke bawah ke bagian **Identity providers**:
- Centang **One-time PIN** (default tersedia, akan kirim OTP 6 digit ke email).
- Identity provider lain (Google, GitHub, dll) tidak wajib — One-time PIN sudah cukup untuk personal use.

Bagian **Application Appearance** dan lain-lain biarkan default.

Klik **Next** di kanan bawah.

### D.3. Tambahkan Policy

Halaman "Add policies".

| Field | Value |
|---|---|
| **Policy name** | `Owner only` |
| **Action** | `Allow` |
| **Session duration** | `Same as application session timeout` |

Scroll ke **Configure rules**:
1. Di bagian **Include**:
   - **Selector**: pilih **Emails** dari dropdown.
   - **Value**: ketik email kamu yang valid (misal `kamu@gmail.com`), tekan **Enter** setelah ketik.
   - Bisa tambah lebih dari satu email — ulangi: ketik email berikutnya, Enter.

> Penting: email harus **persis sama** dengan yang akan kamu pakai login (case-insensitive sebenarnya, tapi lebih aman pakai lowercase semua). Jangan typo.

Klik **Next**.

### D.4. Setup tambahan (opsional)

Halaman berikutnya minta konfigurasi:
- **CORS settings**: biarkan default (kosong).
- **Cookie settings**: biarkan default.
- **Additional settings**: biarkan default.

Klik **Add application** di kanan bawah.

Aplikasi terdaftar. Kamu akan kembali ke list Applications dengan `Server DMR Dashboard` muncul di list.

### D.5. Test login flow

1. Buka **incognito window** (Ctrl+Shift+N di Chrome) — supaya tidak terbawa cookie session lama.
2. Akses `https://server-dmr.devplay.online`.
3. Akan diarahkan otomatis ke `https://dmr-server.cloudflareaccess.com/...` (halaman login Cloudflare).
4. Pilih metode **One-time PIN**.
5. Masukkan email yang kamu whitelist tadi → klik **Send me a code**.
6. Buka inbox email, cari email dari **noreply@notify.cloudflare.com** dengan subject **"Your one-time PIN"**.
7. Copy **6-digit PIN** dari email.
8. Paste di field PIN → klik **Sign in**.
9. Setelah verify, kamu akan diredirect kembali ke `https://server-dmr.devplay.online` — dan sekarang sampai ke aplikasi (akan ketemu halaman login dashboard JWT kamu).

> Berhasil sampai sini = kamu punya **2 lapis auth**: Cloudflare Access (email OTP) → JWT login dashboard.

---

## Bagian E — Verifikasi Akhir (5 menit)

### E.1. Cek tunnel status

1. Zero Trust → **Networks** → **Tunnels** → klik tunnel `server-dmr`.
2. Tab **Connectors**: harus ada minimal 1 connector dengan status **HEALTHY** (badge hijau).
3. Di terminal:
   ```bash
   docker compose logs cloudflared --tail=50
   ```
   Cari baris seperti:
   ```
   INF Registered tunnel connection connIndex=0 ...
   INF Registered tunnel connection connIndex=1 ...
   ```
   Biasanya ada 4 connection (ke 4 lokasi edge berbeda untuk redundansi).

### E.2. Cek end-to-end

```bash
# Test dari WSL/Linux:
curl -I https://server-dmr.devplay.online
```

Output yang diharapkan (karena Access aktif):
```
HTTP/2 302
location: https://dmr-server.cloudflareaccess.com/cdn-cgi/access/login/...
server: cloudflare
cf-ray: 8x...
```

302 redirect ke `cloudflareaccess.com` membuktikan Access policy bekerja — request publik tidak bisa langsung sampai ke aplikasi tanpa login.

Dari browser yang sudah login Access: bisa akses dashboard normal.

### E.3. Cek security headers

1. Di browser yang sudah login, buka DevTools (F12) → tab **Network**.
2. Reload halaman (Ctrl+R).
3. Klik request pertama (yang dokumen HTML) → tab **Headers** → scroll ke **Response Headers**.
4. Confirm ada:
   - `cf-ray: <id>-<location>` — Cloudflare request ID + edge location
   - `cf-cache-status: <status>` — status caching Cloudflare
   - `server: cloudflare`

Kalau ketiganya ada → traffic confirmed lewat Cloudflare edge, bukan langsung ke origin.

---

## Bagian F — Troubleshooting

### F.1. Tunnel status "Disconnected" / "Inactive"

1. Cek container running:
   ```bash
   docker compose ps
   ```
   Container `server-dmr-tunnel` harus status `Up`.
2. Cek log lengkap:
   ```bash
   docker compose logs cloudflared --tail=100
   ```
3. Error umum dan solusinya:
   - `failed to dial to edge` → masalah konektivitas internet WSL. Test: `curl https://1.1.1.1`. Kalau gagal, restart WSL: `wsl --shutdown` di PowerShell, lalu start ulang.
   - `Unauthorized: Invalid tunnel secret` → token salah / tunnel sudah dihapus. Lihat F.4.
   - `context deadline exceeded` → firewall blokir port outbound 7844. Cek `iptables` / firewall rule.
4. Restart container:
   ```bash
   docker compose restart cloudflared
   ```

### F.2. 502 Bad Gateway saat akses domain

Tunnel jalan, tapi tidak bisa reach `web:80`.

1. Cek nginx running:
   ```bash
   docker compose logs web --tail=50
   ```
   Harus ada baris `nginx: configuration file ... is successful`.
2. Cek network:
   ```bash
   docker network inspect server-dmr-net
   ```
   Di list `Containers`, harus ada minimal 3: `server-dmr-tunnel`, `server-dmr-web` (atau nama service `web`), dan service lain.
3. Test dari dalam container cloudflared:
   ```bash
   docker compose exec cloudflared wget -qO- http://web:80
   ```
   Kalau ini error juga, masalah bukan di tunnel tapi di komunikasi antar container.
4. Pastikan service URL di tunnel config persis `web:80` (bukan `web`, bukan `http://web:80`).

### F.3. Cloudflare Access loop / tidak bisa login

1. Pastikan email yang di-whitelist **persis sama** dengan yang dipakai login.
2. Pastikan policy **Allow** ada (bukan **Block** / **Bypass**), dan ada di paling atas kalau ada policy lain.
3. Clear cookie:
   - Buka `https://dmr-server.cloudflareaccess.com` di browser
   - DevTools → Application → Cookies → hapus semua cookie domain `cloudflareaccess.com`
   - Coba login ulang dari incognito.
4. Cek logs Access: Zero Trust → **Logs** → **Access** — lihat apakah ada attempt yang di-deny dan alasannya.

### F.4. Token expired / invalid

Token tunnel **tidak expired** secara default. Kalau dapat error invalid:

1. Kemungkinan: tunnel di-delete dari dashboard (bisa terjadi kalau tidak sengaja klik delete).
2. Atau token salah copy (kurang karakter di akhir karena terpotong saat select).
3. Solusi:
   - Bikin tunnel baru dari awal (Bagian B), atau
   - Refresh token: di Zero Trust → Tunnels → klik tunnel → **Configure** → **Refresh token** → copy token baru → update `.env` → `docker compose restart cloudflared`.

### F.5. Cek metric tunnel (advanced)

Untuk monitoring detail, enable metrics endpoint:

1. Edit `vps-dashboard/docker-compose.yml`, di service `cloudflared`:
   ```yaml
   command: tunnel --no-autoupdate --metrics 0.0.0.0:2000 run
   ```
2. Restart:
   ```bash
   docker compose up -d cloudflared
   ```
3. Cek metrics:
   ```bash
   docker compose exec cloudflared wget -qO- http://127.0.0.1:2000/metrics
   ```
   Akan keluar Prometheus-format metrics: jumlah request, latency, error rate, dll.

---

## Bagian G — Tambahkan Subdomain Lain (Bonus)

Skenario: kamu mau `api.devplay.online` untuk akses API langsung (bypass nginx, untuk testing atau client mobile).

### G.1. Tambah Public Hostname baru

1. Zero Trust → **Networks** → **Tunnels** → klik `server-dmr`.
2. Klik tab **Public Hostname**.
3. Klik **Add a public hostname**.
4. Isi:

   | Field | Value |
   |---|---|
   | **Subdomain** | `api` |
   | **Domain** | `devplay.online` |
   | **Path** | (kosongkan) |
   | **Type** | `HTTP` |
   | **URL** | `api:3001` |

5. Klik **Save hostname**.
6. DNS record CNAME otomatis dibuat di `devplay.online` (verifikasi seperti C.2).

### G.2. Tambah Access policy untuk subdomain baru

Ulangi **Bagian D** dengan adjustment:
- Application name: `Server DMR API`
- Subdomain: `api`
- Domain: `devplay.online`
- Policy: bisa pakai email whitelist yang sama, atau bikin terpisah kalau API mau di-share ke tim lain.

> Tip: kalau mau API bypass Access (misal untuk webhook eksternal), buat policy **Bypass** dengan rule "Everyone" — tapi ingat, tanpa Access berarti API harus punya auth sendiri yang kuat (API key / JWT).

---

## Bagian H — Cheatsheet (Reference Cepat)

Tabel ringkas semua value yang dipakai di setup ini:

| Item | Value |
|---|---|
| Tunnel name | `server-dmr` |
| Public hostname | `server-dmr.devplay.online` |
| Service URL (di tunnel) | `web:80` |
| Container tunnel name | `server-dmr-tunnel` |
| Compose service name | `cloudflared` |
| Docker network | `server-dmr-net` |
| Access app name | `Server DMR Dashboard` |
| Access policy name | `Owner only` |
| Access action | `Allow` |
| Identity provider | One-time PIN (email OTP) |
| Session duration | 24 hours |
| Team domain | `https://<team-name>.cloudflareaccess.com` |
| ENV variable | `TUNNEL_TOKEN` di `vps-dashboard/.env` |

Command paling sering dipakai:

```bash
# Start tunnel
docker compose up -d cloudflared

# Lihat status
docker compose ps cloudflared

# Lihat log
docker compose logs cloudflared --tail=50 -f

# Restart setelah ganti token
docker compose restart cloudflared

# Test end-to-end (harus dapat 302 ke cloudflareaccess.com)
curl -I https://server-dmr.devplay.online
```

---

## Catatan Keamanan & Operasional

- **Jangan commit `.env` ke git.** Pastikan `.env` ada di `.gitignore`. Kalau bocor, refresh token (lihat F.4).
- **Backup team name**: kalau team name hilang dari memori, semua URL Access kamu akan susah ditemukan. Catat di password manager.
- **Audit Access logs** rutin: Zero Trust → Logs → Access. Cek ada attempt mencurigakan atau tidak.
- **Rotate token** kalau ada pegawai/kolaborator yang keluar dan dia punya akses ke `.env`.
- **Free plan limit**: 50 user di Access. Lebih dari cukup untuk personal / tim kecil.

Untuk operasional harian (deploy, update, restart, backup) baca: [`DEPLOY_DOCKER.md`](./DEPLOY_DOCKER.md).
