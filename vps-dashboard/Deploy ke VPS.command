#!/bin/bash
# ============================================
# Double-click file ini untuk deploy ke VPS
# Credentials & host loaded from deploy.env via scripts/lib/load_env.sh
# ============================================

set -e
clear

PROJECT_DIR_LOCAL="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR_LOCAL"

# shellcheck source=scripts/lib/load_env.sh
source "$PROJECT_DIR_LOCAL/scripts/lib/load_env.sh"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

print_step() {
  echo ""
  echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${BOLD}${CYAN}  $1${NC}"
  echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

print_ok()  { echo -e "  ${GREEN}✓${NC} $1"; }
print_err() { echo -e "  ${RED}✗${NC} $1"; }

echo -e "${BOLD}"
echo "  ╔══════════════════════════════════════╗"
echo "  ║       VPS DASHBOARD - DEPLOY         ║"
echo "  ║       Target: ${VPS_HOST}"
echo "  ╚══════════════════════════════════════╝"
echo -e "${NC}"

# ==========================================
print_step "[1/6] Building frontend..."
# ==========================================
cd "$PROJECT_DIR/frontend"
if npm run build 2>&1; then
  print_ok "Frontend built successfully"
else
  print_err "Frontend build failed!"
  echo "Press any key to exit..."; read -n 1; exit 1
fi
cd "$PROJECT_DIR"

# ==========================================
print_step "[2/6] Testing SSH connection..."
# ==========================================
if RESULT=$(vpsd_ssh "echo CONNECTION_SUCCESS" 2>&1) && echo "$RESULT" | grep -q "CONNECTION_SUCCESS"; then
  print_ok "SSH connection successful"
else
  print_err "Cannot connect to VPS"
  echo "$RESULT"
  echo "Press any key to exit..."; read -n 1; exit 1
fi

# ==========================================
print_step "[3/6] Setting up server..."
# ==========================================
vpsd_ssh "bash -s" <<'SETUP_EOF'
set -e
if ! command -v node >/dev/null 2>&1; then
  curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
  sudo apt install -y nodejs
fi
if ! command -v pm2 >/dev/null 2>&1; then
  sudo npm i -g pm2
fi
if ! command -v nginx >/dev/null 2>&1; then
  sudo apt update -qq
  sudo apt install -y nginx
  sudo systemctl enable nginx
  sudo systemctl start nginx
fi
echo "NODE:$(node -v) PM2:$(pm2 -v) NGINX:OK"
SETUP_EOF
print_ok "Server environment ready"

# ==========================================
print_step "[4/6] Uploading files..."
# ==========================================
vpsd_ssh "mkdir -p '$REMOTE_APP_DIR/backend/routes' '$REMOTE_APP_DIR/backend/services' '$REMOTE_APP_DIR/frontend'"

echo "  Backend..."
vpsd_scp    "backend/server.js"           "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
vpsd_scp    "backend/package.json"        "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
vpsd_scp    "backend/ecosystem.config.js" "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
vpsd_scp -r "backend/routes"              "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
vpsd_scp -r "backend/services"            "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/backend/"
print_ok "Backend uploaded"

echo "  Frontend..."
vpsd_scp -r "frontend/dist" "$VPS_USER@$VPS_HOST:$REMOTE_APP_DIR/frontend/"
print_ok "Frontend uploaded"

echo "  Nginx config..."
vpsd_scp "nginx-dashboard.conf" "$VPS_USER@$VPS_HOST:/tmp/vps-dashboard.conf"
print_ok "Nginx config uploaded"

# ==========================================
print_step "[5/6] Configuring server..."
# ==========================================
vpsd_ssh "bash -s" <<EOF
set -e
cd "$REMOTE_APP_DIR/backend"
npm install --production
sudo cp /tmp/vps-dashboard.conf /etc/nginx/sites-available/vps-dashboard
sudo ln -sf /etc/nginx/sites-available/vps-dashboard /etc/nginx/sites-enabled/vps-dashboard
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t
sudo systemctl reload nginx
echo CONFIG_DONE
EOF
print_ok "Dependencies installed & Nginx configured"

# ==========================================
print_step "[6/6] Starting backend..."
# ==========================================
vpsd_ssh "bash -s" <<EOF
set -e
cd "$REMOTE_APP_DIR/backend"
pm2 delete vps-dashboard-api 2>/dev/null || true
pm2 start ecosystem.config.js
pm2 save
pm2 list
EOF
print_ok "Backend running with PM2"

# ==========================================
echo ""
echo -e "${GREEN}${BOLD}"
echo "  ╔══════════════════════════════════════╗"
echo "  ║      DEPLOYMENT COMPLETE!            ║"
echo "  ╠══════════════════════════════════════╣"
echo "  ║  Dashboard: http://${VPS_HOST}"
echo "  ║  API:       http://${VPS_HOST}/health"
echo "  ╚══════════════════════════════════════╝"
echo -e "${NC}"

open "http://${VPS_HOST}" 2>/dev/null || true

echo "Press any key to close..."
read -n 1
