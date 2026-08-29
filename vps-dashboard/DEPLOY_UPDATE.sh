#!/bin/bash
# =====================================================
# Quick redeploy - build & upload update ke VPS
# Credentials & host loaded from deploy.env
# =====================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# shellcheck source=scripts/lib/load_env.sh
source "$SCRIPT_DIR/scripts/lib/load_env.sh"

cd "$PROJECT_DIR"

echo ""
echo "=========================================="
echo "  VPS Dashboard - Quick Update Deploy"
echo "=========================================="

# ==========================================
echo ""
echo "=== [1/3] BUILD FRONTEND ==="
( cd "$PROJECT_DIR/frontend" && npm run build 2>&1 )
echo "✓ Frontend built"

# ==========================================
echo ""
echo "=== [2/3] UPLOAD FILES ==="
echo "  Backend..."
vpsd_scp    "backend/server.js"    "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
vpsd_scp    "backend/package.json" "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
vpsd_scp -r "backend/routes"       "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
vpsd_scp -r "backend/services"     "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
echo "  Frontend..."
vpsd_scp -r "frontend/dist"        "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/frontend/"
echo "✓ All files uploaded"

# ==========================================
echo ""
echo "=== [3/3] FORCE UPDATE & RESTART ==="

vpsd_ssh "bash -s" <<EOF
set -e

echo "--- Clear old frontend from $REMOTE_WEB_DIR ---"
sudo rm -rf "$REMOTE_WEB_DIR"/*

echo "--- Copy new frontend ---"
sudo mkdir -p "$REMOTE_WEB_DIR"
sudo cp -r "$REMOTE_APP_DIR/frontend/dist/." "$REMOTE_WEB_DIR/"
sudo chown -R www-data:www-data "$REMOTE_WEB_DIR"
sudo chmod -R 755 "$REMOTE_WEB_DIR"

echo "--- New files ---"
ls -la "$REMOTE_WEB_DIR/"
ls -la "$REMOTE_WEB_DIR/assets/" 2>/dev/null || true

echo "--- Install backend deps ---"
cd "$REMOTE_APP_DIR/backend"
npm install --production 2>&1

echo "--- Restart PM2 ---"
pm2 restart vps-dashboard-api 2>/dev/null || pm2 start ecosystem.config.js
sleep 2
pm2 list

echo "--- Reload Nginx ---"
sudo nginx -t 2>&1
sudo systemctl reload nginx

echo "--- Test tunnels API ---"
curl -s http://localhost/system/tunnels | python3 -m json.tool 2>/dev/null || curl -s http://localhost/system/tunnels
echo ""

echo "UPDATE_DONE"
EOF
echo "✓ VPS updated & restarted"

# ==========================================
echo ""
echo "=========================================="
echo "  UPDATE DEPLOYED!"
if [ -n "${VPS_HOSTNAME:-}" ]; then
  echo "  https://${VPS_HOSTNAME}"
else
  echo "  http://${VPS_HOST}"
fi
echo ""
echo "  Hard refresh browser: Cmd+Shift+R"
echo "=========================================="

sleep 3
if [ -n "${VPS_HOSTNAME:-}" ]; then
  open "https://${VPS_HOSTNAME}" 2>/dev/null || true
else
  open "http://${VPS_HOST}" 2>/dev/null || true
fi
