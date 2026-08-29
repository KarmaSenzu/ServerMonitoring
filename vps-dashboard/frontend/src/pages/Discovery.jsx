import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  FiRefreshCw,
  FiSearch,
  FiBox,
  FiZap,
  FiCloud,
  FiCheckCircle,
  FiAlertCircle,
} from 'react-icons/fi'
import { discovery } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import { describeError } from '../ui/errors.js'
import { Modal } from './Projects.jsx'
import './Discovery.css'

const SOURCE_LABELS = {
  docker: 'Docker',
  pm2: 'PM2',
  tunnel: 'Tunnel',
}

const ERROR_LABELS = {
  docker_unavailable: 'Docker unavailable',
  pm2_unavailable: 'PM2 unavailable',
  tunnel_unavailable: 'Tunnel unavailable',
  docker_error: 'Docker error',
  pm2_error: 'PM2 error',
}

export default function DiscoveryPage() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const queryClient = useQueryClient()
  const toast = useToast()
  const [adopting, setAdopting] = useState(null) // candidate

  const snapshotQ = useQuery({
    queryKey: ['discovery', 'snapshot'],
    queryFn: discovery.snapshot,
    refetchOnMount: 'always',
  })

  const adoptM = useMutation({
    mutationFn: ({ candidate, overrides }) => discovery.adopt(candidate, overrides),
    onSuccess: (project) => {
      toast.push({ type: 'success', message: `Adopted "${project.name}"` })
      queryClient.invalidateQueries({ queryKey: ['projects'] })
      queryClient.invalidateQueries({ queryKey: ['discovery', 'snapshot'] })
      setAdopting(null)
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Adopt failed') })
    },
  })

  const adoptManyM = useMutation({
    mutationFn: (candidates) => discovery.adoptMany(candidates),
    onSuccess: (results) => {
      const ok = (results || []).filter((r) => r.status === 'created').length
      const skipped = (results || []).filter((r) => r.status === 'skipped').length
      const failed = (results || []).filter((r) => r.status === 'error').length
      if (ok > 0) toast.push({ type: 'success', message: `Adopted ${ok} project(s)` })
      if (skipped > 0) toast.push({ type: 'info', message: `${skipped} skipped (already adopted)` })
      if (failed > 0) {
        toast.push({ type: 'warning', message: `${failed} failed to adopt` })
      }
      queryClient.invalidateQueries({ queryKey: ['projects'] })
      queryClient.invalidateQueries({ queryKey: ['discovery', 'snapshot'] })
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Bulk adopt failed') })
    },
  })

  const handleRefresh = () => {
    queryClient.invalidateQueries({ queryKey: ['discovery', 'snapshot'] })
  }

  const snap = snapshotQ.data
  const candidates = snap?.candidates || []
  const errors = snap?.errors || []
  const errorSet = new Set(errors)

  const containers = snap?.containers || []
  const pm2Apps = snap?.pm2_apps || []
  const tunnels = snap?.tunnels || []

  const nonAdopted = candidates.filter((c) => !c.already_adopted)

  return (
    <div className="discovery-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>Discovery</h1>
            <p>Detect containers, PM2 apps, and Cloudflare tunnels you can adopt</p>
          </div>
          <div className="header-actions">
            {isAdmin && nonAdopted.length > 0 && (
              <button
                type="button"
                className="primary-btn"
                onClick={() => adoptManyM.mutate(nonAdopted)}
                disabled={adoptManyM.isPending}
              >
                {adoptManyM.isPending ? <Spinner size={14} /> : null}
                Adopt all ({nonAdopted.length})
              </button>
            )}
            <button
              type="button"
              className="refresh-btn glass"
              onClick={handleRefresh}
              disabled={snapshotQ.isFetching}
            >
              <FiRefreshCw className={snapshotQ.isFetching ? 'spinning' : ''} />
              Refresh
            </button>
          </div>
        </div>
      </div>

      <div className="discovery-counts">
        <CountCard
          icon={<FiBox />}
          label="Docker"
          count={containers.length}
          tone="docker"
          error={errorSet.has('docker_unavailable') ? 'docker_unavailable' : errorSet.has('docker_error') ? 'docker_error' : null}
        />
        <CountCard
          icon={<FiZap />}
          label="PM2"
          count={pm2Apps.length}
          tone="pm2"
          error={errorSet.has('pm2_unavailable') ? 'pm2_unavailable' : errorSet.has('pm2_error') ? 'pm2_error' : null}
        />
        <CountCard
          icon={<FiCloud />}
          label="Tunnels"
          count={tunnels.length}
          tone="tunnel"
          error={errorSet.has('tunnel_unavailable') ? 'tunnel_unavailable' : null}
        />
      </div>

      <div className="section-header">
        <div className="section-title">
          <FiSearch />
          <h2>Candidates</h2>
          <span className="container-count">{candidates.length}</span>
        </div>
      </div>

      {snapshotQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : snapshotQ.isError ? (
        <EmptyState
          icon={<FiAlertCircle size={40} />}
          title="Failed to load discovery snapshot"
          description={describeError(snapshotQ.error)}
        />
      ) : candidates.length === 0 ? (
        <EmptyState
          icon={<FiSearch size={40} />}
          title="Nothing to discover"
          description="No containers, tunnels, or PM2 apps detected. Try refreshing or run something on the server."
        />
      ) : (
        <div className="candidates-list">
          {candidates.map((c, i) => (
            <CandidateRow
              key={`${c.suggested_name}-${i}`}
              candidate={c}
              isAdmin={isAdmin}
              onAdopt={() => setAdopting(c)}
            />
          ))}
        </div>
      )}

      {adopting && isAdmin && (
        <AdoptModal
          key={adopting.suggested_name + (adopting.tunnel_service || '') + (adopting.container_name || '')}
          candidate={adopting}
          submitting={adoptM.isPending}
          error={adoptM.isError ? adoptM.error : null}
          onSubmit={(overrides) =>
            adoptM.mutate({ candidate: adopting, overrides })
          }
          onClose={() => {
            adoptM.reset()
            setAdopting(null)
          }}
        />
      )}
    </div>
  )
}

function CountCard({ icon, label, count, tone, error }) {
  return (
    <div className={`count-card glass tone-${tone}`}>
      <div className="count-icon">{icon}</div>
      <div className="count-info">
        <div className="count-label">{label}</div>
        <div className="count-value">{count}</div>
      </div>
      {error && (
        <div className="count-error" title={ERROR_LABELS[error] || error}>
          <FiAlertCircle />
          <span>{ERROR_LABELS[error] || error}</span>
        </div>
      )}
    </div>
  )
}

function CandidateRow({ candidate, isAdmin, onAdopt }) {
  const sources = Array.isArray(candidate.sources) ? candidate.sources : []
  return (
    <div className="candidate-card glass animate-in">
      <div className="candidate-head">
        <div className="candidate-title">
          <h3>{candidate.suggested_name || '(unnamed)'}</h3>
          <div className="candidate-sources">
            {sources.map((s) => (
              <span key={s} className={`source-badge source-${s}`}>
                {SOURCE_LABELS[s] || s}
              </span>
            ))}
          </div>
        </div>
        {candidate.already_adopted ? (
          <span className="adopted-badge">
            <FiCheckCircle size={12} />
            Adopted
            {candidate.adopted_as && (
              <Link to={`/projects`} className="adopted-link">
                view
              </Link>
            )}
          </span>
        ) : (
          isAdmin && (
            <button type="button" className="primary-btn small" onClick={onAdopt}>
              Adopt
            </button>
          )
        )}
      </div>

      <div className="candidate-meta">
        {candidate.domain && (
          <div className="meta-row">
            <span className="meta-label">Domain</span>
            <span className="meta-value">{candidate.domain}</span>
          </div>
        )}
        {candidate.port > 0 && (
          <div className="meta-row">
            <span className="meta-label">Port</span>
            <span className="meta-value">{candidate.port}</span>
          </div>
        )}
        {candidate.container_name && (
          <div className="meta-row">
            <span className="meta-label">Container</span>
            <span className="meta-value">{candidate.container_name}</span>
          </div>
        )}
        {candidate.pm2_name && (
          <div className="meta-row">
            <span className="meta-label">PM2</span>
            <span className="meta-value">{candidate.pm2_name}</span>
          </div>
        )}
        {candidate.tunnel_service && (
          <div className="meta-row">
            <span className="meta-label">Tunnel</span>
            <span className="meta-value">{candidate.tunnel_service}</span>
          </div>
        )}
        {candidate.health_url && (
          <div className="meta-row">
            <span className="meta-label">Health</span>
            <span className="meta-value">{candidate.health_url}</span>
          </div>
        )}
      </div>

      <div className="candidate-footer">
        <ConfidenceBar value={candidate.confidence || 0} />
        {candidate.reason && <div className="candidate-reason">{candidate.reason}</div>}
      </div>
    </div>
  )
}

function ConfidenceBar({ value }) {
  const v = Math.max(0, Math.min(100, Number(value) || 0))
  return (
    <div className="confidence">
      <div className="confidence-label">Confidence</div>
      <div className="confidence-bar">
        <div className="confidence-fill" style={{ width: `${v}%` }} />
      </div>
      <div className="confidence-value">{v}</div>
    </div>
  )
}

function AdoptModal({ candidate, submitting, error, onSubmit, onClose }) {
  const [name, setName] = useState(candidate.suggested_name || '')
  const [healthUrl, setHealthUrl] = useState(candidate.health_url || '')

  const handleSubmit = (e) => {
    e.preventDefault()
    if (submitting) return
    const overrides = {}
    if (name && name !== candidate.suggested_name) overrides.name = name.trim()
    if (healthUrl && healthUrl !== candidate.health_url) overrides.health_url = healthUrl.trim()
    onSubmit(overrides)
  }

  const errorText = error ? describeError(error, 'Adopt failed') : ''

  return (
    <Modal title={`Adopt ${candidate.suggested_name}`} onClose={onClose} size="small">
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="modal-grid">
          <div className="form-group full">
            <label>Project name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              autoFocus
            />
          </div>
          <div className="form-group full">
            <label>Health URL (optional)</label>
            <input
              type="text"
              value={healthUrl}
              onChange={(e) => setHealthUrl(e.target.value)}
              placeholder="https://app.example.com/health"
            />
          </div>
        </div>

        {errorText && <div className="modal-error">{errorText}</div>}

        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button type="submit" className="primary-btn" disabled={submitting || !name.trim()}>
            {submitting ? <Spinner size={14} /> : null}
            Adopt
          </button>
        </div>
      </form>
    </Modal>
  )
}
