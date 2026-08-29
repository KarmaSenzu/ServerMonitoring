#!/bin/bash
# =====================================================
# Fix: arahkan tunnel ke Nginx (port 80) bukan Express (3001)
# Nginx serves frontend + proxies API.
# Credentials & host loaded from deploy.env
# =====================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# shellcheck source=scripts/lib/load_env.sh
source "$SCRIPT_DIR/scripts/lib/load_env.sh"

cd "$PROJECT_DIR"

if [ -z "${VPS_HOSTNAME:-}" ]; then
  echo "ERROR: VPS_HOSTNAME is not set in deploy.env" >&2
  exit 1
fi

echo ""
echo "=== FIX TUNNEL: arahkan ke port 80 (Nginx) ==="

vpsd_ssh "bash -s" <<EOF
set -e

echo "===== Cari tunnel ID vps-dashboard ====="
TUNNEL_ID=\$(cloudflared tunnel list 2>/dev/null | grep "vps-dashboard" | awk '{print \$1}')
echo "Tunnel ID: \$TUNNEL_ID"

if [ -z "\$TUNNEL_ID" ]; then
  echo "ERROR: cannot find a 'vps-dashboard' tunnel" >&2
  exit 1
fi

echo ""
echo "===== Update config: port 3001 → port 80 ====="
sudo tee /etc/cloudflared/config-dashboard.yml > /dev/null <<YAMLEOF
tunnel: \${TUNNEL_ID}
credentials-file: /home/ubuntu/.cloudflared/\${TUNNEL_ID}.json
ingress:
- hostname: ${VPS_HOSTNAME}
  service: http://localhost:80
- service: http_status:404
YAMLEOF

cat /etc/cloudflared/config-dashboard.yml

echo ""
echo "===== Restart tunnel ====="
sudo systemctl restart cloudflared-dashboard
sleep 5
sudo systemctl status cloudflared-dashboard --no-pager | head -10

echo ""
echo "===== Test ====="
echo "Nginx (port 80):"
curl -s -o /dev/null -w "  HTTP %{http_code}\n" http://localhost/
echo "API health:"
curl -s http://localhost/health
echo ""
echo "With Host header:"
curl -s -o /dev/null -w "  HTTP %{http_code}\n" -H "Host: ${VPS_HOSTNAME}" http://localhost/

echo ""
echo "FIX_PORT_DONE"
EOF

echo ""
echo "=== Waiting 10 seconds... ==="
sleep 10

echo ""
echo "=== TESTING ==="
HTTP=$(curl -s -o /dev/null -w "%{http_code}" "https://${VPS_HOSTNAME}/" 2>/dev/null || echo "000")
echo "  https://${VPS_HOSTNAME} → HTTP $HTTP"

HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "https://${VPS_HOSTNAME}/health" 2>/dev/null || echo "000")
echo "  /health → HTTP $HEALTH"

echo ""
if [ "$HTTP" = "200" ]; then
  echo "=========================================="
  echo "  BERHASIL!"
  echo "  https://${VPS_HOSTNAME}"
  echo "=========================================="
  open "https://${VPS_HOSTNAME}" 2>/dev/null || true
else
  echo "  Paste output ini ke chat."
fi
