# Cloudflare Tunnel - Setup Public Hostname (Routes)

## Masalah: Routes masih 0

Artinya routing belum dikonfigurasi di Cloudflare. Tunnel berjalan tapi tidak ada instruksi mana traffic yang harus diteruskan ke mana.

---

## Solusi: Setup Public Hostname di Cloudflare Dashboard

### Step 1: Buka Zero Trust Dashboard
```
https://one.dash.cloudflare.com
```

### Step 2: Navigasi ke Tunnel Configuration
```
Sidebar kiri → Networks → Tunnels
```

### Step 3: Klik Tunnel "server-dmr"
Di list tunnel, cari dan klik **server-dmr**

### Step 4: Buka Tab "Public Hostname"
Di halaman detail tunnel, cari tab atau section **"Public Hostname"** atau **"Routes"**

### Step 5: Tambahkan Public Hostname
Klik tombol **"Add a public hostname"** atau **"Add route"**

### Step 6: Isi Form Routing

Isi form dengan data berikut:

| Field | Value | Keterangan |
|-------|-------|-----------|
| **Subdomain** | `server-dmr` | Bagian subdomain (tanpa domain) |
| **Domain** | `devplay.online` | Pilih dari dropdown |
| **Path** | (kosongkan) | Biarkan kosong untuk route semua |
| **Type** | `HTTP` | Dropdown - pilih HTTP |
| **URL** | `web:80` | Nama service Docker + port |

### Step 7: Simpan
Klik tombol **"Save hostname"** atau **"Add route"**

---

## Yang Terjadi Setelah Save:

1. **Routes akan berubah dari 0 menjadi 1** ✓
2. **DNS record otomatis ter-create** (CNAME ke cfargotunnel.com)
3. **Traffic akan di-routing**: 
   ```
   User → https://server-dmr.devplay.online
        ↓ (Cloudflare)
        → Tunnel server-dmr-tunnel
        ↓ (Docker network)
        → Container web:80
        ↓
        → Frontend + Reverse Proxy ke API
   ```

---

## Verifikasi Setelah Setup:

### Di Cloudflare Dashboard:
1. Networks → Tunnels → server-dmr
2. **Public Hostname** atau **Routes** tab menunjukkan:
   ```
   server-dmr.devplay.online → web:80 (HTTP)
   ```
3. Routes counter berubah dari 0 → 1

### DNS Records (Optional Check):
1. Buka domain devplay.online
2. Tab DNS → Records
3. Cari CNAME record:
   ```
   Name: server-dmr
   Type: CNAME
   Content: dac510d2-6152-42a7-9abc-bcff931948ee.cfargotunnel.com
   Proxy: 🟠 Proxied
   ```

### Test dari Browser:
```
https://server-dmr.devplay.online
```
Seharusnya menampilkan frontend React dashboard (bukan Error 1033 lagi)

---

## Troubleshooting Jika Masih Error:

### Error 1033 masih muncul
- Verifikasi Routes menunjukkan 1 (bukan 0)
- Tunggu 1-2 menit untuk DNS propagate
- Clear browser cache (Ctrl+Shift+Delete)
- Coba incognito mode

### Route tidak ter-save
- Pastikan hostname format benar (no spaces, no special char)
- Pastikan domain devplay.online ada di dropdown
- Pastikan Type = HTTP

### Tunnel status masih "Inactive"
- NORMAL jika belum ada public hostname
- Setelah add route, status berubah ke "Healthy"

---

## Next Steps Setelah Routes Setup:

1. ✅ Verifikasi https://server-dmr.devplay.online accessible
2. ✅ Login dengan admin/AdminMonitoring@2026
3. ✅ Ganti password admin dari UI
4. ✅ Setup projects di dashboard
5. ✅ Configure monitoring untuk VPS

---

## Summary

| Komponen | Status |
|----------|--------|
| WSL2 + Docker | ✅ Setup |
| vps-dashboard Container | ✅ Running |
| Cloudflare Tunnel Connector | ✅ Connected (Routes=0) |
| **Public Hostname Route** | ❌ **BELUM SETUP** ← Do This Now |
| DNS Record | ⏳ Akan auto-create setelah route add |
| Public Access | ⏳ Akan working setelah route add |

---

**Lanjutkan dengan SETUP PUBLIC HOSTNAME sekarang!**
