#!/bin/bash

# Setup Cloudflare Tunnel Routing via CLI
# This script fixes the HTTP/HTTPS issue with tunnel routing

set -e

# Cloudflare credentials — JANGAN hardcode di file ini.
# Set via env sebelum run script:
#   export CF_EMAIL="your@email"
#   export CF_API_KEY="your-token"
EMAIL="${CF_EMAIL:?CF_EMAIL harus di-set via env (export CF_EMAIL=...)}"
API_KEY="${CF_API_KEY:?CF_API_KEY harus di-set via env (export CF_API_KEY=...)}"
TUNNEL_NAME="${TUNNEL_NAME:-server-dmr}"
DOMAIN="devplay.online"
SUBDOMAIN="server-dmr"
SERVICE_URL="http://web:80"

echo "📋 Cloudflare Tunnel Routing Setup"
echo "=================================="
echo "Email: $EMAIL"
echo "Tunnel: $TUNNEL_NAME"
echo "Hostname: $SUBDOMAIN.$DOMAIN"
echo "Service: $SERVICE_URL"
echo ""

# Create cloudflared config directory
mkdir -p ~/.cloudflared

# Create proper config.yml
cat > ~/.cloudflared/config.yml << EOF
tunnel: $TUNNEL_NAME
credentials-file: ~/.cloudflared/credentials.json

ingress:
  - hostname: $SUBDOMAIN.$DOMAIN
    service: $SERVICE_URL
  - service: http_status:404
EOF

echo "✅ Config file created at ~/.cloudflared/config.yml"
echo ""
echo "📌 Manual steps needed:"
echo "1. Login to Cloudflare dashboard: https://one.dash.cloudflare.com"
echo "2. Networks → Tunnels → $TUNNEL_NAME"
echo "3. Public Hostname tab"
echo "4. EDIT the existing route (not add new)"
echo "5. Change:"
echo "   - Protocol: HTTP (not HTTPS)"
echo "   - Service URL: http://web:80 (not https://web:80)"
echo "6. Save"
echo ""
echo "⏱️  Wait 30 seconds for DNS to propagate"
echo "🌐 Test: https://$SUBDOMAIN.$DOMAIN"
echo ""
echo "After manual fix in Cloudflare dashboard, tunnel will work!"
