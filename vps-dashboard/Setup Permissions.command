#!/bin/bash
# ============================================
# Double-click ini PERTAMA KALI untuk set
# permissions semua shortcut files
# ============================================

clear

DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

echo "Setting permissions for all .command files..."
echo ""

chmod +x "$DIR/Deploy ke VPS.command"
echo "✓ Deploy ke VPS.command"

chmod +x "$DIR/Start Dev Local.command"
echo "✓ Start Dev Local.command"

chmod +x "$DIR/SSH ke VPS.command"
echo "✓ SSH ke VPS.command"

chmod +x "$DIR/Cek Status VPS.command"
echo "✓ Cek Status VPS.command"

chmod +x "$DIR/deploy.sh"
echo "✓ deploy.sh"

chmod +x "$DIR/DEPLOY_UPDATE.sh" 2>/dev/null && echo "✓ DEPLOY_UPDATE.sh" || true
chmod +x "$DIR/FIX_NGINX.sh"     2>/dev/null && echo "✓ FIX_NGINX.sh"     || true
chmod +x "$DIR/FIX_TUNNEL.sh"    2>/dev/null && echo "✓ FIX_TUNNEL.sh"    || true
chmod +x "$DIR/JALANKAN_INI.sh"  2>/dev/null && echo "✓ JALANKAN_INI.sh"  || true
chmod +x "$DIR/deploy-manual.sh" 2>/dev/null && echo "✓ deploy-manual.sh" || true
chmod +x "$DIR/scripts/lib/load_env.sh"  2>/dev/null && echo "✓ scripts/lib/load_env.sh"  || true
chmod +x "$DIR/scripts/setup_ssh_key.sh" 2>/dev/null && echo "✓ scripts/setup_ssh_key.sh" || true

# deploy.env contains secrets — restrict to owner only
if [ -f "$DIR/deploy.env" ]; then
  chmod 600 "$DIR/deploy.env"
  echo "✓ deploy.env (mode 600)"
fi

chmod +x "$DIR/Setup Permissions.command"
echo "✓ Setup Permissions.command"

echo ""
echo "═══════════════════════════════════════"
echo "  All permissions set! ✓"
echo "  Sekarang kamu bisa double-click"
echo "  file .command lainnya dari Finder."
echo "═══════════════════════════════════════"
echo ""
echo "Press any key to close..."
read -n 1
