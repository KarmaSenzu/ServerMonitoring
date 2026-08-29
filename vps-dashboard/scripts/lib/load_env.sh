#!/usr/bin/env bash
# scripts/lib/load_env.sh
# Shared environment loader for vps-dashboard deploy scripts.
#
# Usage from another script:
#     SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
#     # shellcheck source=scripts/lib/load_env.sh
#     source "$SCRIPT_DIR/scripts/lib/load_env.sh"
#
# Or, when sourced from a script that already cd'd to PROJECT_DIR:
#     source "scripts/lib/load_env.sh"
#
# After sourcing, the following functions are available:
#     vpsd_ssh "command..."
#     vpsd_scp <local> <remote>           (or any scp-style args)
#     vpsd_rsync <local> <remote>         (or any rsync-style args)
#
# Required vars (validated): VPS_HOST, VPS_USER, PROJECT_DIR,
#     REMOTE_APP_DIR, REMOTE_WEB_DIR
# At least one of VPS_SSH_KEY / VPS_PASS must be set.

# Resolve this loader's directory (handles symlinks reasonably on macOS bash 3.2)
_VPSD_LOADER_SRC="${BASH_SOURCE[0]:-$0}"
_VPSD_LOADER_DIR="$(cd "$(dirname "$_VPSD_LOADER_SRC")" && pwd)"

# deploy.env is expected two levels up from scripts/lib/
_VPSD_PROJECT_ROOT="$(cd "$_VPSD_LOADER_DIR/../.." && pwd)"
_VPSD_ENV_FILE="$_VPSD_PROJECT_ROOT/deploy.env"

if [ ! -f "$_VPSD_ENV_FILE" ]; then
  echo "[load_env] ERROR: deploy.env not found at $_VPSD_ENV_FILE" >&2
  echo "[load_env] Copy deploy.env.example to deploy.env and fill it in." >&2
  exit 1
fi

# shellcheck disable=SC1090
set -a
source "$_VPSD_ENV_FILE"
set +a

# --- Validate required vars ---
_vpsd_missing=""
for _v in VPS_HOST VPS_USER PROJECT_DIR REMOTE_APP_DIR REMOTE_WEB_DIR; do
  if [ -z "${!_v:-}" ]; then
    _vpsd_missing="$_vpsd_missing $_v"
  fi
done
if [ -n "$_vpsd_missing" ]; then
  echo "[load_env] ERROR: missing required var(s) in deploy.env:$_vpsd_missing" >&2
  echo "[load_env] See deploy.env.example for the expected fields." >&2
  exit 1
fi
unset _vpsd_missing _v

# --- Auth mode resolution ---
# Prefer SSH key when present and the key file exists.
_VPSD_AUTH_MODE=""
if [ -n "${VPS_SSH_KEY:-}" ] && [ -f "$VPS_SSH_KEY" ]; then
  _VPSD_AUTH_MODE="key"
elif [ -n "${VPS_PASS:-}" ]; then
  if command -v sshpass >/dev/null 2>&1; then
    _VPSD_AUTH_MODE="pass"
  else
    echo "[load_env] ERROR: VPS_PASS is set but 'sshpass' is not installed." >&2
    echo "[load_env] Install with: brew install hudochenkov/sshpass/sshpass" >&2
    echo "[load_env] Or set VPS_SSH_KEY to use key auth (recommended)." >&2
    exit 1
  fi
else
  echo "[load_env] ERROR: no auth configured." >&2
  echo "[load_env] Set VPS_SSH_KEY (recommended) or VPS_PASS in deploy.env." >&2
  echo "[load_env] To provision a key, run: scripts/setup_ssh_key.sh" >&2
  exit 1
fi

# Common SSH options
_VPSD_SSH_OPTS=(-o StrictHostKeyChecking=accept-new -o ServerAliveInterval=30)

# vpsd_ssh "command string"
#   Runs a remote command using whichever auth mode is active.
vpsd_ssh() {
  if [ "$_VPSD_AUTH_MODE" = "key" ]; then
    ssh "${_VPSD_SSH_OPTS[@]}" -i "$VPS_SSH_KEY" "$VPS_USER@$VPS_HOST" "$@"
  else
    sshpass -p "$VPS_PASS" ssh "${_VPSD_SSH_OPTS[@]}" "$VPS_USER@$VPS_HOST" "$@"
  fi
}

# vpsd_scp <args...>
#   Wraps scp. Caller passes scp-style positional args, e.g.:
#     vpsd_scp local.txt "$VPS_USER@$VPS_HOST:/tmp/"
#     vpsd_scp -r local_dir "$VPS_USER@$VPS_HOST:/tmp/"
vpsd_scp() {
  if [ "$_VPSD_AUTH_MODE" = "key" ]; then
    scp "${_VPSD_SSH_OPTS[@]}" -i "$VPS_SSH_KEY" "$@"
  else
    sshpass -p "$VPS_PASS" scp "${_VPSD_SSH_OPTS[@]}" "$@"
  fi
}

# vpsd_rsync <args...>
#   Wraps rsync over ssh with the same auth preference.
vpsd_rsync() {
  if ! command -v rsync >/dev/null 2>&1; then
    echo "[load_env] ERROR: rsync not installed." >&2
    return 1
  fi
  local _ssh_cmd
  if [ "$_VPSD_AUTH_MODE" = "key" ]; then
    _ssh_cmd="ssh -o StrictHostKeyChecking=accept-new -o ServerAliveInterval=30 -i $VPS_SSH_KEY"
    rsync -avz -e "$_ssh_cmd" "$@"
  else
    _ssh_cmd="ssh -o StrictHostKeyChecking=accept-new -o ServerAliveInterval=30"
    sshpass -p "$VPS_PASS" rsync -avz -e "$_ssh_cmd" "$@"
  fi
}

# Export helpers for sub-shells that need them
export -f vpsd_ssh
export -f vpsd_scp
export -f vpsd_rsync

# Make the resolved values available to child processes too
export VPS_HOST VPS_USER VPS_HOSTNAME VPS_SSH_KEY VPS_PASS \
       PROJECT_DIR REMOTE_APP_DIR REMOTE_WEB_DIR
