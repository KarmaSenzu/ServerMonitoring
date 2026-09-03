#!/bin/bash
# vpsdash — Smart wrapper for VPS Dashboard
# Usage:
#   vpsdash              Start server (background)
#   vpsdash start        Same as above
#   vpsdash stop         Stop server
#   vpsdash restart      Restart server
#   vpsdash status       Check if running (PID, uptime, memory)
#   vpsdash logs         Tail logs (Ctrl+C to exit)
#   vpsdash config       Show config & credentials
#   vpsdash update       Download & install latest version from GitHub
#   vpsdash --version    Show version
#   vpsdash help         Show full help (commands + examples)
#
# First run auto-generates config with random secrets.
# Config: ~/.vpsdash/config.env
# Logs:   ~/.vpsdash/vpsdash.log
# PID:    /tmp/vpsdash.pid

set -e

# === Configuration ===
VPSDASH_HOME="$HOME/.vpsdash"
CONFIG_FILE="$VPSDASH_HOME/config.env"
LOG_FILE="$VPSDASH_HOME/vpsdash.log"
PID_FILE="/tmp/vpsdash.pid"
BINARY_NAME="vpsdashd"  # The actual Go binary (renamed)

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# === Helper functions ===

info()  { echo -e "${GREEN}$1${NC}"; }
warn()  { echo -e "${YELLOW}$1${NC}"; }
error() { echo -e "${RED}$1${NC}"; }

# Check if binary exists
find_binary() {
    # Check common locations for vpsdashd (the Go binary)
    for path in "/usr/local/bin/$BINARY_NAME" "/usr/bin/$BINARY_NAME" "$HOME/.local/bin/$BINARY_NAME" "./$BINARY_NAME"; do
        if [ -x "$path" ]; then
            echo "$path"
            return 0
        fi
    done
    # Check if there's a 'vpsdash' binary in PATH that is NOT this wrapper script
    if command -v vpsdash &>/dev/null; then
        VPSDASH_PATH=$(command -v vpsdash)
        # Make sure it's not the wrapper script itself
        if [ "$VPSDASH_PATH" != "$0" ] && [ "$VPSDASH_PATH" != "/usr/local/bin/vpsdash" ]; then
            echo "$VPSDASH_PATH"
            return 0
        fi
    fi
    return 1
}

# Generate config on first run
ensure_config() {
    if [ ! -f "$CONFIG_FILE" ]; then
        mkdir -p "$VPSDASH_HOME"
        
        # Generate secrets
        JWT_SECRET=$(openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64)
        ADMIN_PASS=$(openssl rand -base64 12 2>/dev/null || head -c 12 /dev/urandom | base64)
        
        cat > "$CONFIG_FILE" <<EOF
# VPS Dashboard Configuration (auto-generated)
# Generated: $(date)

# Required
JWT_SECRET=$JWT_SECRET

# Admin credentials (used only on first boot to create admin user)
BOOTSTRAP_ADMIN_USERNAME=admin
BOOTSTRAP_ADMIN_PASSWORD=$ADMIN_PASS

# Server settings
ENV=production
HTTP_ADDR=:3001
LOG_LEVEL=info
CORS_ORIGINS=http://localhost:3001,http://127.0.0.1:3001

# Database (SQLite default, switch via: vpsdash db)
DB_PATH=$VPSDASH_HOME/vpsdash.db
VPSDASH_DB_CONFIG=$VPSDASH_HOME/database.json

# Data directories
BACKUP_DIR=$VPSDASH_HOME/backups
SSH_KEYS_DIR=$VPSDASH_HOME/ssh-keys

# Monitoring intervals
REMOTE_POLL_INTERVAL=60s
SYSTEM_TICK_INTERVAL=30s
HEALTHCHECK_INTERVAL=60s
EOF
        
        chmod 600 "$CONFIG_FILE"
        
        echo ""
        info "✓ First-time setup complete!"
        echo ""
        echo -e "${RED}═══════════════════════════════════════════${NC}"
        echo -e "${RED}  ⚠️  SAVE THESE CREDENTIALS  ⚠️${NC}"
        echo -e "${RED}═══════════════════════════════════════════${NC}"
        echo ""
        echo -e "${CYAN}Username:${NC} admin"
        echo -e "${CYAN}Password:${NC} $ADMIN_PASS"
        echo -e "${CYAN}Dashboard:${NC} http://localhost:3001"
        echo ""
        echo -e "${YELLOW}Config file: $CONFIG_FILE${NC}"
        echo -e "${YELLOW}Edit it to change settings, then restart: vpsdash restart${NC}"
        echo ""
    fi
}

# Load config
load_config() {
    ensure_config
    set -a
    source "$CONFIG_FILE"
    set +a
}

# Check if running
is_running() {
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE" 2>/dev/null || echo "")
        if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
            return 0
        fi
    fi
    return 1
}

# === Commands ===

cmd_start() {
    if is_running; then
        warn "VPS Dashboard is already running (PID: $(cat $PID_FILE))"
        echo "Use: vpsdash restart  to restart"
        echo "Use: vpsdash stop     to stop"
        return 0
    fi
    
    load_config
    
    BINARY=$(find_binary)
    if [ -z "$BINARY" ]; then
        error "❌ Binary 'vpsdashd' or 'vpsdash' not found!"
        echo ""
        echo "Install options:"
        echo "  1. Download: https://github.com/KarmaSenzu/ServerMonitoring/releases"
        echo "  2. Or build from source:"
        echo "     git clone https://github.com/KarmaSenzu/ServerMonitoring.git"
        echo "     cd ServerMonitoring/vps-dashboard && ./scripts/build.sh"
        exit 1
    fi
    
    # Determine data dir from config (or use default)
    DATA_DIR=$(dirname "$DB_PATH")
    mkdir -p "$DATA_DIR" "$BACKUP_DIR" "$SSH_KEYS_DIR"
    
    echo -e "${CYAN}Starting VPS Dashboard...${NC}"
    
    # Start in background with proper environment
    export ENV="${ENV:-production}"
    export HTTP_ADDR="${HTTP_ADDR:-:3001}"
    export DB_PATH="${DB_PATH:-$VPSDASH_HOME/vpsdash.db}"
    export VPSDASH_DB_CONFIG="${VPSDASH_DB_CONFIG:-$VPSDASH_HOME/database.json}"
    export JWT_SECRET="${JWT_SECRET}"
    export BOOTSTRAP_ADMIN_USERNAME="${BOOTSTRAP_ADMIN_USERNAME:-admin}"
    export BOOTSTRAP_ADMIN_PASSWORD="${BOOTSTRAP_ADMIN_PASSWORD}"
    export LOG_LEVEL="${LOG_LEVEL:-info}"
    export CORS_ORIGINS="${CORS_ORIGINS:-http://localhost:3001,http://127.0.0.1:3001,http://0.0.0.0:3001}"
    export BACKUP_DIR="${BACKUP_DIR:-$VPSDASH_HOME/backups}"
    export SSH_KEYS_DIR="${SSH_KEYS_DIR:-$VPSDASH_HOME/ssh-keys}"
    export SUPABASE_DB_PASSWORD="${SUPABASE_DB_PASSWORD:-}"
    export POSTGRES_DB_PASSWORD="${POSTGRES_DB_PASSWORD:-}"
    
    # Start in background
    nohup "$BINARY" > "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
    sleep 2
    
    if is_running; then
        info "✓ VPS Dashboard started successfully!"
        echo ""
        echo -e "${CYAN}PID:${NC}       $(cat $PID_FILE)"
        echo -e "${CYAN}Dashboard:${NC} http://localhost:3001"
        echo -e "${CYAN}Logs:${NC}     vpsdash logs"
        echo -e "${CYAN}Stop:${NC}     vpsdash stop"
        echo ""
        warn "Server runs in background. You can close this terminal."
    else
        error "❌ Failed to start. Check logs:"
        echo ""
        tail -30 "$LOG_FILE"
        rm -f "$PID_FILE"
        exit 1
    fi
}

cmd_stop() {
    if ! is_running; then
        warn "VPS Dashboard is not running"
        rm -f "$PID_FILE"
        return 0
    fi
    
    PID=$(cat "$PID_FILE")
    echo -e "${CYAN}Stopping VPS Dashboard (PID: $PID)...${NC}"
    kill "$PID" 2>/dev/null || true
    
    # Wait for process to exit
    for i in $(seq 1 10); do
        if ! kill -0 "$PID" 2>/dev/null; then
            break
        fi
        sleep 0.5
    done
    
    # Force kill if still running
    if kill -0 "$PID" 2>/dev/null; then
        warn "Process didn't stop gracefully, force killing..."
        kill -9 "$PID" 2>/dev/null || true
    fi
    
    rm -f "$PID_FILE"
    info "✓ VPS Dashboard stopped"
}

cmd_restart() {
    cmd_stop
    sleep 1
    cmd_start
}

cmd_status() {
    if is_running; then
        PID=$(cat "$PID_FILE")
        info "✓ VPS Dashboard is running"
        echo -e "${CYAN}PID:${NC}        $PID"
        echo -e "${CYAN}Dashboard:${NC}  http://localhost:3001"
        echo -e "${CYAN}Uptime:${NC}     $(ps -o etime= -p $PID | tr -d ' ')"
        echo -e "${CYAN}Memory:${NC}     $(ps -o rss= -p $PID | awk '{printf "%.1f MB\n", $1/1024}')"
        echo -e "${CYAN}Log file:${NC}   $LOG_FILE"
    else
        warn "VPS Dashboard is not running"
        echo "Start with: vpsdash start"
    fi
}

cmd_logs() {
    if [ ! -f "$LOG_FILE" ]; then
        warn "No log file found. Start the server first: vpsdash start"
        exit 1
    fi
    
    echo -e "${CYAN}Tailing logs (Ctrl+C to exit)...${NC}"
    echo ""
    tail -f "$LOG_FILE"
}

cmd_config() {
    ensure_config
    echo -e "${CYAN}VPS Dashboard Configuration${NC}"
    echo ""
    echo -e "${CYAN}Config file:${NC} $CONFIG_FILE"
    echo -e "${CYAN}Log file:${NC}   $LOG_FILE"
    echo -e "${CYAN}Database:${NC}   $DB_PATH"
    echo ""
    echo -e "${CYAN}Admin credentials (from config):${NC}"
    echo "  Username: $BOOTSTRAP_ADMIN_USERNAME"
    echo "  Password: $BOOTSTRAP_ADMIN_PASSWORD"
    echo ""
    echo -e "${YELLOW}To change settings:${NC}"
    echo "  1. Edit: nano $CONFIG_FILE"
    echo "  2. Restart: vpsdash restart"
}

cmd_version() {
    BINARY=$(find_binary)
    if [ -n "$BINARY" ]; then
        "$BINARY" --version 2>/dev/null || echo "vpsdash (binary: $BINARY)"
    else
        echo "vpsdash wrapper (binary not found - run: vpsdash update)"
    fi
}

# === UPDATE COMMAND ===
# Downloads the latest release from GitHub, replaces the binary, and
# restarts the server if it was running. Preserves config & data.
cmd_update() {
    GITHUB_REPO="KarmaSenzu/ServerMonitoring"
    
    echo -e "${CYAN}Checking for latest release...${NC}"
    
    # Get latest version from GitHub API
    LATEST_VERSION=$(curl -sL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | cut -d'"' -f4)
    
    if [ -z "$LATEST_VERSION" ]; then
        error "❌ Failed to fetch latest version from GitHub"
        echo ""
        echo "Possible causes:"
        echo "  - GitHub API rate limit (try again later)"
        echo "  - No releases published yet"
        echo "  - Network issues"
        echo ""
        echo "Manual download: https://github.com/${GITHUB_REPO}/releases"
        exit 1
    fi
    
    # Get current version
    BINARY=$(find_binary)
    CURRENT_VERSION=""
    if [ -n "$BINARY" ]; then
        CURRENT_VERSION=$("$BINARY" --version 2>/dev/null | head -1 | awk '{print $3}')
    fi
    
    echo -e "${CYAN}Current version:${NC} ${CURRENT_VERSION:-unknown}"
    echo -e "${CYAN}Latest version:${NC}  ${LATEST_VERSION}"
    
    if [ "$CURRENT_VERSION" = "$LATEST_VERSION" ]; then
        info "✓ Already up to date!"
        exit 0
    fi
    
    echo ""
    echo -e "${YELLOW}Updating to ${LATEST_VERSION}...${NC}"
    
    # Detect OS and architecture
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    case $ARCH in
        x86_64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *)
            error "❌ Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac
    
    # Download URL
    DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_VERSION}/vpsdash-${OS}-${ARCH}.tar.gz"
    
    echo -e "${CYAN}Downloading ${OS}/${ARCH} binary...${NC}"
    
    # Download to temp directory
    TMP_DIR=$(mktemp -d)
    trap "rm -rf ${TMP_DIR}" EXIT
    
    curl -sSL "$DOWNLOAD_URL" | tar xz -C "$TMP_DIR"
    
    BINARY_FILE="${TMP_DIR}/vpsdash-${OS}-${ARCH}"
    if [ ! -f "$BINARY_FILE" ]; then
        error "❌ Binary not found in archive"
        echo "Expected: vpsdash-${OS}-${ARCH}"
        echo "Download URL: $DOWNLOAD_URL"
        exit 1
    fi
    
    # Determine install location
    INSTALL_DIR="/usr/local/bin"
    INSTALL_PATH="${INSTALL_DIR}/${BINARY_NAME}"
    
    # Also update the wrapper script itself
    WRAPPER_URL="https://raw.githubusercontent.com/${GITHUB_REPO}/main/vps-dashboard/vpsdash-wrapper.sh"
    echo -e "${CYAN}Installing binary to ${INSTALL_PATH}...${NC}"
    
    WAS_RUNNING=false
    if is_running; then
        WAS_RUNNING=true
        echo -e "${YELLOW}Stopping running server...${NC}"
        cmd_stop
    fi
    
    # Install binary
    if [ "$EUID" -ne 0 ]; then
        sudo mv "$BINARY_FILE" "$INSTALL_PATH"
        sudo chmod +x "$INSTALL_PATH"
    else
        mv "$BINARY_FILE" "$INSTALL_PATH"
        chmod +x "$INSTALL_PATH"
    fi
    
    # Also update wrapper script
    echo -e "${CYAN}Updating wrapper script...${NC}"
    WRAPPER_PATH="${INSTALL_DIR}/vpsdash"
    if [ "$EUID" -ne 0 ]; then
        sudo curl -sSL "$WRAPPER_URL" -o "$WRAPPER_PATH" 2>/dev/null || true
        sudo chmod +x "$WRAPPER_PATH" 2>/dev/null || true
    else
        curl -sSL "$WRAPPER_URL" -o "$WRAPPER_PATH" 2>/dev/null || true
        chmod +x "$WRAPPER_PATH" 2>/dev/null || true
    fi
    
    echo ""
    info "✓ Update complete!"
    echo ""
    echo -e "${CYAN}Version:${NC}    ${LATEST_VERSION}"
    echo -e "${CYAN}Binary:${NC}     ${INSTALL_PATH}"
    echo -e "${CYAN}Wrapper:${NC}    ${WRAPPER_PATH}"
    echo -e "${CYAN}Config:${NC}     ${CONFIG_FILE} (preserved)"
    echo -e "${CYAN}Database:${NC}   ${VPSDASH_HOME}/vpsdash.db (preserved)"
    echo ""
    
    # Restart if it was running
    if [ "$WAS_RUNNING" = true ]; then
        echo -e "${YELLOW}Restarting server...${NC}"
        cmd_start
    else
        warn "Server is stopped. Run 'vpsdash start' to start."
    fi
}

# === DATABASE SETUP COMMAND ===
# Interactive wizard to switch the database backend (SQLite/PostgreSQL/Supabase).
# Writes database.json and stores the password as an env-var reference,
# so no plaintext password lands in the config file.
cmd_db() {
    DB_CONFIG_FILE="$VPSDASH_HOME/database.json"
    mkdir -p "$VPSDASH_HOME"

    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║        Database Connection Setup Wizard          ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════╝${NC}"
    echo ""

    # Show current config if exists
    if [ -f "$DB_CONFIG_FILE" ]; then
        echo -e "${YELLOW}Current database config:${NC}"
        cat "$DB_CONFIG_FILE" | grep -v password | sed 's/^/  /'
        echo ""
    fi

    # Step 1: choose database type
    echo -e "${GREEN}Step 1/4 — Choose database type:${NC}"
    echo "  1) SQLite (embedded, no setup)"
    echo "  2) PostgreSQL (self-hosted)"
    echo "  3) Supabase (managed PostgreSQL)"
    echo ""
    read -p "Choice [1-3]: " DB_CHOICE
    echo ""

    case "$DB_CHOICE" in
        1|"")
            echo -e "${CYAN}Configuring SQLite (default)...${NC}"
            cat > "$DB_CONFIG_FILE" <<EOF
{
  "type": "sqlite",
  "sqlite": {
    "path": "$VPSDASH_HOME/vpsdash.db"
  }
}
EOF
            info "✓ SQLite configured."
            echo ""
            ;;
        2)
            cmd_db_pg "postgres"
            ;;
        3)
            cmd_db_pg "supabase"
            ;;
        *)
            error "Invalid choice: $DB_CHOICE"
            exit 1
            ;;
    esac

    echo -e "${YELLOW}Database config written to: $DB_CONFIG_FILE${NC}"
    echo ""
    echo -e "${YELLOW}Restarting server to apply changes...${NC}"
    if is_running; then
        cmd_restart
    else
        warn "Server is not running. Start it with: vpsdash start"
    fi
}

# Helper: prompt for PostgreSQL/Supabase connection details and write config.
cmd_db_pg() {
    local DB_TYPE="$1"  # "postgres" or "supabase"
    local LABEL
    if [ "$DB_TYPE" = "supabase" ]; then
        LABEL="Supabase"
    else
        LABEL="PostgreSQL"
    fi

    echo -e "${GREEN}Step 2/4 — $LABEL connection details:${NC}"
    echo ""
    read -p "Host (e.g. aws-0-ap-southeast-1.pooler.supabase.com): " DB_HOST
    read -p "Port [5432]: " DB_PORT
    DB_PORT=${DB_PORT:-5432}
    read -p "Database name [postgres]: " DB_NAME
    DB_NAME=${DB_NAME:-postgres}
    read -p "Username (e.g. postgres.abcdefghijklm): " DB_USER
    read -s -p "Password: " DB_PASS
    echo ""
    echo ""

    if [ -z "$DB_HOST" ] || [ -z "$DB_USER" ] || [ -z "$DB_PASS" ]; then
        error "❌ Host, username, and password are required."
        exit 1
    fi

    # Step 3: optional project ref for Supabase
    local PROJECT_REF=""
    local PROJECT_URL=""
    if [ "$DB_TYPE" = "supabase" ]; then
        echo -e "${GREEN}Step 3/4 — Supabase project info (optional):${NC}"
        echo ""
        read -p "Project ref (from https://<ref>.supabase.co, leave empty to skip): " PROJECT_REF
        if [ -n "$PROJECT_REF" ]; then
            PROJECT_URL="https://${PROJECT_REF}.supabase.co"
        fi
        echo ""
    fi

    # Step 4: SSL mode
    echo -e "${GREEN}Step 4/4 — SSL mode:${NC}"
    echo "  1) require (recommended for Supabase)"
    echo "  2) verify-full"
    echo "  3) disable"
    echo ""
    read -p "Choice [1-3]: " SSL_CHOICE
    echo ""
    case "$SSL_CHOICE" in
        2) SSL_MODE="verify-full" ;;
        3) SSL_MODE="disable" ;;
        *) SSL_MODE="require" ;;
    esac

    # Store password as env-var reference in config.env
    ENV_VAR_NAME="SUPABASE_DB_PASSWORD"
    if [ "$DB_TYPE" = "postgres" ]; then
        ENV_VAR_NAME="POSTGRES_DB_PASSWORD"
    fi

    # Remove existing env var line, then append new one
    grep -v "^${ENV_VAR_NAME}=" "$CONFIG_FILE" > "$CONFIG_FILE.tmp" 2>/dev/null || true
    mv "$CONFIG_FILE.tmp" "$CONFIG_FILE"
    echo "${ENV_VAR_NAME}=${DB_PASS}" >> "$CONFIG_FILE"
    chmod 600 "$CONFIG_FILE"

    # Write database.json with password as $ENV_VAR reference (no plaintext)
    if [ "$DB_TYPE" = "supabase" ]; then
        cat > "$DB_CONFIG_FILE" <<EOF
{
  "type": "supabase",
  "supabase": {
    "project_ref": "$PROJECT_REF",
    "project_url": "$PROJECT_URL",
    "database": {
      "host": "$DB_HOST",
      "port": $DB_PORT,
      "database": "$DB_NAME",
      "username": "$DB_USER",
      "password": "\$$ENV_VAR_NAME",
      "ssl_mode": "$SSL_MODE"
    }
  }
}
EOF
    else
        cat > "$DB_CONFIG_FILE" <<EOF
{
  "type": "postgres",
  "postgres": {
    "host": "$DB_HOST",
    "port": $DB_PORT,
    "database": "$DB_NAME",
    "username": "$DB_USER",
    "password": "\$$ENV_VAR_NAME",
    "ssl_mode": "$SSL_MODE"
  }
}
EOF
    fi

    chmod 600 "$DB_CONFIG_FILE"
    info "✓ $LABEL configured. Password stored in $CONFIG_FILE as $ENV_VAR_NAME"
    echo ""
}

# === Main dispatch ===

case "${1:-start}" in
    start)
        cmd_start
        ;;
    stop)
        cmd_stop
        ;;
    restart)
        cmd_restart
        ;;
    status)
        cmd_status
        ;;
    logs)
        cmd_logs
        ;;
    config)
        cmd_config
        ;;
    --version|-v)
        cmd_version
        ;;
    update|upgrade)
        cmd_update
        ;;
    db|database)
        cmd_db
        ;;
    help|--help|-h)
        echo ""
        echo -e "${CYAN}╔══════════════════════════════════════════════════╗${NC}"
        echo -e "${CYAN}║        VPS Dashboard - Smart Server Manager      ║${NC}"
        echo -e "${CYAN}╚══════════════════════════════════════════════════╝${NC}"
        echo ""
        echo -e "${GREEN}USAGE:${NC}"
        echo "  vpsdash [command]"
        echo ""
        echo -e "${GREEN}COMMANDS:${NC}"
        echo -e "  ${CYAN}start${NC}      Start server in background (default action)"
        echo -e "  ${CYAN}stop${NC}       Stop server"
        echo -e "  ${CYAN}restart${NC}    Restart server"
        echo -e "  ${CYAN}status${NC}     Check if running (shows PID, uptime, memory)"
        echo -e "  ${CYAN}logs${NC}       Tail logs in real-time (Ctrl+C to exit)"
        echo -e "  ${CYAN}config${NC}     Show config file & admin credentials"
        echo -e "  ${CYAN}db${NC}         Switch database (SQLite/PostgreSQL/Supabase)"
        echo -e "  ${CYAN}update${NC}     Download & install latest version from GitHub"
        echo -e "  ${CYAN}--version${NC}  Show version info"
        echo -e "  ${CYAN}help${NC}       Show this help message"
        echo ""
        echo -e "${GREEN}EXAMPLES:${NC}"
        echo -e "  ${YELLOW}vpsdash${NC}              # Start server (first run = auto setup)"
        echo -e "  ${YELLOW}vpsdash status${NC}       # Check if running"
        echo -e "  ${YELLOW}vpsdash logs${NC}         # View live logs"
        echo -e "  ${YELLOW}vpsdash stop${NC}         # Stop server"
        echo -e "  ${YELLOW}vpsdash restart${NC}     # Restart after config change"
        echo -e "  ${YELLOW}vpsdash db${NC}           # Switch to Supabase/PostgreSQL"
        echo -e "  ${YELLOW}vpsdash update${NC}       # Update to latest version from GitHub"
        echo -e "  ${YELLOW}vpsdash config${NC}      # Show credentials & config path"
        echo ""
        echo -e "${GREEN}FIRST RUN:${NC}"
        echo "  Auto-generates config with random secrets at:"
        echo -e "  ${CYAN}$CONFIG_FILE${NC}"
        echo ""
        echo "  Admin credentials shown once — save them!"
        echo ""
        echo -e "${GREEN}CONFIG:${NC}"
        echo "  Edit config:  nano $CONFIG_FILE"
        echo "  Then restart:  vpsdash restart"
        echo ""
        echo -e "${GREEN}DATABASE:${NC}"
        echo "  Default: SQLite at $VPSDASH_HOME/vpsdash.db"
        echo "  Switch to PostgreSQL/Supabase: edit database.json"
        echo ""
        echo -e "${GREEN}DOCUMENTATION:${NC}"
        echo "  https://github.com/KarmaSenzu/ServerMonitoring"
        echo ""
        ;;
    *)
        # Pass through to binary for unknown commands (e.g., --version flags)
        BINARY=$(find_binary)
        if [ -n "$BINARY" ]; then
            exec "$BINARY" "$@"
        else
            error "Unknown command: $1"
            echo "Run: vpsdash help"
            exit 1
        fi
        ;;
esac
