#!/bin/bash
# =====================================================
# VPS Dashboard - Fix & Deploy FINAL
# Fix 500 + tambah cloudflare tunnel hostname
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
echo "=========================================="
echo "  VPS Dashboard - Fix & Deploy"
echo "  Domain: ${VPS_HOSTNAME}"
echo "=========================================="

# ==========================================
echo ""
echo "=== [1/3] UPLOAD NGINX CONFIG ==="
vpsd_scp "nginx-dashboard.conf" "$VPS_USER@$VPS_HOST:/tmp/vps-dashboard.conf"
echo "✓ Config uploaded"

# ==========================================
echo ""
echo "=== [2/3] FIX NGINX + PERMISSIONS ==="

vpsd_ssh "bash -s" <<EOF
set -e

echo "=== Copy frontend to $REMOTE_WEB_DIR ==="
sudo mkdir -p "$REMOTE_WEB_DIR"
sudo cp -r "$REMOTE_APP_DIR/frontend/dist/." "$REMOTE_WEB_DIR/"
sudo chown -R www-data:www-data "$REMOTE_WEB_DIR"
sudo chmod -R 755 "$REMOTE_WEB_DIR"
ls -la "$REMOTE_WEB_DIR/"

echo ""
echo "=== Apply Nginx config ==="
sudo cp /tmp/vps-dashboard.conf /etc/nginx/sites-available/vps-dashboard
sudo ln -sf /etc/nginx/sites-available/vps-dashboard /etc/nginx/sites-enabled/vps-dashboard
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t 2>&1
sudo systemctl reload nginx
echo "Nginx OK"

echo ""
echo "=== Check PM2 ==="
pm2 list

echo ""
echo "=== Test localhost ==="
curl -s -o /dev/null -w "HTTP: %{http_code}\n" http://localhost/
curl -s http://localhost/health
echo ""
echo "NGINX_FIXED"
EOF
echo "✓ Nginx fixed"

# ==========================================
echo ""
echo "=== [3/3] ADD CLOUDFLARE TUNNEL HOSTNAME ==="

# Pass VPS_HOSTNAME into the remote shell via env so the heredoc stays single-quoted-ish.
vpsd_ssh "VPS_HOSTNAME='$VPS_HOSTNAME' bash -s" <<'REMOTE_EOF'
set -e

echo "=== Finding cloudflared config ==="
CONFIG_FILE=""
for f in /etc/cloudflared/config.yml /home/ubuntu/.cloudflared/config.yml /root/.cloudflared/config.yml /usr/local/etc/cloudflared/config.yml; do
  if [ -f "$f" ]; then
    CONFIG_FILE="$f"
    echo "Found: $f"
    break
  fi
done

if [ -z "$CONFIG_FILE" ]; then
  SYSTEMD_FILE=$(sudo systemctl cat cloudflared 2>/dev/null | grep -oP '(?<=--config )\S+' || true)
  if [ -n "$SYSTEMD_FILE" ] && [ -f "$SYSTEMD_FILE" ]; then
    CONFIG_FILE="$SYSTEMD_FILE"
    echo "Found via systemd: $CONFIG_FILE"
  fi
fi

if [ -z "$CONFIG_FILE" ]; then
  echo "Searching entire system..."
  CONFIG_FILE=$(sudo find / -name "config.yml" -path "*/cloudflared/*" 2>/dev/null | head -1)
  if [ -n "$CONFIG_FILE" ]; then
    echo "Found: $CONFIG_FILE"
  fi
fi

if [ -z "$CONFIG_FILE" ]; then
  echo "ERROR: cloudflared config not found!"
  echo "Listing cloudflared related files:"
  sudo find / -name "*.yml" -path "*cloudflare*" 2>/dev/null || true
  sudo find / -name "*.yaml" -path "*cloudflare*" 2>/dev/null || true
  exit 1
fi

echo ""
echo "=== Current config ==="
sudo cat "$CONFIG_FILE"

echo ""
echo "=== Checking if ${VPS_HOSTNAME} already exists ==="
if sudo grep -q "$VPS_HOSTNAME" "$CONFIG_FILE"; then
  echo "$VPS_HOSTNAME already in config, skipping"
else
  echo "Adding $VPS_HOSTNAME..."

  sudo cp "$CONFIG_FILE" "${CONFIG_FILE}.bak.$(date +%s)"

  if ! sudo VPS_HOSTNAME="$VPS_HOSTNAME" CONFIG_FILE="$CONFIG_FILE" python3 -c '
import os, sys, yaml

cfg_path = os.environ["CONFIG_FILE"]
hostname = os.environ["VPS_HOSTNAME"]

with open(cfg_path, "r") as f:
    config = yaml.safe_load(f)

if "ingress" not in config:
    print("ERROR: no ingress section found")
    sys.exit(1)

for rule in config["ingress"]:
    if rule.get("hostname") == hostname:
        print("Already exists")
        sys.exit(0)

new_rule = {"hostname": hostname, "service": "http://localhost:80"}
config["ingress"].insert(-1, new_rule)

with open(cfg_path, "w") as f:
    yaml.dump(config, f, default_flow_style=False, sort_keys=False)

print("Rule added successfully")
'; then
    echo "Python yaml failed, trying sed fallback..."
    sudo sed -i "/- service: http_status:404/i\\  - hostname: ${VPS_HOSTNAME}\\n    service: http://localhost:80" "$CONFIG_FILE" 2>/dev/null \
      || sudo sed -i "/service: http_status:404/i\\  - hostname: ${VPS_HOSTNAME}\\n    service: http://localhost:80" "$CONFIG_FILE"
    echo "Added via sed"
  fi

  echo ""
  echo "=== Updated config ==="
  sudo cat "$CONFIG_FILE"
fi

echo ""
echo "=== Restarting cloudflared ==="
sudo systemctl restart cloudflared 2>/dev/null || sudo service cloudflared restart 2>/dev/null || {
  echo "Trying to find cloudflared service..."
  sudo systemctl list-units | grep -i cloud || true
  sudo ps aux | grep cloudflared || true
}

sleep 3
echo ""
echo "=== Cloudflared status ==="
sudo systemctl status cloudflared --no-pager 2>/dev/null | head -15 || echo "Could not get status"

echo ""
echo "TUNNEL_DONE"
REMOTE_EOF
echo "✓ Tunnel configured"

# ==========================================
echo ""
echo "=== VERIFYING ==="
sleep 5

echo "  Testing http://${VPS_HOST}..."
HTTP_IP=$(curl -s -o /dev/null -w "%{http_code}" "http://${VPS_HOST}/" 2>/dev/null || echo "000")
echo "  → HTTP $HTTP_IP"

echo "  Testing https://${VPS_HOSTNAME}..."
HTTP_DOMAIN=$(curl -s -o /dev/null -w "%{http_code}" "https://${VPS_HOSTNAME}/" 2>/dev/null || echo "000")
echo "  → HTTP $HTTP_DOMAIN"

echo ""
echo "=========================================="
echo "  DEPLOY COMPLETE!"
echo ""
echo "  Dashboard: https://${VPS_HOSTNAME}"
echo "  API:       https://${VPS_HOSTNAME}/health"
echo "=========================================="
echo ""

open "https://${VPS_HOSTNAME}" 2>/dev/null || true
echo "Done!"
