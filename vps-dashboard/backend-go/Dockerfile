# syntax=docker/dockerfile:1.7

# ---- Build stage ----
FROM golang:1.22-alpine AS builder

# Build deps untuk modernc.org/sqlite (pure-Go, tapi butuh git untuk go mod)
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /src

# Cache go module download terlebih dulu
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build statically-linked binary, strip debug info untuk size
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64
RUN go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/vps-dashboard-api \
    ./cmd/api

# ---- Runtime stage ----
FROM alpine:3.20

# Runtime + CLI tools yang dipanggil backend Go via shell (hybrid host-control mode):
#   - ca-certificates, tzdata : HTTPS outbound (Telegram, GitHub, Cloudflare API) + waktu lokal
#   - tini                    : PID 1 supaya signal handling rapi (ctrl-C, docker stop)
#   - docker-cli              : endpoint /docker/* — connect ke DOCKER_HOST=tcp://dockerproxy:2375
#                               (TIDAK install daemon `docker`, hanya client)
#   - nodejs + npm            : prerequisite untuk pm2
#   - wget                    : download cloudflared binary di bawah + healthcheck
#
# pm2 di-install via npm (-g) supaya bisa hit PM2 daemon di host lewat socket
# yang di-mount ke /home/app/.pm2 (lihat docker-compose.yml).
#
# cloudflared TIDAK ada di repo Alpine 3.20, jadi download binary statik dari GitHub
# release resmi Cloudflare. Versi di-pin via build-arg supaya reproducible.
ARG CLOUDFLARED_VERSION=2024.12.2
RUN apk add --no-cache \
        ca-certificates \
        tzdata \
        tini \
        docker-cli \
        nodejs \
        npm \
        wget && \
    npm install -g pm2@latest && \
    npm cache clean --force && \
    wget -qO /usr/local/bin/cloudflared \
        "https://github.com/cloudflare/cloudflared/releases/download/${CLOUDFLARED_VERSION}/cloudflared-linux-amd64" && \
    chmod +x /usr/local/bin/cloudflared && \
    addgroup -S -g 1000 app && \
    adduser -S -u 1000 -G app -h /home/app app

# /data dimiliki user app supaya non-root bisa tulis SQLite + backup.
# /home/app/.pm2 disiapkan sebagai mount-point untuk PM2_HOME (overlay-mounted dari host).
RUN mkdir -p /data /data/backups /home/app/.pm2 && \
    chown -R app:app /data /home/app

WORKDIR /app
COPY --from=builder /out/vps-dashboard-api /app/vps-dashboard-api

USER app:app

ENV ENV=production \
    HTTP_ADDR=:3001 \
    DB_PATH=/data/vps-dashboard.db \
    BACKUP_DIR=/data/backups \
    LOG_LEVEL=info \
    TZ=Asia/Jakarta \
    PM2_HOME=/home/app/.pm2

EXPOSE 3001

VOLUME ["/data"]

# Healthcheck: ping /health endpoint
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:3001/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/app/vps-dashboard-api"]
