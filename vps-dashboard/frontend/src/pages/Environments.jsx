import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  FiSettings,
  FiRefreshCw,
  FiEdit2,
  FiRotateCcw,
  FiInfo,
} from 'react-icons/fi'
import { environments as envApi } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import EnvBadge from '../ui/EnvBadge.jsx'
import RelativeTime from '../ui/RelativeTime.jsx'
import { describeError } from '../ui/errors.js'
import { Modal } from './Projects.jsx'
import './Environments.css'

const SEVERITIES = ['info', 'warning', 'error', 'critical']
const ENV_ORDER = ['development', 'staging', 'production']

export default function EnvironmentsPage() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const queryClient = useQueryClient()
  const toast = useToast()
  const [editing, setEditing] = useState(null) // null | EnvOverride
  const [resetting, setResetting] = useState(false)

  const envQ = useQuery({
    queryKey: ['environments'],
    queryFn: envApi.list,
  })

  const updateM = useMutation({
    mutationFn: ({ env, payload }) => envApi.update(env, payload),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Environment updated' })
      queryClient.invalidateQueries({ queryKey: ['environments'] })
      setEditing(null)
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Update failed') })
    },
  })

  const ordered = useMemo(() => {
    const list = Array.isArray(envQ.data) ? envQ.data : []
    const byName = new Map(list.map((row) => [row.environment, row]))
    const out = []
    for (const name of ENV_ORDER) {
      if (byName.has(name)) out.push(byName.get(name))
    }
    for (const row of list) {
      if (!ENV_ORDER.includes(row.environment)) out.push(row)
    }
    return out
  }, [envQ.data])

  const handleResetDefaults = async () => {
    if (!isAdmin) return
    setResetting(true)
    try {
      const defaults = await envApi.defaults()
      const targets = Array.isArray(defaults) ? defaults : []
      for (const d of targets) {
        await envApi.update(d.environment, { config: d.config || {} })
      }
      toast.push({ type: 'success', message: 'Reset all environments to defaults' })
      queryClient.invalidateQueries({ queryKey: ['environments'] })
    } catch (err) {
      toast.push({ type: 'error', message: describeError(err, 'Reset failed') })
    } finally {
      setResetting(false)
    }
  }

  return (
    <div className="environments-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>Environments</h1>
            <p>Per-environment configuration overrides</p>
          </div>
          <div className="header-actions">
            <button
              type="button"
              className="ghost-btn"
              onClick={() =>
                queryClient.invalidateQueries({ queryKey: ['environments'] })
              }
              disabled={envQ.isFetching}
            >
              <FiRefreshCw className={envQ.isFetching ? 'spinning' : ''} />
              Refresh
            </button>
          </div>
        </div>
      </div>

      <div className="env-help glass">
        <FiInfo />
        <ul>
          <li>
            <strong>Healthcheck multiplier:</strong> scales the base
            health-check interval. 1.0 = base, 2.0 = twice as long, 0.5 =
            twice as often.
          </li>
          <li>
            <strong>Alert severity floor:</strong> alerts below this severity
            level are suppressed in this environment.
          </li>
        </ul>
      </div>

      {envQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : envQ.isError ? (
        <EmptyState
          icon={<FiSettings size={40} />}
          title="Failed to load environments"
          description={describeError(envQ.error)}
        />
      ) : ordered.length === 0 ? (
        <EmptyState
          icon={<FiSettings size={40} />}
          title="No environment overrides"
          description="Defaults are applied"
        />
      ) : (
        <div className="env-grid">
          {ordered.map((row) => (
            <EnvCard
              key={row.environment}
              row={row}
              isAdmin={isAdmin}
              onEdit={() => setEditing(row)}
            />
          ))}
        </div>
      )}

      {isAdmin && (
        <div className="env-reset-row">
          <button
            type="button"
            className="ghost-btn"
            onClick={handleResetDefaults}
            disabled={resetting}
          >
            {resetting ? <Spinner size={14} /> : <FiRotateCcw />}
            Reset all to defaults
          </button>
        </div>
      )}

      {editing && isAdmin && (
        <EnvEditModal
          row={editing}
          submitting={updateM.isPending}
          error={updateM.isError ? updateM.error : null}
          onSubmit={(config) =>
            updateM.mutate({ env: editing.environment, payload: { config } })
          }
          onClose={() => {
            updateM.reset()
            setEditing(null)
          }}
        />
      )}
    </div>
  )
}

function EnvCard({ row, isAdmin, onEdit }) {
  const cfg = row.config || {}
  const multiplier =
    cfg.healthcheck_multiplier != null
      ? Number(cfg.healthcheck_multiplier)
      : null
  const floor = cfg.alert_severity_floor || 'info'
  return (
    <div className="env-card glass">
      <div className="env-card-head">
        <EnvBadge environment={row.environment} size="md" />
        {row.updated_at && !String(row.updated_at).startsWith('0001-01-01') ? (
          <span className="env-updated">
            updated <RelativeTime value={row.updated_at} />
          </span>
        ) : (
          <span className="env-updated">defaults</span>
        )}
      </div>

      <dl className="env-fields">
        <div className="env-field">
          <dt>Healthcheck multiplier</dt>
          <dd>
            <code>
              {multiplier != null ? formatMultiplier(multiplier) : '—'}
            </code>
          </dd>
        </div>
        <div className="env-field">
          <dt>Alert severity floor</dt>
          <dd>
            <span className={`severity-badge severity-${normalizeSeverity(floor)}`}>
              {floor}
            </span>
          </dd>
        </div>
      </dl>

      {isAdmin ? (
        <button type="button" className="action-btn" onClick={onEdit}>
          <FiEdit2 />
          Edit
        </button>
      ) : (
        <p className="env-readonly-note">Read-only (admin can edit)</p>
      )}
    </div>
  )
}

function EnvEditModal({ row, submitting, error, onSubmit, onClose }) {
  const cfg = row.config || {}
  const [multiplier, setMultiplier] = useState(
    cfg.healthcheck_multiplier != null
      ? String(cfg.healthcheck_multiplier)
      : '1.0'
  )
  const [floor, setFloor] = useState(cfg.alert_severity_floor || 'info')
  const [localError, setLocalError] = useState(null)

  const handleSubmit = (e) => {
    e.preventDefault()
    if (submitting) return
    const num = Number(multiplier)
    if (!Number.isFinite(num) || num <= 0 || num > 100) {
      setLocalError(new Error('Multiplier must be a number > 0 and <= 100'))
      return
    }
    setLocalError(null)
    onSubmit({
      ...cfg,
      healthcheck_multiplier: num,
      alert_severity_floor: floor,
    })
  }

  // Server-side error takes precedence over the stale local validation
  // message — the next successful submit clears localError above.
  const errorText =
    (error ? describeError(error, 'Save failed') : '') ||
    (localError && localError.message) ||
    ''

  return (
    <Modal title={`Edit ${row.environment}`} onClose={onClose} size="small">
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="modal-grid">
          <div className="form-group full">
            <label>Healthcheck multiplier</label>
            <input
              type="number"
              step="0.1"
              min="0.1"
              max="100"
              value={multiplier}
              onChange={(e) => setMultiplier(e.target.value)}
              required
            />
            <div className="form-help">1.0 = base, &gt;1 slower, &lt;1 faster</div>
          </div>
          <div className="form-group full">
            <label>Alert severity floor</label>
            <select value={floor} onChange={(e) => setFloor(e.target.value)}>
              {SEVERITIES.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
            <div className="form-help">
              Suppresses alerts below this level in this environment
            </div>
          </div>
        </div>

        {errorText && <div className="modal-error">{errorText}</div>}

        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button type="submit" className="primary-btn" disabled={submitting}>
            {submitting ? <Spinner size={14} /> : null}
            Save
          </button>
        </div>
      </form>
    </Modal>
  )
}

function normalizeSeverity(s) {
  switch (String(s || '').toLowerCase()) {
    case 'warning':
    case 'error':
    case 'critical':
    case 'info':
      return s
    default:
      return 'info'
  }
}

function formatMultiplier(n) {
  if (!Number.isFinite(n)) return '—'
  if (n >= 10) return n.toFixed(0)
  return n.toFixed(2).replace(/\.?0+$/, '') || '0'
}
