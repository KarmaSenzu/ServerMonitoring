import { useMemo, useState } from 'react'
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query'
import {
  FiShuffle,
  FiPlus,
  FiEdit2,
  FiTrash2,
  FiPlay,
  FiPause,
} from 'react-icons/fi'
import { tunnelApi, servers as serversApi } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import { describeError } from '../ui/errors.js'
import { Modal } from './Projects.jsx'
import './Tunnels.css'

const STATUS_COLORS = {
  stopped: 'status-unknown',
  connecting: 'status-degraded',
  active: 'status-online',
  error: 'status-offline',
}

const TYPE_LABELS = {
  local: 'Local Forward (-L)',
  remote: 'Remote Forward (-R)',
  socks: 'SOCKS5 (-D)',
}

function formatDate(s) {
  if (!s) return '-'
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString()
}

export default function TunnelsPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const toast = useToast()
  const isAdmin = user?.role === 'admin'

  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState(null)
  const [confirmDelete, setConfirmDelete] = useState(null)

  const tunnelsQ = useQuery({
    queryKey: ['ssh-tunnels'],
    queryFn: () => tunnelApi.list(),
    refetchInterval: 10000,
  })

  const serversQ = useQuery({
    queryKey: ['servers'],
    queryFn: () => serversApi.list(),
    enabled: isAdmin,
  })

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['ssh-tunnels'] })
  }

  const deleteM = useMutation({
    mutationFn: (id) => tunnelApi.remove(id),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Tunnel deleted' })
      invalidate()
      setConfirmDelete(null)
    },
    onError: (err) => toast.push({ type: 'error', message: describeError(err, 'Delete failed') }),
  })

  const connectM = useMutation({
    mutationFn: (id) => tunnelApi.connect(id),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Tunnel connected' })
      invalidate()
    },
    onError: (err) => toast.push({ type: 'error', message: describeError(err, 'Connect failed') }),
  })

  const disconnectM = useMutation({
    mutationFn: (id) => tunnelApi.disconnect(id),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Tunnel disconnected' })
      invalidate()
    },
    onError: (err) => toast.push({ type: 'error', message: describeError(err, 'Disconnect failed') }),
  })

  const tunnels = useMemo(
    () => (Array.isArray(tunnelsQ.data) ? tunnelsQ.data : []),
    [tunnelsQ.data]
  )
  const servers = useMemo(
    () => (Array.isArray(serversQ.data) ? serversQ.data : []),
    [serversQ.data]
  )

  const serverName = (id) => {
    const s = servers.find((x) => x.id === id)
    return s ? s.name : id
  }

  return (
    <div className="tunnels-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>SSH Tunnels</h1>
            <p>
              Port forwarding over SSH — local, remote, and SOCKS5
            </p>
          </div>
          {isAdmin && (
            <button type="button" className="primary-btn" onClick={() => setCreating(true)}>
              <FiPlus />
              New tunnel
            </button>
          )}
        </div>
      </div>

      {tunnelsQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : tunnelsQ.isError ? (
        <EmptyState icon={<FiShuffle size={40} />} title="Failed to load tunnels" description={describeError(tunnelsQ.error)} />
      ) : tunnels.length === 0 ? (
        <EmptyState
          icon={<FiShuffle size={40} />}
          title="No SSH tunnels"
          description={isAdmin ? 'Create a tunnel to forward ports through an SSH connection' : 'An administrator has not created any tunnels yet'}
        />
      ) : (
        <div className="tunnels-list glass">
          <div className="tunnels-row tunnels-head">
            <div>Name</div>
            <div>Server</div>
            <div>Type</div>
            <div>Local</div>
            <div>Remote</div>
            <div>Status</div>
            <div className="actions-col">Actions</div>
          </div>
          {tunnels.map((t) => (
            <div key={t.id} className="tunnels-row">
              <div className="tunnel-name">{t.name}</div>
              <div className="tunnel-server">{serverName(t.server_id)}</div>
              <div>
                <span className="tunnel-type-badge">{TYPE_LABELS[t.type] || t.type}</span>
              </div>
              <div className="mono">{t.local_addr}</div>
              <div className="mono">{t.type === 'socks' ? '-' : t.remote_addr}</div>
              <div>
                <span className={`status-badge ${STATUS_COLORS[t.status] || 'status-unknown'}`}>
                  <span className="status-dot" />
                  {t.status}
                </span>
                {t.started_at && t.status === 'active' && (
                  <div className="muted tunnel-started">since {formatDate(t.started_at)}</div>
                )}
                {t.error && <div className="tunnel-error">{t.error}</div>}
              </div>
              <div className="actions-col">
                {isAdmin && (
                  <>
                    {t.status === 'active' ? (
                      <button
                        type="button"
                        className="action-btn small"
                        disabled={disconnectM.isPending}
                        onClick={() => disconnectM.mutate(t.id)}
                        title="Disconnect"
                      >
                        <FiPause />
                        Stop
                      </button>
                    ) : (
                      <button
                        type="button"
                        className="action-btn small"
                        disabled={connectM.isPending}
                        onClick={() => connectM.mutate(t.id)}
                        title="Connect"
                      >
                        <FiPlay />
                        Start
                      </button>
                    )}
                    <button type="button" className="action-btn small" onClick={() => setEditing(t)} title="Edit">
                      <FiEdit2 />
                      Edit
                    </button>
                    <button
                      type="button"
                      className="action-btn small danger"
                      disabled={deleteM.isPending}
                      onClick={() => setConfirmDelete(t)}
                      title="Delete"
                    >
                      <FiTrash2 />
                      Delete
                    </button>
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {(creating || editing) && (
        <TunnelFormModal
          title={editing ? `Edit ${editing.name}` : 'New tunnel'}
          tunnel={editing}
          servers={servers}
          onSubmit={async (payload) => {
            if (editing) {
              await tunnelApi.update(editing.id, payload)
              toast.push({ type: 'success', message: 'Tunnel updated' })
            } else {
              await tunnelApi.create(payload)
              toast.push({ type: 'success', message: 'Tunnel created' })
            }
            invalidate()
            setCreating(false)
            setEditing(null)
          }}
          onClose={() => { setCreating(false); setEditing(null) }}
        />
      )}

      {confirmDelete && (
        <Modal title="Delete tunnel?" onClose={() => setConfirmDelete(null)} size="small">
          <p className="modal-message">
            This will remove &quot;{confirmDelete.name}&quot; and disconnect it if active.
          </p>
          <div className="modal-actions">
            <button type="button" className="ghost-btn" onClick={() => setConfirmDelete(null)} disabled={deleteM.isPending}>
              Cancel
            </button>
            <button
              type="button"
              className="danger-btn"
              disabled={deleteM.isPending}
              onClick={() => deleteM.mutate(confirmDelete.id)}
            >
              {deleteM.isPending ? <Spinner size={14} /> : null}
              Delete
            </button>
          </div>
        </Modal>
      )}
    </div>
  )
}

function TunnelFormModal({ title, tunnel, servers, onSubmit, onClose }) {
  const [form, setForm] = useState({
    name: tunnel?.name || '',
    server_id: tunnel?.server_id || (servers[0]?.id || ''),
    type: tunnel?.type || 'local',
    local_addr: tunnel?.local_addr || '127.0.0.1:8080',
    remote_addr: tunnel?.remote_addr || '',
    auto_start: tunnel?.auto_start || false,
  })
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const set = (key) => (e) => {
    const value = e.target.type === 'checkbox' ? e.target.checked : e.target.value
    setForm((f) => ({ ...f, [key]: value }))
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (submitting) return
    setSubmitting(true)
    setError('')
    try {
      await onSubmit(form)
    } catch (err) {
      setError(describeError(err, 'Save failed'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal title={title} onClose={onClose} size="normal">
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="modal-grid">
          <div className="form-group">
            <label>Name</label>
            <input type="text" autoFocus value={form.name} onChange={set('name')} required />
          </div>
          <div className="form-group">
            <label>Server</label>
            <select value={form.server_id} onChange={set('server_id')} required>
              {servers.map((s) => (
                <option key={s.id} value={s.id}>{s.name}</option>
              ))}
            </select>
          </div>
          <div className="form-group">
            <label>Type</label>
            <select value={form.type} onChange={set('type')}>
              <option value="local">Local Forward (-L)</option>
              <option value="remote">Remote Forward (-R)</option>
              <option value="socks">SOCKS5 (-D)</option>
            </select>
          </div>
          <div className="form-group">
            <label>Local address</label>
            <input type="text" className="mono" placeholder="127.0.0.1:8080" value={form.local_addr} onChange={set('local_addr')} required />
          </div>
          {form.type !== 'socks' && (
            <div className="form-group full">
              <label>Remote address</label>
              <input type="text" className="mono" placeholder="db.internal:5432" value={form.remote_addr} onChange={set('remote_addr')} required />
            </div>
          )}
          <div className="form-group full checkbox-group">
            <label className="checkbox-label">
              <input type="checkbox" checked={form.auto_start} onChange={set('auto_start')} />
              Auto-start when backend boots
            </label>
          </div>
        </div>
        {error && <div className="modal-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button type="submit" className="primary-btn" disabled={submitting || !form.name.trim() || !form.server_id}>
            {submitting ? <Spinner size={14} /> : null}
            {tunnel ? 'Save' : 'Create'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
