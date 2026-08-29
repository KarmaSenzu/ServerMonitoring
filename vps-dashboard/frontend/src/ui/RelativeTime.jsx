import { humanizeDuration, formatRelative, formatAbsolute } from './format.js'
import { useEffect, useState } from 'react'

// RelativeTime renders a short relative phrase ("3m ago") with the
// absolute formatted timestamp as the title attribute. Updates once
// per minute so the relative copy stays fresh while the page is open.
//
// `value` accepts: Date, ISO 8601 string, ms epoch, or 0/empty for
// "never". When `fallback` is provided it is rendered when value is
// empty/zero.
export default function RelativeTime({ value, fallback = 'never', className }) {
  const [, setTick] = useState(0)
  useEffect(() => {
    const id = setInterval(() => setTick((n) => n + 1), 60_000)
    return () => clearInterval(id)
  }, [])

  if (!value) {
    return <span className={className}>{fallback}</span>
  }
  // Treat a zero/empty timestamp ("0001-01-01T00:00:00Z") as never.
  if (
    typeof value === 'string' &&
    (value === '' || value.startsWith('0001-01-01'))
  ) {
    return <span className={className}>{fallback}</span>
  }

  const rel = formatRelative(value)
  const abs = formatAbsolute(value)
  return (
    <span className={className} title={abs}>
      {rel}
    </span>
  )
}

// Pre-formatted duration label, kept here so callers can use a single
// import for time formatting.
export function DurationLabel({ seconds, fallback = '0s' }) {
  if (seconds == null) return <span>{fallback}</span>
  return <span>{humanizeDuration(seconds)}</span>
}
