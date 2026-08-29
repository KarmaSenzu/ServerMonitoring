#!/bin/bash
# ============================================
# scripts/setup_ssh_key.sh
# Generate an SSH key, install it on the VPS, and update deploy.env
# so future runs use key auth.
# ============================================

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ENV_FILE="$PROJECT_ROOT/deploy.env"

# We source load_env.sh AFTER deciding whether we have a key, because
# load_env.sh itself will exit if no auth is configured at all. So we
# do a minimal pre-source check first.
if [ ! -f "$ENV_FILE" ]; then
  echo "[setup_ssh_key] ERROR: $ENV_FILE not found." >&2
  echo "[setup_ssh_key] Copy deploy.env.example to deploy.env and fill it in first." >&2
  exit 1
fi

# Pull a couple of values from deploy.env without invoking the full loader.
# shellcheck disable=SC1090
set -a
source "$ENV_FILE"
set +a

if [ -z "${VPS_HOST:-}" ] || [ -z "${VPS_USER:-}" ]; then
  echo "[setup_ssh_key] ERROR: VPS_HOST and VPS_USER must be set in deploy.env" >&2
  exit 1
fi

DEFAULT_KEY="$HOME/.ssh/vps-dashboard_ed25519"
TARGET_KEY="${VPS_SSH_KEY:-$DEFAULT_KEY}"

echo "=========================================="
echo "  SSH key setup for ${VPS_USER}@${VPS_HOST}"
echo "  Key path: $TARGET_KEY"
echo "=========================================="

# 1. Generate key if missing
if [ -f "$TARGET_KEY" ]; then
  echo "  Key already exists at $TARGET_KEY — skipping generation."
else
  echo ""
  echo "  Generating new ed25519 key..."
  mkdir -p "$(dirname "$TARGET_KEY")"
  chmod 700 "$(dirname "$TARGET_KEY")"
  # ssh-keygen will prompt for a passphrase interactively; that is fine.
  ssh-keygen -t ed25519 -f "$TARGET_KEY" -C "vps-dashboard deploy"
  echo "  Generated: $TARGET_KEY"
fi

PUB_KEY="${TARGET_KEY}.pub"
if [ ! -f "$PUB_KEY" ]; then
  echo "[setup_ssh_key] ERROR: public key $PUB_KEY missing." >&2
  exit 1
fi

# 2. Install on VPS via ssh-copy-id
echo ""
echo "  Installing public key on VPS..."

if [ -n "${VPS_PASS:-}" ] && command -v sshpass >/dev/null 2>&1; then
  echo "  Using VPS_PASS via sshpass for the one-time copy."
  sshpass -p "$VPS_PASS" ssh-copy-id \
    -o StrictHostKeyChecking=accept-new \
    -i "$PUB_KEY" \
    "$VPS_USER@$VPS_HOST"
else
  echo "  Running ssh-copy-id interactively (you'll be asked for the password once)."
  ssh-copy-id \
    -o StrictHostKeyChecking=accept-new \
    -i "$PUB_KEY" \
    "$VPS_USER@$VPS_HOST"
fi

# 3. Update deploy.env: set VPS_SSH_KEY=<TARGET_KEY>
echo ""
echo "  Updating $ENV_FILE → VPS_SSH_KEY=$TARGET_KEY"

TMP_ENV="$(mktemp "${ENV_FILE}.XXXXXX")"
# Use awk for a portable in-place edit (BSD sed on macOS is awkward).
awk -v key="$TARGET_KEY" '
  BEGIN { replaced = 0 }
  /^VPS_SSH_KEY=/ {
    print "VPS_SSH_KEY=" key
    replaced = 1
    next
  }
  { print }
  END {
    if (!replaced) {
      print "VPS_SSH_KEY=" key
    }
  }
' "$ENV_FILE" > "$TMP_ENV"

# Preserve permissions of the original file as best we can.
mv "$TMP_ENV" "$ENV_FILE"
chmod 600 "$ENV_FILE" 2>/dev/null || true

echo "  deploy.env updated."

# 4. Test it via the shared loader (which now sees VPS_SSH_KEY set).
echo ""
echo "  Testing key auth via vpsd_ssh..."
# Re-source load_env.sh in this shell — it picks up the freshly updated env file.
# shellcheck source=lib/load_env.sh
source "$SCRIPT_DIR/lib/load_env.sh"

if vpsd_ssh "echo connected"; then
  echo ""
  echo "=========================================="
  echo "  SSH key auth working."
  echo "  You can now safely clear VPS_PASS in deploy.env."
  echo "=========================================="
else
  echo ""
  echo "[setup_ssh_key] WARNING: test command failed. Check key and VPS config." >&2
  exit 1
fi
