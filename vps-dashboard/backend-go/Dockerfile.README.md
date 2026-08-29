# Dockerfile — vps-dashboard-api

Multi-stage build untuk backend Go (Gin + modernc.org/sqlite, pure-Go, no CGO).

## Build manual

```bash
docker build -t server-dmr-api:latest .
```

Build memakai `golang:1.22-alpine` sebagai builder, lalu copy binary ke `alpine:3.20`
sebagai runtime. Migrations (`internal/db/migrations/*.sql`) di-embed via `embed.FS`
saat compile, jadi tidak perlu di-copy terpisah ke image.

## Test cepat (standalone, tanpa monitoring host)

```bash
docker run --rm -p 3001:3001 \
    -e JWT_SECRET=test \
    -e BOOTSTRAP_ADMIN_PASSWORD=test \
    server-dmr-api:latest

# di terminal lain:
curl localhost:3001/health
```

Mode standalone ini **tidak** akan punya akses ke `/proc`, `/sys`, `/etc` host,
sehingga metrik gopsutil hanya melaporkan resource container itu sendiri (bukan host).

## Mode produksi (monitoring host VPS)

Untuk monitoring host yang sebenarnya, jalankan via `docker-compose.yml` di root
project. Compose file melakukan bind-mount `/proc:/host/proc:ro`, `/sys:/host/sys:ro`,
`/etc:/host/etc:ro` plus set env `HOST_PROC`, `HOST_SYS`, `HOST_ETC` agar gopsutil
membaca data host, bukan data container.

## Spesifikasi image

- Base runtime: `alpine:3.20` + `tini` (PID 1) + `ca-certificates` + `tzdata`
- Run as: user `app` UID/GID `1000:1000` (non-root)
- Volume: `/data` (SQLite DB + backup)
- Port: `3001` (HTTP)
- Healthcheck: `wget` ke `/health` tiap 30s
- Ukuran final image: ~25–30 MB
