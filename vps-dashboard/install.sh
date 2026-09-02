#!/bin/bash
# VPS Dashboard Installation Script
# Installs vpsdash as a single binary with optional systemd service

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
BINARY_NAME="vpsdash"
INSTALL_DIR="/usr/local/bin"
DATA_DIR="/var/lib/vpsdash"
SERVICE_USER="vpsdash"
GITHUB_REPO="yourusername/monitoring-server"  # TODO: Update with actual repo

echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   VPS Dashboard Installation Script   ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════╝${NC}"
echo ""

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo -e "${RED}❌ Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

case $OS in
    linux|darwin) ;;
    *)
        echo -e "${RED}❌ Unsupported OS: $OS${NC}"
        echo -e "${YELLOW}Supported: Linux, macOS${NC}"
        exit 1
        ;;
esac

echo -e "${BLUE}Detected:${NC} $OS/$ARCH"
echo ""

# Check for required commands
check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo -e "${RED}❌ Required command not found: $1${NC}"
        exit 1
    fi
}

check_command curl
check_command tar

# Get latest version from GitHub
echo -e "${YELLOW}Fetching latest release...${NC}"
LATEST_VERSION=$(curl -sL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)

if [ -z "$LATEST_VERSION" ]; then
    echo -e "${RED}❌ Failed to fetch latest version${NC}"
    exit 1
fi

echo -e "${GREEN}Latest version: ${LATEST_VERSION}${NC}"

# Download URL
DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_VERSION}/vpsdash-${OS}-${ARCH}.tar.gz"

echo -e "${YELLOW}Downloading from: ${DOWNLOAD_URL}${NC}"

# Download and extract
TMP_DIR=$(mktemp -d)
trap "rm -rf ${TMP_DIR}" EXIT

curl -sL "$DOWNLOAD_URL" | tar xz -C "$TMP_DIR"

if [ ! -f "${TMP_DIR}/vpsdash-${OS}-${ARCH}" ]; then
    echo -e "${RED}❌ Binary not found in archive${NC}"
    exit 1
fi

# Install binary
echo -e "${YELLOW}Installing binary to ${INSTALL_DIR}...${NC}"

if [ "$EUID" -ne 0 ]; then
    echo -e "${YELLOW}Requesting sudo access...${NC}"
    sudo mv "${TMP_DIR}/vpsdash-${OS}-${ARCH}" "${INSTALL_DIR}/${BINARY_NAME}"
    sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
else
    mv "${TMP_DIR}/vpsdash-${OS}-${ARCH}" "${INSTALL_DIR}/${BINARY_NAME}"
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo -e "${GREEN}✓ Binary installed to ${INSTALL_DIR}/${BINARY_NAME}${NC}"

# Verify installation
VERSION_OUTPUT=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>&1 || echo "version check failed")
echo -e "${BLUE}Installed version: ${VERSION_OUTPUT}${NC}"
echo ""

# Ask about systemd service (Linux only)
if [ "$OS" = "linux" ] && command -v systemctl &> /dev/null; then
    echo -e "${BLUE}═══════════════════════════════════════${NC}"
    echo -e "${BLUE}Setup systemd service?${NC}"
    echo -e "${YELLOW}This will run vpsdash as a system service${NC}"
    read -p "Install systemd service? [y/N]: " -r
    echo ""
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        # Create service user
        if ! id "$SERVICE_USER" &>/dev/null; then
            echo -e "${YELLOW}Creating service user: ${SERVICE_USER}...${NC}"
            sudo useradd --system --no-create-home --shell /bin/false "$SERVICE_USER"
        fi
        
        # Create data directory
        echo -e "${YELLOW}Creating data directory: ${DATA_DIR}...${NC}"
        sudo mkdir -p "$DATA_DIR"
        sudo chown "${SERVICE_USER}:${SERVICE_USER}" "$DATA_DIR"
        
        # Generate random JWT secret
        JWT_SECRET=$(openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64)
        
        # Generate random admin password (shown ONCE, user must save it)
        ADMIN_PASSWORD=$(openssl rand -base64 12 2>/dev/null || head -c 12 /dev/urandom | base64)
        
        # Create systemd service file
        echo -e "${YELLOW}Creating systemd service...${NC}"
        sudo tee /etc/systemd/system/vpsdash.service > /dev/null <<EOF
[Unit]
Description=VPS Dashboard - Server Monitoring and Management
Documentation=https://github.com/${GITHUB_REPO}
After=network.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
ExecStart=${INSTALL_DIR}/${BINARY_NAME}
WorkingDirectory=${DATA_DIR}
Restart=on-failure
RestartSec=5s

# Environment
Environment="ENV=production"
Environment="HTTP_ADDR=:3001"
Environment="DB_PATH=${DATA_DIR}/vpsdash.db"
Environment="JWT_SECRET=${JWT_SECRET}"
Environment="BOOTSTRAP_ADMIN_USERNAME=admin"
Environment="BOOTSTRAP_ADMIN_PASSWORD=${ADMIN_PASSWORD}"
Environment="LOG_LEVEL=info"
Environment="BACKUP_DIR=${DATA_DIR}/backups"
Environment="SSH_KEYS_DIR=${DATA_DIR}/ssh-keys"

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${DATA_DIR}

[Install]
WantedBy=multi-user.target
EOF

        # Reload systemd
        sudo systemctl daemon-reload
        sudo systemctl enable vpsdash
        
        echo -e "${GREEN}✓ Systemd service created${NC}"
        echo ""
        echo -e "${RED}═══════════════════════════════════════════════════════${NC}"
        echo -e "${RED}  ⚠️  SAVE THESE CREDENTIALS — shown only once!  ⚠️${NC}"
        echo -e "${RED}═══════════════════════════════════════════════════════${NC}"
        echo ""
        echo -e "${BLUE}Admin Username:${NC} admin"
        echo -e "${BLUE}Admin Password:${NC} ${ADMIN_PASSWORD}"
        echo -e "${BLUE}Dashboard URL:${NC}  http://localhost:3001"
        echo ""
        echo -e "${YELLOW}To change the password later:${NC}"
        echo -e "  1. Edit /etc/systemd/system/vpsdash.service"
        echo -e "  2. Update BOOTSTRAP_ADMIN_PASSWORD (only used on first boot)"
        echo -e "  3. Or change it from the UI: Settings → Users"
        echo ""
        echo -e "${YELLOW}Other config:${NC}"
        echo -e "  - CORS_ORIGINS: edit service file for your domain"
        echo -e "  - DATABASE: edit ./data/database.json to switch to PostgreSQL/Supabase"
        echo ""
        echo -e "${BLUE}Start service:${NC}    sudo systemctl start vpsdash"
        echo -e "${BLUE}View status:${NC}     sudo systemctl status vpsdash"
        echo -e "${BLUE}View logs:${NC}       sudo journalctl -u vpsdash -f"
    fi
fi

# Final instructions
echo ""
echo -e "${GREEN}╔════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║     Installation Complete! 🎉         ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════╝${NC}"
echo ""
echo -e "${BLUE}Quick Start:${NC}"
echo ""

if systemctl is-enabled vpsdash &>/dev/null; then
    echo -e "1. Configure the service:"
    echo -e "   ${YELLOW}sudo nano /etc/systemd/system/vpsdash.service${NC}"
    echo ""
    echo -e "2. Start the service:"
    echo -e "   ${YELLOW}sudo systemctl start vpsdash${NC}"
    echo ""
    echo -e "3. Access dashboard:"
    echo -e "   ${YELLOW}http://localhost:3001${NC}"
    echo -e "   Default credentials: admin / changeme123"
else
    echo -e "1. Set required environment variable:"
    echo -e "   ${YELLOW}export JWT_SECRET=\$(openssl rand -base64 32)${NC}"
    echo ""
    echo -e "2. Run vpsdash:"
    echo -e "   ${YELLOW}${BINARY_NAME}${NC}"
    echo ""
    echo -e "3. Access dashboard:"
    echo -e "   ${YELLOW}http://localhost:3001${NC}"
fi

echo ""
echo -e "${BLUE}Documentation:${NC} https://github.com/${GITHUB_REPO}"
echo ""
