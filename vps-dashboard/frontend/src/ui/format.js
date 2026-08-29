// Shared formatting helpers used by multiple pages.

// humanizeUptime renders a number of seconds as a coarse uptime
// string (e.g. "3d 4h", "2h 15m", "47s"). Returns "-" for invalid
// or zero/negative input.
export function humanizeUptime(seconds) {
  const s = Number(seconds)
  if (!Number.isFinite(s) || s <= 0) return '-'
  if (s < 60) return `${Math.floor(s)}s`
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

// humanizeDuration renders a number of seconds as a more granular
// duration suitable for alert rule "for"/"cooldown" labels. Examples:
// "0s", "30s", "5m", "1h 30m", "2d 3h".
export function humanizeDuration(seconds) {
  const s = Math.floor(Number(seconds))
  if (!Number.isFinite(s) || s <= 0) return '0s'
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  if (d > 0) return h > 0 ? `${d}d ${h}h` : `${d}d`
  if (h > 0) return m > 0 ? `${h}h ${m}m` : `${h}h`
  if (m > 0) return sec > 0 ? `${m}m ${sec}s` : `${m}m`
  return `${sec}s`
}

// Format a number of bytes-per-second to the largest sensible unit.
export function humanizeBytesPerSec(bps) {
  const n = Number(bps)
  if (!Number.isFinite(n) || n < 0) return 'n/a'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i += 1
  }
  return `${v.toFixed(v >= 100 || i === 0 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}/s`
}

// humanizeBytes renders a byte count as a binary-prefixed value
// ("4.2 MiB", "112 GiB"). Returns "0 B" for falsy/invalid input.
export function humanizeBytes(bytes) {
  const n = Number(bytes)
  if (!Number.isFinite(n) || n <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i += 1
  }
  const decimals = i === 0 ? 0 : v >= 100 ? 0 : v >= 10 ? 1 : 2
  return `${v.toFixed(decimals)} ${units[i]}`
}

// formatRelative converts an instant (Date, ISO string, ms epoch) into
// a short relative phrase, e.g. "just now", "47s ago", "3m ago",
// "2h ago", "4d ago", "in 5m". Returns "" for missing/invalid input.
export function formatRelative(input) {
  if (!input) return ''
  const ms =
    input instanceof Date
      ? input.getTime()
      : typeof input === 'number'
        ? input
        : Date.parse(input)
  if (!Number.isFinite(ms)) return ''
  const diff = Date.now() - ms
  const abs = Math.abs(diff)
  const future = diff < 0
  if (abs < 5_000) return 'just now'
  if (abs < 60_000) {
    const s = Math.round(abs / 1000)
    return future ? `in ${s}s` : `${s}s ago`
  }
  if (abs < 3_600_000) {
    const m = Math.round(abs / 60_000)
    return future ? `in ${m}m` : `${m}m ago`
  }
  if (abs < 86_400_000) {
    const h = Math.round(abs / 3_600_000)
    return future ? `in ${h}h` : `${h}h ago`
  }
  const d = Math.round(abs / 86_400_000)
  return future ? `in ${d}d` : `${d}d ago`
}

// formatAbsolute renders an instant in the user's locale.
export function formatAbsolute(input) {
  if (!input) return ''
  const d = input instanceof Date ? input : new Date(input)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleString()
}
