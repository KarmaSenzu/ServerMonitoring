import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  FiBox,
  FiRefreshCw,
  FiPlay,
  FiPause,
  FiRewind,
  FiPlus,
  FiSearch,
  FiServer,
  FiCloud,
  FiDatabase,
  FiActivity,
  FiTerminal,
  FiCopy,
  FiMaximize2,
} from 'react-icons/fi'
import { containerApi } from '../api/endpoints.js'
import { useAuth, canMutate } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import { describeError } from '../ui/errors.js'
import './Containers.css'

export default function ContainersPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const toast = useToast()
  const isAdmin = canMutate(user?.role)

  const [expanded, setExpanded] = useState(new Set())
  const [filter, setFilter] = useState('')

  const fleetQ = useQuery({
    queryKey: ['containers-fleet'],
    queryFn: () => containerApi.fleet(),
    refetchInterval: 30000,
  })

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['containers-fleet'] })
  }

  const toggle = (serverId) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(serverId)) next.delete(serverId)
      else next.add(serverId)
      return next
    })
  }

  const servers = useMemo(
    () => (Array.isArray(fleetQ.data) ? fleetQ.data : []),
    [fleetQ.data]
  )

  // Flatten all containers across servers
  const allContainers = useMemo(() => {
    const flat = []
    for (const s of servers) {
      for (const c of s.containers || []) {
        flat.push({ ...c, server_name: s.server_name, server_id: s.server_id })
      }
    }
    return flat
  }, [servers])

  const filtered = useMemo(() => {
    if (!filter.trim()) return allContainers
    const q = filter.toLowerCase()
    return allContainers.filter(
      (c) =>
        (c.name || '').toLowerCase().includes(q) ||
        (c.image || '').toLowerCase().includes(q) ||
        (c.server_name || '').toLowerCase().includes(q)
    )
  }, [allContainers, filter])

  const totalContainers = allContainers.length
  const runningContainers = allContainers.filter((c) => c.state === 'running').length
  const exitedContainers = allContainers.filter((c) => c.state === 'exited' || c.state === 'dead').length

  // Derive engine name for hero badge
  const engineName = servers.find((s) => s.engine)?.engine || 'Docker Engine'

  return (
    <div className="containers-page">
      {/* === HERO HEADER === */}
      <div className="containers-hero">
        <div className="containers-hero-top">
          <div className="containers-hero-left">
            <div className="containers-hero-title-row">
              <h1 className="containers-hero-title">Containers &amp; Runtime Orchestration</h1>
              <span className="engine-badge-hero">{engineName}</span>
              <span className="pm2-live-badge">
                <span className="pm2-live-dot" />
                PM2 Live
              </span>
            </div>
            <p className="containers-hero-subtitle">
              Unified control plane for {totalContainers} container runtimes, distributed PM2 microservices, and active Cloudflare edge ingress tunnels.
            </p>
          </div>
          <div className="containers-hero-actions">
            <button className="containers-btn ghost" onClick={refresh} disabled={fleetQ.isFetching}>
              <FiRefreshCw className={fleetQ.isFetching ? 'spinning' : ''} size={14} />
              <span>Restart All Services</span>
            </button>
            <button className="containers-btn primary">
              <FiPlus size={14} />
              <span>+ Deploy Container</span>
            </button>
          </div>
        </div>

        {/* Filter bar + live counters */}
        <div className="containers-filter-row">
          <div className="containers-filter-left">
            <div className="filter-input-wrap">
              <FiSearch size={14} />
              <input
                type="text"
                className="filter-input"
                placeholder="Filter containers, node services, tunnels... (⌘K)"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
              />
              <span className="filter-esc">ESC</span>
            </div>
            <div className="host-selector">
              <FiServer size={14} />
              <span>Host: All Nodes ({servers.length})</span>
            </div>
          </div>
          <div className="live-counters">
            <div className="counter counter-running">
              <span className="counter-dot" />
              <span className="counter-label">RUNNING:</span>
              <span className="counter-value">{runningContainers}</span>
            </div>
            <div className="counter counter-exited">
              <span className="counter-dot" />
              <span className="counter-label">EXITED:</span>
              <span className="counter-value">{exitedContainers}</span>
            </div>
            <div className="counter counter-pm2">
              <span className="counter-dot" />
              <span className="counter-label">TOTAL:</span>
              <span className="counter-value">{totalContainers}</span>
            </div>
            <div className="counter counter-cf">
              <span className="counter-dot" />
              <span className="counter-label">NODES:</span>
              <span className="counter-value">{servers.length}</span>
            </div>
          </div>
        </div>
      </div>

      {/* === TRI-PANEL ANALYTICS GRID === */}
      <div className="containers-tri-panel">
        {/* Panel 1: Container Fleet Summary */}
        <div className="tri-panel-card">
          <div className="tri-panel-header">
            <span className="tri-panel-label">
              <FiCloud size={16} /> Container Fleet
            </span>
            <span className="tri-panel-status healthy">
              <span className="tri-panel-dot" /> HEALTHY
            </span>
          </div>
          <div className="tri-panel-metrics">
            <div className="tri-metric">
              <span className="tri-metric-label">Running</span>
              <span className="tri-metric-value">{runningContainers}</span>
            </div>
            <div className="tri-metric">
              <span className="tri-metric-label">Exited</span>
              <span className="tri-metric-value amber">{exitedContainers}</span>
            </div>
          </div>
          <div className="tri-panel-breakdown">
            {servers.map((s) => (
              <div key={s.server_id} className="tri-breakdown-row">
                <span className="tri-breakdown-name">{s.server_name}</span>
                <span className="tri-breakdown-status">
                  {s.containers?.filter((c) => c.state === 'running').length || 0} running
                </span>
              </div>
            ))}
            {servers.length === 0 && <div className="tri-breakdown-name muted">No nodes</div>}
          </div>
        </div>

        {/* Panel 2: Runtime States */}
        <div className="tri-panel-card">
          <div className="tri-panel-header">
            <span className="tri-panel-label">
              <FiActivity size={16} /> Runtime States
            </span>
            <span className="tri-panel-sub">{totalContainers} total tasks</span>
          </div>
          <div className="tri-panel-states">
            <div className="state-row">
              <span className="state-row-label">
                <span className="state-dot running" /> Running
              </span>
              <span className="state-row-value">
                {runningContainers} <span className="muted">/ {totalContainers}</span>
              </span>
            </div>
            <div className="state-bar">
              <div
                className="state-bar-fill running"
                style={{ width: `${totalContainers ? (runningContainers / totalContainers) * 100 : 0}%` }}
              />
              <div className="state-bar-rest" />
            </div>
            <div className="state-row">
              <span className="state-row-label">
                <span className="state-dot exited" /> Exited
              </span>
              <span className="state-row-value">
                {exitedContainers} <span className="muted">/ {totalContainers}</span>
              </span>
            </div>
            <div className="state-bar">
              <div
                className="state-bar-fill exited"
                style={{ width: `${totalContainers ? (exitedContainers / totalContainers) * 100 : 0}%` }}
              />
              <div className="state-bar-rest" />
            </div>
          </div>
          <div className="tri-panel-footer">
            <span>Nodes: <b>{servers.length}</b></span>
            <span>Engines: <b className="green">{new Set(servers.map((s) => s.engine).filter(Boolean)).size || 0}</b></span>
          </div>
        </div>

        {/* Panel 3: Docker Daemon / Storage */}
        <div className="tri-panel-card">
          <div className="tri-panel-header">
            <span className="tri-panel-label">
              <FiDatabase size={16} /> Docker Daemon Host
            </span>
            <span className="tri-panel-status idle">STANDBY IDLE</span>
          </div>
          <div className="tri-panel-daemon">
            <div className="daemon-row">
              <span className="muted">Registered Servers:</span>
              <span className="daemon-value">{servers.length}</span>
            </div>
            <div className="daemon-row">
              <span className="muted">Total Containers:</span>
              <span className="daemon-value">{totalContainers}</span>
            </div>
            <div className="daemon-row">
              <span className="muted">Exited (Require Start):</span>
              <span className="daemon-value red">{exitedContainers} Stopped</span>
            </div>
          </div>
        </div>
      </div>

      {/* === CONTAINER SECTION HEADER === */}
      <div className="containers-section-header">
        <div className="containers-section-title">
          <FiBox size={18} />
          <h2>Docker Containers</h2>
          <span className="containers-count-badge">{totalContainers} Registered</span>
        </div>
        <div className="containers-batch-ops">
          <button className="batch-btn" title="Start all stopped containers">
            <FiPlay size={12} /> Start All
          </button>
          <button className="batch-btn" title="Prune stopped containers">
            <FiBox size={12} /> Prune Stopped
          </button>
        </div>
      </div>

      {/* === CONTAINER CARDS 3-COL GRID === */}
      {fleetQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : fleetQ.isError ? (
        <EmptyState
          icon={<FiBox size={40} />}
          title="Failed to load containers"
          description={describeError(fleetQ.error)}
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={<FiBox size={40} />}
          title={filter ? 'No matching containers' : 'No containers found'}
          description={filter ? 'Try a different filter' : 'Register servers first, then their containers will appear here'}
        />
      ) : (
        <div className="containers-grid">
          {filtered.map((c) => (
            <ContainerCard
              key={c.id}
              container={c}
              isAdmin={isAdmin}
              onAction={async (action) => {
                try {
                  if (action === 'start') await containerApi.start(c.server_id, c.name)
                  else if (action === 'stop') await containerApi.stop(c.server_id, c.name)
                  else if (action === 'restart') await containerApi.restart(c.server_id, c.name)
                  toast.push({ type: 'success', message: `${action} ${c.name}` })
                  refresh()
                } catch (err) {
                  toast.push({ type: 'error', message: describeError(err, `${action} failed`) })
                }
              }}
            />
          ))}
        </div>
      )}

      {/* === LIVE TELEMETRY STREAM === */}
      <div className="telemetry-stream">
        <div className="telemetry-stream-header">
          <div className="telemetry-stream-title">
            <span className="telemetry-live-dot" />
            <span className="telemetry-title">Daemon Stream &amp; Container Event Logs</span>
            <span className="telemetry-badge">STREAMING [SOCK://RUN/DOCKER.SOCK]</span>
          </div>
          <div className="telemetry-actions">
            <button className="telemetry-btn">
              <FiCopy size={12} /> Copy Trace
            </button>
            <button className="telemetry-btn">
              <FiMaximize2 size={12} /> Expand Terminal
            </button>
          </div>
        </div>
        <div className="telemetry-console">
          {filtered.slice(0, 8).map((c) => (
            <div key={c.id} className="telemetry-line">
              <span className="telemetry-time">{new Date().toLocaleTimeString([], { hour12: false })}</span>
              <span className={`telemetry-tag ${c.state === 'running' ? 'running' : 'exited'}`}>
                [container]
              </span>
              <span>
                {c.state === 'running' ? 'healthy' : 'stopped'}: {c.name} ({c.id ? String(c.id).slice(0, 12) : '?'}) on {c.server_name}
              </span>
            </div>
          ))}
          {filtered.length === 0 && (
            <div className="telemetry-line">
              <span className="telemetry-time">--:--:--</span>
              <span className="telemetry-tag running">[docker:daemon]</span>
              <span>No container events detected. Waiting for stream...</span>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function ContainerCard({ container, isAdmin, onAction }) {
  const [busy, setBusy] = useState(false)

  const handleAction = async (action) => {
    if (busy) return
    setBusy(true)
    try {
      await onAction(action)
    } finally {
      setBusy(false)
    }
  }

  const isRunning = container.state === 'running'
  const shortId = container.id ? String(container.id).slice(0, 12) : '?'

  return (
    <div className="container-card">
      <div className="container-card-top">
        <div className="container-card-icon">
          <FiBox size={18} />
        </div>
        <div className="container-card-title">
          <span className="container-card-name">{container.name}</span>
          <span className="container-card-image">{container.image}</span>
        </div>
        <span className={`container-state-badge ${isRunning ? 'running' : 'exited'}`}>
          <span className="state-badge-dot" />
          {container.state ? container.state.toUpperCase() : 'UNKNOWN'}
        </span>
      </div>
      <div className="container-card-details">
        <div className="container-detail-row">
          <span className="detail-label">ID:</span>
          <span className="detail-value mono">{shortId}</span>
        </div>
        <div className="container-detail-row">
          <span className="detail-label">STATUS:</span>
          <span className={`detail-value ${isRunning ? '' : 'red'}`}>{container.status || container.state}</span>
        </div>
        <div className="container-detail-row">
          <span className="detail-label">PORTS:</span>
          <span className="detail-value">{container.ports || 'None Mapped'}</span>
        </div>
        <div className="container-detail-row">
          <span className="detail-label">NODE:</span>
          <span className="detail-value amber">{container.server_name}</span>
        </div>
      </div>
      {isAdmin && (
        <div className="container-card-actions">
          <button className="container-action-btn" title="Logs">
            <FiTerminal size={12} /> Logs
          </button>
          {isRunning ? (
            <button className="container-action-btn" disabled={busy} onClick={() => handleAction('stop')} title="Stop">
              <FiPause size={12} /> Stop
            </button>
          ) : (
            <button className="container-action-btn start" disabled={busy} onClick={() => handleAction('start')} title="Start">
              <FiPlay size={12} /> Start
            </button>
          )}
          <button className="container-action-btn" disabled={busy} onClick={() => handleAction('restart')} title="Restart">
            <FiRewind size={12} /> Restart
          </button>
        </div>
      )}
    </div>
  )
}
