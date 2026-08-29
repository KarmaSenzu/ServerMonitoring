#!/bin/bash
# ============================================
# VPS Dashboard - Full Deploy Script
# Run from local machine. Uses deploy.env for credentials.
#
# Usage: chmod +x deploy.sh && ./deploy.sh
# ============================================

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# shellcheck source=scripts/lib/load_env.sh
source "$SCRIPT_DIR/scripts/lib/load_env.sh"

echo ""
echo "=========================================="
echo "  VPS Dashboard - Full Deploy"
echo "  Target: ${VPS_USER}@${VPS_HOST}"
echo "=========================================="

# ==========================================
# STEP 1: Build frontend locally
# ==========================================
echo ""
echo "[1/6] Building frontend..."
cd "$PROJECT_DIR/frontend"
npm run build
cd "$PROJECT_DIR"
echo "  Frontend built successfully."

# ==========================================
# STEP 2: Test SSH connection
# ==========================================
echo ""
echo "[2/6] Testing SSH connection..."
vpsd_ssh "echo 'SSH connection OK - \$(hostname)'"

# ==========================================
# STEP 3: Setup server (Node, PM2, Nginx)
# ==========================================
echo ""
echo "[3/6] Setting up server environment..."
vpsd_ssh "bash -s" <<'SETUP'
set -e

if ! command -v node >/dev/null 2>&1; then
  echo "  Installing Node.js 20..."
  curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
  sudo apt install -y nodejs
else
  echo "  Node.js already installed: $(node --version)"
fi

if ! command -v pm2 >/dev/null 2>&1; then
  echo "  Installing PM2..."
  sudo npm install -g pm2
else
  echo "  PM2 already installed: $(pm2 --version)"
fi

if ! command -v nginx >/dev/null 2>&1; then
  echo "  Installing Nginx..."
  sudo apt update -qq
  sudo apt install -y nginx
  sudo systemctl enable nginx
  sudo systemctl start nginx
else
  echo "  Nginx already installed"
fi

echo "  Server environment ready."
SETUP

# ==========================================
# STEP 4: Upload files
# ==========================================
echo ""
echo "[4/6] Uploading project files..."

vpsd_ssh "mkdir -p '$REMOTE_APP_DIR/backend/routes' '$REMOTE_APP_DIR/backend/services' '$REMOTE_APP_DIR/frontend'"

echo "  Uploading backend..."
vpsd_scp    "backend/server.js"           "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
vpsd_scp    "backend/package.json"        "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
vpsd_scp    "backend/ecosystem.config.js" "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
vpsd_scp -r "backend/routes"              "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
vpsd_scp -r "backend/services"            "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"

echo "  Uploading frontend build..."
vpsd_scp -r "frontend/dist" "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/frontend/"

echo "  Uploading Nginx config..."
vpsd_scp "nginx-dashboard.conf" "$VPS_USER@$VPS_HOST:/tmp/vps-dashboard.conf"

echo "  All files uploaded."

# ==========================================
# STEP 5: Install deps & configure Nginx
# ==========================================
echo ""
echo "[5/6] Installing dependencies & configuring Nginx..."
vpsd_ssh "bash -s" <<EOF
set -e

cd "$REMOTE_APP_DIR/backend"
npm install --production

sudo cp /tmp/vps-dashboard.conf /etc/nginx/sites-available/vps-dashboard
sudo ln -sf /etc/nginx/sites-available/vps-dashboard /etc/nginx/sites-enabled/vps-dashboard
sudo rm -f /etc/nginx/sites-enabled/default

sudo nginx -t
sudo systemctl reload nginx

echo "  Dependencies installed & Nginx configured."
EOF

# ==========================================
# STEP 6: Start backend with PM2
# ==========================================
echo ""
echo "[6/6] Starting backend with PM2..."
vpsd_ssh "bash -s" <<EOF
set -e

cd "$REMOTE_APP_DIR/backend"
pm2 delete vps-dashboard-api 2>/dev/null || true
pm2 start ecosystem.config.js
pm2 save
pm2 list

echo ""
echo "=========================================="
echo "  DEPLOYMENT COMPLETE!"
echo "=========================================="
echo "  Dashboard : http://${VPS_HOST}"
echo "  API Check : http://${VPS_HOST}/health"
echo "=========================================="
EOF

echo ""
echo "Done! Buka http://${VPS_HOST} di browser kamu."
