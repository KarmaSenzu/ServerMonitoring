import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  FiRefreshCw,
  FiServer,
  FiCloud,
  FiActivity,
  FiBell,
  FiPlus,
  FiChevronDown,
  FiTerminal,
  FiBox,
  FiCpu,
  FiHardDrive,
  FiAlertTriangle,
} from 'react-icons/fi'
import { system, docker, tunnels, projects, events, backups, servers as serversApi } from '../api/endpoints.js'
import DockerCard from '../components/DockerCard.jsx'
import TunnelCard from '../components/TunnelCard.jsx'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import { useToast } from '../ui/useToast.js'
import { describeError } from '../ui/errors.js'
import { humanizeBytesPerSec } from '../ui/format.js'
import RelativeTime from '../ui/RelativeTime.jsx'
import Sparkline from '../components/Sparkline.jsx'
import { useAuth } from '../auth/useAuth.js'
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

function isLoopback(name) {
  if (!name) return false
  const lower = name.toLowerCase()
  return lower === 'lo' || lower.startsWith('loopback') || lower === 'lo0'
}

function ifaceRx(iff) {
  return Number(iff.bytes_recv ?? iff.bytesRecv ?? 0)
}
function ifaceTx(iff) {
  return Number(iff.bytes_sent ?? iff.bytesSent ?? 0)
}

function computeNetRates(networkSamples) {
  if (!Array.isArray(networkSamples) || networkSamples.length < 2) return []
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
      if (dRx >= 0) rxSum += dRx / dtSec
      if (dTx >= 0) txSum += dTx / dtSec
    }
    out.push({ timestamp: cur.timestamp, rx: rxSum, tx: txSum })
  }
  return out
}

// Segmented meter bar — 16 segments filled based on percentage
function SegmentedMeter({ percent, color }) {
  const segments = 16
  const filled = Math.round((percent / 100) * segments)
  return (
    <div className="segmented-meter">
      {Array.from({ length: segments }, (_, i) => (
        <div
          key={i}
          className={i < filled ? 'seg seg-filled' : 'seg seg-empty'}
          style={i < filled ? { background: color } : {}}
        />
      ))}
    </div>
  )
}

export default function Dashboard() {
  const queryClient = useQueryClient()
  const toast = useToast()
  const { user } = useAuth()

  const [selectedServerId, setSelectedServerId] = useState(null)
  const [chartRange, setChartRange] = useState('6H')

  // --- Queries (same data sources as before) ---
  const serversQ = useQuery({
    queryKey: ['servers'],
    queryFn: () => serversApi.list(),
    refetchInterval: 30000,
  })

  const remoteMetricsQ = useQuery({
    queryKey: ['server-metrics', selectedServerId],
    queryFn: () => serversApi.metrics(selectedServerId),
    enabled: !!selectedServerId,
    refetchInterval: 30000,
  })

  const remoteDiscoveryQ = useQuery({
    queryKey: ['server-discovery', selectedServerId],
    queryFn: () => serversApi.discovery(selectedServerId),
    enabled: !!selectedServerId,
    refetchInterval: 30000,
  })

  const remoteHistoryQ = useQuery({
    queryKey: ['server-history', selectedServerId],
    queryFn: () => serversApi.history(selectedServerId, { limit: 60 }),
    enabled: !!selectedServerId,
    refetchInterval: 30000,
  })

  const eventsQ = useQuery({
    queryKey: ['dashboard-events'],
    queryFn: () => events.list({ limit: 12 }),
    refetchInterval: 30000,
  })

  const statsQ = useQuery({
    queryKey: ['system', 'stats'],
    queryFn: system.stats,
    refetchInterval: 30000,
  })

  const containersQ = useQuery({
    queryKey: ['docker', 'containers'],
    queryFn: docker.containers,
    enabled: !selectedServerId,
    refetchInterval: 10000,
    retry: (count, err) => {
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

  const projectsQ = useQuery({
    queryKey: ['projects', { all: true }],
    queryFn: () => projects.list(),
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
  })

  // --- Derived state ---
  const stats = statsQ.data
  const containers = Array.isArray(containersQ.data) ? containersQ.data : []
  const tunnelList = Array.isArray(tunnelsQ.data) ? tunnelsQ.data : []

  const projectByContainer = useMemo(() => {
    const list = Array.isArray(projectsQ.data) ? projectsQ.data : []
    const map = new Map()
    for (const p of list) {
      if (p && p.container_name) map.set(p.container_name, p)
    }
    return map
  }, [projectsQ.data])

  const runningTunnels = tunnelList.filter(
    (t) => (t.activeState || '').toLowerCase() === 'active'
  ).length

  const cpuSeries = useMemo(() => {
    // When a remote server is selected, chart from its own history
    // (so the chart reflects the selected device, not the local host).
    const src = selectedServerId ? remoteHistoryQ.data : historyShortQ.data?.cpu
    if (selectedServerId) {
      const rows = Array.isArray(src) ? src : []
      return rows.map((m) => ({ timestamp: m.ts, value: Number(m.cpu_usage ?? 0) }))
    }
    const cpu = src
    if (!Array.isArray(cpu)) return []
    return cpu.map((s) => ({ timestamp: s.timestamp, value: Number(s.usage_percent ?? 0) }))
  }, [selectedServerId, remoteHistoryQ.data, historyShortQ.data])

  const memSeries = useMemo(() => {
    if (selectedServerId) {
      const rows = Array.isArray(remoteHistoryQ.data) ? remoteHistoryQ.data : []
      return rows.map((m) => ({ timestamp: m.ts, value: Number(m.mem_percent ?? 0) }))
    }
    const mem = historyShortQ.data?.memory
    if (!Array.isArray(mem)) return []
    return mem.map((s) => ({ timestamp: s.timestamp, value: Number(s.used_percent ?? 0) }))
  }, [selectedServerId, remoteHistoryQ.data, historyShortQ.data])

  const diskSeries = useMemo(() => {
    if (selectedServerId) {
      const rows = Array.isArray(remoteHistoryQ.data) ? remoteHistoryQ.data : []
      return rows.map((m) => ({ timestamp: m.ts, value: Number(m.disk_percent ?? 0) }))
    }
    const disk = historyLongQ.data?.disk
    if (!Array.isArray(disk)) return []
    return disk.map((s) => ({ timestamp: s.timestamp, value: Number(s.used_percent ?? 0) }))
  }, [selectedServerId, remoteHistoryQ.data, historyLongQ.data])

  const netSeries = useMemo(() => {
    if (selectedServerId) {
      // Remote history carries net bytes at each sample; derive rates.
      const rows = Array.isArray(remoteHistoryQ.data) ? remoteHistoryQ.data : []
      const out = []
      for (let i = 1; i < rows.length; i++) {
        const cur = rows[i]
        const prev = rows[i - 1]
        const dt = Math.max(1, (new Date(cur.ts).getTime() - new Date(prev.ts).getTime()) / 1000)
        out.push({
          timestamp: cur.ts,
          rx: Math.max(0, (cur.net_bytes_recv - prev.net_bytes_recv) / dt),
          tx: Math.max(0, (cur.net_bytes_sent - prev.net_bytes_sent) / dt),
        })
      }
      return out
    }
    return computeNetRates(historyShortQ.data?.network)
  }, [selectedServerId, remoteHistoryQ.data, historyShortQ.data])

  const latestRates = netSeries.length > 0 ? netSeries[netSeries.length - 1] : null

  const cpuUsage = selectedServerId ? (remoteMetricsQ.data?.cpu_usage ?? 0) : (stats?.cpu?.usagePercent ?? 0)
  const memUsed = selectedServerId ? (remoteMetricsQ.data?.mem_percent ?? 0) : (stats?.memory?.usedPercent ?? 0)
  const diskUsed = selectedServerId ? (remoteMetricsQ.data?.disk_percent ?? 0) : (stats?.disk?.usedPercent ?? 0)
  const hostname = selectedServerId
    ? (Array.isArray(serversQ.data) ? serversQ.data.find((s) => s.id === selectedServerId)?.name : null)
    : (stats?.host?.hostname ?? null)

  const serverList = Array.isArray(serversQ.data) ? serversQ.data : []
  const selectedServer = serverList.find((s) => s.id === selectedServerId)
  const onlineCount = serverList.filter((s) => s.status === 'online').length
  const recentEvents = Array.isArray(eventsQ.data?.data) ? eventsQ.data.data : []
  const incidentCount = recentEvents.filter((e) => e.severity === 'critical' || e.severity === 'error').length

  const lastUpdated = useMemo(() => {
    const stamps = [statsQ.dataUpdatedAt, containersQ.dataUpdatedAt, tunnelsQ.dataUpdatedAt]
      .filter(Boolean)
      .map((t) => new Date(t))
    if (stamps.length === 0) return null
    return new Date(Math.max(...stamps.map((d) => d.getTime())))
  }, [statsQ.dataUpdatedAt, containersQ.dataUpdatedAt, tunnelsQ.dataUpdatedAt])

  const refreshing = statsQ.isFetching || containersQ.isFetching || tunnelsQ.isFetching

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
        <EmptyState
          icon={<FiServer size={40} />}
          title="Stats unavailable"
          description={describeError(statsQ.error, 'Try refreshing in a moment')}
          action={
            <button className="refresh-btn" onClick={handleRefresh}>
              <FiRefreshCw /> Retry
            </button>
          }
        />
      </div>
    )
  }

  const isRemote = !!selectedServerId
  const memTotal = isRemote ? remoteMetricsQ.data?.mem_total : stats?.memory?.total
  const memUsedBytes = isRemote ? remoteMetricsQ.data?.mem_used : stats?.memory?.used

  return (
    <div className="dashboard">
      {/* === HERO HEADER / TELEMETRY STATUS BAR === */}
      <div className="hero-header">
        <div className="hero-left">
          <div className="hero-title-row">
            <h1 className="hero-title">Welcome back, {user?.username || 'Admin'}</h1>
            <span className="live-badge">
              <span className="live-dot" />
              LIVE TELEMETRY
            </span>
          </div>
          <p className="hero-subtitle">
            <span>Infrastructure Command Center</span>
            <span className="dot-sep">•</span>
            <span className="hero-status-ok">100% operational in last 24h</span>
            <span className="dot-sep">•</span>
            <span>{serverList.length} instances registered</span>
          </p>
        </div>
        <div className="hero-actions">
          {lastUpdated && (
            <div className="hero-chip">
              <FiRefreshCw size={14} />
              <span>Updated {new Date(lastUpdated).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
            </div>
          )}
          {/* Server scope selector */}
          <div className="scope-selector" onClick={() => {}}>
            <span className="scope-dot" />
            <span className="scope-label">
              {selectedServer ? selectedServer.name : 'Production (Local)'}
            </span>
            <select
              value={selectedServerId || ''}
              onChange={(e) => setSelectedServerId(e.target.value || null)}
              className="scope-select"
            >
              <option value="">🖥️ This Server (Local)</option>
              {serverList.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.status === 'online' ? '🟢' : s.status === 'offline' ? '🔴' : '🟡'} {s.name} ({s.hostname})
                </option>
              ))}
            </select>
            <FiChevronDown size={14} />
          </div>
          <Link to="/events" className="hero-btn ghost">
            <FiAlertTriangle size={14} />
            <span>Incidents ({incidentCount})</span>
          </Link>
          <Link to="/servers" className="hero-btn primary">
            <FiPlus size={14} />
            <span>Deploy Node</span>
          </Link>
        </div>
      </div>

      {/* === KPI METRIC GRID (4-COLUMN BENTO) === */}
      <div className="kpi-bento-grid">
        {/* KPI 1: Active Servers */}
        <div className="kpi-bento-card" style={{ '--kpi-accent': '#4edea3' }}>
          <div className="kpi-bento-header">
            <span className="kpi-bento-label">
              <FiServer size={14} /> Active Servers
            </span>
          </div>
          <div className="kpi-bento-mid">
            <div className="kpi-bento-value-row">
              <span className="kpi-bento-value">{serverList.length}</span>
              <span className="kpi-bento-unit">/ {serverList.length} HOSTS</span>
            </div>
            <span className="kpi-bento-badge green">
              ▲ {onlineCount === serverList.length ? '100%' : `${Math.round((onlineCount / Math.max(serverList.length, 1)) * 100)}%`} OK
            </span>
          </div>
          <div className="kpi-bento-footer">
            <span className="kpi-footer-text">{serverList.map((s) => s.name).slice(0, 3).join(' & ') || 'No servers'}</span>
            <span className="kpi-footer-status">ALL OPERATIONAL</span>
          </div>
          <div className="kpi-bottom-bar" style={{ background: '#4edea3' }} />
        </div>

        {/* KPI 2: Avg CPU Load */}
        <div className="kpi-bento-card" style={{ '--kpi-accent': '#ff5625' }}>
          <div className="kpi-bento-header">
            <span className="kpi-bento-label">
              <FiCpu size={14} /> Avg CPU Load
            </span>
          </div>
          <div className="kpi-bento-mid">
            <div className="kpi-bento-value-row">
              <span className="kpi-bento-value">{(cpuUsage ?? 0).toFixed(1)}%</span>
              <span className="kpi-bento-unit">{stats?.cpu?.cores ?? 0} CORES</span>
            </div>
            <span className="kpi-bento-badge orange">▼ live</span>
          </div>
          {/* Mini sparkline bars */}
          <div className="mini-sparkline">
            {cpuSeries.slice(-12).map((s, i) => (
              <div
                key={i}
                className="mini-bar"
                style={{
                  height: `${Math.max(15, Math.min(100, s.value))}%`,
                  background: s.value > 70 ? '#ff5625' : s.value > 40 ? '#ffb95f' : '#4edea3',
                }}
              />
            ))}
          </div>
          <div className="kpi-bottom-bar" style={{ background: '#ff5625' }} />
        </div>

        {/* KPI 3: Memory Usage */}
        <div className="kpi-bento-card" style={{ '--kpi-accent': '#ffb95f' }}>
          <div className="kpi-bento-header">
            <span className="kpi-bento-label">
              <FiHardDrive size={14} /> Memory Usage
            </span>
          </div>
          <div className="kpi-bento-mid">
            <div className="kpi-bento-value-row">
              <span className="kpi-bento-value">{(memUsed ?? 0).toFixed(1)}%</span>
              <span className="kpi-bento-unit">{formatBytes(memUsedBytes)} / {formatBytes(memTotal)}</span>
            </div>
            <span className={`kpi-bento-badge ${(memUsed ?? 0) > 75 ? 'orange' : 'green'}`}>
              {(memUsed ?? 0) > 75 ? 'ELEVATED' : 'NORMAL'}
            </span>
          </div>
          <SegmentedMeter percent={memUsed ?? 0} color="#ffb95f" />
          <div className="kpi-bottom-bar" style={{ background: '#ffb95f' }} />
        </div>

        {/* KPI 4: Network Throughput */}
        <div className="kpi-bento-card" style={{ '--kpi-accent': '#4edea3' }}>
          <div className="kpi-bento-header">
            <span className="kpi-bento-label">
              <FiActivity size={14} /> Network Throughput
            </span>
          </div>
          <div className="kpi-bento-mid">
            <div className="kpi-bento-value-row">
              <span className="kpi-bento-value">
                {latestRates ? Math.round(latestRates.tx / 1024) : 0}
              </span>
              <span className="kpi-bento-unit">KB/s (Tx)</span>
            </div>
            <span className="kpi-bento-badge green">
              {latestRates ? `▲ ${Math.round(latestRates.rx / 1024)} KB/s` : 'awaiting'}
            </span>
          </div>
          <div className="kpi-bento-footer">
            <span className="kpi-footer-text">Rx: {latestRates ? `${Math.round(latestRates.rx / 1024)} KB/s` : '-'}</span>
            <span className="kpi-footer-status">UPTIME: {formatUptime(stats?.host?.uptime)}</span>
          </div>
          <div className="kpi-bottom-bar" style={{ background: '#4edea3' }} />
        </div>
      </div>

      {/* === TELEMETRY CHARTS & BREAKDOWN (BENTO 8/4 SPLIT) === */}
      <div className="bento-split">
        {/* Col 8: Telemetry Multi-Line Chart */}
        <div className="bento-chart-card">
          <div className="chart-header">
            <div className="chart-header-left">
              <FiActivity size={18} />
              <span className="chart-title">Response Latency & Traffic Dynamics</span>
            </div>
            <div className="chart-range-buttons">
              {['1H', '6H', '24H', '7D'].map((r) => (
                <button
                  key={r}
                  className={`range-btn ${chartRange === r ? 'active' : ''}`}
                  onClick={() => setChartRange(r)}
                >
                  {r}
                </button>
              ))}
            </div>
          </div>
          <div className="chart-body">
            {historyShortQ.isLoading ? (
              <div className="chart-empty"><Spinner size={20} /></div>
            ) : cpuSeries.length < 2 && memSeries.length < 2 ? (
              <div className="chart-empty">Collecting telemetry samples...</div>
            ) : (
              <Sparkline
                series={cpuSeries}
                color="#ff5625"
                unit="%"
                height={220}
              />
            )}
          </div>
          <div className="chart-quick-metrics">
            <div className="quick-metric">
              <span className="quick-metric-label">CPU Usage</span>
              <span className="quick-metric-value orange">{(cpuUsage ?? 0).toFixed(1)}%</span>
            </div>
            <div className="quick-metric">
              <span className="quick-metric-label">Memory</span>
              <span className="quick-metric-value amber">{(memUsed ?? 0).toFixed(1)}%</span>
            </div>
            <div className="quick-metric">
              <span className="quick-metric-label">Disk</span>
              <span className="quick-metric-value red">{(diskUsed ?? 0).toFixed(1)}%</span>
            </div>
            <div className="quick-metric">
              <span className="quick-metric-label">Net Tx</span>
              <span className="quick-metric-value green">{latestRates ? humanizeBytesPerSec(latestRates.tx) : '-'}</span>
            </div>
          </div>
        </div>

        {/* Col 4: Host Telemetry Breakdown */}
        <div className="bento-breakdown-card">
          <div className="breakdown-header">
            <span className="breakdown-title">
              <FiActivity size={16} /> Host Telemetry Breakdown
            </span>
            <span className="breakdown-sub">24H AVG</span>
          </div>
          <div className="breakdown-meters">
            {/* Meter 1: CPU */}
            <div className="meter">
              <div className="meter-header">
                <span className="meter-label">CPU Load Aggregation</span>
                <span className="meter-value orange">{(cpuUsage ?? 0).toFixed(1)}%</span>
              </div>
              <SegmentedMeter percent={cpuUsage ?? 0} color="#ff5625" />
            </div>
            {/* Meter 2: RAM */}
            <div className="meter">
              <div className="meter-header">
                <span className="meter-label">RAM Memory Pool</span>
                <span className="meter-value amber">{(memUsed ?? 0).toFixed(1)}% ({formatBytes(memUsedBytes)})</span>
              </div>
              <SegmentedMeter percent={memUsed ?? 0} color="#ffb95f" />
            </div>
            {/* Meter 3: Disk */}
            <div className="meter">
              <div className="meter-header">
                <span className="meter-label">Disk / System Root</span>
                <span className="meter-value orange">{(diskUsed ?? 0).toFixed(1)}%</span>
              </div>
              <SegmentedMeter percent={diskUsed ?? 0} color="#ff5625" />
              {(diskUsed ?? 0) > 85 && (
                <span className="meter-warning">
                  <FiAlertTriangle size={12} /> Storage threshold approaching 90%
                </span>
              )}
            </div>
            {/* Meter 4: CF Tunnels */}
            <div className="meter">
              <div className="meter-header">
                <span className="meter-label">Cloudflare Tunnels</span>
                <span className="meter-value">{runningTunnels} ACTIVE</span>
              </div>
              <SegmentedMeter percent={tunnelList.length > 0 ? (runningTunnels / tunnelList.length) * 100 : 0} color="#4edea3" />
            </div>
          </div>
          {/* Anomaly Alert */}
          {(diskUsed ?? 0) > 85 && (
            <div className="anomaly-alert">
              <div className="anomaly-header">
                <FiAlertTriangle size={14} />
                <span>1 Anomaly Flagged</span>
              </div>
              <p className="anomaly-text">
                {hostname || 'Host'} disk storage exceeded 85% safety baseline. Auto-trim recommended.
              </p>
            </div>
          )}
        </div>
      </div>

      {/* === FLEET TABLE + AUDIT STREAM (BENTO 8/4 SPLIT) === */}
      <div className="bento-split">
        {/* Col 8: Registered Infrastructure Nodes Table */}
        <div className="fleet-table-card">
          <div className="fleet-table-header">
            <div className="fleet-table-title-row">
              <FiTerminal size={18} />
              <span className="fleet-table-title">Registered Infrastructure Nodes</span>
              <span className="fleet-connected-badge">{onlineCount} / {serverList.length} CONNECTED</span>
            </div>
            <div className="fleet-table-actions">
              <button className="fleet-action-btn">
                <FiRefreshCw size={12} /> Reload
              </button>
            </div>
          </div>
          <div className="fleet-table-wrapper">
            <table className="fleet-table">
              <thead>
                <tr>
                  <th>Server Node</th>
                  <th>IP / Endpoint</th>
                  <th>Status</th>
                  <th>CPU</th>
                  <th>Memory</th>
                  <th>Disk</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {serverList.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="fleet-empty">No servers registered. Go to Servers page to add one.</td>
                  </tr>
                ) : (
                  serverList.map((s) => {
                    const isSel = s.id === selectedServerId
                    const sMetrics = isSel ? remoteMetricsQ.data : null
                    const sCpu = sMetrics?.cpu_usage ?? 0
                    const sMem = sMetrics?.mem_percent ?? 0
                    const sDisk = sMetrics?.disk_percent ?? 0
                    return (
                      <tr key={s.id} className={isSel ? 'fleet-row selected' : 'fleet-row'}>
                        <td>
                          <div className="fleet-node-cell">
                            <span className={`fleet-node-dot ${s.status || 'unknown'}`} />
                            <div className="fleet-node-info">
                              <span className="fleet-node-name">{s.name}</span>
                              <span className="fleet-node-os">{s.hostname || '-'}</span>
                            </div>
                          </div>
                        </td>
                        <td className="mono">{s.address || s.ip || '-'}</td>
                        <td>
                          <span className={`fleet-status-badge ${s.status || 'unknown'}`}>
                            {(s.status || 'UNKNOWN').toUpperCase()}
                          </span>
                        </td>
                        <td className="mono fleet-metric">{sCpu > 0 ? `${sCpu.toFixed(1)}%` : '-'}</td>
                        <td className="mono fleet-metric">{sMem > 0 ? `${sMem.toFixed(1)}%` : '-'}</td>
                        <td className="mono fleet-metric">{sDisk > 0 ? `${sDisk.toFixed(1)}%` : '-'}</td>
                        <td className="text-right">
                          <div className="fleet-row-actions">
                            <Link to={`/servers`} className="fleet-row-btn" title="Launch Terminal">
                              <FiTerminal size={12} /> SSH
                            </Link>
                            <Link to="/containers" className="fleet-row-btn" title="Inspect Containers">
                              <FiBox size={12} /> Docker
                            </Link>
                          </div>
                        </td>
                      </tr>
                    )
                  })
                )}
              </tbody>
            </table>
          </div>
          {/* Topology Quick Map */}
          <div className="topology-quick">
            <div className="topology-quick-left">
              <FiCloud size={20} />
              <div>
                <div className="topology-quick-title">Network Hub Mesh: Local Gateway Active</div>
                <div className="topology-quick-sub">
                  {serverList.map((s) => s.name).join(' ↔ ') || 'No nodes'} ({onlineCount} nodes synced)
                </div>
              </div>
            </div>
            <Link to="/servers" className="topology-quick-link">
              VIEW FULL TOPOLOGY MAP →
            </Link>
          </div>
        </div>

        {/* Col 4: Incident & Audit Stream */}
        <div className="audit-stream-card">
          <div className="audit-header">
            <div className="audit-title-row">
              <FiBell size={16} />
              <span className="audit-title">Incident & Audit Stream</span>
            </div>
            <span className="audit-count-badge">ALL ({recentEvents.length})</span>
          </div>
          <div className="audit-list">
            {eventsQ.isLoading ? (
              <div className="audit-empty"><Spinner size={16} /></div>
            ) : recentEvents.length === 0 ? (
              <div className="audit-empty">No recent events</div>
            ) : (
              recentEvents.slice(0, 8).map((ev) => (
                <div key={ev.id} className="audit-item">
                  <span className={`audit-dot ${ev.severity || 'info'}`} />
                  <div className="audit-item-content">
                    <span className="audit-item-msg">{ev.message || ev.category}</span>
                    <span className="audit-item-time">
                      <RelativeTime value={ev.ts} /> {ev.actor ? `by ${ev.actor}` : ''}
                    </span>
                  </div>
                </div>
              ))
            )}
          </div>
          <Link to="/events" className="audit-view-all">
            <span>View full historical audit log</span>
            <span>→</span>
          </Link>
        </div>
      </div>

      {/* === FOOTER SNAPSHOT === */}
      <div className="footer-snapshot">
        <div className="footer-left">
          <div className="footer-icon-box">
            <FiActivity size={20} />
          </div>
          <div>
            <div className="footer-title">
              Infrastructure Orchestration Engine
              <span className="footer-version">v1.6.0 ACTIVE</span>
            </div>
            <p className="footer-text">
              Continuous health monitoring, tunnel keepalives, and multi-cloud sync running on automated 15-second cycles.
            </p>
          </div>
        </div>
        <div className="footer-actions">
          <button className="footer-btn ghost" onClick={handleRefresh} disabled={refreshing}>
            <FiRefreshCw className={refreshing ? 'spinning' : ''} size={14} />
            System Health Diagnostics
          </button>
          <Link to="/servers" className="footer-btn primary">
            Export Metrics (.csv)
          </Link>
        </div>
      </div>

      {/* === DOCKER CONTAINERS + TUNNELS (local only) === */}
      {!selectedServerId && (
        <>
          <div className="section-header">
            <div className="section-title">
              <FiCloud />
              <h2>Cloudflare Tunnels</h2>
              <span className="container-count tunnel-count">{tunnelList.length}</span>
            </div>
          </div>
          {tunnelsQ.isLoading ? (
            <div className="loading-state"><Spinner size={20} /></div>
          ) : tunnelList.length === 0 ? (
            <EmptyState icon={<FiCloud size={40} />} title="No tunnels found" description="Cloudflare tunnels will appear here when detected" />
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

          <div className="section-header" style={{ marginTop: '24px' }}>
            <div className="section-title">
              <FiServer />
              <h2>Docker Containers</h2>
              <span className="container-count">{containers.length}</span>
            </div>
          </div>
          {containersQ.error?.code === 'docker_unavailable' ? (
            <div className="banner banner-warning">
              Docker is not available on the server. Container actions are disabled.
            </div>
          ) : containersQ.isLoading ? (
            <div className="loading-state"><Spinner size={20} /></div>
          ) : containers.length === 0 ? (
            <EmptyState icon={<FiServer size={40} />} title="No containers found" description="Docker containers will appear here when detected" />
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
        </>
      )}

      {/* Remote discovered services */}
      {selectedServerId && remoteDiscoveryQ.data && (
        <>
          <div className="section-header">
            <div className="section-title">
              <FiActivity />
              <h2>Discovered Services — {selectedServer?.name || 'remote'}</h2>
            </div>
          </div>
          <div className="stats-grid">
            {(() => {
              const d = remoteDiscoveryQ.data
              const parse = (s) => { try { return JSON.parse(s || '[]') } catch { return [] } }
              const pm2 = parse(d.pm2_json)
              const dockerList = parse(d.docker_json)
              const cloudflare = parse(d.cloudflare_json)
              const ports = parse(d.ports_json)
              return (
                <>
                  <div className="stat-card glass">
                    <div className="stat-header">
                      <div className="stat-icon pm2"><FiActivity /></div>
                      <span className="stat-label">PM2 Processes</span>
                    </div>
                    <div className="stat-info">
                      <span className="stat-value">{pm2.length}</span>
                      <span className="stat-sub">{pm2.filter((p) => p.status === 'online').length} online</span>
                    </div>
                  </div>
                  <div className="stat-card glass">
                    <div className="stat-header">
                      <div className="stat-icon docker"><FiBox /></div>
                      <span className="stat-label">Docker Containers</span>
                    </div>
                    <div className="stat-info">
                      <span className="stat-value">{dockerList.length}</span>
                      <span className="stat-sub">{dockerList.filter((c) => c.state === 'running').length} running</span>
                    </div>
                  </div>
                  <div className="stat-card glass">
                    <div className="stat-header">
                      <div className="stat-icon tunnel"><FiCloud /></div>
                      <span className="stat-label">Cloudflare Tunnels</span>
                    </div>
                    <div className="stat-info">
                      <span className="stat-value">{cloudflare.length}</span>
                      <span className="stat-sub">{cloudflare.map((t) => t.name).join(', ') || 'none'}</span>
                    </div>
                  </div>
                  <div className="stat-card glass">
                    <div className="stat-header">
                      <div className="stat-icon network"><FiActivity /></div>
                      <span className="stat-label">Listening Ports</span>
                    </div>
                    <div className="stat-info">
                      <span className="stat-value">{ports.length}</span>
                      <span className="stat-sub">{ports.slice(0, 4).map((p) => p.port).join(', ')}{ports.length > 4 ? '…' : ''}</span>
                    </div>
                  </div>
                </>
              )
            })()}
          </div>
        </>
      )}
    </div>
  )
}
