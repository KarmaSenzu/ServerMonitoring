#!/bin/bash
# ============================================
# Double-click untuk cek status VPS
# Credentials & host loaded from deploy.env via scripts/lib/load_env.sh
# ============================================

set -e
clear

PROJECT_DIR_LOCAL="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR_LOCAL"

# shellcheck source=scripts/lib/load_env.sh
source "$PROJECT_DIR_LOCAL/scripts/lib/load_env.sh"

GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${BOLD}"
echo "  ╔══════════════════════════════════════╗"
echo "  ║      VPS STATUS CHECK                ║"
echo "  ║      ${VPS_HOST}"
echo "  ╚══════════════════════════════════════╝"
echo -e "${NC}"

echo -e "${CYAN}Connecting...${NC}"

# Send the status check as a heredoc to bash -s on the remote.
vpsd_ssh "bash -s" <<'REMOTE_EOF'
echo "━━━ SYSTEM ━━━"
echo "  Hostname : $(hostname)"
echo "  Uptime   : $(uptime -p)"
echo ""
echo "━━━ CPU ━━━"
echo "  Load Avg : $(awk '{print $1, $2, $3}' /proc/loadavg)"
echo "  Cores    : $(nproc)"
echo ""
echo "━━━ MEMORY ━━━"
free -h | awk '/^Mem:/ {printf "  Total: %s | Used: %s | Free: %s\n", $2, $3, $4}'
echo ""
echo "━━━ DISK ━━━"
df -h / | awk 'NR==2 {printf "  Total: %s | Used: %s | Free: %s | Usage: %s\n", $2, $3, $4, $5}'
echo ""
echo "━━━ DOCKER ━━━"
if command -v docker >/dev/null 2>&1; then
  docker ps --format "  {{.Names}} | {{.Status}}" 2>/dev/null || echo "  Docker not reachable"
else
  echo "  Docker not available"
fi
echo ""
echo "━━━ PM2 ━━━"
if command -v pm2 >/dev/null 2>&1; then
  pm2 list 2>/dev/null || echo "  PM2 error"
else
  echo "  PM2 not installed"
fi
echo ""
echo "━━━ NGINX ━━━"
if systemctl is-active nginx >/dev/null 2>&1; then
  echo "  Status: running"
else
  echo "  Status: stopped/not installed"
fi
echo ""
echo "━━━ CLOUDFLARE TUNNEL ━━━"
if systemctl is-active cloudflared >/dev/null 2>&1; then
  echo "  Status: running"
else
  echo "  Status: stopped/not installed"
fi
echo ""
echo "━━━ NETWORK ━━━"
echo "  Public IP  : $(curl -s --max-time 5 ifconfig.me 2>/dev/null || echo unknown)"
echo "  Private IP : $(hostname -I 2>/dev/null | awk '{print $1}')"
echo ""
echo "STATUS_CHECK_DONE"
REMOTE_EOF

echo ""
echo -e "${GREEN}Status check complete.${NC}"
echo ""
echo "Press any key to close..."
read -n 1
