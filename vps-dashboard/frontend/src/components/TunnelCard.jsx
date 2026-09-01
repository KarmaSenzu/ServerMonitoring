import { useState } from 'react'
import {
  FiCloud,
  FiGlobe,
  FiArrowRight,
  FiActivity,
  FiClock,
  FiRotateCw,
  FiRefreshCw,
} from 'react-icons/fi'
import { tunnels as tunnelsApi } from '../api/endpoints.js'
import { useAuth, canMutate } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import { describeError } from '../ui/errors.js'
import './TunnelCard.css'

function formatTunnelUptime(uptime, startedAt) {
  if (uptime && typeof uptime === 'string' && uptime.trim() !== '') {
    return uptime
  }
  if (!startedAt) return '-'
  const start = new Date(startedAt)
  const now = new Date()
  const diff = Math.floor((now - start) / 1000)
  if (Number.isNaN(diff) || diff < 0) return '-'
  const d = Math.floor(diff / 86400)
  const h = Math.floor((diff % 86400) / 3600)
  const m = Math.floor((diff % 3600) / 60)
  if (d > 0) return `${d}d ${h}h ${m}m`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

function deriveStatus(t) {
  const state = (t.activeState || '').toLowerCase()
  if (state === 'active') return 'running'
  if (state === 'inactive' || state === 'failed') return 'stopped'
  return 'unknown'
}

function formatStreams(t) {
  if (typeof t.activeStreams !== 'number') return 'n/a'
  if (t.activeStreams < 0) return 'n/a'
  return String(t.activeStreams)
}

function formatLatency(t) {
  // Backend uses snake_case for the new field; tolerate camel-case too.
  const ms = t.latency_ms ?? t.latencyMs
  if (typeof ms !== 'number' || ms < 0) return 'n/a'
  return `${ms} ms`
}

function latencyClass(t) {
  const ms = t.latency_ms ?? t.latencyMs
  if (typeof ms !== 'number' || ms < 0) return 'na'
  if (ms < 100) return 'good'
  if (ms < 300) return 'mid'
  return 'bad'
}

export default function TunnelCard({ tunnel, onRestart }) {
  const status = deriveStatus(tunnel)
  const isRunning = status === 'running'
  const ingress = Array.isArray(tunnel.ingress) ? tunnel.ingress : []
  const visibleRoutes = ingress.filter((r) => !r.catchall && r.hostname && r.hostname !== '*')

  const { user } = useAuth()
  const isAdmin = canMutate(user?.role)
  const toast = useToast()
  const [restarting, setRestarting] = useState(false)

  const service = tunnel.serviceName || tunnel.service_name
  const canRestart = Boolean(service)

  const handleRestart = async () => {
    if (!service || restarting) return
    setRestarting(true)
    try {
      const res = await tunnelsApi.restart(service)
      const ok = res?.restarted || res?.service
      toast.push({
        type: 'success',
        message: ok
          ? `Tunnel ${res.service || service} restarted`
          : `Tunnel ${service} restart requested`,
      })
      if (onRestart) onRestart()
    } catch (err) {
      toast.push({ type: 'error', message: describeError(err, 'Tunnel restart failed') })
    } finally {
      setRestarting(false)
    }
  }

  return (
    <div className="tunnel-card glass animate-in">
      <div className="tunnel-card-header">
        <div className="tunnel-card-info">
          <div className={`tunnel-card-icon ${isRunning ? 'active' : 'inactive'}`}>
            <FiCloud />
          </div>
          <div>
            <h3 className="tunnel-card-name">{tunnel.name}</h3>
            <span className="tunnel-card-id">
              {tunnel.id ? `${tunnel.id.slice(0, 8)}...` : service}
            </span>
          </div>
        </div>
        <div className="tunnel-header-right">
          <div className={`tunnel-status-badge ${status}`}>
            <span className={`status-dot ${isRunning ? 'running' : 'stopped'}`}></span>
            {status}
          </div>
        </div>
      </div>

      <div className="tunnel-details">
        <div className="tunnel-detail-row">
          <FiActivity size={13} />
          <span className="tunnel-detail-label">Service</span>
          <span className="tunnel-detail-value">{service || '-'}</span>
        </div>
        {tunnel.mainPid > 0 && (
          <div className="tunnel-detail-row">
            <FiActivity size={13} />
            <span className="tunnel-detail-label">PID</span>
            <span className="tunnel-detail-value">{tunnel.mainPid}</span>
          </div>
        )}
        <div className="tunnel-detail-row">
          <FiClock size={13} />
          <span className="tunnel-detail-label">Uptime</span>
          <span className="tunnel-detail-value">
            {formatTunnelUptime(tunnel.uptime, tunnel.startedAt)}
          </span>
        </div>
        <div className="tunnel-detail-row">
          <FiActivity size={13} />
          <span className="tunnel-detail-label">Streams</span>
          <span className="tunnel-detail-value tunnel-streams-value">
            {formatStreams(tunnel)}
            <span className={`tunnel-latency-badge ${latencyClass(tunnel)}`}>
              {formatLatency(tunnel)}
            </span>
          </span>
        </div>
      </div>

      <div className="tunnel-routes">
        <div className="tunnel-routes-header">
          <FiGlobe size={13} />
          <span>Ingress Routes</span>
          <span className="route-count">{visibleRoutes.length}</span>
        </div>
        <div className="tunnel-routes-list">
          {ingress.map((rule, i) => (
            <div
              key={`${rule.hostname || 'catchall'}-${i}`}
              className={`tunnel-route ${rule.catchall || rule.hostname === '*' || !rule.hostname ? 'catchall' : ''}`}
            >
              <span className="route-hostname">
                {rule.catchall || !rule.hostname || rule.hostname === '*'
                  ? 'catch-all'
                  : rule.hostname}
              </span>
              <FiArrowRight size={12} className="route-arrow" />
              <span className="route-service">{rule.service || '-'}</span>
            </div>
          ))}
        </div>
      </div>

      {isAdmin && canRestart && (
        <div className="tunnel-card-actions">
          <button
            type="button"
            className="tunnel-action-btn"
            onClick={handleRestart}
            disabled={restarting}
          >
            {restarting ? <FiRefreshCw className="spinning" /> : <FiRotateCw />}
            Restart
          </button>
        </div>
      )}
    </div>
  )
}
