import { useEffect, useRef, useState } from 'react'
import {
  FiServer,
  FiCheckCircle,
  FiAlertTriangle,
  FiActivity,
} from 'react-icons/fi'

// AnimatedCounter animates from 0 to the target value.
export function AnimatedCounter({ value, duration = 800, suffix = '' }) {
  const [display, setDisplay] = useState(0)
  const prevRef = useRef(0)

  useEffect(() => {
    const start = prevRef.current
    const target = Number(value) || 0
    const startTime = performance.now()

    let raf
    const tick = (now) => {
      const elapsed = now - startTime
      const progress = Math.min(elapsed / duration, 1)
      // ease-out cubic
      const eased = 1 - Math.pow(1 - progress, 3)
      setDisplay(Math.round(start + (target - start) * eased))
      if (progress < 1) {
        raf = requestAnimationFrame(tick)
      } else {
        prevRef.current = target
      }
    }

    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [value, duration])

  return <>{display}{suffix}</>
}

// KPICard displays a single metric with icon, label, value, and optional sub-text.
export function KPICard({ icon, label, value, sub, accent, suffix }) {
  return (
    <div
      className="kpi-card"
      style={{
        '--kpi-accent': accent || 'var(--accent)',
        '--kpi-bg': `${accent || 'var(--accent)'}15`,
      }}
    >
      <div className="kpi-header">
        <span className="kpi-label">{label}</span>
        <span className="kpi-icon">{icon}</span>
      </div>
      <span className="kpi-value">
        <AnimatedCounter value={value} suffix={suffix} />
      </span>
      {sub && <span className="kpi-sub">{sub}</span>}
    </div>
  )
}

// NetworkTopology renders an SVG force-directed-style graph of servers.
// Servers are positioned in a circle with connection lines to a central node.
export function NetworkTopology({ servers }) {
  if (!servers || servers.length === 0) {
    return (
      <div className="topology-map" style={{ color: 'var(--text-muted)', fontSize: 13 }}>
        No servers registered. Add servers to see the network topology.
      </div>
    )
  }

  const width = 400
  const height = 280
  const cx = width / 2
  const cy = height / 2
  const radius = Math.min(width, height) / 2 - 50

  // Position servers in a circle around the center.
  const nodes = servers.slice(0, 12).map((srv, i) => {
    const angle = (i / Math.min(servers.length, 12)) * Math.PI * 2 - Math.PI / 2
    return {
      ...srv,
      x: cx + Math.cos(angle) * radius,
      y: cy + Math.sin(angle) * radius,
    }
  })

  const statusColor = {
    online: 'var(--success)',
    degraded: 'var(--warning)',
    offline: 'var(--danger)',
    unknown: 'var(--text-muted)',
  }

  return (
    <div className="topology-map">
      <svg className="topology-svg" viewBox={`0 0 ${width} ${height}`}>
        {/* Connection lines from center to each node */}
        {nodes.map((node) => (
          <line
            key={`link-${node.id}`}
            x1={cx}
            y1={cy}
            x2={node.x}
            y2={node.y}
            className={`topology-link ${node.status === 'online' ? 'active' : ''}`}
          />
        ))}

        {/* Central hub */}
        <circle cx={cx} cy={cy} r={14} fill="var(--accent)" opacity={0.2} />
        <circle cx={cx} cy={cy} r={8} fill="var(--accent)" />
        <text x={cx} y={cy + 28} className="topology-node-label" style={{ fontSize: 10 }}>
          HUB
        </text>

        {/* Server nodes */}
        {nodes.map((node) => (
          <g
            key={node.id}
            className="topology-node"
            onClick={() => {
              if (node.id) window.location.href = `/servers`
            }}
          >
            {/* Glow ring */}
            <circle
              cx={node.x}
              cy={node.y}
              r={12}
              fill={statusColor[node.status] || statusColor.unknown}
              opacity={0.15}
            />
            {/* Node circle */}
            <circle
              cx={node.x}
              cy={node.y}
              r={7}
              fill={statusColor[node.status] || statusColor.unknown}
              style={{ filter: `drop-shadow(0 0 4px ${statusColor[node.status] || statusColor.unknown})` }}
            />
            {/* Label */}
            <text x={node.x} y={node.y - 14} className="topology-node-label">
              {node.name && node.name.length > 12 ? node.name.slice(0, 10) + '..' : node.name}
            </text>
          </g>
        ))}
      </svg>
    </div>
  )
}

// ServerStatusList shows a compact list of servers with status dots.
export function ServerStatusList({ servers }) {
  if (!servers || servers.length === 0) {
    return <p style={{ color: 'var(--text-muted)', fontSize: 13 }}>No servers registered.</p>
  }

  const sorted = [...servers].sort((a, b) => {
    // Critical first: offline > degraded > unknown > online
    const order = { offline: 0, degraded: 1, unknown: 2, online: 3 }
    return (order[a.status] ?? 3) - (order[b.status] ?? 3)
  })

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      {sorted.slice(0, 10).map((srv) => (
        <div
          key={srv.id}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            padding: '6px 8px',
            borderRadius: 8,
            background: 'rgba(255,255,255,0.02)',
          }}
        >
          <span className={`status-dot status-${srv.status}`} />
          <span style={{ flex: 1, fontSize: 13, color: 'var(--text-primary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {srv.name}
          </span>
          {srv.hostname && (
            <span style={{ fontSize: 11, color: 'var(--text-muted)', fontFamily: 'var(--mono, monospace)' }}>
              {srv.hostname}
            </span>
          )}
        </div>
      ))}
    </div>
  )
}

// IncidentTimeline shows recent events as a vertical timeline.
export function IncidentTimeline({ events }) {
  if (!events || events.length === 0) {
    return <p style={{ color: 'var(--text-muted)', fontSize: 13 }}>No recent events.</p>
  }

  const severityClass = {
    info: 'severity-info',
    warning: 'severity-warning',
    error: 'severity-error',
    critical: 'severity-critical',
  }

  return (
    <div className="incident-timeline">
      {events.slice(0, 15).map((ev, i) => (
        <div key={ev.id || i} className="incident-item">
          <span className={`incident-dot ${severityClass[ev.severity] || 'severity-info'}`}>
            ●
          </span>
          <div className="incident-content">
            <span className="incident-message">{ev.message || ev.source || 'Event'}</span>
            <span className="incident-time">
              {ev.ts ? new Date(ev.ts).toLocaleString() : ''}
            </span>
          </div>
        </div>
      ))}
    </div>
  )
}

// MetricMiniChart renders a compact sparkline-style bar chart.
export function MetricMiniChart({ data, color, height = 60 }) {
  if (!data || data.length === 0) {
    return <div style={{ height, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-muted)', fontSize: 11 }}>No data</div>
  }

  const max = Math.max(...data.map((d) => d.value || 0), 1)
  const barWidth = 100 / data.length

  return (
    <div style={{ display: 'flex', alignItems: 'flex-end', gap: 1, height }}>
      {data.slice(-40).map((d, i) => {
        const h = ((d.value || 0) / max) * height
        return (
          <div
            key={i}
            style={{
              width: `${barWidth}%`,
              height: Math.max(h, 2),
              background: color || 'var(--accent)',
              borderRadius: 2,
              opacity: 0.3 + 0.7 * ((d.value || 0) / max),
            }}
          />
        )
      })}
    </div>
  )
}
