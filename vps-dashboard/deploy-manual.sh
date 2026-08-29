#!/bin/bash
# ============================================
# VPS Dashboard - Manual Step-by-Step Deploy
# Use this only if deploy.sh fails. Read deploy.env for connection info.
# ============================================

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# shellcheck source=scripts/lib/load_env.sh
source "$SCRIPT_DIR/scripts/lib/load_env.sh"

echo "=========================================="
echo "  VPS Dashboard - Manual Deploy Guide"
echo "=========================================="
echo ""
echo "Jalankan perintah berikut SATU PER SATU."
echo "Gunakan SSH key (lihat scripts/setup_ssh_key.sh)."
echo ""
echo "=========================================="
echo "STEP 1: Build frontend (di local)"
echo "=========================================="
echo "cd \"$PROJECT_DIR/frontend\""
echo "npm run build"
echo ""
echo "=========================================="
echo "STEP 2: SSH ke VPS"
echo "=========================================="
if [ -n "${VPS_SSH_KEY:-}" ] && [ -f "${VPS_SSH_KEY:-/dev/null}" ]; then
  echo "ssh -i \"$VPS_SSH_KEY\" ${VPS_USER}@${VPS_HOST}"
else
  echo "ssh ${VPS_USER}@${VPS_HOST}"
  echo "# (set VPS_SSH_KEY in deploy.env to use key auth — recommended)"
fi
echo ""
echo "=========================================="
echo "STEP 3: Setup VPS (jalankan di VPS)"
echo "=========================================="
cat << 'EOF'
sudo apt update && sudo apt upgrade -y
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs nginx
sudo npm install -g pm2

mkdir -p /home/ubuntu/vps-dashboard/backend/routes
mkdir -p /home/ubuntu/vps-dashboard/backend/services
mkdir -p /home/ubuntu/vps-dashboard/frontend
EOF
echo ""
echo "=========================================="
echo "STEP 4: Upload files (di local, terminal baru)"
echo "=========================================="
SCP_AUTH=""
if [ -n "${VPS_SSH_KEY:-}" ] && [ -f "${VPS_SSH_KEY:-/dev/null}" ]; then
  SCP_AUTH="-i \"$VPS_SSH_KEY\" "
fi
echo "# Upload backend"
echo "scp ${SCP_AUTH}\"$PROJECT_DIR/backend/server.js\"           ${VPS_USER}@${VPS_HOST}:${REMOTE_APP_DIR}/backend/"
echo "scp ${SCP_AUTH}\"$PROJECT_DIR/backend/package.json\"        ${VPS_USER}@${VPS_HOST}:${REMOTE_APP_DIR}/backend/"
echo "scp ${SCP_AUTH}\"$PROJECT_DIR/backend/ecosystem.config.js\" ${VPS_USER}@${VPS_HOST}:${REMOTE_APP_DIR}/backend/"
echo "scp ${SCP_AUTH}-r \"$PROJECT_DIR/backend/routes\"           ${VPS_USER}@${VPS_HOST}:${REMOTE_APP_DIR}/backend/"
echo "scp ${SCP_AUTH}-r \"$PROJECT_DIR/backend/services\"         ${VPS_USER}@${VPS_HOST}:${REMOTE_APP_DIR}/backend/"
echo ""
echo "# Upload frontend build"
echo "scp ${SCP_AUTH}-r \"$PROJECT_DIR/frontend/dist\"             ${VPS_USER}@${VPS_HOST}:${REMOTE_APP_DIR}/frontend/"
echo ""
echo "# Upload nginx config"
echo "scp ${SCP_AUTH}\"$PROJECT_DIR/nginx-dashboard.conf\"          ${VPS_USER}@${VPS_HOST}:/tmp/vps-dashboard.conf"
echo ""
echo "=========================================="
echo "STEP 5: Configure di VPS (SSH ke VPS)"
echo "=========================================="
cat << EOF
cd ${REMOTE_APP_DIR}/backend
npm install --production

sudo cp /tmp/vps-dashboard.conf /etc/nginx/sites-available/vps-dashboard
sudo ln -sf /etc/nginx/sites-available/vps-dashboard /etc/nginx/sites-enabled/vps-dashboard
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t
sudo systemctl reload nginx

pm2 start ecosystem.config.js
pm2 save
pm2 startup

echo "Done! Buka http://${VPS_HOST} di browser"
EOF
