// SeverityBadge renders the colored severity pill used by Events,
// Notifications, and the Dashboard alert card. Severities mirror the
// backend enum: info | warning | error | critical. Anything else
// renders as the neutral "info" style.
export default function SeverityBadge({ severity, size = 'normal', dot = false }) {
  const sev = normalize(severity)
  const cls = `severity-badge severity-${sev}${size === 'small' ? ' severity-sm' : ''}${dot ? ' severity-dot-only' : ''}`
  if (dot) {
    return <span className={cls} aria-label={sev} title={sev} />
  }
  return <span className={cls}>{sev}</span>
}

function normalize(severity) {
  const s = String(severity || '').toLowerCase()
  switch (s) {
    case 'warning':
    case 'error':
    case 'critical':
    case 'info':
      return s
    default:
      return 'info'
  }
}
