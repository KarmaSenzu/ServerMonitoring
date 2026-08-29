// Convert a normalized api error (see api/client.js) into a friendly
// human-readable string suitable for toasts and inline messages.
export function describeError(err, fallback = 'Something went wrong') {
  if (!err) return fallback
  const code = err.code
  const detail = typeof err.detail === 'string' ? err.detail : ''
  const friendly = FRIENDLY[code]
  if (friendly) {
    return detail ? `${friendly}: ${detail}` : friendly
  }
  if (detail) return detail
  if (err.message) return err.message
  return fallback
}

const FRIENDLY = {
  invalid_credentials: 'Invalid username or password',
  unauthorized: 'Session expired. Please sign in again',
  user_not_found: 'User not found',
  username_taken: 'That username is already taken',
  last_admin: 'You cannot remove or demote the last admin',
  cannot_delete_self: 'You cannot delete your own account',
  duplicate_name: 'A project with that name already exists',
  not_found: 'Not found',
  invalid_body: 'Invalid input',
  invalid_query: 'Invalid query parameters',
  validation_failed: 'Validation failed',
  docker_unavailable: 'Docker is not available on the server',
  pm2_unavailable: 'PM2 is not available on the server',
  tunnel_unavailable: 'Cloudflare tunnel data is not available',
  docker_command_failed: 'Docker command failed',
  pm2_command_failed: 'PM2 command failed',
  no_runtime: 'No container, pm2 process, or script linked to this project',
  invalid_action: 'Action is not supported',
  invalid_tunnel_service: 'Unknown tunnel service',
  tunnel_restart_unauthorized: 'Tunnel restart is not authorized',
  tunnel_restart_failed: 'Tunnel restart failed',
  // Wave 3 — channels and alert rules
  channel_not_found: 'Channel not found',
  duplicate_channel_name: 'A channel with that name already exists',
  invalid_channel_config: 'Invalid channel configuration',
  rule_not_found: 'Alert rule not found',
  invalid_rule: 'Invalid alert rule',
  invalid_threshold: 'Threshold is out of range',
  events_query_failed: 'Failed to query events',
  notifier_unavailable: 'Notifier service is not available',
  evaluator_unavailable: 'Alert evaluator is not available',
  // Wave 4 — deploy/webhook/backup/env
  invalid_signature: 'Webhook signature mismatch',
  already_running: 'A deployment is already running for this project',
  precondition_failed: 'Webhook secret not configured',
  webhook_secret_missing: 'Webhook secret not configured',
  deploy_disabled: 'Auto-deploy is disabled for this project',
  deploy_unavailable: 'Deploy service is not available',
  deploy_not_configured: 'Deploy is not configured for this project',
  backup_in_progress: 'A backup is already in progress',
  backup_unavailable: 'Backup service is not available',
  backup_failed: 'Backup is marked as failed and cannot be downloaded',
  last_backup: 'Cannot delete the last remaining backup',
  path_outside_dir: 'Backup path is outside configured directory',
  path_outside_backup_dir: 'Backup path is outside configured directory',
  file_missing: 'Backup file is missing on disk',
  invalid_env: 'Unknown environment',
  env_unavailable: 'Environment overrides are not available',
  wait_timeout: 'Deployment did not finish in time',
  network_error: 'Network error. Check your connection',
  server_error: 'Server error',
  internal_error: 'Server error',
}
