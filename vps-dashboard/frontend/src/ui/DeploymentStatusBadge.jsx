// DeploymentStatusBadge renders a small pill describing the lifecycle
// state of a deployment record. States mirror the backend enum:
// pending, running, success, failed, timeout.
const STATUS_LABELS = {
  pending: 'Pending',
  running: 'Running',
  success: 'Success',
  failed: 'Failed',
  timeout: 'Timeout',
}

export default function DeploymentStatusBadge({ status, className = '' }) {
  const s = normalize(status)
  const cls = ['deploy-status-badge', `dep-${s}`, className]
    .filter(Boolean)
    .join(' ')
  return <span className={cls}>{STATUS_LABELS[s] || s}</span>
}

function normalize(status) {
  const s = String(status || '').toLowerCase()
  switch (s) {
    case 'pending':
    case 'running':
    case 'success':
    case 'failed':
    case 'timeout':
      return s
    default:
      return 'pending'
  }
}
