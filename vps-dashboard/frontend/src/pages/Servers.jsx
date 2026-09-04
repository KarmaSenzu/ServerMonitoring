import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import {
  FiServer,
  FiPlus,
  FiEdit2,
  FiTrash2,
  FiSearch,
  FiTag,
  FiZap,
  FiTerminal,
  FiActivity,
  FiFolder,
} from 'react-icons/fi'
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { servers as serversApi, sshApi } from '../api/endpoints.js'
import { useAuth, canMutate, canManage } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import EnvBadge from '../ui/EnvBadge.jsx'
import { describeError } from '../ui/errors.js'
import { Modal } from './Projects.jsx'
import TerminalModal from '../ui/TerminalModal.jsx'
import FileBrowser from '../ui/FileBrowser.jsx'
import './Servers.css'

const STATUS_OPTIONS = ['all', 'online', 'degraded', 'offline', 'unknown']
const ENV_OPTIONS = ['all', 'development', 'staging', 'production']

function formatDate(s) {
  if (!s) return '-'
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return s
  return d.toLocaleString()
}

export default function ServersPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const toast = useToast()
  const [searchParams, setSearchParams] = useSearchParams()

  const isAdmin = canManage(user?.role)
  const canOperate = canMutate(user?.role)

  const [search, setSearch] = useState('')
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState(null) // server object
  const [confirmDelete, setConfirmDelete] = useState(null)
  const [testing, setTesting] = useState(null) // server object being tested
  const [testResult, setTestResult] = useState(null)
  const [commanding, setCommanding] = useState(null) // server object
  const [commandResult, setCommandResult] = useState(null)
  const [metricsServer, setMetricsServer] = useState(null)
  const [terminalServer, setTerminalServer] = useState(null)
  const [filesServer, setFilesServer] = useState(null)

  const environment = searchParams.get('environment') || 'all'
  const status = searchParams.get('status') || 'all'
  const tag = searchParams.get('tag') || ''

  const serversQ = useQuery({
    queryKey: ['servers'],
    queryFn: () => serversApi.list(),
  })

  const tagsQ = useQuery({
    queryKey: ['server-tags'],
    queryFn: () => serversApi.tags(),
  })

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['servers'] })
    queryClient.invalidateQueries({ queryKey: ['server-tags'] })
  }

  const deleteM = useMutation({
    mutationFn: (id) => serversApi.remove(id),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Server deleted' })
      invalidate()
      setConfirmDelete(null)
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Delete failed') })
    },
  })

  const runTest = async (server) => {
    setTesting(server)
    setTestResult(null)
    try {
      const res = await sshApi.test(server.id)
      setTestResult({ ok: true, ...res })
      toast.push({ type: 'success', message: `SSH test succeeded for ${server.name}` })
      invalidate()
    } catch (err) {
      setTestResult({ ok: false, error: describeError(err, 'SSH test failed') })
      toast.push({ type: 'error', message: `SSH test failed for ${server.name}` })
      invalidate()
    }
  }

  const list = useMemo(
    () => (Array.isArray(serversQ.data) ? serversQ.data : []),
    [serversQ.data]
  )
  const tags = Array.isArray(tagsQ.data) ? tagsQ.data.map((t) => t.name) : []

  const filtered = useMemo(() => {
    let out = list
    if (environment !== 'all') out = out.filter((s) => s.environment === environment)
    if (status !== 'all') out = out.filter((s) => s.status === status)
    if (tag) out = out.filter((s) => (s.tags || []).includes(tag))
    const q = search.trim().toLowerCase()
    if (q) {
      out = out.filter(
        (s) =>
          s.name.toLowerCase().includes(q) ||
          s.hostname.toLowerCase().includes(q) ||
          (s.ip_address || '').toLowerCase().includes(q)
      )
    }
    return out
  }, [list, environment, status, tag, search])

  const counts = useMemo(() => {
    const c = { online: 0, degraded: 0, offline: 0, unknown: 0 }
    for (const s of list) {
      if (c[s.status] != null) c[s.status] += 1
    }
    return c
  }, [list])

  const setParam = (key, value) => {
    const next = new URLSearchParams(searchParams)
    if (value && value !== 'all') next.set(key, value)
    else next.delete(key)
    setSearchParams(next, { replace: true })
  }

  return (
    <div className="servers-page">
      {/* === HERO HEADER === */}
      <div className="servers-hero">
        <div className="servers-hero-top">
          <div className="servers-hero-left">
            <div className="servers-hero-title-row">
              <h1 className="servers-hero-title">Server Registry</h1>
              <span className="servers-live-badge">
                <span className="servers-live-dot" />
                {counts.online} NODES CONNECTED
              </span>
            </div>
            <p className="servers-hero-subtitle">
              <span>Central identity of every managed host</span>
              <span className="dot-sep">•</span>
              <span className="servers-hero-ok">{list.length} registered</span>
            </p>
          </div>
          {isAdmin && (
            <button type="button" className="servers-add-btn" onClick={() => setCreating(true)}>
              <FiPlus size={14} />
              <span>+ Add Server</span>
            </button>
          )}
        </div>
      </div>

      {/* === KPI METRICS BAR === */}
      <div className="servers-summary">
        <div className="summary-chip status-online">
          <span className="chip-value">{counts.online}</span>
          <span className="chip-label">Online</span>
        </div>
        <div className="summary-chip status-degraded">
          <span className="chip-value">{counts.degraded}</span>
          <span className="chip-label">Degraded</span>
        </div>
        <div className="summary-chip status-offline">
          <span className="chip-value">{counts.offline}</span>
          <span className="chip-label">Offline</span>
        </div>
        <div className="summary-chip status-unknown">
          <span className="chip-value">{counts.unknown}</span>
          <span className="chip-label">Unknown</span>
        </div>
      </div>

      <div className="servers-filters glass">
        <div className="filter-item">
          <FiSearch className="filter-icon" />
          <input
            type="text"
            placeholder="Search servers, hostnames, IPs..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <select className="filter-select" value={environment} onChange={(e) => setParam('environment', e.target.value)}>
          {ENV_OPTIONS.map((e) => (
            <option key={e} value={e}>
              {e === 'all' ? 'All environments' : e}
            </option>
          ))}
        </select>
        <select className="filter-select" value={status} onChange={(e) => setParam('status', e.target.value)}>
          {STATUS_OPTIONS.map((s) => (
            <option key={s} value={s}>
              {s === 'all' ? 'All statuses' : s}
            </option>
          ))}
        </select>
        <select className="filter-select" value={tag} onChange={(e) => setParam('tag', e.target.value)}>
          <option value="">All tags</option>
          {tags.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
      </div>

      {serversQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : serversQ.isError ? (
        <EmptyState
          icon={<FiServer size={40} />}
          title="Failed to load servers"
          description={describeError(serversQ.error)}
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={<FiServer size={40} />}
          title={list.length === 0 ? 'No servers registered' : 'No servers match the filters'}
          description={
            list.length === 0
              ? 'Add the first server to start building the infrastructure registry'
              : 'Try clearing the search or filters'
          }
        />
      ) : (
        <div className="servers-list glass">
          <div className="servers-row servers-head">
            <div>Server</div>
            <div>Address</div>
            <div>SSH</div>
            <div>Environment</div>
            <div>Status</div>
            <div className="actions-col">Actions</div>
          </div>
          {filtered.map((s) => (
            <div key={s.id} className="servers-row">
              <div className="server-name">
                <span className="server-title">{s.name}</span>
                {(s.tags || []).length > 0 && (
                  <span className="server-tags">
                    {s.tags.map((t) => (
                      <button
                        key={t}
                        type="button"
                        className={`mini-tag${tag === t ? ' active' : ''}`}
                        onClick={() => setParam('tag', tag === t ? '' : t)}
                        title={`Filter by tag: ${t}`}
                      >
                        <FiTag />
                        {t}
                      </button>
                    ))}
                  </span>
                )}
              </div>
              <div className="server-address">
                <div className="mono">{s.hostname}</div>
                {s.ip_address && <div className="mono muted">{s.ip_address}</div>}
              </div>
              <div className="server-ssh mono">
                {s.ssh_username ? `${s.ssh_username}@${s.hostname.split('.')[0]}:${s.ssh_port}` : '-'}
              </div>
              <div>
                <EnvBadge environment={s.environment} />
              </div>
              <div>
                <span className={`status-badge status-${s.status}`}>
                  <span className="status-dot" />
                  {s.status}
                </span>
                {s.last_seen_at && (
                  <div className="last-seen" title={s.status_detail || ''}>
                    seen {formatDate(s.last_seen_at)}
                  </div>
                )}
              </div>
              <div className="actions-col">
                {canOperate && (
                  <>
                    <button
                      type="button"
                      className="action-btn small"
                      onClick={() => setTerminalServer(s)}
                      title="Open SSH terminal"
                    >
                      <FiTerminal />
                      Terminal
                    </button>
                    <button
                      type="button"
                      className="action-btn small"
                      onClick={() => setFilesServer(s)}
                      title="Browse files (SFTP)"
                    >
                      <FiFolder />
                      Files
                    </button>
                    <button
                      type="button"
                      className="action-btn small"
                      onClick={() => runTest(s)}
                      title="Test SSH connectivity"
                    >
                      <FiZap />
                      Test
                    </button>
                    <button
                      type="button"
                      className="action-btn small"
                      onClick={() => {
                        setCommanding(s)
                        setCommandResult(null)
                      }}
                      title="Run a command over SSH"
                    >
                      <FiTerminal />
                      Command
                    </button>
                    <button
                      type="button"
                      className="action-btn small"
                      onClick={() => setMetricsServer(s)}
                      title="View metrics"
                    >
                      <FiActivity />
                      Metrics
                    </button>
                  </>
                )}
                {isAdmin && (
                  <>
                    <button
                      type="button"
                      className="action-btn small"
                      onClick={() => setEditing(s)}
                      title="Edit server"
                    >
                      <FiEdit2 />
                      Edit
                    </button>
                    <button
                      type="button"
                      className="action-btn small danger"
                      disabled={deleteM.isPending}
                      onClick={() => setConfirmDelete(s)}
                      title="Delete server"
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

      {testing && (
        <Modal title={`SSH test — ${testing.name}`} onClose={() => { setTesting(null); setTestResult(null) }} size="normal">
          {!testResult ? (
            <div className="ssh-test-pending">
              <Spinner size={20} />
              <p>Connecting to {testing.hostname}:{testing.ssh_port}…</p>
            </div>
          ) : testResult.ok ? (
            <div className="ssh-test-result">
              <div className="ssh-test-ok status-badge status-online">
                <span className="status-dot" />
                Connected
              </div>
              <div className="ssh-test-details">
                <div className="ssh-test-row">
                  <span className="ssh-test-label">Latency</span>
                  <span className="mono">{testResult.latency_ms} ms</span>
                </div>
                <div className="ssh-test-row">
                  <span className="ssh-test-label">Host key</span>
                  <span className="mono">{testResult.fingerprint}</span>
                </div>
                <div className="ssh-test-row">
                  <span className="ssh-test-label">Server version</span>
                  <span className="mono">{testResult.server_version}</span>
                </div>
                <div className="ssh-test-row">
                  <span className="ssh-test-label">Authenticated as</span>
                  <span className="mono">{testResult.username}</span>
                </div>
              </div>
            </div>
          ) : (
            <div className="ssh-test-result">
              <div className="status-badge status-offline">
                <span className="status-dot" />
                Failed
              </div>
              <div className="ssh-test-error mono">{testResult.error}</div>
            </div>
          )}
          <div className="modal-actions">
            <button
              type="button"
              className="primary-btn"
              onClick={() => runTest(testing)}
              disabled={!testResult}
            >
              <FiZap />
              Test again
            </button>
          </div>
        </Modal>
      )}

      {commanding && (
        <CommandModal
          server={commanding}
          initialResult={commandResult}
          onClose={() => {
            setCommanding(null)
            setCommandResult(null)
          }}
        />
      )}

      {metricsServer && (
        <MetricsModal
          server={metricsServer}
          onClose={() => setMetricsServer(null)}
        />
      )}

      {terminalServer && (
        <TerminalModal
          server={terminalServer}
          onClose={() => setTerminalServer(null)}
        />
      )}

      {filesServer && (
        <FileBrowser
          server={filesServer}
          onClose={() => setFilesServer(null)}
        />
      )}

      {creating && (
        <ServerFormModal
          title="Add server"
          submitting={false}
          onSubmit={async (payload) => {
            try {
              await serversApi.create(payload)
              toast.push({ type: 'success', message: 'Server registered' })
              invalidate()
              setCreating(false)
            } catch (err) {
              toast.push({ type: 'error', message: describeError(err, 'Create failed') })
            }
          }}
          onClose={() => setCreating(false)}
          availableTags={tags}
        />
      )}

      {editing && (
        <ServerFormModal
          title={`Edit ${editing.name}`}
          server={editing}
          onSubmit={async (payload) => {
            try {
              await serversApi.update(editing.id, payload)
              toast.push({ type: 'success', message: 'Server updated' })
              invalidate()
              setEditing(null)
            } catch (err) {
              toast.push({ type: 'error', message: describeError(err, 'Update failed') })
            }
          }}
          onClose={() => setEditing(null)}
          availableTags={tags}
        />
      )}

      {confirmDelete && (
        <Modal title="Delete server?" onClose={() => setConfirmDelete(null)} size="small">
          <p className="modal-message">
            This will permanently remove &quot;{confirmDelete.name}&quot; from the registry.
            Monitoring data and future SSH sessions for this host will be disconnected.
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

const CREDENTIAL_TYPES = [
  { value: 'ssh_key', label: 'SSH key (reference)' },
  { value: 'password', label: 'Password (direct input)' },
  { value: 'agent', label: 'SSH agent' },
]

function ServerFormModal({ title, server, onSubmit, onClose, availableTags }) {
  const [form, setForm] = useState(() => ({
    name: server?.name || '',
    hostname: server?.hostname || '',
    ip_address: server?.ip_address || '',
    ssh_port: server?.ssh_port ?? 22,
    ssh_username: server?.ssh_username || '',
    credential_type: server?.credential_type || 'ssh_key',
    credential_ref: server?.credential_ref || '',
    credential_password: '',  // Always start empty for security
    operating_system: server?.operating_system || '',
    architecture: server?.architecture || '',
    provider: server?.provider || '',
    provider_instance_id: server?.provider_instance_id || '',
    environment: server?.environment || 'production',
    notes: server?.notes || '',
    enabled: server?.enabled ?? true,
    tags: server?.tags ? [...server.tags] : [],
  }))
  const [tagInput, setTagInput] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const set = (key) => (e) => {
    const value = e.target.type === 'checkbox' ? e.target.checked : e.target.value
    setForm((f) => ({ ...f, [key]: value }))
  }

  const addTag = (t) => {
    const tag = (t ?? tagInput).trim()
    if (!tag) return
    setForm((f) => {
      if (f.tags.includes(tag)) return f
      return { ...f, tags: [...f.tags, tag] }
    })
    setTagInput('')
  }

  const removeTag = (t) => {
    setForm((f) => ({ ...f, tags: f.tags.filter((x) => x !== t) }))
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (submitting) return
    setSubmitting(true)
    setError('')
    try {
      const payload = {
        ...form,
        ssh_port: Number(form.ssh_port) || 22,
        tags: form.tags,
      }
      // Only send credential_password when type is password
      // (don't send empty string for other credential types)
      if (form.credential_type !== 'password') {
        delete payload.credential_password
      }
      await onSubmit(payload)
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
          {/* === REQUIRED: Connection === */}
          <div className="form-group">
            <label>Name <span className="field-required">*</span></label>
            <input type="text" autoFocus value={form.name} onChange={set('name')} required placeholder="prod-web-01" />
            <p className="field-help">
              Nama untuk mengidentifikasi server ini di dashboard. Bebas, tapi sebaiknya gunakan format yang konsisten
              (misal: <code>role-environment-number</code>). Nama ini dipakai di seluruh UI, tag, dan log.
            </p>
          </div>

          <div className="form-group">
            <label>Hostname / IP <span className="field-required">*</span></label>
            <input
              type="text"
              className="mono"
              placeholder="server-01.example.com atau 10.0.0.5"
              value={form.hostname}
              onChange={set('hostname')}
              required
            />
            <p className="field-help">
              Alamat yang dipakai sistem untuk SSH ke server. Bisa domain (<code>db1.example.com</code>)
              atau IP langsung (<code>10.0.0.5</code>). Pastikan ini bisa di-resolve/dijangkau
              dari server tempat dashboard berjalan.
            </p>
          </div>

          {/* === OPTIONAL: Metadata === */}
          <div className="form-group">
            <label>IP address (optional)</label>
            <input
              type="text"
              className="mono"
              placeholder="203.0.113.10"
              value={form.ip_address}
              onChange={set('ip_address')}
            />
            <p className="field-help">
              IP publik/privat server. Dipakai hanya untuk tampilan di UI dan pencarian.
              Bisa dikosongkan jika hostname sudah berupa IP.
            </p>
          </div>

          <div className="form-group">
            <label>SSH port</label>
            <input type="number" min="1" max="65535" value={form.ssh_port} onChange={set('ssh_port')} />
            <p className="field-help">
              Port SSH server. Default <code>22</code>. Ubah jika server Anda pakai port
              non-standar (misal <code>2222</code> untuk alasan keamanan).
            </p>
          </div>

          <div className="form-group">
            <label>SSH username <span className="field-required">*</span></label>
            <input type="text" className="mono" placeholder="deploy" value={form.ssh_username} onChange={set('ssh_username')} required />
            <p className="field-help">
              User yang dipakai untuk login SSH ke server. Contoh: <code>root</code>, <code>ubuntu</code>,
              atau <code>deploy</code>. User ini harus punya akses ke perintah yang dibutuhkan
              untuk monitoring (misal <code>top</code>, <code>df</code>, <code>docker</code>).
            </p>
          </div>

          {/* === CREDENTIALS === */}
          <div className="form-group">
            <label>Credential type</label>
            <select value={form.credential_type} onChange={set('credential_type')}>
              {CREDENTIAL_TYPES.map((c) => (
                <option key={c.value} value={c.value}>
                  {c.label}
                </option>
              ))}
            </select>
            <p className="field-help">
              <strong>SSH key (reference):</strong> Pakai private key yang sudah terdaftar di menu SSH Keys.
              <br />
              <strong>Password (direct input):</strong> Masukkan password SSH langsung di field password di bawah.
              <br />
              <strong>SSH agent:</strong> Pakai <code>SSH_AUTH_SOCK</code> yang sedang aktif di server dashboard.
            </p>
          </div>

          {/* Password field — only show when credential_type = password */}
          {form.credential_type === 'password' && (
            <div className="form-group">
              <label>Password <span className="field-required">*</span></label>
              <div className="password-input-wrapper">
                <input
                  type={showPassword ? 'text' : 'password'}
                  className="mono"
                  placeholder="Masukkan password SSH server"
                  value={form.credential_password}
                  onChange={set('credential_password')}
                  autoComplete="new-password"
                />
                <button
                  type="button"
                  className="password-toggle"
                  onClick={() => setShowPassword(!showPassword)}
                  tabIndex={-1}
                >
                  {showPassword ? '🙈' : '👁️'}
                </button>
              </div>
              <p className="field-help">
                Password SSH untuk user <code>{form.ssh_username || 'username'}</code>.
                Disimpan di database server dashboard. Kosongkan jika mau pakai
                env var <code>VPSD_SSH_PASSWORD_&lt;REF&gt;</code> instead (isi Credential reference di bawah).
              </p>
            </div>
          )}

          {/* Credential reference — only show when NOT using direct password, OR for ssh_key/agent */}
          {form.credential_type !== 'password' || form.credential_password === '' ? (
            <div className="form-group">
              <label>Credential reference {form.credential_type === 'password' ? '(optional, env var fallback)' : ''}</label>
              <input
                type="text"
                className="mono"
                placeholder={form.credential_type === 'password'
                  ? "nama env var (opsional, kalau password di atas kosong)"
                  : "production-key (nama saja — bukan secret)"
                }
                value={form.credential_ref}
                onChange={set('credential_ref')}
              />
              <p className="field-help">
                {form.credential_type === 'password' ? (
                  <>Hanya dipakai jika password di atas kosong. Sistem cari env var <code>VPSD_SSH_PASSWORD_&lt;REF&gt;</code>.</>
                ) : (
                  <>Nama referensi (bukan password/key-nya!). Untuk SSH key: nama key yang didaftarkan
                  di menu SSH Keys.</>
                )}
                <strong className="field-warn">Jangan masukkan password atau private key di sini!</strong>
              </p>
            </div>
          ) : null}

          {/* === ENVIRONMENT === */}
          <div className="form-group">
            <label>Environment</label>
            <select value={form.environment} onChange={set('environment')}>
              <option value="development">development</option>
              <option value="staging">staging</option>
              <option value="production">production</option>
            </select>
            <p className="field-help">
              Lingkungan server. Memengaruhi <strong>threshold alert</strong> dan
              <strong>health check multiplier</strong>. Contoh: di development,
              health check 3x lebih lambat dan alert level lebih rendah.
            </p>
          </div>

          {/* === OPTIONAL: System Info — auto-detected via SSH === */}
          <div className="form-group">
            <label>Operating system <span className="field-auto">auto-detected</span></label>
            <input
              type="text"
              placeholder="Terisi otomatis saat SSH berhasil"
              value={form.operating_system}
              onChange={set('operating_system')}
              disabled
              className="auto-field"
            />
            <p className="field-help">
              Terisi otomatis dari server saat koneksi SSH pertama berhasil.
              Tidak perlu diisi manual.
            </p>
          </div>

          <div className="form-group">
            <label>Architecture <span className="field-auto">auto-detected</span></label>
            <input
              type="text"
              placeholder="Terisi otomatis saat SSH berhasil"
              value={form.architecture}
              onChange={set('architecture')}
              disabled
              className="auto-field"
            />
            <p className="field-help">
              Terisi otomatis dari server (misal <code>amd64</code> atau <code>arm64</code>).
            </p>
          </div>

          {/* === CLOUD PROVIDER === */}
          <div className="form-group">
            <label>Provider (optional)</label>
            <input type="text" placeholder="aws / hetzner / digitalocean" value={form.provider} onChange={set('provider')} />
            <p className="field-help">
              Nama cloud provider. Dipakai untuk grouping dan filter di UI.
              Bisa diisi manual atau otomatis dari fitur Cloud Discovery.
            </p>
          </div>

          <div className="form-group">
            <label>Provider instance ID (optional)</label>
            <input
              type="text"
              className="mono"
              placeholder="i-0abc123... (dari console cloud provider)"
              value={form.provider_instance_id}
              onChange={set('provider_instance_id')}
            />
            <p className="field-help">
              ID instance di cloud provider (misal <code>i-0abc123</code> untuk AWS).
              Dipakai untuk sinkronisasi dengan Cloud Discovery. Bisa dikosongkan.
            </p>
          </div>

          {/* === TAGS === */}
          <div className="form-group full">
            <label>Tags</label>
            <p className="field-help">
              Kategori bebas untuk grouping dan filter. Contoh: <code>web</code>,
              <code>database</code>, <code>production</code>, <code>critical</code>.
              Enter untuk menambah, klik tag untuk hapus.
            </p>
            <div className="tag-editor">
              {form.tags.map((t) => (
                <button key={t} type="button" className="mini-tag removable" onClick={() => removeTag(t)}>
                  <FiTag />
                  {t}
                  <FiTrash2 />
                </button>
              ))}
              <input
                type="text"
                className="tag-input"
                placeholder="Add tag + Enter"
                value={tagInput}
                onChange={(e) => setTagInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault()
                    addTag()
                  }
                }}
              />
            </div>
            {availableTags.length > 0 && (
              <div className="tag-suggestions">
                {availableTags
                  .filter((t) => !form.tags.includes(t))
                  .slice(0, 8)
                  .map((t) => (
                    <button key={t} type="button" className="mini-tag suggested" onClick={() => addTag(t)}>
                      <FiTag />
                      {t}
                    </button>
                  ))}
              </div>
            )}
          </div>

          {/* === NOTES === */}
          <div className="form-group full">
            <label>Notes (optional)</label>
            <textarea rows={2} value={form.notes} onChange={set('notes')} placeholder="Catatan internal tentang server ini..." />
            <p className="field-help">
              Catatan bebas untuk tim. Misal: "Restart setiap hari Senin 3AM" atau
              "Disk hampir penuh, perlu upgrade".
            </p>
          </div>

          {/* === ENABLED === */}
          <div className="form-group full checkbox-group">
            <label className="checkbox-label">
              <input type="checkbox" checked={form.enabled} onChange={set('enabled')} />
              Enabled (include in monitoring &amp; operations)
            </label>
            <p className="field-help">
              Jika dicentang: server akan dimonitor otomatis (metrics tiap 60 detik),
              masuk di alert evaluation, dan bisa dioperasikan (container, command, terminal).
              Jika tidak: server "paused" — tidak dimonitor tapi tetap di registry.
            </p>
          </div>
        </div>

        {/* === INFO BANNER: What happens next === */}
        <div className="form-info-banner">
          <span className="info-icon">ℹ️</span>
          <div>
            <strong>Setelah server didaftarkan:</strong>
            <ul>
              <li>Sistem akan coba SSH setiap 60 detik untuk ambil metrics (CPU, RAM, disk, network)</li>
              <li>Server muncul di dashboard dengan status <code>unknown</code> sampai koneksi SSH pertama berhasil</li>
              <li>Jika koneksi gagal, status jadi <code>offline</code> dan event error dicatat</li>
              <li>Anda bisa test koneksi SSH langsung dari menu Servers → Test SSH</li>
            </ul>
          </div>
        </div>

        {error && <div className="modal-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button
            type="submit"
            className="primary-btn"
            disabled={submitting || !form.name.trim() || !form.hostname.trim() || !form.ssh_username.trim()}
          >
            {submitting ? <Spinner size={14} /> : null}
            {server ? 'Save changes' : 'Register server'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

// CommandModal runs a single bounded SSH command against a server and
// displays stdout/stderr/exit code.
function CommandModal({ server, onClose }) {
  const [command, setCommand] = useState('uptime')
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState(null)
  const [error, setError] = useState('')

  const run = async (e) => {
    e.preventDefault()
    if (running || !command.trim()) return
    setRunning(true)
    setError('')
    setResult(null)
    try {
      const res = await sshApi.command(server.id, { command: command.trim() })
      setResult(res)
    } catch (err) {
      setError(describeError(err, 'Command failed'))
    } finally {
      setRunning(false)
    }
  }

  return (
    <Modal title={`Run command — ${server.name}`} onClose={onClose} size="normal">
      <form className="modal-form" onSubmit={run}>
        <div className="modal-grid">
          <div className="form-group full">
            <label>Command</label>
            <div className="command-input-row">
              <span className="mono command-prompt">$</span>
              <input
                type="text"
                className="mono"
                autoFocus
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                placeholder="uptime"
              />
            </div>
          </div>
        </div>
        <p className="modal-hint">
          Executed as <code className="inline-code">{server.ssh_username}</code> on{' '}
          <code className="inline-code">
            {server.hostname}:{server.ssh_port}
          </code>
          , bounded by a 30s timeout and a 1 MiB output cap. Every execution is
          audited.
        </p>

        {running && (
          <div className="command-output glass">
            <Spinner size={16} />
            <span>Running…</span>
          </div>
        )}

        {error && <div className="modal-error">{error}</div>}

        {result && !running && (
          <div className="command-result">
            <div className="command-result-header">
              <span className={`status-badge ${result.exit_code === 0 ? 'status-online' : 'status-degraded'}`}>
                <span className="status-dot" />
                exit {result.exit_code}
              </span>
              <span className="mono muted">{result.duration_ms} ms</span>
            </div>
            {result.stdout && (
              <div className="command-block">
                <span className="command-block-label">stdout</span>
                <pre className="mono">{result.stdout}</pre>
              </div>
            )}
            {result.stderr && (
              <div className="command-block stderr">
                <span className="command-block-label">stderr</span>
                <pre className="mono">{result.stderr}</pre>
              </div>
            )}
            {!result.stdout && !result.stderr && (
              <div className="command-empty mono">(no output)</div>
            )}
          </div>
        )}

        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={running}>
            Close
          </button>
          <button type="submit" className="primary-btn" disabled={running || !command.trim()}>
            {running ? <Spinner size={14} /> : <FiTerminal />}
            Run
          </button>
        </div>
      </form>
    </Modal>
  )
}

// MetricsModal shows the latest metric snapshot and a CPU/memory history
// chart for a single server.
function MetricsModal({ server, onClose }) {
  const metricsQ = useQuery({
    queryKey: ['server-metrics-latest', server.id],
    queryFn: () => serversApi.metrics(server.id),
    refetchInterval: 15000,
  })

  const historyQ = useQuery({
    queryKey: ['server-metrics-history', server.id],
    queryFn: () => serversApi.history(server.id, { limit: 60 }),
    refetchInterval: 15000,
  })

  const latest = metricsQ.data || null
  const history = Array.isArray(historyQ.data) ? historyQ.data : []

  const chartData = history.map((m) => ({
    time: new Date(m.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    cpu: m.cpu_usage != null ? Math.round(m.cpu_usage * 10) / 10 : 0,
    mem: m.mem_percent != null ? Math.round(m.mem_percent * 10) / 10 : 0,
    disk: m.disk_percent != null ? Math.round(m.disk_percent * 10) / 10 : 0,
  }))

  const fmtBytes = (b) => {
    if (!b || b <= 0) return '-'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    let i = 0
    let val = b
    while (val >= 1024 && i < units.length - 1) {
      val /= 1024
      i++
    }
    return `${Math.round(val * 100) / 100} ${units[i]}`
  }

  const fmtUptime = (s) => {
    if (!s || s <= 0) return '-'
    const days = Math.floor(s / 86400)
    const hours = Math.floor((s % 86400) / 3600)
    const mins = Math.floor((s % 3600) / 60)
    if (days > 0) return `${days}d ${hours}h`
    if (hours > 0) return `${hours}h ${mins}m`
    return `${mins}m`
  }

  return (
    <Modal title={`Metrics — ${server.name}`} onClose={onClose} size="normal">
      {metricsQ.isLoading ? (
        <div className="ssh-test-pending">
          <Spinner size={20} />
          <span>Loading metrics…</span>
        </div>
      ) : metricsQ.isError ? (
        <div className="ssh-test-error mono">
          {describeError(metricsQ.error, 'Failed to load metrics')}
        </div>
      ) : !latest ? (
        <p className="modal-message">
          No metrics collected yet. The remote monitoring engine polls
          every 60 seconds; the first sample should appear shortly after
          the server is registered and its SSH credentials configured.
        </p>
      ) : latest.error ? (
        <div className="ssh-test-error mono">{latest.error}</div>
      ) : (
        <div className="metrics-modal">
          <div className="metrics-grid">
            <div className="metric-card">
              <span className="metric-label">CPU</span>
              <span className="metric-value">{latest.cpu_usage?.toFixed(1)}%</span>
              <span className="metric-sub">load: {latest.cpu_load1?.toFixed(2)}</span>
            </div>
            <div className="metric-card">
              <span className="metric-label">Memory</span>
              <span className="metric-value">{latest.mem_percent?.toFixed(1)}%</span>
              <span className="metric-sub">{fmtBytes(latest.mem_used)} / {fmtBytes(latest.mem_total)}</span>
            </div>
            <div className="metric-card">
              <span className="metric-label">Disk</span>
              <span className="metric-value">{latest.disk_percent?.toFixed(1)}%</span>
              <span className="metric-sub">{fmtBytes(latest.disk_used)} / {fmtBytes(latest.disk_total)}</span>
            </div>
            <div className="metric-card">
              <span className="metric-label">Uptime</span>
              <span className="metric-value">{fmtUptime(latest.uptime)}</span>
              <span className="metric-sub">&nbsp;</span>
            </div>
          </div>

          {chartData.length > 1 && (
            <div className="metrics-chart">
              <h4>CPU / Memory / Disk %</h4>
              <ResponsiveContainer width="100%" height={200}>
                <LineChart data={chartData} margin={{ top: 5, right: 20, bottom: 5, left: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.1)" />
                  <XAxis dataKey="time" tick={{ fontSize: 10, fill: '#8888aa' }} />
                  <YAxis domain={[0, 100]} tick={{ fontSize: 10, fill: '#8888aa' }} />
                  <Tooltip
                    contentStyle={{ background: 'rgba(20,20,30,0.95)', border: '1px solid rgba(255,255,255,0.15)', borderRadius: 8 }}
                  />
                  <Line type="monotone" dataKey="cpu" stroke="var(--success)" strokeWidth={2} dot={false} name="CPU" />
                  <Line type="monotone" dataKey="mem" stroke="var(--warning)" strokeWidth={2} dot={false} name="Memory" />
                  <Line type="monotone" dataKey="disk" stroke="var(--danger)" strokeWidth={2} dot={false} name="Disk" />
                </LineChart>
              </ResponsiveContainer>
            </div>
          )}

          <div className="metrics-meta">
            <span className="muted mono">Last seen: {formatDate(latest.ts)}</span>
          </div>
        </div>
      )}

      <div className="modal-actions">
        <button type="button" className="ghost-btn" onClick={onClose}>Close</button>
      </div>
    </Modal>
  )
}
