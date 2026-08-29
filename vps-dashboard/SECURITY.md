# Security

This document covers the credential and access hygiene for the
`vps-dashboard` project after the Wave 1.2B cleanup.

## TL;DR

- All deploy scripts now read VPS connection info from `deploy.env`
  (gitignored). There is no hardcoded password anywhere in the tracked
  scripts.
- Prefer SSH key auth. Password auth via `sshpass` / `expect` is kept
  only as a legacy fallback.
- The previously-committed VPS password is **compromised** and must be
  rotated. See "Rotate the leaked password" below.

## `deploy.env` is required

The shared loader `scripts/lib/load_env.sh` looks for `deploy.env` in
the project root. Every deploy / fix / status script sources it.

Setup:

```bash
cp deploy.env.example deploy.env
$EDITOR deploy.env       # fill in real values
```

Required keys: `VPS_HOST`, `VPS_USER`, `PROJECT_DIR`,
`REMOTE_APP_DIR`, `REMOTE_WEB_DIR`. Plus at least one of `VPS_SSH_KEY`
or `VPS_PASS`.

**`deploy.env` must NEVER be committed.** It is listed in
`.gitignore`. Treat its contents as secrets.

## Use SSH keys (recommended)

Password auth via `sshpass` / `expect` leaks the password into:

- shell history
- process listings (`ps auxf` shows the full `sshpass -p ...` line)
- script files on disk

SSH keys avoid all of those. Generate and install one:

```bash
# Generate a project-specific ed25519 key
ssh-keygen -t ed25519 -f ~/.ssh/vps-dashboard_ed25519 -C "vps-dashboard deploy"

# Copy it to the VPS (this is the one time you'll need the password)
ssh-copy-id -i ~/.ssh/vps-dashboard_ed25519.pub ubuntu@<VPS_HOST>

# Update deploy.env
#   VPS_SSH_KEY=/Users/<you>/.ssh/vps-dashboard_ed25519
#   VPS_PASS=
```

Or run the helper:

```bash
./scripts/setup_ssh_key.sh
```

It generates the key, installs it, and updates `deploy.env` for you.

## Rotate the leaked password

The password `nebula-59#-panda` was hardcoded across multiple scripts
and is therefore **considered compromised**. Even after this cleanup,
it likely still exists in:

- local backups
- any prior commits / git history (if this project was ever committed
  to a repo)
- shell scrollback

You must rotate it.

1. SSH into the VPS (using the new key once provisioned, or the old
   password one last time):
   ```bash
   ssh ubuntu@<VPS_HOST>
   ```
2. Change the user password:
   ```bash
   sudo passwd ubuntu
   ```
3. Confirm SSH key auth works without password:
   ```bash
   exit
   ssh -i ~/.ssh/vps-dashboard_ed25519 ubuntu@<VPS_HOST> "echo ok"
   ```
4. Disable password auth entirely on the SSH daemon:
   ```bash
   sudo nano /etc/ssh/sshd_config
   # set: PasswordAuthentication no
   sudo systemctl reload ssh
   ```
5. Optionally lock the password on the account so it cannot be used at
   all:
   ```bash
   sudo passwd -l ubuntu
   ```

## Git history note

If this project has ever been pushed to a remote (GitHub/GitLab/etc.),
the leaked password is **still in history** even after these file
edits. Rotation (above) is the correct mitigation — do not rely on
"force-push to rewrite history" as the only fix; assume the value has
been seen and act accordingly.

## What `deploy.env` should NOT contain

- API keys for third-party services (use a separate secrets manager)
- Long-lived production credentials beyond the SSH password fallback
- Anything that would also be needed by the running backend — those
  belong in the backend's own env loader

## Reporting

If you find a credential in a tracked file, treat it as a security
incident: rotate, then patch the file, then audit history.
