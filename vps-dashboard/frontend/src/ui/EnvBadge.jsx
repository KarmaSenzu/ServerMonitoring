// EnvBadge renders a colored chip for a project environment.
//
// Environments map to a fixed palette: development=blue, staging=amber,
// production=red. Anything else falls back to a neutral chip. The badge
// can be rendered with size="sm" (default) or "md", and an optional dot
// indicator for use beside small text.
const ENV_LABELS = {
  development: 'Development',
  staging: 'Staging',
  production: 'Production',
}

export default function EnvBadge({ environment, size = 'sm', dot = false, className = '' }) {
  const env = normalize(environment)
  const cls = [
    'env-badge',
    `env-${env}`,
    size === 'md' ? 'env-md' : '',
    className,
  ]
    .filter(Boolean)
    .join(' ')
  const label = ENV_LABELS[env] || (environment || 'unknown')
  return (
    <span className={cls} title={label}>
      {dot && <span className="env-dot" aria-hidden="true" />}
      {label}
    </span>
  )
}

function normalize(env) {
  const s = String(env || '').toLowerCase()
  switch (s) {
    case 'development':
    case 'staging':
    case 'production':
      return s
    default:
      return 'unknown'
  }
}
