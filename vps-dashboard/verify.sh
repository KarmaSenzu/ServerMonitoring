#!/usr/bin/env bash
# verify.sh — cek kesehatan stack monitoring server-dmr.
# Jalankan dari directory vps-dashboard/ setelah `docker compose up -d`.

set -u

# Warna terminal
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS=0
FAIL=0
WARN=0

ok()    { echo -e "${GREEN}✓${NC} $1"; PASS=$((PASS+1)); }
fail()  { echo -e "${RED}✗${NC} $1"; FAIL=$((FAIL+1)); }
warn()  { echo -e "${YELLOW}!${NC} $1"; WARN=$((WARN+1)); }
info()  { echo -e "${BLUE}i${NC} $1"; }

echo ""
echo "==== Server DMR Monitoring — Health Check ===="
echo ""

# --- 1. Docker tersedia ---
echo "[1/6] Docker daemon"
if ! command -v docker >/dev/null 2>&1; then
    fail "docker command tidak ditemukan"
    exit 1
fi
if ! docker info >/dev/null 2>&1; then
    fail "Docker daemon tidak responsif. Cek Docker Desktop."
    exit 1
fi
ok "Docker daemon aktif"

# --- 2. .env ada ---
echo ""
echo "[2/6] Konfigurasi"
if [ ! -f .env ]; then
    fail ".env tidak ditemukan. Jalankan: cp .env.docker.example .env"
    exit 1
fi
ok ".env ditemukan"

# Cek var penting tidak kosong
for var in JWT_SECRET; do
    val=$(grep -E "^${var}=" .env | head -1 | cut -d= -f2-)
    if [ -z "$val" ]; then
        fail "${var} kosong di .env"
    else
        ok "${var} ter-set"
    fi
done

# Cek cloudflared credentials & config (file-based tunnel, bukan TUNNEL_TOKEN)
if [ ! -f cloudflared-config/credentials.json ]; then
    fail "cloudflared-config/credentials.json hilang. Copy dari ~/.cloudflared/<UUID>.json"
else
    ok "cloudflared credentials.json ada"
fi
if [ ! -f cloudflared-config/config.yml ]; then
    fail "cloudflared-config/config.yml hilang"
else
    ok "cloudflared config.yml ada"
fi

# --- 3. Container status ---
echo ""
echo "[3/6] Container status"
expected_containers=("server-dmr-api" "server-dmr-web" "server-dmr-docker-proxy" "server-dmr-tunnel")

for c in "${expected_containers[@]}"; do
    state=$(docker inspect -f '{{.State.Status}}' "$c" 2>/dev/null || echo "missing")
    if [ "$state" = "running" ]; then
        # Cek health kalau ada healthcheck
        health=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$c" 2>/dev/null)
        case "$health" in
            healthy)  ok "${c} — running, healthy" ;;
            none)     ok "${c} — running" ;;
            starting) warn "${c} — running, healthcheck masih starting" ;;
            *)        warn "${c} — running, health: ${health}" ;;
        esac
    elif [ "$state" = "missing" ]; then
        fail "${c} — tidak ada (belum di-create?)"
    else
        fail "${c} — state: ${state}"
    fi
done

# --- 4. Internal API health ---
echo ""
echo "[4/6] API health endpoint (internal)"
api_health=$(docker exec server-dmr-api wget -qO- --timeout=3 http://127.0.0.1:3001/health 2>/dev/null || echo "")
if echo "$api_health" | grep -q '"status":"ok"'; then
    ok "API /health OK: ${api_health}"
else
    fail "API /health tidak respond. Cek: docker compose logs api"
fi

# --- 5. Tunnel status ---
echo ""
echo "[5/6] Cloudflare Tunnel"
tunnel_log=$(docker logs --tail=200 server-dmr-tunnel 2>&1 || echo "")
if echo "$tunnel_log" | grep -q "Registered tunnel connection"; then
    conn_count=$(echo "$tunnel_log" | grep -c "Registered tunnel connection" || true)
    ok "Tunnel connected (${conn_count} connections terbentuk)"
elif echo "$tunnel_log" | grep -q "context deadline exceeded"; then
    fail "Tunnel timeout saat dial edge. Cek koneksi internet WSL: curl https://1.1.1.1"
elif echo "$tunnel_log" | grep -qi "invalid token\|bad token\|unauthorized\|Couldn't read credentials\|tunnel credentials"; then
    fail "Credentials tunnel tidak valid. Cek cloudflared-config/credentials.json + UUID di config.yml."
elif echo "$tunnel_log" | grep -q "No ingress rules"; then
    fail "Ingress rules kosong. Cek cloudflared-config/config.yml bagian 'ingress:'."
else
    warn "Tunnel belum connected (mungkin baru start). Tunggu 30 detik lalu cek log: docker compose logs cloudflared"
fi

# --- 6. End-to-end via Cloudflare ---
echo ""
echo "[6/6] End-to-end via Cloudflare"
if ! command -v curl >/dev/null 2>&1; then
    warn "curl tidak ada, skip test E2E"
else
    domain="${DOMAIN:-server-dmr.devplay.online}"
    code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "https://${domain}/" 2>/dev/null || echo "000")
    case "$code" in
        302|303)
            ok "Domain redirect (kemungkinan ke Cloudflare Access). HTTP ${code}"
            info "  → Itu artinya Cloudflare Access aktif. Login lewat browser."
            ;;
        200)
            ok "Domain reachable. HTTP 200"
            warn "  → Tidak ada redirect Access. Pastikan Cloudflare Access ON kalau dashboard ini publik!"
            ;;
        000)
            fail "Domain tidak reachable (timeout/DNS error). Cek tunnel + DNS record di Cloudflare."
            ;;
        5*)
            fail "Domain return ${code}. Backend/web ada masalah. Cek log."
            ;;
        *)
            warn "Domain return ${code}. Investigasi manual."
            ;;
    esac
fi

# --- Summary ---
echo ""
echo "============================================"
echo -e "Hasil: ${GREEN}${PASS} OK${NC} / ${YELLOW}${WARN} warning${NC} / ${RED}${FAIL} fail${NC}"
echo "============================================"

if [ "$FAIL" -gt 0 ]; then
    echo ""
    echo "Ada ${FAIL} pemeriksaan gagal. Step debug:"
    echo "  1. docker compose ps           — lihat state container"
    echo "  2. docker compose logs --tail=100 — lihat log error"
    echo "  3. cat .env                    — cek secret tidak kosong"
    echo "  4. Lihat DEPLOY_DOCKER.md bagian 13 (Troubleshooting)"
    exit 1
fi

if [ "$WARN" -gt 0 ]; then
    echo ""
    echo "Stack jalan tapi ada ${WARN} warning. Investigasi opsional di atas."
fi

echo ""
echo "✓ Stack ready. Buka https://server-dmr.devplay.online"
exit 0
