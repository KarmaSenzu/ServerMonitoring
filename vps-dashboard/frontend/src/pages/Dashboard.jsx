import { useMemo } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  FiCpu,
  FiHardDrive,
  FiClock,
  FiRefreshCw,
  FiServer,
  FiCloud,
  FiDatabase,
  FiActivity,
  FiBell,
  FiUploadCloud,
} from 'react-icons/fi'
import { system, docker, tunnels, projects, events, backups } from '../api/endpoints.js'
import DockerCard from '../components/DockerCard.jsx'
import TunnelCard from '../components/TunnelCard.jsx'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import { useToast } from '../ui/useToast.js'
import { describeError } from '../ui/errors.js'
import { humanizeBytesPerSec, formatRelative } from '../ui/format.js'
import SeverityBadge from '../ui/SeverityBadge.jsx'
import DeploymentStatusBadge from '../ui/DeploymentStatusBadge.jsx'
import RelativeTime from '../ui/RelativeTime.jsx'
import Sparkline from '../components/Sparkline.jsx'
import './Dashboard.css'

function formatBytes(bytes) {
  if (!bytes || Number.isNaN(bytes)) return '0 GB'
  const gb = Number(bytes) / (1024 * 1024 * 1024)
  return `${gb.toFixed(2)} GB`
}

function formatUptime(seconds) {
  if (!seconds) return '-'
  const s = Number(seconds)
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (d > 0) return `${d}d ${h}h ${m}m`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

function formatTimeOfDay(d) {
  if (!d) return ''
  return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function isLoopback(name) {
  if (!name) return false
  const lower = name.toLowerCase()
  return lower === 'lo' || lower.startsWith('loopback') || lower === 'lo0'
}

// computeNetRates derives per-second rx/tx rates from cumulative byte counters
// in the network history samples. We sum across all non-loopback interfaces
// (consistent rule: ignore lo/loopback, sum the rest) and produce a series of
// {timestamp, rx, tx} entries. The first sample yields no rate; we drop it.
//
// The backend serializes per-interface counters in either snake_case
// (`bytes_sent`/`bytes_recv`) or camelCase (`bytesSent`/`bytesRecv`)
// depending on which sampler produced the row. Tolerate both.
function ifaceRx(iff) {
  return Number(iff.bytes_recv ?? iff.bytesRecv ?? 0)
}

function ifaceTx(iff) {
  return Number(iff.bytes_sent ?? iff.bytesSent ?? 0)
}

function computeNetRates(networkSamples) {
  if (!Array.isArray(networkSamples) || networkSamples.length < 2) {
    return []
  }
  const out = []
  for (let i = 1; i < networkSamples.length; i += 1) {
    const cur = networkSamples[i]
    const prev = networkSamples[i - 1]
    if (!cur || !prev) continue
    const tCur = new Date(cur.timestamp).getTime()
    const tPrev = new Date(prev.timestamp).getTime()
    const dtSec = Math.max(1, (tCur - tPrev) / 1000)
    const curIfs = Array.isArray(cur.per_interface) ? cur.per_interface : []
    const prevIfs = Array.isArray(prev.per_interface) ? prev.per_interface : []
    const prevByName = new Map(prevIfs.map((iff) => [iff.name, iff]))
    let rxSum = 0
    let txSum = 0
    for (const iff of curIfs) {
      if (isLoopback(iff.name)) continue
      const previous = prevByName.get(iff.name)
      if (!previous) continue
      const dRx = ifaceRx(iff) - ifaceRx(previous)
      const dTx = ifaceTx(iff) - ifaceTx(previous)
      // counter resets (e.g. interface down/up) appear as negative deltas — skip.
      if (dRx >= 0) rxSum += dRx / dtSec
      if (dTx >= 0) txSum += dTx / dtSec
    }
    out.push({ timestamp: cur.timestamp, rx: rxSum, tx: txSum })
  }
  return out
}

export default function Dashboard() {
  const queryClient = useQueryClient()
  const toast = useToast()

  const statsQ = useQuery({
    queryKey: ['system', 'stats'],
    queryFn: system.stats,
    // History endpoint covers detailed trends; the snapshot only needs to
    // refresh occasionally for the headline stat cards.
    refetchInterval: 30000,
  })

  const containersQ = useQuery({
    queryKey: ['docker', 'containers'],
    queryFn: docker.containers,
    refetchInterval: 10000,
    retry: (count, err) => {
      // Don't retry on docker_unavailable; surface the state cleanly.
      if (err?.code === 'docker_unavailable') return false
      return count < 1
    },
  })

  const tunnelsQ = useQuery({
    queryKey: ['system', 'tunnels'],
    queryFn: tunnels.list,
    refetchInterval: 10000,
  })

  const historyShortQ = useQuery({
    queryKey: ['system', 'history', '1h'],
    queryFn: () => system.history({ window: '1h' }),
    refetchInterval: 60000,
    retry: (count, err) => {
      // 503 with no samples yet is expected after a fresh boot.
      if (err?.status === 503) return false
      return count < 1
    },
  })

  const historyLongQ = useQuery({
    queryKey: ['system', 'history', '12h'],
    queryFn: () => system.history({ window: '12h' }),
    refetchInterval: 300000,
    retry: (count, err) => {
      if (err?.status === 503) return false
      return count < 1
    },
  })

  // Cache projects with a longer stale time. Used to mark which docker
  // containers are linked to a registered project so we can show a small
  // "Linked" pill.
  const projectsQ = useQuery({
    queryKey: ['projects', { all: true }],
    queryFn: () => projects.list(),
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
  })

  // Recent alerts feed for the dashboard card. Polls every 30s. Falls
  // back to an empty list on error so the card still renders.
  const alertsQ = useQuery({
    queryKey: ['events', 'alerts', 'recent'],
    queryFn: () => events.list({ category: 'alert', limit: 5 }),
    refetchInterval: 30000,
    retry: (count, err) => {
      if (err?.status === 401) return false
      return count < 1
    },
  })

  // Recent deploy events for the dashboard panel. Wave 4 backend appends
  // events with category="deploy" (verified in deploy/service.go).
  const deployEventsQ = useQuery({
    queryKey: ['events', 'deploy', 'recent'],
    queryFn: () => events.list({ category: 'deploy', limit: 5 }),
    refetchInterval: 60000,
    retry: (count, err) => {
      if (err?.status === 401) return false
      return count < 1
    },
  })

  // Backups quick-status. Polls every 60s. Soft-fails so the dashboard
  // still renders even when the backups feature is unavailable.
  const backupsQ = useQuery({
    queryKey: ['backups'],
    queryFn: backups.list,
    refetchInterval: 60000,
    retry: (count, err) => {
      if (err?.status === 401 || err?.status === 503) return false
      return count < 1
    },
  })

  const dockerUnavailable = containersQ.error?.code === 'docker_unavailable'
  const historyUnavailable =
    historyShortQ.error?.status === 503 || historyLongQ.error?.status === 503

  const lastUpdated = useMemo(() => {
    const stamps = [statsQ.dataUpdatedAt, containersQ.dataUpdatedAt, tunnelsQ.dataUpdatedAt]
      .filter(Boolean)
      .map((t) => new Date(t))
    if (stamps.length === 0) return null
    return new Date(Math.max(...stamps.map((d) => d.getTime())))
  }, [statsQ.dataUpdatedAt, containersQ.dataUpdatedAt, tunnelsQ.dataUpdatedAt])

  const refreshing =
    statsQ.isFetching || containersQ.isFetching || tunnelsQ.isFetching

  const handleRefresh = async () => {
    try {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['system', 'stats'] }),
        queryClient.invalidateQueries({ queryKey: ['docker', 'containers'] }),
        queryClient.invalidateQueries({ queryKey: ['system', 'tunnels'] }),
        queryClient.invalidateQueries({ queryKey: ['system', 'history', '1h'] }),
        queryClient.invalidateQueries({ queryKey: ['system', 'history', '12h'] }),
      ])
    } catch (err) {
      toast.push({ type: 'error', message: describeError(err, 'Refresh failed') })
    }
  }

  const stats = statsQ.data
  const containers = Array.isArray(containersQ.data) ? containersQ.data : []
  const tunnelList = Array.isArray(tunnelsQ.data) ? tunnelsQ.data : []

  // Index projects by container_name so DockerCard can look up its project.
  const projectByContainer = useMemo(() => {
    const list = Array.isArray(projectsQ.data) ? projectsQ.data : []
    const map = new Map()
    for (const p of list) {
      if (p && p.container_name) {
        map.set(p.container_name, p)
      }
    }
    return map
  }, [projectsQ.data])

  const runningTunnels = tunnelList.filter(
    (t) => (t.activeState || '').toLowerCase() === 'active'
  ).length
  const totalRoutes = tunnelList.reduce(
    (acc, t) =>
      acc +
      (Array.isArray(t.ingress)
        ? t.ingress.filter((r) => !r.catchall && r.hostname && r.hostname !== '*').length
        : 0),
    0
  )

  const cpuSeries = useMemo(() => {
    const cpu = historyShortQ.data?.cpu
    if (!Array.isArray(cpu)) return []
    return cpu.map((s) => ({ timestamp: s.timestamp, value: Number(s.usage_percent ?? 0) }))
  }, [historyShortQ.data])

  const memSeries = useMemo(() => {
    const mem = historyShortQ.data?.memory
    if (!Array.isArray(mem)) return []
    return mem.map((s) => ({ timestamp: s.timestamp, value: Number(s.used_percent ?? 0) }))
  }, [historyShortQ.data])

  const diskSeries = useMemo(() => {
    const disk = historyLongQ.data?.disk
    if (!Array.isArray(disk)) return []
    return disk.map((s) => ({ timestamp: s.timestamp, value: Number(s.used_percent ?? 0) }))
  }, [historyLongQ.data])

  const netSeries = useMemo(
    () => computeNetRates(historyShortQ.data?.network),
    [historyShortQ.data]
  )

  // Latest network rates for the Network stat card.
  const latestRates = netSeries.length > 0 ? netSeries[netSeries.length - 1] : null

  if (statsQ.isLoading) {
    return (
      <div className="loading-state">
        <Spinner size={32} />
        <p>Loading dashboard...</p>
      </div>
    )
  }

  if (statsQ.isError) {
    return (
      <div className="dashboard">
        <div className="page-header">
          <h1>Dashboard</h1>
          <p>Could not load system stats.</p>
        </div>
        <EmptyState
          icon={<FiServer size={40} />}
          title="Stats unavailable"
          description={describeError(statsQ.error, 'Try refreshing in a moment')}
          action={
            <button className="refresh-btn glass" onClick={handleRefresh}>
              <FiRefreshCw />
              Retry
            </button>
          }
        />
      </div>
    )
  }

  const cpuUsage = stats?.cpu?.usagePercent
  const cpuLoad = stats?.cpu?.load1
  const memUsed = stats?.memory?.usedPercent
  const diskUsed = stats?.disk?.usedPercent
  const hostname = stats?.host?.hostname

  return (
    <div className="dashboard">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>Dashboard</h1>
            <p>System monitoring, Docker &amp; Tunnel management</p>
          </div>
          <div className="header-actions">
            {lastUpdated && (
              <span className="last-updated">Last updated {formatTimeOfDay(lastUpdated)}</span>
            )}
            <button
              type="button"
              className="refresh-btn glass"
              onClick={handleRefresh}
              disabled={refreshing}
            >
              <FiRefreshCw className={refreshing ? 'spinning' : ''} />
              Refresh
            </button>
          </div>
        </div>
      </div>

      {stats && (
        <div className="stats-grid">
          <div className="stat-card glass animate-in" style={{ animationDelay: '0.05s' }}>
            <div className="stat-icon cpu">
              <FiCpu />
            </div>
            <div className="stat-info">
              <span className="stat-label">CPU Usage</span>
              <span className="stat-value">{(cpuUsage ?? 0).toFixed(1)}%</span>
              <span className="stat-sub">
                load {Number(cpuLoad ?? 0).toFixed(2)} / {stats.cpu?.cores ?? 0} cores
              </span>
            </div>
            <div className="stat-bar">
              <div
                className="stat-bar-fill cpu"
                style={{ width: `${Math.min(cpuUsage ?? 0, 100)}%` }}
              />
            </div>
          </div>

          <div className="stat-card glass animate-in" style={{ animationDelay: '0.1s' }}>
            <div className="stat-icon ram">
              <FiHardDrive />
            </div>
            <div className="stat-info">
              <span className="stat-label">Memory</span>
              <span className="stat-value">{(memUsed ?? 0).toFixed(1)}%</span>
              <span className="stat-sub">
                {formatBytes(stats.memory?.used)} / {formatBytes(stats.memory?.total)}
              </span>
            </div>
            <div className="stat-bar">
              <div
                className="stat-bar-fill ram"
                style={{ width: `${Math.min(memUsed ?? 0, 100)}%` }}
              />
            </div>
          </div>

          <div className="stat-card glass animate-in" style={{ animationDelay: '0.15s' }}>
            <div className="stat-icon storage">
              <FiDatabase />
            </div>
            <div className="stat-info">
              <span className="stat-label">Storage</span>
              <span className="stat-value">{(diskUsed ?? 0).toFixed(1)}%</span>
              <span className="stat-sub">
                {formatBytes(stats.disk?.used)} / {formatBytes(stats.disk?.total)}
              </span>
            </div>
            <div className="stat-bar">
              <div
                className="stat-bar-fill storage"
                style={{ width: `${Math.min(diskUsed ?? 0, 100)}%` }}
              />
            </div>
          </div>

          <div className="stat-card glass animate-in" style={{ animationDelay: '0.2s' }}>
            <div className="stat-icon uptime">
              <FiClock />
            </div>
            <div className="stat-info">
              <span className="stat-label">Uptime</span>
              <span className="stat-value">{formatUptime(stats.host?.uptime)}</span>
              <span className="stat-sub">{hostname || '-'}</span>
            </div>
          </div>

          <div className="stat-card glass animate-in" style={{ animationDelay: '0.25s' }}>
            <div className="stat-icon tunnel">
              <FiCloud />
            </div>
            <div className="stat-info">
              <span className="stat-label">CF Tunnels</span>
              <span className="stat-value">
                {runningTunnels}/{tunnelList.length}
              </span>
              <span className="stat-sub">{totalRoutes} routes active</span>
            </div>
          </div>

          <div className="stat-card glass animate-in" style={{ animationDelay: '0.3s' }}>
            <div className="stat-icon network">
              <FiActivity />
            </div>
            <div className="stat-info">
              <span className="stat-label">Network</span>
              <span className="stat-value net-stat">
                <span className="net-down">↓ {humanizeBytesPerSec(latestRates?.rx)}</span>
                <span className="net-up">↑ {humanizeBytesPerSec(latestRates?.tx)}</span>
              </span>
              <span className="stat-sub">
                {latestRates ? 'rx / tx (last 1m)' : 'awaiting samples'}
              </span>
            </div>
          </div>

          <RecentAlertsCard alertsQ={alertsQ} />
          <RecentDeploymentsCard
            eventsQ={deployEventsQ}
            projectsList={Array.isArray(projectsQ.data) ? projectsQ.data : []}
          />
          <BackupsStatusCard backupsQ={backupsQ} />
        </div>
      )}

      <div className="section-header">
        <div className="section-title">
          <FiActivity />
          <h2>Trends</h2>
        </div>
      </div>

      {historyUnavailable ? (
        <div className="banner banner-warning">
          History recorder is warming up. Trend charts will appear after the first sample.
        </div>
      ) : (
        <div className="charts-grid">
          <ChartCard
            title="CPU"
            subtitle="usage % • last 1h"
            color="var(--chart-cpu)"
            unit="%"
            series={cpuSeries}
            loading={historyShortQ.isLoading}
          />
          <ChartCard
            title="Memory"
            subtitle="used % • last 1h"
            color="var(--chart-mem)"
            unit="%"
            series={memSeries}
            loading={historyShortQ.isLoading}
          />
          <ChartCard
            title="Disk"
            subtitle="used % • last 12h"
            color="var(--chart-disk)"
            unit="%"
            series={diskSeries}
            loading={historyLongQ.isLoading}
          />
          <ChartCard
            title="Network"
            subtitle="rx / tx • last 1h"
            color="var(--chart-net-rx)"
            secondColor="var(--chart-net-tx)"
            valueFormatter={humanizeBytesPerSec}
            multi={netSeries.map((s) => ({
              timestamp: s.timestamp,
              rx: s.rx,
              tx: s.tx,
            }))}
            loading={historyShortQ.isLoading}
          />
        </div>
      )}

      <div className="section-header" style={{ marginTop: '40px' }}>
        <div className="section-title">
          <FiCloud />
          <h2>Cloudflare Tunnels</h2>
          <span className="container-count tunnel-count">{tunnelList.length}</span>
        </div>
      </div>

      {tunnelsQ.isLoading ? (
        <div className="loading-state"><Spinner size={20} /></div>
      ) : tunnelList.length === 0 ? (
        <EmptyState
          icon={<FiCloud size={40} />}
          title="No tunnels found"
          description="Cloudflare tunnels will appear here when detected"
        />
      ) : (
        <div className="tunnel-grid">
          {tunnelList.map((tunnel, i) => (
            <TunnelCard
              key={tunnel.id || tunnel.serviceName || i}
              tunnel={tunnel}
              onRestart={() => queryClient.invalidateQueries({ queryKey: ['system', 'tunnels'] })}
            />
          ))}
        </div>
      )}

      <div className="section-header" style={{ marginTop: '40px' }}>
        <div className="section-title">
          <FiServer />
          <h2>Docker Containers</h2>
          <span className="container-count">{containers.length}</span>
        </div>
      </div>

      {dockerUnavailable ? (
        <div className="banner banner-warning">
          Docker is not available on the server. Container actions are disabled.
        </div>
      ) : containersQ.isLoading ? (
        <div className="loading-state"><Spinner size={20} /></div>
      ) : containers.length === 0 ? (
        <EmptyState
          icon={<FiServer size={40} />}
          title="No containers found"
          description="Docker containers will appear here when detected"
        />
      ) : (
        <div className="docker-grid">
          {containers.map((container, i) => (
            <DockerCard
              key={container.id || container.name || i}
              container={container}
              project={projectByContainer.get(container.name) || null}
              onRefresh={() => queryClient.invalidateQueries({ queryKey: ['docker', 'containers'] })}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function ChartCard({
  title,
  subtitle,
  color,
  secondColor,
  unit,
  valueFormatter,
  series,
  multi,
  loading,
}) {
  const hasData = (series && series.length > 1) || (multi && multi.length > 1)
  return (
    <div className="chart-card glass">
      <div className="chart-card-header">
        <div>
          <h4>{title}</h4>
          <span className="chart-card-sub">{subtitle}</span>
        </div>
      </div>
      <div className="chart-card-body">
        {loading ? (
          <div className="chart-empty"><Spinner size={16} /></div>
        ) : !hasData ? (
          <div className="chart-empty">no data yet</div>
        ) : (
          <Sparkline
            series={series}
            multi={multi}
            color={color}
            secondColor={secondColor}
            unit={unit}
            valueFormatter={valueFormatter}
          />
        )}
      </div>
    </div>
  )
}

// RecentAlertsCard renders the top-5 alert events. Clicks navigate to
// /events with the alert category preselected. Renders gracefully on
// error or while loading; never blocks the dashboard.
function RecentAlertsCard({ alertsQ }) {
  const list = Array.isArray(alertsQ.data?.data) ? alertsQ.data.data : []
  return (
    <Link
      to="/events?category=alert"
      className="stat-card alerts-card glass animate-in"
      style={{ animationDelay: '0.35s' }}
    >
      <div className="alerts-head">
        <div className="stat-icon alerts">
          <FiBell />
        </div>
        <div>
          <span className="stat-label">Recent alerts</span>
          <span className="alerts-link">View all</span>
        </div>
      </div>
      {alertsQ.isLoading ? (
        <div className="alerts-empty"><Spinner size={14} /></div>
      ) : alertsQ.isError ? (
        <div className="alerts-empty">unavailable</div>
      ) : list.length === 0 ? (
        <div className="alerts-empty">No recent alerts</div>
      ) : (
        <ul className="alerts-list">
          {list.map((ev) => (
            <li key={ev.id} className="alerts-item">
              <SeverityBadge severity={ev.severity} dot />
              <span className="alerts-msg" title={ev.message}>{ev.message || ev.category}</span>
              <span className="alerts-time">
                <RelativeTime value={ev.ts} />
              </span>
            </li>
          ))}
        </ul>
      )}
    </Link>
  )
}

// RecentDeploymentsCard polls /events?category=deploy. Each row links
// directly to the project drawer's deployments tab so the user can
// jump from "deployment finished" notification to the run output.
function RecentDeploymentsCard({ eventsQ, projectsList }) {
  const list = Array.isArray(eventsQ.data?.data) ? eventsQ.data.data : []
  const projectById = useMemo(() => {
    const map = new Map()
    for (const p of projectsList || []) {
      if (p?.id) map.set(p.id, p)
    }
    return map
  }, [projectsList])

  return (
    <div
      className="stat-card alerts-card glass animate-in"
      style={{ animationDelay: '0.4s' }}
    >
      <div className="alerts-head">
        <div className="stat-icon alerts deploy">
          <FiUploadCloud />
        </div>
        <div>
          <span className="stat-label">Recent deployments</span>
          <Link to="/projects" className="alerts-link">View projects</Link>
        </div>
      </div>
      {eventsQ.isLoading ? (
        <div className="alerts-empty"><Spinner size={14} /></div>
      ) : eventsQ.isError ? (
        <div className="alerts-empty">unavailable</div>
      ) : list.length === 0 ? (
        <div className="alerts-empty">No recent deployments</div>
      ) : (
        <ul className="alerts-list">
          {list.map((ev) => {
            const status = (ev.data && ev.data.status) || inferStatusFromMessage(ev)
            const project = ev.project_id ? projectById.get(ev.project_id) : null
            const projectName = project?.name || ev.project_id || 'unknown'
            const href = ev.project_id
              ? `/projects?focus=${encodeURIComponent(ev.project_id)}&tab=deployments`
              : '/projects'
            return (
              <li key={ev.id} className="alerts-item deploy-item">
                <Link to={href} className="deploy-link" title={ev.message}>
                  <DeploymentStatusBadge status={status} />
                  <span className="alerts-msg">{projectName}</span>
                  <span className="alerts-time">
                    <RelativeTime value={ev.ts} />
                  </span>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

// BackupsStatusCard surfaces the most recent successful backup. The card
// turns amber when the latest is older than 36h, red when the most
// recent attempt failed, green otherwise.
function BackupsStatusCard({ backupsQ }) {
  const list = Array.isArray(backupsQ.data) ? backupsQ.data : []
  const lastSuccess = list.find((b) => b.ok)
  const latest = list[0]

  // Use the timestamp of the latest successful query fetch as our
  // "now" reference; it's stored in react state and updates after each
  // poll, which is sufficient for an "older than 36h" stale check.
  const referenceMs = backupsQ.dataUpdatedAt || 0
  const STALE_MS = 36 * 3600 * 1000
  const lastSuccessAge =
    lastSuccess && referenceMs > 0
      ? referenceMs - new Date(lastSuccess.ts).getTime()
      : Infinity

  let badgeClass = 'backup-ok'
  let badgeLabel = 'OK'

  if (latest && !latest.ok) {
    badgeClass = 'backup-fail'
    badgeLabel = 'FAIL'
  } else if (!lastSuccess || lastSuccessAge > STALE_MS) {
    badgeClass = 'backup-stale'
    badgeLabel = 'Stale'
  }

  const subtitle = lastSuccess
    ? `last success ${formatRelative(lastSuccess.ts)}`
    : 'no successful backup'

  return (
    <Link
      to="/backups"
      className="stat-card backups-tile glass animate-in"
      style={{ animationDelay: '0.45s' }}
    >
      <div className="alerts-head">
        <div className="stat-icon storage">
          <FiDatabase />
        </div>
        <div>
          <span className="stat-label">Last backup</span>
          <span className="alerts-link">View backups</span>
        </div>
      </div>
      {backupsQ.isLoading ? (
        <div className="alerts-empty"><Spinner size={14} /></div>
      ) : backupsQ.isError ? (
        <div className="alerts-empty">unavailable</div>
      ) : list.length === 0 ? (
        <div className="alerts-empty">No backups yet</div>
      ) : (
        <div className="backups-tile-body">
          <div className={`backups-tile-badge ${badgeClass}`}>{badgeLabel}</div>
          <div className="backups-tile-meta">
            <div className="backups-tile-time">
              <RelativeTime value={latest?.ts} />
            </div>
            <div className="backups-tile-sub">{subtitle}</div>
          </div>
        </div>
      )}
    </Link>
  )
}

function inferStatusFromMessage(ev) {
  const msg = (ev?.message || '').toLowerCase()
  if (msg.includes('success')) return 'success'
  if (msg.includes('failed') || msg.includes('failure')) return 'failed'
  if (msg.includes('timeout')) return 'timeout'
  if (msg.includes('running') || msg.includes('started')) return 'running'
  return 'pending'
}
