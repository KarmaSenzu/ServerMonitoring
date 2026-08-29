#!/bin/bash
# ============================================
# VPS Dashboard - Server Setup Script
# Run this ON the VPS after SSH-ing in
# ============================================

set -e

echo "=========================================="
echo "  VPS Dashboard - Server Setup"
echo "=========================================="

# Update system
echo "[1/6] Updating system packages..."
sudo apt update && sudo apt upgrade -y

# Install Node.js 20 LTS
echo "[2/6] Installing Node.js 20 LTS..."
if ! command -v node &> /dev/null; then
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt install -y nodejs
else
    echo "  Node.js already installed: $(node --version)"
fi

# Install PM2
echo "[3/6] Installing PM2..."
if ! command -v pm2 &> /dev/null; then
    sudo npm install -g pm2
    pm2 startup systemd -u ubuntu --hp /home/ubuntu
else
    echo "  PM2 already installed: $(pm2 --version)"
fi

# Install Nginx
echo "[4/6] Installing Nginx..."
if ! command -v nginx &> /dev/null; then
    sudo apt install -y nginx
    sudo systemctl enable nginx
    sudo systemctl start nginx
else
    echo "  Nginx already installed"
fi

# Install Docker (if not present)
echo "[5/6] Checking Docker..."
if ! command -v docker &> /dev/null; then
    echo "  Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    sudo usermod -aG docker ubuntu
    echo "  Docker installed. You may need to re-login for group changes."
else
    echo "  Docker already installed: $(docker --version)"
fi

# Create project directory
echo "[6/6] Creating project directory..."
mkdir -p /home/ubuntu/vps-dashboard

echo ""
echo "=========================================="
echo "  Setup complete!"
echo "  Node: $(node --version)"
echo "  NPM: $(npm --version)"
echo "  PM2: $(pm2 --version)"
echo "  Nginx: $(nginx -v 2>&1)"
echo "=========================================="
echo ""
echo "Next step: Run deploy.sh from your local machine"
