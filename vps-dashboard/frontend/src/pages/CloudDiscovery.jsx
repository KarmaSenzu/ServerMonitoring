import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  FiCloud,
  FiRefreshCw,
  FiDownload,
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

  const [importing, setImporting] = useState(null) // instance

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

  const totalDiscovered = allInstances.length

  return (
    <div className="cloud-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>Cloud Discovery</h1>
            <p>
              Discover cloud instances and import them into the Server Registry
            </p>
          </div>
          <button type="button" className="ghost-btn" onClick={refresh}>
            <FiRefreshCw />
            Refresh
          </button>
        </div>
      </div>

      <div className="cloud-summary">
        <div className="summary-chip">
          <span className="chip-value">{Object.keys(results).length}</span>
          <span className="chip-label">Providers</span>
        </div>
        <div className="summary-chip status-online">
          <span className="chip-value">{totalDiscovered}</span>
          <span className="chip-label">Discovered</span>
        </div>
      </div>

      {instancesQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : instancesQ.isError ? (
        <EmptyState icon={<FiCloud size={40} />} title="Failed to discover instances" description={describeError(instancesQ.error)} />
      ) : totalDiscovered === 0 && Object.keys(errors).length === 0 ? (
        <EmptyState
          icon={<FiCloud size={40} />}
          title="No instances discovered"
          description="No cloud providers are configured, or they returned no instances. Configure provider credentials to start discovering."
        />
      ) : (
        <div className="cloud-providers">
          {Object.entries(results).map(([provider, instances]) => (
            <div key={provider} className="cloud-provider-section glass">
              <div className="cloud-provider-header">
                <h3>{provider}</h3>
                <span className="muted">{instances.length} instances</span>
              </div>
              {instances.length === 0 ? (
                <p className="muted cloud-empty">No instances visible to this provider.</p>
              ) : (
                <div className="cloud-instances">
                  {instances.map((inst) => (
                    <div key={inst.id} className="cloud-instance-card">
                      <div className="cloud-instance-header">
                        <span className="cloud-instance-name">{inst.name}</span>
                        <span className={`cloud-state state-${inst.state}`}>{inst.state}</span>
                      </div>
                      <div className="cloud-instance-body">
                        <div className="cloud-meta-row">
                          <span className="muted">ID:</span>
                          <span className="mono">{inst.id}</span>
                        </div>
                        <div className="cloud-meta-row">
                          <span className="muted">Type:</span>
                          <span>{inst.type || '-'}</span>
                        </div>
                        <div className="cloud-meta-row">
                          <span className="muted">Public IP:</span>
                          <span className="mono">{inst.public_ip || '-'}</span>
                        </div>
                        <div className="cloud-meta-row">
                          <span className="muted">Private IP:</span>
                          <span className="mono">{inst.private_ip || '-'}</span>
                        </div>
                        {inst.region && (
                          <div className="cloud-meta-row">
                            <span className="muted">Region:</span>
                            <span>{inst.region}</span>
                          </div>
                        )}
                      </div>
                      {isAdmin && (
                        <div className="cloud-instance-actions">
                          <button
                            type="button"
                            className="action-btn small primary"
                            onClick={() => setImporting({ ...inst, provider })}
                          >
                            <FiDownload />
                            Import
                          </button>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          ))}
          {Object.entries(errors).map(([provider, err]) => (
            <div key={provider} className="cloud-provider-section glass">
              <div className="cloud-provider-header">
                <h3>{provider}</h3>
                <span className="status-badge status-offline">Error</span>
              </div>
              <div className="cloud-error mono">{err}</div>
            </div>
          ))}
        </div>
      )}

      {importing && (
        <ImportModal
          instance={importing}
          onClose={() => setImporting(null)}
        />
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
