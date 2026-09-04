import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  FiCloud,
  FiRefreshCw,
  FiDownload,
  FiAlertTriangle,
  FiServer,
  FiBox,
  FiDatabase,
} from 'react-icons/fi'
import { cloudApi } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import { describeError } from '../ui/errors.js'
import { Modal } from './Projects.jsx'
import { useToast } from '../ui/useToast.js'
import './CloudDiscovery.css'

export default function CloudDiscoveryPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const isAdmin = user?.role === 'admin'

  const [importing, setImporting] = useState(null)
  const [filter, setFilter] = useState('')

  const instancesQ = useQuery({
    queryKey: ['cloud-instances'],
    queryFn: () => cloudApi.instances(),
    refetchInterval: 60000,
  })

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['cloud-instances'] })
  }

  const data = instancesQ.data || {}
  const results = useMemo(() => data.data || {}, [data])
  const errors = data.errors || {}

  const allInstances = useMemo(() => {
    const flat = []
    for (const [provider, instances] of Object.entries(results)) {
      for (const inst of instances) {
        flat.push({ ...inst, provider })
      }
    }
    return flat
  }, [results])

  const filtered = useMemo(() => {
    if (!filter.trim()) return allInstances
    const q = filter.toLowerCase()
    return allInstances.filter(
      (inst) =>
        (inst.name || '').toLowerCase().includes(q) ||
        (inst.provider || '').toLowerCase().includes(q) ||
        (inst.public_ip || '').toLowerCase().includes(q) ||
        (inst.region || '').toLowerCase().includes(q)
    )
  }, [allInstances, filter])

  const totalDiscovered = allInstances.length
  const providerCount = Object.keys(results).length
  const runningCount = allInstances.filter((i) => i.state === 'running').length
  const errorCount = Object.keys(errors).length

  return (
    <div className="cloud-page">
      {/* === HERO HEADER === */}
      <div className="cloud-hero">
        <div className="cloud-hero-top">
          <div className="cloud-hero-left">
            <div className="cloud-hero-title-row">
              <h1 className="cloud-hero-title">Cloud Discovery &amp; Edge Ingress</h1>
              <span className="cloud-hero-badge">
                <span className="cloud-hero-dot" />
                DISCOVERY ACTIVE
              </span>
            </div>
            <p className="cloud-hero-subtitle">
              <span>Multi-cloud instance discovery</span>
              <span className="dot-sep">•</span>
              <span>{providerCount} providers configured</span>
              <span className="dot-sep">•</span>
              <span className="cloud-hero-ok">{runningCount} running</span>
            </p>
          </div>
          <div className="cloud-hero-actions">
            <button className="cloud-btn ghost" onClick={refresh} disabled={instancesQ.isFetching}>
              <FiRefreshCw className={instancesQ.isFetching ? 'spinning' : ''} size={14} />
              <span>Refresh Discovery</span>
            </button>
          </div>
        </div>
      </div>

      {/* === UNLINKED / PENDING RESOURCES ALERT BANNER === */}
      {errorCount > 0 && (
        <div className="cloud-alert-banner">
          <div className="cloud-alert-accent" />
          <div className="cloud-alert-icon">
            <FiAlertTriangle size={18} />
          </div>
          <div className="cloud-alert-content">
            <div className="cloud-alert-title-row">
              <span className="cloud-alert-title">Unlinked Edge Ingress Detected</span>
              <span className="cloud-alert-action-badge">ACTION REQUIRED</span>
            </div>
            <p className="cloud-alert-text">
              {errorCount} provider{errorCount > 1 ? 's' : ''} returned discovery errors. Review credentials and retry.
            </p>
          </div>
        </div>
      )}

      {/* === METRICS BAR (4-col) === */}
      <div className="cloud-metrics-bar">
        <div className="cloud-metric-card">
          <span className="cloud-metric-label">
            <FiCloud size={14} /> Providers
          </span>
          <span className="cloud-metric-value">{providerCount}</span>
        </div>
        <div className="cloud-metric-card">
          <span className="cloud-metric-label">
            <FiServer size={14} /> Discovered
          </span>
          <span className="cloud-metric-value green">{totalDiscovered}</span>
        </div>
        <div className="cloud-metric-card">
          <span className="cloud-metric-label">
            <FiBox size={14} /> Running
          </span>
          <span className="cloud-metric-value green">{runningCount}</span>
        </div>
        <div className="cloud-metric-card">
          <span className="cloud-metric-label">
            <FiDatabase size={14} /> Errors
          </span>
          <span className="cloud-metric-value red">{errorCount}</span>
        </div>
      </div>

      {/* === FILTER BAR === */}
      <div className="cloud-filter-bar">
        <div className="cloud-filter-input-wrap">
          <FiServer size={14} />
          <input
            type="text"
            className="cloud-filter-input"
            placeholder="Filter instances by name, provider, IP, region..."
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
        </div>
        <span className="cloud-filter-count">{filtered.length} / {totalDiscovered} instances</span>
      </div>

      {/* === DISCOVERED ASSETS TABLE === */}
      {instancesQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : instancesQ.isError ? (
        <EmptyState icon={<FiCloud size={40} />} title="Failed to discover instances" description={describeError(instancesQ.error)} />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={<FiCloud size={40} />}
          title={filter ? 'No matching instances' : 'No instances discovered'}
          description={filter ? 'Try a different filter' : 'No cloud providers are configured, or they returned no instances. Configure provider credentials to start discovering.'}
        />
      ) : (
        <div className="cloud-assets-table-card">
          <div className="cloud-assets-header">
            <div className="cloud-assets-title-row">
              <FiCloud size={18} />
              <span className="cloud-assets-title">Discovered Cloud Assets</span>
              <span className="cloud-assets-badge">{filtered.length} ASSETS</span>
            </div>
          </div>
          <div className="cloud-assets-table-wrap">
            <table className="cloud-assets-table">
              <thead>
                <tr>
                  <th>Instance</th>
                  <th>Provider</th>
                  <th>Public IP</th>
                  <th>Private IP</th>
                  <th>Region</th>
                  <th>State</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((inst) => (
                  <tr key={`${inst.provider}-${inst.id}`} className="cloud-asset-row">
                    <td>
                      <div className="cloud-asset-name-cell">
                        <span className={`cloud-asset-dot ${inst.state || 'unknown'}`} />
                        <div className="cloud-asset-name-info">
                          <span className="cloud-asset-name">{inst.name}</span>
                          <span className="cloud-asset-id mono">{inst.id}</span>
                        </div>
                      </div>
                    </td>
                    <td className="mono cloud-provider-name">{inst.provider}</td>
                    <td className="mono">{inst.public_ip || '-'}</td>
                    <td className="mono">{inst.private_ip || '-'}</td>
                    <td className="mono">{inst.region || '-'}</td>
                    <td>
                      <span className={`cloud-asset-state ${inst.state || 'unknown'}`}>
                        {(inst.state || 'unknown').toUpperCase()}
                      </span>
                    </td>
                    <td className="text-right">
                      {isAdmin && (
                        <button
                          className="cloud-import-btn"
                          onClick={() => setImporting({ ...inst, provider: inst.provider })}
                        >
                          <FiDownload size={12} /> Import
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* === PROVIDER ERRORS === */}
      {errorCount > 0 && (
        <div className="cloud-errors-section">
          {Object.entries(errors).map(([provider, err]) => (
            <div key={provider} className="cloud-error-card">
              <span className="cloud-error-provider">{provider}</span>
              <span className="cloud-error-msg mono">{err}</span>
            </div>
          ))}
        </div>
      )}

      {importing && (
        <ImportModal instance={importing} onClose={() => setImporting(null)} />
      )}
    </div>
  )
}

function ImportModal({ instance, onClose }) {
  const toast = useToast()
  const queryClient = useQueryClient()
  const [form, setForm] = useState({
    server_name: instance.name || '',
    hostname: instance.public_ip || instance.private_ip || '',
    ssh_port: 22,
    ssh_username: 'root',
    environment: 'production',
    tags: [],
  })
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const set = (key) => (e) => setForm((f) => ({ ...f, [key]: e.target.value }))

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (submitting) return
    setSubmitting(true)
    setError('')
    try {
      await cloudApi.import({
        instance_id: instance.id,
        provider: instance.provider,
        ...form,
      })
      toast.push({ type: 'success', message: 'Server imported into registry' })
      queryClient.invalidateQueries({ queryKey: ['servers'] })
      queryClient.invalidateQueries({ queryKey: ['cloud-instances'] })
      onClose()
    } catch (err) {
      setError(describeError(err, 'Import failed'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal title={`Import ${instance.name}`} onClose={onClose} size="normal">
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="modal-grid">
          <div className="form-group">
            <label>Server name</label>
            <input type="text" autoFocus value={form.server_name} onChange={set('server_name')} required />
          </div>
          <div className="form-group">
            <label>Hostname / IP</label>
            <input type="text" className="mono" value={form.hostname} onChange={set('hostname')} required />
          </div>
          <div className="form-group">
            <label>SSH port</label>
            <input type="number" value={form.ssh_port} onChange={set('ssh_port')} />
          </div>
          <div className="form-group">
            <label>SSH username</label>
            <input type="text" value={form.ssh_username} onChange={set('ssh_username')} required />
          </div>
          <div className="form-group">
            <label>Environment</label>
            <select value={form.environment} onChange={set('environment')}>
              <option value="development">development</option>
              <option value="staging">staging</option>
              <option value="production">production</option>
            </select>
          </div>
        </div>
        <p className="modal-hint">
          The server will be registered with status <strong>unknown</strong>. Configure SSH
          credentials and run a connectivity test before enabling monitoring.
        </p>
        {error && <div className="modal-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button type="submit" className="primary-btn" disabled={submitting}>
            {submitting ? <Spinner size={14} /> : null}
            Import
          </button>
        </div>
      </form>
    </Modal>
  )
}
