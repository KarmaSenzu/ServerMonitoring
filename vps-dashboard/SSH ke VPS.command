#!/bin/bash
# ============================================
# Double-click untuk SSH langsung ke VPS
# Credentials & host loaded from deploy.env via scripts/lib/load_env.sh
# ============================================

set -e
clear

# Resolve project dir even when launched from Finder.
PROJECT_DIR_LOCAL="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJECT_DIR_LOCAL"

# shellcheck source=scripts/lib/load_env.sh
source "$PROJECT_DIR_LOCAL/scripts/lib/load_env.sh"

echo "╔══════════════════════════════════════╗"
echo "║    Connecting to VPS...              ║"
echo "║    ${VPS_USER}@${VPS_HOST}"
echo "╚══════════════════════════════════════╝"
echo ""

# Use vpsd_ssh helper with no command → interactive shell.
# vpsd_ssh handles key-vs-password auth automatically.
vpsd_ssh

echo ""
echo "SSH session ended."
echo "Press any key to close..."
read -n 1
