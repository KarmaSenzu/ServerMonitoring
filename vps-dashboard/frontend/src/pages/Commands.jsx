import { useMemo, useState } from 'react'
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query'
import {
  FiTerminal,
  FiPlus,
  FiEdit2,
  FiTrash2,
  FiPlay,
  FiAlertTriangle,
  FiCopy,
  FiClock,
} from 'react-icons/fi'
import { commandApi, servers as serversApi } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import { describeError } from '../ui/errors.js'
import { Modal } from './Projects.jsx'
import './Commands.css'

const DANGER_COLORS = {
  safe: 'status-online',
  caution: 'status-degraded',
  dangerous: 'status-offline',
}

function formatDate(s) {
  if (!s) return '-'
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString()
}

export default function CommandsPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const toast = useToast()
  const isAdmin = user?.role === 'admin'

  const [editing, setEditing] = useState(null)
  const [creating, setCreating] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(null)
  const [executing, setExecuting] = useState(null) // snippet

  const snippetsQ = useQuery({
    queryKey: ['command-snippets'],
    queryFn: () => commandApi.snippets(),
  })

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['command-snippets'] })
  }

  const deleteM = useMutation({
    mutationFn: (id) => commandApi.deleteSnippet(id),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Snippet deleted' })
      invalidate()
      setConfirmDelete(null)
    },
    onError: (err) => toast.push({ type: 'error', message: describeError(err, 'Delete failed') }),
  })

  const snippets = useMemo(
    () => (Array.isArray(snippetsQ.data) ? snippetsQ.data : []),
    [snippetsQ.data]
  )

  return (
    <div className="commands-page">
      {/* === HERO HEADER === */}
      <div className="commands-hero">
        <div className="commands-hero-top">
          <div className="commands-hero-left">
            <div className="commands-hero-title-row">
              <h1 className="commands-hero-title">SSH &amp; Fleet Commands</h1>
              <span className="commands-live-badge">
                <span className="commands-live-dot" />
                E2E ENCRYPTED (ED25519)
              </span>
            </div>
            <p className="commands-hero-subtitle">
              Secure multi-session SSH terminal, credential vault, and broadcast execution engine across all managed nodes.
            </p>
          </div>
          {isAdmin && (
            <button type="button" className="commands-add-btn" onClick={() => setCreating(true)}>
              <FiPlus size={14} />
              <span>+ New Snippet</span>
            </button>
          )}
        </div>
      </div>

      {snippetsQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : snippetsQ.isError ? (
        <EmptyState icon={<FiTerminal size={40} />} title="Failed to load snippets" description={describeError(snippetsQ.error)} />
      ) : snippets.length === 0 ? (
        <EmptyState
          icon={<FiTerminal size={40} />}
          title="No command snippets yet"
          description={isAdmin ? 'Create reusable commands for common operations across your fleet' : 'An administrator has not created any snippets yet'}
        />
      ) : (
        <div className="commands-list glass">
          {snippets.map((s) => (
            <div key={s.id} className="command-snippet-card">
              <div className="snippet-header">
                <span className="snippet-name">{s.name}</span>
                <span className={`danger-badge ${DANGER_COLORS[s.danger_level] || DANGER_COLORS.safe}`}>
                  {s.danger_level}
                </span>
              </div>
              {s.description && <p className="snippet-description">{s.description}</p>}
              <div className="snippet-command">
                <code className="mono">{s.command}</code>
              </div>
              {s.variables && s.variables.length > 0 && (
                <div className="snippet-vars">
                  {s.variables.map((v) => (
                    <span key={v} className="snippet-var">{`{{${v}}}`}</span>
                  ))}
                </div>
              )}
              <div className="snippet-meta muted">
                <FiClock /> {formatDate(s.updated_at)}
                {s.updated_by && <span> · {s.updated_by}</span>}
              </div>
              <div className="snippet-actions">
                {isAdmin && (
                  <>
                    <button
                      type="button"
                      className="action-btn small primary"
                      onClick={() => setExecuting(s)}
                    >
                      <FiPlay />
                      Execute
                    </button>
                    <button type="button" className="action-btn small" onClick={() => setEditing(s)}>
                      <FiEdit2 />
                      Edit
                    </button>
                    <button
                      type="button"
                      className="action-btn small danger"
                      disabled={deleteM.isPending}
                      onClick={() => setConfirmDelete(s)}
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

      {creating && (
        <SnippetFormModal
          title="New snippet"
          onSubmit={async (payload) => {
            await commandApi.createSnippet(payload)
            toast.push({ type: 'success', message: 'Snippet created' })
            invalidate()
            setCreating(false)
          }}
          onClose={() => setCreating(false)}
        />
      )}

      {editing && (
        <SnippetFormModal
          title={`Edit ${editing.name}`}
          snippet={editing}
          onSubmit={async (payload) => {
            await commandApi.updateSnippet(editing.id, payload)
            toast.push({ type: 'success', message: 'Snippet updated' })
            invalidate()
            setEditing(null)
          }}
          onClose={() => setEditing(null)}
        />
      )}

      {executing && (
        <ExecuteModal
          snippet={executing}
          onClose={() => setExecuting(null)}
        />
      )}

      {confirmDelete && (
        <Modal title="Delete snippet?" onClose={() => setConfirmDelete(null)} size="small">
          <p className="modal-message">
            This will permanently remove &quot;{confirmDelete.name}&quot;.
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

function SnippetFormModal({ title, snippet, onSubmit, onClose }) {
  const [form, setForm] = useState({
    name: snippet?.name || '',
    description: snippet?.description || '',
    command: snippet?.command || '',
    variables: snippet?.variables ? [...snippet.variables] : [],
    danger_level: snippet?.danger_level || 'safe',
  })
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const set = (key) => (e) => setForm({ ...form, [key]: e.target.value })

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
            <label>Danger level</label>
            <select value={form.danger_level} onChange={set('danger_level')}>
              <option value="safe">safe</option>
              <option value="caution">caution</option>
              <option value="dangerous">dangerous</option>
            </select>
          </div>
          <div className="form-group full">
            <label>Description</label>
            <input type="text" value={form.description} onChange={set('description')} />
          </div>
          <div className="form-group full">
            <label>Command</label>
            <textarea
              rows={3}
              className="mono"
              placeholder="docker ps -a"
              value={form.command}
              onChange={set('command')}
              required
            />
          </div>
        </div>
        {error && <div className="modal-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button type="submit" className="primary-btn" disabled={submitting || !form.name.trim() || !form.command.trim()}>
            {submitting ? <Spinner size={14} /> : null}
            {snippet ? 'Save' : 'Create'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

function ExecuteModal({ snippet, onClose }) {
  const toast = useToast()
  const serversQ = useQuery({
    queryKey: ['servers'],
    queryFn: () => serversApi.list(),
  })
  const [selected, setSelected] = useState(new Set())
  const [preview, setPreview] = useState(null)
  const [result, setResult] = useState(null)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')

  const servers = useMemo(
    () => (Array.isArray(serversQ.data) ? serversQ.data : []),
    [serversQ.data]
  )

  const toggle = (id) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const doPreview = async () => {
    setError('')
    setPreview(null)
    try {
      const res = await commandApi.preview({
        command: snippet.command,
        server_ids: [...selected],
      })
      setPreview(res)
    } catch (err) {
      setError(describeError(err, 'Preview failed'))
    }
  }

  const doExecute = async () => {
    setRunning(true)
    setError('')
    setResult(null)
    try {
      const res = await commandApi.execute({
        command: snippet.command,
        server_ids: [...selected],
        snippet_id: snippet.id,
      })
      setResult(res)
      toast.push({ type: 'success', message: `Executed on ${res.total} hosts` })
    } catch (err) {
      setError(describeError(err, 'Execution failed'))
    } finally {
      setRunning(false)
    }
  }

  return (
    <Modal title={`Execute — ${snippet.name}`} onClose={onClose} size="normal">
      <div className="execute-modal">
        <div className="execute-command">
          <label>Command</label>
          <code className="mono">{snippet.command}</code>
        </div>

        <div className="execute-servers">
          <label>Select target servers ({selected.size} selected)</label>
          {serversQ.isLoading ? (
            <Spinner size={16} />
          ) : (
            <div className="server-select-list">
              {servers.map((s) => (
                <label key={s.id} className="server-select-item">
                  <input
                    type="checkbox"
                    checked={selected.has(s.id)}
                    onChange={() => toggle(s.id)}
                  />
                  <span className="server-select-name">{s.name}</span>
                  <span className={`status-badge status-${s.status}`}>
                    <span className="status-dot" />
                    {s.status}
                  </span>
                </label>
              ))}
            </div>
          )}
        </div>

        {preview && (
          <div className="blast-preview glass">
            <div className="blast-header">
              <FiAlertTriangle className={DANGER_COLORS[preview.danger_level]} />
              <span className={`danger-badge ${DANGER_COLORS[preview.danger_level]}`}>
                {preview.danger_level}
              </span>
              <span className="blast-count">{preview.target_count} hosts will be affected</span>
            </div>
            {preview.warning && (
              <p className="blast-warning">{preview.warning}</p>
            )}
            <div className="blast-targets">
              {preview.targets.map((t) => (
                <span key={t} className="blast-target">{t}</span>
              ))}
            </div>
          </div>
        )}

        {error && <div className="modal-error">{error}</div>}

        {result && (
          <div className="execute-results">
            <div className="results-summary">
              <span className="result-stat status-online">{result.success} success</span>
              <span className="result-stat status-offline">{result.failed} failed</span>
            </div>
            {result.results.map((r) => (
              <div key={r.server_id} className={`result-row status-${r.status}`}>
                <div className="result-header">
                  <span className="result-server">{r.server_name}</span>
                  <span className={`status-badge status-${r.status}`}>
                    <span className="status-dot" />
                    {r.status}
                  </span>
                  <span className="mono muted">{r.duration_ms}ms</span>
                </div>
                {r.stdout && (
                  <pre className="mono result-output">{r.stdout}</pre>
                )}
                {r.stderr && (
                  <pre className="mono result-output stderr">{r.stderr}</pre>
                )}
                {r.error && (
                  <pre className="mono result-output error">{r.error}</pre>
                )}
              </div>
            ))}
          </div>
        )}

        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={running}>
            Close
          </button>
          <button
            type="button"
            className="ghost-btn"
            onClick={doPreview}
            disabled={selected.size === 0 || running}
          >
            <FiAlertTriangle />
            Preview
          </button>
          <button
            type="button"
            className="primary-btn"
            onClick={doExecute}
            disabled={selected.size === 0 || running}
          >
            {running ? <Spinner size={14} /> : <FiPlay />}
            Execute ({selected.size})
          </button>
        </div>
      </div>
    </Modal>
  )
}
