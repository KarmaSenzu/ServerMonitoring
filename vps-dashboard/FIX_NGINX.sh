#!/bin/bash
# Fix Nginx 500 error - permission issue
# Credentials & host loaded from deploy.env

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# shellcheck source=scripts/lib/load_env.sh
source "$SCRIPT_DIR/scripts/lib/load_env.sh"

cd "$PROJECT_DIR"

# Run the remote fix via a heredoc — no need to scp a temp file.
vpsd_ssh "bash -s" <<EOF
set -e

echo "=== Checking files ==="
ls -la "$REMOTE_APP_DIR/frontend/dist/"
echo ""

echo "=== Fixing permissions ==="
chmod 755 /home/ubuntu
chmod -R 755 "$REMOTE_APP_DIR"
echo "Permissions fixed"

echo ""
echo "=== Checking nginx error log ==="
sudo tail -20 /var/log/nginx/error.log
echo ""

echo "=== Reloading nginx ==="
sudo nginx -t 2>&1
sudo systemctl reload nginx
echo "FIX_DONE"
EOF

echo ""
echo "Fix applied. Testing..."
curl -s -o /dev/null -w "HTTP Status: %{http_code}\n" "http://${VPS_HOST}/"
echo "Done!"
