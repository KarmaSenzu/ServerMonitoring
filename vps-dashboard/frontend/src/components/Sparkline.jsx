import { useMemo } from 'react'
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  LineChart,
  Line,
  Tooltip,
  YAxis,
  XAxis,
} from 'recharts'

// Sparkline renders a thin trend line for the dashboard chart cards.
//
//   - `series` (single line): array of { timestamp, value }. Renders as
//     an Area with a subtle gradient fill in `color`.
//   - `multi` (two lines): array of { timestamp, rx, tx }. Renders rx
//     in `color` and tx in `secondColor`.
//
// Axes are hidden but the tooltip surfaces the hovered timestamp + value.
// `unit` (e.g. '%') is appended in the tooltip; `valueFormatter` overrides
// it entirely when provided (used for the Network chart's bytes/sec).

function formatTime(ts) {
  if (!ts) return ''
  const d = new Date(ts)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}

function makeFormatter({ unit, valueFormatter }) {
  if (typeof valueFormatter === 'function') return valueFormatter
  if (unit === '%') {
    return (v) => `${Number(v ?? 0).toFixed(1)}%`
  }
  return (v) => `${Number(v ?? 0).toFixed(2)}${unit ? ` ${unit}` : ''}`
}

function CustomTooltip({ active, payload, label, format }) {
  if (!active || !payload || payload.length === 0) return null
  return (
    <div className="sparkline-tooltip">
      <div className="sparkline-tooltip-time">{formatTime(label)}</div>
      {payload.map((entry) => (
        <div key={entry.dataKey} className="sparkline-tooltip-row">
          <span className="sparkline-tooltip-dot" style={{ background: entry.color }} />
          <span className="sparkline-tooltip-name">{entry.name || entry.dataKey}</span>
          <span className="sparkline-tooltip-value">{format(entry.value)}</span>
        </div>
      ))}
    </div>
  )
}

export default function Sparkline({
  series,
  multi,
  color = '#6c63ff',
  secondColor,
  unit,
  valueFormatter,
  height = 90,
}) {
  const formatter = useMemo(
    () => makeFormatter({ unit, valueFormatter }),
    [unit, valueFormatter]
  )

  if (multi && multi.length > 0) {
    return (
      <ResponsiveContainer width="100%" height={height}>
        <LineChart data={multi} margin={{ top: 6, right: 6, left: 0, bottom: 0 }}>
          <XAxis dataKey="timestamp" hide />
          <YAxis hide domain={[0, 'dataMax']} />
          <Tooltip
            content={<CustomTooltip format={formatter} />}
            cursor={{ stroke: 'rgba(255,255,255,0.1)' }}
          />
          <Line
            type="monotone"
            dataKey="rx"
            name="rx"
            stroke={color}
            strokeWidth={1.6}
            dot={false}
            isAnimationActive={false}
          />
          <Line
            type="monotone"
            dataKey="tx"
            name="tx"
            stroke={secondColor || '#40c4ff'}
            strokeWidth={1.6}
            dot={false}
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    )
  }

  const id = `sparkline-grad-${color.replace(/[^a-z0-9]/gi, '')}`

  return (
    <ResponsiveContainer width="100%" height={height}>
      <AreaChart data={series || []} margin={{ top: 6, right: 6, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity={0.35} />
            <stop offset="100%" stopColor={color} stopOpacity={0} />
          </linearGradient>
        </defs>
        <XAxis dataKey="timestamp" hide />
        <YAxis
          hide
          domain={unit === '%' ? [0, 100] : ['auto', 'auto']}
        />
        <Tooltip
          content={<CustomTooltip format={formatter} />}
          cursor={{ stroke: 'rgba(255,255,255,0.1)' }}
        />
        <Area
          type="monotone"
          dataKey="value"
          name={unit === '%' ? 'usage' : 'value'}
          stroke={color}
          strokeWidth={1.8}
          fill={`url(#${id})`}
          isAnimationActive={false}
        />
      </AreaChart>
    </ResponsiveContainer>
  )
}
