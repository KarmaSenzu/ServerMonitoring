# Fase 2 — Cloudflare Tunnel Manual Setup Guide

## Quick Reference

**Tunnel Details:**
- Tunnel Name: `server-dmr`
- Domain: `devplay.online`
- Public Hostname: `server-dmr.devplay.online`
- Service: `web:80` (Docker service + port)

---

## Step-by-Step Manual Setup

### STEP 1: Verifikasi Domain (2 menit)
```
1. Buka https://dash.cloudflare.com
2. Login dengan akun Cloudflare
3. Cari domain devplay.online di list
4. Status harus "Active" ✓
   - Kalau Pending: tunggu 5-24 jam
   - Kalau tidak ada: click "Add a site", setup nameserver
```

**Dokumentasi:** https://developers.cloudflare.com/fundamentals/setup/

---

### STEP 2: Akses Zero Trust (2 menit)
```
1. Buka https://one.dash.cloudflare.com
2. Kalau pertama kali:
   - Setup team name (contoh: dmr-server)
   - Pilih plan: Free
   - Skip payment method
3. Catat team URL yang muncul
   Contoh: https://dmr-server.cloudflareaccess.com
```

**Dokumentasi:** https://developers.cloudflare.com/cloudflare-one/setup/

---

### STEP 3: Buat Tunnel (5 menit)
```
1. Di Zero Trust dashboard, sidebar kiri:
   Networks → Tunnels

2. Klik tombol "Create a tunnel" (pojok kanan atas)

3. Halaman "Select your connector":
   - Pilih "Cloudflared" (bukan WARP)
   - Klik Next

4. Halaman "Name your tunnel":
   - Tunnel name: server-dmr
   - Klik "Save tunnel"
```

**Dokumentasi:** https://developers.cloudflare.com/cloudflare-one/connections/connect-applications/

---

### STEP 4: Copy Token (2 menit) ⚠️ PENTING
```
1. Halaman berikutnya: "Install and run a connector"

2. Pilih tab "Docker" (bukan Kubernetes/Linux/macOS)

3. Akan muncul docker command panjang, misal:
   docker run cloudflare/cloudflared:latest tunnel \
     --no-autoupdate run \
     --token eyJhIjoiMzM3OTY0YjBlMTczN2EyZTk5NjEzZjVjN2IzZDAyOWMi...

4. COPY HANYA BAGIAN TOKEN (eyJ... sampai akhir):
   eyJhIjoiMzM3OTY0YjBlMTczN2EyZTk5NjEzZjVjN2IzZDAyOWMiLCJ0IjoiNGVmMjE2MWMtMzEzZS00...

5. Paste ke file: TUNNEL_TOKEN.txt (simpan di folder project)
   ⚠️ JANGAN commit ke git, JANGAN share

6. JANGAN klik Next sekarang — biarkan tab terbuka
```

**⚠️ Security Warning:**
- Token ini adalah kunci untuk tunnel Anda
- Siapa pun yang punya token bisa connect ke tunnel Anda
- Simpan dengan aman, JANGAN share, JANGAN di-commit ke git
- Jika terbuka publik, regenerate token di Cloudflare dashboard

**Dokumentasi:** https://developers.cloudflare.com/cloudflare-one/connections/connect-applications/install-and-setup/tunnel-guide/

---

### STEP 5: Add Public Hostname (3 menit)
```
1. Masih di tab Cloudflare (Install and run a connector)

2. Klik Next (atau scroll down)

3. Halaman "Route traffic":

   Field          | Value
   --------------|------------------
   Subdomain      | server-dmr
   Domain         | devplay.online (dropdown)
   Path           | (kosongkan)
   Type           | HTTP
   URL            | web:80

4. Penjelasan URL "web:80":
   - "web" = nama service di docker-compose.yml
   - "80" = port yang di-expose service
   - Cloudflared container dan web container
     berada di network yang sama, bisa resolve via DNS

5. Klik "Save tunnel"
```

**Dokumentasi:** https://developers.cloudflare.com/cloudflare-one/connections/connect-applications/install-and-setup/tunnel-guide/#set-up-a-public-hostname

---

### STEP 6: Verifikasi DNS Record (2 menit)
```
1. Buka https://dash.cloudflare.com

2. Klik domain devplay.online

3. Tab "DNS" → "Records"

4. Cari record dengan criteria:
   - Type: CNAME
   - Name: server-dmr (atau full: server-dmr.devplay.online)
   - Content: <UUID>.cfargotunnel.com (panjang)
   - Proxy status: 🟠 Proxied (orange cloud)

5. Jika record belum ada:
   - Tunggu 1-2 menit
   - Refresh halaman
   - Record seharusnya auto-created

Status Tunnel Saat Ini:
- Zero Trust → Networks → Tunnels
- server-dmr akan show "Inactive" (merah/abu)
  → NORMAL, akan berubah "Healthy" setelah container running
```

**Dokumentasi:** https://developers.cloudflare.com/dns/

---

## Checklist Fase 2

Sebelum lanjut ke Fase 3, pastikan semua ini selesai:

- [ ] Step 1: Domain devplay.online status "Active"
- [ ] Step 2: Bisa akses Zero Trust dashboard
- [ ] Step 3: Tunnel "server-dmr" terbuat
- [ ] Step 4: Token eyJ... ter-copy dan disimpan
- [ ] Step 5: Public hostname server-dmr.devplay.online → web:80 ter-save
- [ ] Step 6: DNS record CNAME terlihat di tab DNS

---

## Token Penyimpanan Sementara

Setelah copy token di Step 4, simpan di file `.env` nanti di Fase 3:

File: `.env` (di root project folder)
```
TUNNEL_TOKEN=eyJhIjoiMzM3OTY0YjBlMTczN2EyZTk5NjEzZjVjN2IzZDAyOWMi...
```

⚠️ File `.env` ada di `.gitignore`, AMAN untuk disimpan token di sini.

---

## Troubleshooting

### Domain status masih Pending
- Tunggu 5-24 jam untuk DNS propagate
- Jika sudah lebih dari 24 jam, cek nameserver di registrar sudah di-update ke Cloudflare

### DNS record tidak auto-create
- Refresh halaman DNS Records
- Tunggu 1-2 menit
- Pastikan tunnel sudah di-save dengan benar

### Tunnel status tetap Inactive
- NORMAL kalau belum ada container running
- Status akan berubah "Healthy" setelah `docker-compose up`

### Token tidak bisa di-copy
- Mungkin sudah expired (saat setup memang singkat)
- Buka tab Setup lagi atau generate token baru

---

## Next: Fase 3

Setelah semua Step 1-6 selesai dan token sudah disimpan, siap untuk Fase 3:
- Setup backend (Go)
- Setup frontend (React)
- Setup docker-compose.yml dengan tunnel container
- Test local

Link: `FASE3_BACKEND_SETUP.md`
