import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import {
  FiPlus,
  FiSearch,
  FiX,
  FiEdit2,
  FiTrash2,
  FiExternalLink,
  FiTag,
  FiPlay,
  FiSquare,
  FiRotateCw,
  FiRefreshCw,
  FiActivity,
  FiInfo,
  FiUploadCloud,
  FiCopy,
  FiKey,
  FiAlertTriangle,
  FiChevronDown,
  FiChevronRight,
} from 'react-icons/fi'
import {
  ScatterChart,
  Scatter,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  ReferenceLine,
} from 'recharts'
import { projects, events as eventsApi } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import SeverityBadge from '../ui/SeverityBadge.jsx'
import EnvBadge from '../ui/EnvBadge.jsx'
import DeploymentStatusBadge from '../ui/DeploymentStatusBadge.jsx'
import RelativeTime from '../ui/RelativeTime.jsx'
import { formatAbsolute } from '../ui/format.js'
import { describeError } from '../ui/errors.js'
import './Projects.css'

const ENABLED_OPTIONS = [
  { value: 'all', label: 'All' },
  { value: 'true', label: 'Enabled only' },
  { value: 'false', label: 'Disabled only' },
]

const ENVIRONMENTS = ['development', 'staging', 'production']
const DEFAULT_DEPLOY_TIMEOUT = 300

const emptyForm = {
  name: '',
  description: '',
  domain: '',
  port: '',
  container_name: '',
  pm2_name: '',
  tunnel_service: '',
  health_url: '',
  enabled: true,
  tags: '',
  notes: '',
  environment: 'production',
  deploy_enabled: false,
  deploy_command: '',
  deploy_timeout_seconds: String(DEFAULT_DEPLOY_TIMEOUT),
  deploy_working_dir: '',
}

function projectToForm(p) {
  return {
    name: p.name || '',
    description: p.description || '',
    domain: p.domain || '',
    port: p.port ? String(p.port) : '',
    container_name: p.container_name || '',
    pm2_name: p.pm2_name || '',
    tunnel_service: p.tunnel_service || '',
    health_url: p.health_url || '',
    enabled: p.enabled !== false,
    tags: Array.isArray(p.tags) ? p.tags.join(', ') : '',
    notes: p.notes || '',
    environment: p.environment || 'production',
    deploy_enabled: !!p.deploy_enabled,
    deploy_command: p.deploy_command || '',
    deploy_timeout_seconds:
      p.deploy_timeout_seconds != null && p.deploy_timeout_seconds > 0
        ? String(p.deploy_timeout_seconds)
        : String(DEFAULT_DEPLOY_TIMEOUT),
    deploy_working_dir: p.deploy_working_dir || '',
  }
}

function formToPayload(form) {
  const timeoutNum = Number(form.deploy_timeout_seconds)
  return {
    name: form.name.trim(),
    description: form.description.trim(),
    domain: form.domain.trim(),
    port: form.port ? Number(form.port) : 0,
    container_name: form.container_name.trim(),
    pm2_name: form.pm2_name.trim(),
    tunnel_service: form.tunnel_service.trim(),
    health_url: form.health_url.trim(),
    enabled: form.enabled,
    tags: form.tags
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean),
    notes: form.notes,
    environment: form.environment || 'production',
    deploy_enabled: !!form.deploy_enabled,
    deploy_command: form.deploy_command.trim(),
    deploy_timeout_seconds:
      Number.isFinite(timeoutNum) && timeoutNum > 0
        ? Math.floor(timeoutNum)
        : DEFAULT_DEPLOY_TIMEOUT,
    deploy_working_dir: form.deploy_working_dir.trim(),
  }
}

export default function ProjectsPage() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const queryClient = useQueryClient()
  const toast = useToast()
  const [searchParams, setSearchParams] = useSearchParams()

  const [searchInput, setSearchInput] = useState('')
  const [search, setSearch] = useState('')
  const [tag, setTag] = useState('')
  const [enabledFilter, setEnabledFilter] = useState('all')
  const [editing, setEditing] = useState(null) // null | "new" | project
  const [confirmDelete, setConfirmDelete] = useState(null)
  const [detailsId, setDetailsId] = useState(searchParams.get('focus') || null)
  const [detailsTab, setDetailsTab] = useState(
    searchParams.get('tab') === 'deployments' ? 'deployments' : 'overview'
  )

  // Mirror the focus param so deep links to a project's details work.
  useEffect(() => {
    const next = new URLSearchParams(searchParams)
    if (detailsId) next.set('focus', detailsId)
    else next.delete('focus')
    if (detailsId && detailsTab === 'deployments') {
      next.set('tab', 'deployments')
    } else {
      next.delete('tab')
    }
    setSearchParams(next, { replace: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detailsId, detailsTab])

  // Debounce search input.
  useEffect(() => {
    const t = setTimeout(() => setSearch(searchInput.trim()), 300)
    return () => clearTimeout(t)
  }, [searchInput])

  const filterParams = useMemo(() => {
    const params = {}
    if (search) params.q = search
    if (tag) params.tag = tag
    if (enabledFilter === 'true') params.enabled = 'true'
    if (enabledFilter === 'false') params.enabled = 'false'
    return params
  }, [search, tag, enabledFilter])

  const projectsQ = useQuery({
    queryKey: ['projects', filterParams],
    queryFn: () => projects.list(filterParams),
  })

  const createM = useMutation({
    mutationFn: (payload) => projects.create(payload),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Project created' })
      queryClient.invalidateQueries({ queryKey: ['projects'] })
      setEditing(null)
    },
  })

  const updateM = useMutation({
    mutationFn: ({ id, payload }) => projects.update(id, payload),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Project updated' })
      queryClient.invalidateQueries({ queryKey: ['projects'] })
      setEditing(null)
    },
  })

  const deleteM = useMutation({
    mutationFn: (id) => projects.remove(id),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Project deleted' })
      queryClient.invalidateQueries({ queryKey: ['projects'] })
      setConfirmDelete(null)
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Delete failed') })
    },
  })

  const list = useMemo(
    () => (Array.isArray(projectsQ.data) ? projectsQ.data : []),
    [projectsQ.data]
  )
  const detailsProject = useMemo(() => {
    if (!detailsId) return null
    return list.find((p) => p.id === detailsId) || null
  }, [list, detailsId])

  return (
    <div className="projects-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>Projects</h1>
            <p>Registry of projects detected and adopted on this VPS</p>
          </div>
          {isAdmin && (
            <button
              type="button"
              className="primary-btn"
              onClick={() => setEditing('new')}
            >
              <FiPlus />
              New project
            </button>
          )}
        </div>
      </div>

      <div className="projects-toolbar glass">
        <div className="toolbar-search">
          <FiSearch />
          <input
            type="text"
            placeholder="Search by name, domain, description"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
          />
          {searchInput && (
            <button
              type="button"
              className="toolbar-clear"
              onClick={() => setSearchInput('')}
              aria-label="Clear search"
            >
              <FiX />
            </button>
          )}
        </div>
        <div className="toolbar-filter">
          <FiTag />
          <input
            type="text"
            placeholder="Tag filter"
            value={tag}
            onChange={(e) => setTag(e.target.value.trim())}
          />
        </div>
        <div className="toolbar-toggle">
          {ENABLED_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              type="button"
              className={`toggle-btn ${enabledFilter === opt.value ? 'active' : ''}`}
              onClick={() => setEnabledFilter(opt.value)}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      {projectsQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : projectsQ.isError ? (
        <EmptyState
          icon={<FiX size={40} />}
          title="Failed to load projects"
          description={describeError(projectsQ.error)}
        />
      ) : list.length === 0 ? (
        <EmptyState
          icon={<FiSearch size={40} />}
          title="No projects yet"
          description={
            isAdmin
              ? 'Add a project manually or adopt one from the Discovery page'
              : 'Ask an administrator to add or adopt a project'
          }
        />
      ) : (
        <div className="projects-grid">
          {list.map((p) => (
            <ProjectCard
              key={p.id}
              project={p}
              isAdmin={isAdmin}
              onOpen={() => {
                setDetailsTab('overview')
                setDetailsId(p.id)
              }}
              onEdit={() => setEditing(p)}
              onDelete={() => setConfirmDelete(p)}
              onDeploy={() => {
                setDetailsTab('deployments')
                setDetailsId(p.id)
              }}
            />
          ))}
        </div>
      )}
      {detailsProject && (
        <ProjectDetailsDrawer
          project={detailsProject}
          isAdmin={isAdmin}
          tab={detailsTab}
          onTabChange={setDetailsTab}
          onEdit={() => setEditing(detailsProject)}
          onDelete={() => setConfirmDelete(detailsProject)}
          onClose={() => setDetailsId(null)}
        />
      )}
      {editing && isAdmin && (
        <ProjectFormModal
          mode={editing === 'new' ? 'create' : 'edit'}
          initial={editing === 'new' ? null : editing}
          submitting={createM.isPending || updateM.isPending}
          error={
            (createM.isError && createM.error) ||
            (updateM.isError && updateM.error) ||
            null
          }
          onSubmit={(form) => {
            const payload = formToPayload(form)
            if (editing === 'new') {
              createM.mutate(payload)
            } else {
              updateM.mutate({ id: editing.id, payload })
            }
          }}
          onClose={() => {
            createM.reset()
            updateM.reset()
            setEditing(null)
          }}
        />
      )}

      {confirmDelete && isAdmin && (
        <ConfirmModal
          title="Delete project?"
          message={`This will remove "${confirmDelete.name}" from the registry.`}
          confirmLabel="Delete"
          variant="danger"
          submitting={deleteM.isPending}
          onConfirm={() => deleteM.mutate(confirmDelete.id)}
          onCancel={() => setConfirmDelete(null)}
        />
      )}
    </div>
  )
}

function ProjectCard({ project, isAdmin, onOpen, onEdit, onDelete, onDeploy }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [healthState, setHealthState] = useState({ status: 'idle', data: null, error: null })
  const [actionPending, setActionPending] = useState(null)

  const hasHealthUrl = Boolean(project.health_url && project.health_url.trim())
  const deployConfigured =
    !!project.deploy_enabled && !!(project.deploy_command && project.deploy_command.trim())

  const checkHealth = async () => {
    if (!hasHealthUrl || healthState.status === 'loading') return
    setHealthState({ status: 'loading', data: null, error: null })
    try {
      const data = await projects.health(project.id)
      setHealthState({ status: 'done', data, error: null })
    } catch (err) {
      setHealthState({ status: 'done', data: null, error: err })
    }
  }

  const handleAction = async (action) => {
    if (!isAdmin || actionPending) return
    setActionPending(action)
    try {
      const res = await projects.action(project.id, action)
      const target = res?.target ? ` → ${res.target}` : ''
      toast.push({
        type: 'success',
        message: `Project ${action}${target}`,
      })
      queryClient.invalidateQueries({ queryKey: ['docker', 'containers'] })
      queryClient.invalidateQueries({ queryKey: ['pm2', 'processes'] })
    } catch (err) {
      toast.push({ type: 'error', message: describeError(err, `Failed to ${action} project`) })
    } finally {
      setActionPending(null)
    }
  }

  return (
    <div className="project-card glass animate-in">
      <div className="project-card-header">
        <div>
          <button type="button" className="project-name-btn" onClick={onOpen}>
            <h3>{project.name}</h3>
          </button>
          <div className="project-card-subline">
            <EnvBadge environment={project.environment} dot />
            {project.description && (
              <p className="project-desc">{project.description}</p>
            )}
          </div>
        </div>
        <div className="project-badges">
          <HealthPill
            hasHealthUrl={hasHealthUrl}
            state={healthState}
            onCheck={checkHealth}
          />
          <span className={`enabled-badge ${project.enabled ? 'on' : 'off'}`}>
            {project.enabled ? 'Enabled' : 'Disabled'}
          </span>
        </div>
      </div>

      <dl className="project-fields">
        {project.domain && (
          <div className="field-row">
            <dt>Domain</dt>
            <dd>
              <a
                href={`https://${project.domain}`}
                target="_blank"
                rel="noreferrer"
              >
                {project.domain}
                <FiExternalLink size={11} />
              </a>
            </dd>
          </div>
        )}
        {project.port > 0 && (
          <div className="field-row">
            <dt>Port</dt>
            <dd>{project.port}</dd>
          </div>
        )}
        {project.container_name && (
          <div className="field-row">
            <dt>Container</dt>
            <dd>{project.container_name}</dd>
          </div>
        )}
        {project.pm2_name && (
          <div className="field-row">
            <dt>PM2</dt>
            <dd>{project.pm2_name}</dd>
          </div>
        )}
        {project.tunnel_service && (
          <div className="field-row">
            <dt>Tunnel</dt>
            <dd>{project.tunnel_service}</dd>
          </div>
        )}
        {project.health_url && (
          <div className="field-row">
            <dt>Health</dt>
            <dd className="truncate">{project.health_url}</dd>
          </div>
        )}
      </dl>

      {project.tags && project.tags.length > 0 && (
        <div className="project-tags">
          {project.tags.map((t) => (
            <span key={t} className="tag-chip">{t}</span>
          ))}
        </div>
      )}

      {isAdmin && (
        <div className="project-card-actions">
          <button
            type="button"
            className="action-btn project-run"
            onClick={() => handleAction('start')}
            disabled={actionPending !== null}
            title="Start associated container or pm2 process"
          >
            {actionPending === 'start' ? <FiRefreshCw className="spinning" /> : <FiPlay />}
            Start
          </button>
          <button
            type="button"
            className="action-btn project-run"
            onClick={() => handleAction('stop')}
            disabled={actionPending !== null}
            title="Stop associated container or pm2 process"
          >
            {actionPending === 'stop' ? <FiRefreshCw className="spinning" /> : <FiSquare />}
            Stop
          </button>
          <button
            type="button"
            className="action-btn project-run"
            onClick={() => handleAction('restart')}
            disabled={actionPending !== null}
            title="Restart associated container or pm2 process"
          >
            {actionPending === 'restart' ? <FiRefreshCw className="spinning" /> : <FiRotateCw />}
            Restart
          </button>
          <button
            type="button"
            className="action-btn project-deploy"
            onClick={() => deployConfigured && onDeploy && onDeploy()}
            disabled={!deployConfigured}
            title={
              deployConfigured
                ? 'Trigger a manual deployment'
                : 'Deploy not configured'
            }
          >
            <FiUploadCloud />
            Deploy
          </button>
          <button type="button" className="action-btn" onClick={onOpen}>
            <FiInfo />
            Details
          </button>
          <button type="button" className="action-btn" onClick={onEdit}>
            <FiEdit2 />
            Edit
          </button>
          <button
            type="button"
            className="action-btn danger"
            onClick={onDelete}
          >
            <FiTrash2 />
            Delete
          </button>
        </div>
      )}
      {!isAdmin && (
        <div className="project-card-actions">
          <button type="button" className="action-btn" onClick={onOpen}>
            <FiInfo />
            Details
          </button>
        </div>
      )}
    </div>
  )
}

function HealthPill({ hasHealthUrl, state, onCheck }) {
  if (!hasHealthUrl) {
    return (
      <span className="health-pill na" title="No health_url configured">
        Not configured
      </span>
    )
  }
  if (state.status === 'loading') {
    return (
      <span className="health-pill checking" aria-live="polite">
        <FiRefreshCw className="spinning" size={11} />
        Checking…
      </span>
    )
  }
  if (state.status === 'done') {
    if (state.error) {
      return (
        <button
          type="button"
          className="health-pill down"
          onClick={onCheck}
          title={describeError(state.error, 'Health check failed')}
        >
          <FiActivity size={11} />
          DOWN
        </button>
      )
    }
    if (state.data?.ok) {
      const ms = state.data.latency_ms
      return (
        <button
          type="button"
          className="health-pill ok"
          onClick={onCheck}
          title="Refresh health"
        >
          <FiActivity size={11} />
          OK {typeof ms === 'number' ? `${ms} ms` : ''}
        </button>
      )
    }
    return (
      <button
        type="button"
        className="health-pill down"
        onClick={onCheck}
        title={state.data?.error || 'Health check failed'}
      >
        <FiActivity size={11} />
        DOWN {state.data?.status_code ? state.data.status_code : ''}
      </button>
    )
  }
  return (
    <button
      type="button"
      className="health-pill idle"
      onMouseEnter={onCheck}
      onClick={onCheck}
      title="Check health"
    >
      <FiActivity size={11} />
      Check
    </button>
  )
}

function ProjectFormModal({ mode, initial, submitting, error, onSubmit, onClose }) {
  const isAdmin = true // modal only renders for admins (gated upstream)
  const initialProject = initial && typeof initial === 'object' ? initial : null
  const [form, setForm] = useState(() =>
    initialProject ? projectToForm(initialProject) : { ...emptyForm }
  )
  const [tab, setTabRaw] = useState('basics')
  const setTab = (next) => setTabRaw(next)

  // When the duplicate-name validation comes back from the server we
  // need to surface the error on the Basics tab and focus the input.
  // We compare against the previous error code so the effect only fires
  // on transition, not on every render.
  const lastErrorCodeRef = useRef(null)
  useEffect(() => {
    const code = error?.code || null
    const prev = lastErrorCodeRef.current
    lastErrorCodeRef.current = code
    if (code === 'duplicate_name' && code !== prev) {
      setTabRaw('basics')
      if (nameRef.current) nameRef.current.focus()
    }
  }, [error])
  const [secretReveal, setSecretReveal] = useState(null) // {secret, ack:false}
  const [confirmRegenerate, setConfirmRegenerate] = useState(false)
  const [regenError, setRegenError] = useState(null)
  const [regenPending, setRegenPending] = useState(false)
  const [webhookSecretPresent, setWebhookSecretPresent] = useState(
    !!initialProject?.webhook_secret_present
  )
  const nameRef = useRef(null)
  const toast = useToast()

  const setField = (key, value) => setForm((f) => ({ ...f, [key]: value }))

  const handleSubmit = (e) => {
    e.preventDefault()
    if (submitting) return
    onSubmit(form)
  }

  const errorText = error ? describeError(error, 'Save failed') : ''

  const projectId = initialProject?.id || null
  const origin =
    typeof window !== 'undefined' && window.location ? window.location.origin : ''
  const webhookUrl = projectId ? `${origin}/webhooks/deploy/${projectId}` : ''

  const handleCopyWebhook = async () => {
    if (!webhookUrl) return
    try {
      await navigator.clipboard.writeText(webhookUrl)
      toast.push({ type: 'success', message: 'Webhook URL copied' })
    } catch {
      toast.push({ type: 'error', message: 'Could not copy URL' })
    }
  }

  const handleCopySecret = async () => {
    if (!secretReveal?.secret) return
    try {
      await navigator.clipboard.writeText(secretReveal.secret)
      toast.push({ type: 'success', message: 'Secret copied' })
    } catch {
      toast.push({ type: 'error', message: 'Could not copy secret' })
    }
  }

  const handleRegenerate = async () => {
    if (!projectId || regenPending) return
    setRegenPending(true)
    setRegenError(null)
    try {
      const res = await projects.regenerateWebhookSecret(projectId)
      const secret = res?.webhook_secret || ''
      setSecretReveal({ secret })
      setWebhookSecretPresent(true)
      setConfirmRegenerate(false)
      toast.push({
        type: 'success',
        message: 'Webhook secret regenerated. Copy it now.',
      })
    } catch (err) {
      setRegenError(err)
      toast.push({
        type: 'error',
        message: describeError(err, 'Failed to regenerate secret'),
      })
    } finally {
      setRegenPending(false)
    }
  }

  const showDeployTab = mode === 'edit' || form.deploy_enabled || form.deploy_command
  const showWebhookSection = mode === 'edit' && form.deploy_enabled

  return (
    <Modal title={mode === 'create' ? 'New project' : `Edit ${initialProject?.name || ''}`} onClose={onClose}>
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="modal-tabs">
          <button
            type="button"
            className={`modal-tab ${tab === 'basics' ? 'active' : ''}`}
            onClick={() => setTab('basics')}
          >
            Basics
          </button>
          <button
            type="button"
            className={`modal-tab ${tab === 'deploy' ? 'active' : ''}`}
            onClick={() => setTab('deploy')}
          >
            Deploy
          </button>
          {showDeployTab && null}
        </div>

        {tab === 'basics' && (
          <div className="modal-grid">
            <div className="form-group">
              <label>Name *</label>
              <input
                ref={nameRef}
                type="text"
                value={form.name}
                onChange={(e) => setField('name', e.target.value)}
                required
                autoFocus
              />
            </div>
            <div className="form-group">
              <label>Environment *</label>
              <select
                value={form.environment}
                onChange={(e) => setField('environment', e.target.value)}
                required
              >
                {ENVIRONMENTS.map((env) => (
                  <option key={env} value={env}>
                    {env.charAt(0).toUpperCase() + env.slice(1)}
                  </option>
                ))}
              </select>
            </div>
            <div className="form-group">
              <label>Domain</label>
              <input
                type="text"
                placeholder="app.example.com"
                value={form.domain}
                onChange={(e) => setField('domain', e.target.value)}
              />
            </div>
            <div className="form-group">
              <label>Port</label>
              <input
                type="number"
                min="0"
                max="65535"
                value={form.port}
                onChange={(e) => setField('port', e.target.value)}
              />
            </div>
            <div className="form-group">
              <label>Container name</label>
              <input
                type="text"
                value={form.container_name}
                onChange={(e) => setField('container_name', e.target.value)}
              />
            </div>
            <div className="form-group">
              <label>PM2 name</label>
              <input
                type="text"
                value={form.pm2_name}
                onChange={(e) => setField('pm2_name', e.target.value)}
              />
            </div>
            <div className="form-group">
              <label>Tunnel service</label>
              <input
                type="text"
                placeholder="cloudflared-myapp"
                value={form.tunnel_service}
                onChange={(e) => setField('tunnel_service', e.target.value)}
              />
            </div>
            <div className="form-group full">
              <label>Description</label>
              <input
                type="text"
                value={form.description}
                onChange={(e) => setField('description', e.target.value)}
              />
            </div>
            <div className="form-group full">
              <label>Health URL</label>
              <input
                type="text"
                placeholder="https://app.example.com/health"
                value={form.health_url}
                onChange={(e) => setField('health_url', e.target.value)}
              />
            </div>
            <div className="form-group full">
              <label>Tags (comma separated)</label>
              <input
                type="text"
                value={form.tags}
                onChange={(e) => setField('tags', e.target.value)}
              />
            </div>
            <div className="form-group full">
              <label>Notes</label>
              <textarea
                rows={3}
                value={form.notes}
                onChange={(e) => setField('notes', e.target.value)}
              />
            </div>
            <div className="form-group full checkbox-group">
              <label>
                <input
                  type="checkbox"
                  checked={form.enabled}
                  onChange={(e) => setField('enabled', e.target.checked)}
                />
                Enabled
              </label>
            </div>
          </div>
        )}

        {tab === 'deploy' && (
          <div className="modal-grid">
            <div className="form-group full checkbox-group">
              <label>
                <input
                  type="checkbox"
                  checked={form.deploy_enabled}
                  onChange={(e) => setField('deploy_enabled', e.target.checked)}
                />
                Deploy enabled
              </label>
            </div>
            <div className="form-group full">
              <label>Deploy command</label>
              <textarea
                className="mono"
                rows={3}
                maxLength={4096}
                placeholder="cd /srv/app && git pull && pm2 restart app"
                value={form.deploy_command}
                onChange={(e) => setField('deploy_command', e.target.value)}
              />
              <div className="form-help">
                Single command line. Chained pipelines should call a script file.
              </div>
            </div>
            <div className="form-group">
              <label>Timeout (seconds)</label>
              <input
                type="number"
                min="30"
                max="3600"
                value={form.deploy_timeout_seconds}
                onChange={(e) => setField('deploy_timeout_seconds', e.target.value)}
              />
              <div className="form-help">30..3600, default 300</div>
            </div>
            <div className="form-group">
              <label>Working directory</label>
              <input
                type="text"
                placeholder="/srv/app"
                value={form.deploy_working_dir}
                onChange={(e) => setField('deploy_working_dir', e.target.value)}
              />
              <div className="form-help">Absolute path</div>
            </div>

            {showWebhookSection && (
              <div className="form-group full">
                <div className="webhook-section">
                  <h4>Webhook</h4>
                  <p className="webhook-help">
                    Public webhook URL. Sign requests with HMAC-SHA256 over the
                    raw body using the project secret.
                  </p>
                  <div className="webhook-row">
                    <input
                      type="text"
                      readOnly
                      value={webhookUrl}
                      className="mono"
                    />
                    <button
                      type="button"
                      className="ghost-btn"
                      onClick={handleCopyWebhook}
                    >
                      <FiCopy />
                      Copy URL
                    </button>
                  </div>
                  <div className="webhook-row webhook-secret-row">
                    <span className="webhook-label">Webhook secret</span>
                    <span
                      className={`enabled-badge ${
                        webhookSecretPresent ? 'on' : 'off'
                      }`}
                    >
                      {webhookSecretPresent ? 'Set' : 'Not set'}
                    </span>
                    {isAdmin && (
                      <button
                        type="button"
                        className="ghost-btn"
                        onClick={() => setConfirmRegenerate(true)}
                        disabled={regenPending}
                      >
                        <FiKey />
                        {webhookSecretPresent ? 'Regenerate secret' : 'Generate secret'}
                      </button>
                    )}
                  </div>

                  {confirmRegenerate && (
                    <div className="webhook-confirm">
                      <FiAlertTriangle />
                      <div>
                        <p>
                          {webhookSecretPresent
                            ? 'Regenerating invalidates the existing secret. Existing webhook callers will fail until reconfigured.'
                            : 'Generate a new webhook secret for this project.'}
                        </p>
                        <div className="webhook-confirm-actions">
                          <button
                            type="button"
                            className="ghost-btn"
                            onClick={() => setConfirmRegenerate(false)}
                            disabled={regenPending}
                          >
                            Cancel
                          </button>
                          <button
                            type="button"
                            className="primary-btn"
                            onClick={handleRegenerate}
                            disabled={regenPending}
                          >
                            {regenPending ? <Spinner size={14} /> : <FiKey />}
                            Confirm
                          </button>
                        </div>
                      </div>
                    </div>
                  )}

                  {regenError && (
                    <div className="modal-error">
                      {describeError(regenError, 'Regenerate failed')}
                    </div>
                  )}

                  {secretReveal && (
                    <div className="webhook-secret-banner">
                      <FiAlertTriangle />
                      <div className="webhook-secret-body">
                        <p className="webhook-secret-warn">
                          This is the last time you can see this. Copy now.
                        </p>
                        <code className="webhook-secret-value">
                          {secretReveal.secret}
                        </code>
                        <div className="webhook-confirm-actions">
                          <button
                            type="button"
                            className="ghost-btn"
                            onClick={handleCopySecret}
                          >
                            <FiCopy />
                            Copy secret
                          </button>
                          <button
                            type="button"
                            className="primary-btn"
                            onClick={() => setSecretReveal(null)}
                          >
                            I have copied it
                          </button>
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              </div>
            )}

            {!showWebhookSection && mode === 'create' && (
              <div className="form-group full">
                <div className="webhook-help-block">
                  Save the project first, then enable deploys to manage the
                  webhook URL and secret.
                </div>
              </div>
            )}
          </div>
        )}

        {errorText && <div className="modal-error">{errorText}</div>}

        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button type="submit" className="primary-btn" disabled={submitting || !form.name.trim()}>
            {submitting ? <Spinner size={14} /> : null}
            {mode === 'create' ? 'Create' : 'Save'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

function ConfirmModal({ title, message, confirmLabel, variant, submitting, onConfirm, onCancel }) {
  return (
    <Modal title={title} onClose={onCancel} size="small">
      <p className="modal-message">{message}</p>
      <div className="modal-actions">
        <button type="button" className="ghost-btn" onClick={onCancel} disabled={submitting}>
          Cancel
        </button>
        <button
          type="button"
          className={variant === 'danger' ? 'danger-btn' : 'primary-btn'}
          onClick={onConfirm}
          disabled={submitting}
        >
          {submitting ? <Spinner size={14} /> : null}
          {confirmLabel}
        </button>
      </div>
    </Modal>
  )
}

export function Modal({ title, onClose, children, size = 'normal' }) {
  useEffect(() => {
    const onKey = (e) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div className="modal-backdrop" onMouseDown={onClose}>
      <div
        className={`modal-card glass ${size === 'small' ? 'small' : ''}`}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="modal-header">
          <h3>{title}</h3>
          <button type="button" className="modal-close" onClick={onClose} aria-label="Close">
            <FiX />
          </button>
        </div>
        <div className="modal-body">{children}</div>
      </div>
    </div>
  )
}

// ---------- Details drawer ---------------------------------------------

const DRAWER_TABS = [
  { id: 'overview', label: 'Overview' },
  { id: 'deployments', label: 'Deployments' },
]

function ProjectDetailsDrawer({
  project,
  isAdmin,
  tab = 'overview',
  onTabChange,
  onEdit,
  onDelete,
  onClose,
}) {
  useEffect(() => {
    const onKey = (e) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const setTab = (id) => {
    if (onTabChange) onTabChange(id)
  }

  return (
    <div className="drawer-backdrop" onMouseDown={onClose}>
      <aside
        className="drawer glass"
        onMouseDown={(e) => e.stopPropagation()}
        role="dialog"
        aria-label={`Details: ${project.name}`}
      >
        <header className="drawer-header">
          <div>
            <h3>{project.name}</h3>
            <div className="drawer-header-meta">
              <EnvBadge environment={project.environment} dot />
              {project.description && <p className="drawer-sub">{project.description}</p>}
            </div>
          </div>
          <button type="button" className="modal-close" onClick={onClose} aria-label="Close">
            <FiX />
          </button>
        </header>

        <div className="drawer-tabs">
          {DRAWER_TABS.map((t) => (
            <button
              key={t.id}
              type="button"
              className={`drawer-tab ${tab === t.id ? 'active' : ''}`}
              onClick={() => setTab(t.id)}
            >
              {t.label}
            </button>
          ))}
        </div>

        <div className="drawer-body">
          {tab === 'overview' ? (
            <DrawerOverview
              project={project}
              isAdmin={isAdmin}
              onEdit={onEdit}
              onDelete={onDelete}
            />
          ) : (
            <DrawerDeployments project={project} isAdmin={isAdmin} />
          )}
        </div>
      </aside>
    </div>
  )
}

function DrawerOverview({ project, isAdmin, onEdit, onDelete }) {
  // Anchor "since" once per project.id with a lazy useState initializer
  // so we don't read Date.now() during render and don't need to
  // setState in an effect.
  const [since] = useState(() =>
    new Date(Date.now() - 24 * 3600 * 1000).toISOString()
  )

  const historyQ = useQuery({
    queryKey: ['projects', 'health-history', project.id, since],
    queryFn: () => projects.healthHistory(project.id, { since, limit: 500 }),
    refetchOnWindowFocus: false,
  })

  const eventsQ = useQuery({
    queryKey: ['events', 'project', project.id],
    queryFn: () => eventsApi.list({ projectId: project.id, limit: 10 }),
    refetchOnWindowFocus: false,
  })

  const chartData = useMemo(() => {
    const history = Array.isArray(historyQ.data) ? historyQ.data : []
    return history
      .map((h) => ({
        ...h,
        ts: new Date(h.ts).getTime(),
        latency_ms: Number(h.latency_ms) || 0,
        ok: !!h.ok,
      }))
      .sort((a, b) => a.ts - b.ts)
  }, [historyQ.data])

  const eventsList = Array.isArray(eventsQ.data?.data) ? eventsQ.data.data : []

  const okPoints = chartData.filter((d) => d.ok)
  const failPoints = chartData.filter((d) => !d.ok)

  const successRate = chartData.length === 0
    ? null
    : (chartData.filter((d) => d.ok).length / chartData.length) * 100

  return (
    <>
      <section className="drawer-section">
        <h4>Summary</h4>
        <dl className="drawer-fields">
          <DrawerField label="Status">
            <span className={`enabled-badge ${project.enabled ? 'on' : 'off'}`}>
              {project.enabled ? 'Enabled' : 'Disabled'}
            </span>
          </DrawerField>
          <DrawerField label="Environment">
            <EnvBadge environment={project.environment} size="md" />
          </DrawerField>
          {project.domain && (
            <DrawerField label="Domain">
              <a href={`https://${project.domain}`} target="_blank" rel="noreferrer">
                {project.domain}
                <FiExternalLink size={11} />
              </a>
            </DrawerField>
          )}
          {project.port > 0 && <DrawerField label="Port">{project.port}</DrawerField>}
          {project.container_name && <DrawerField label="Container">{project.container_name}</DrawerField>}
          {project.pm2_name && <DrawerField label="PM2">{project.pm2_name}</DrawerField>}
          {project.tunnel_service && <DrawerField label="Tunnel">{project.tunnel_service}</DrawerField>}
          {project.health_url && (
            <DrawerField label="Health URL">
              <span className="drawer-mono">{project.health_url}</span>
            </DrawerField>
          )}
          <DrawerField label="Deploy">
            {project.deploy_enabled && project.deploy_command ? (
              <span className="enabled-badge on">Configured</span>
            ) : (
              <span className="enabled-badge off">Not configured</span>
            )}
          </DrawerField>
          {project.deploy_enabled && (
            <DrawerField label="Webhook">
              <span
                className={`enabled-badge ${
                  project.webhook_secret_present ? 'on' : 'off'
                }`}
              >
                {project.webhook_secret_present ? 'Secret set' : 'Secret missing'}
              </span>
            </DrawerField>
          )}
          {Array.isArray(project.tags) && project.tags.length > 0 && (
            <DrawerField label="Tags">
              <div className="project-tags">
                {project.tags.map((t) => (
                  <span key={t} className="tag-chip">{t}</span>
                ))}
              </div>
            </DrawerField>
          )}
          {project.notes && (
            <DrawerField label="Notes">
              <pre className="drawer-notes">{project.notes}</pre>
            </DrawerField>
          )}
        </dl>
      </section>

      <section className="drawer-section">
        <div className="drawer-section-head">
          <h4>Health history (24h)</h4>
          {successRate != null && (
            <span className="drawer-meta">
              {chartData.length} probes • {successRate.toFixed(1)}% OK
            </span>
          )}
        </div>
        {historyQ.isLoading ? (
          <div className="drawer-loading"><Spinner size={20} /></div>
        ) : historyQ.isError ? (
          <div className="drawer-error">{describeError(historyQ.error, 'Failed to load history')}</div>
        ) : chartData.length === 0 ? (
          <div className="drawer-empty">No probes recorded yet.</div>
        ) : (
          <div className="drawer-chart">
            <ResponsiveContainer width="100%" height={200}>
              <ScatterChart margin={{ top: 12, right: 12, left: 0, bottom: 12 }}>
                <CartesianGrid stroke="rgba(255,255,255,0.06)" strokeDasharray="3 3" />
                <XAxis
                  type="number"
                  dataKey="ts"
                  domain={['dataMin', 'dataMax']}
                  tick={{ fontSize: 10, fill: 'var(--text-muted)' }}
                  tickFormatter={(t) => new Date(t).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })}
                />
                <YAxis
                  dataKey="latency_ms"
                  tick={{ fontSize: 10, fill: 'var(--text-muted)' }}
                  label={{ value: 'ms', angle: -90, position: 'insideLeft', fill: 'var(--text-muted)', fontSize: 10 }}
                />
                <Tooltip
                  content={({ active, payload }) => {
                    if (!active || !payload || !payload.length) return null
                    const p = payload[0].payload
                    return (
                      <div className="sparkline-tooltip">
                        <div className="sparkline-tooltip-time">
                          {new Date(p.ts).toLocaleString()}
                        </div>
                        <div className="sparkline-tooltip-row">
                          <span
                            className="sparkline-tooltip-dot"
                            style={{ background: p.ok ? 'var(--success)' : 'var(--danger)' }}
                          />
                          <span className="sparkline-tooltip-name">{p.ok ? 'OK' : 'FAIL'}</span>
                          <span className="sparkline-tooltip-value">{p.latency_ms} ms</span>
                        </div>
                        {p.status_code > 0 && (
                          <div className="sparkline-tooltip-row">
                            <span className="sparkline-tooltip-name">HTTP</span>
                            <span className="sparkline-tooltip-value">{p.status_code}</span>
                          </div>
                        )}
                        {p.error && (
                          <div className="sparkline-tooltip-row">
                            <span className="sparkline-tooltip-name">err</span>
                            <span className="sparkline-tooltip-value">{p.error.slice(0, 60)}</span>
                          </div>
                        )}
                      </div>
                    )
                  }}
                />
                <ReferenceLine y={0} stroke="rgba(255,255,255,0.08)" />
                <Scatter name="OK" data={okPoints} fill="var(--success)" />
                <Scatter name="FAIL" data={failPoints} fill="var(--danger)" />
              </ScatterChart>
            </ResponsiveContainer>
          </div>
        )}
      </section>

      <section className="drawer-section">
        <h4>Recent project events</h4>
        {eventsQ.isLoading ? (
          <div className="drawer-loading"><Spinner size={16} /></div>
        ) : eventsQ.isError ? (
          <div className="drawer-error">{describeError(eventsQ.error, 'Failed to load events')}</div>
        ) : eventsList.length === 0 ? (
          <div className="drawer-empty">No events for this project.</div>
        ) : (
          <ul className="drawer-events">
            {eventsList.map((ev) => (
              <li key={ev.id} className="drawer-event">
                <SeverityBadge severity={ev.severity} dot />
                <span className="drawer-event-msg" title={ev.message}>{ev.message}</span>
                <span className="drawer-event-time">
                  <RelativeTime value={ev.ts} />
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>

      {isAdmin && (
        <div className="drawer-actions">
          <button type="button" className="action-btn" onClick={onEdit}>
            <FiEdit2 />
            Edit project
          </button>
          <button type="button" className="action-btn danger" onClick={onDelete}>
            <FiTrash2 />
            Delete project
          </button>
        </div>
      )}
    </>
  )
}

// DrawerDeployments shows the recent deployments list, supports row-expand
// to view stdout/stderr, and (for admins) lets the user trigger a manual
// deploy. After triggering it polls every 2s for up to 30s or until the
// latest record reaches a terminal status.
function DrawerDeployments({ project, isAdmin }) {
  const queryClient = useQueryClient()
  const toast = useToast()
  const [expandedId, setExpandedId] = useState(null)
  const [confirmDeploy, setConfirmDeploy] = useState(false)
  const [deployPending, setDeployPending] = useState(false)
  // polling toggles when the user triggers a deploy. A timeout
  // automatically clears it after 30s so we don't poll forever.
  const [polling, setPolling] = useState(false)
  const reportedTerminalRef = useRef(new Set())
  const pollTimeoutRef = useRef(null)

  const queryKey = useMemo(
    () => ['projects', 'deployments', project.id],
    [project.id],
  )

  const deploymentsQ = useQuery({
    queryKey,
    queryFn: () => projects.deployments(project.id, { limit: 20 }),
    refetchInterval: polling ? 2000 : false,
    refetchOnWindowFocus: false,
  })

  const list = Array.isArray(deploymentsQ.data) ? deploymentsQ.data : []
  const latest = list[0]
  const latestId = latest?.id || null
  const latestStatus = latest?.status || null
  const latestTerminal = latestStatus
    ? isTerminalStatus(latestStatus)
    : false

  // Stop polling once the latest deployment is terminal, and surface a toast.
  // Using IDs and statuses (rather than the latest object reference) keeps
  // the dependency array stable.
  useEffect(() => {
    if (!polling || !latestId || !latestTerminal) return
    // Synchronizing internal "polling" state in response to external
    // (query) data reaching a terminal value is the canonical use of
    // setState in an effect for this pattern.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPolling(false)
    if (pollTimeoutRef.current) {
      clearTimeout(pollTimeoutRef.current)
      pollTimeoutRef.current = null
    }
    if (!reportedTerminalRef.current.has(latestId)) {
      reportedTerminalRef.current.add(latestId)
      const variant =
        latestStatus === 'success'
          ? 'success'
          : latestStatus === 'timeout'
            ? 'warning'
            : 'error'
      toast.push({
        type: variant,
        message: `Deployment ${latestStatus}`,
      })
    }
  }, [polling, latestId, latestStatus, latestTerminal, toast])

  // Cleanup any pending poll-timeout on unmount.
  useEffect(() => {
    return () => {
      if (pollTimeoutRef.current) {
        clearTimeout(pollTimeoutRef.current)
        pollTimeoutRef.current = null
      }
    }
  }, [])

  const triggerDeploy = async () => {
    if (!isAdmin || deployPending) return
    setDeployPending(true)
    try {
      await projects.deploy(project.id, { wait: false })
      setConfirmDeploy(false)
      toast.push({ type: 'success', message: 'Deployment triggered' })
      // Start polling for up to 30s. If the latest deployment is
      // already terminal by the next refetch the effect above will
      // stop polling early.
      setPolling(true)
      if (pollTimeoutRef.current) clearTimeout(pollTimeoutRef.current)
      pollTimeoutRef.current = setTimeout(() => {
        setPolling(false)
        pollTimeoutRef.current = null
      }, 30_000)
      queryClient.invalidateQueries({ queryKey })
    } catch (err) {
      toast.push({
        type: 'error',
        message: describeError(err, 'Failed to trigger deploy'),
      })
    } finally {
      setDeployPending(false)
    }
  }

  return (
    <section className="drawer-section">
      <div className="drawer-section-head">
        <h4>Deployments</h4>
        <div className="drawer-section-actions">
          <button
            type="button"
            className="action-btn"
            onClick={() => queryClient.invalidateQueries({ queryKey })}
            title="Refresh"
          >
            <FiRefreshCw className={deploymentsQ.isFetching ? 'spinning' : ''} />
            Refresh
          </button>
          {isAdmin && (
            <button
              type="button"
              className="action-btn project-deploy"
              onClick={() => setConfirmDeploy(true)}
              disabled={
                !project.deploy_enabled ||
                !project.deploy_command ||
                deployPending
              }
              title={
                !project.deploy_enabled || !project.deploy_command
                  ? 'Deploy not configured'
                  : 'Trigger deploy now'
              }
            >
              <FiUploadCloud />
              Trigger deploy now
            </button>
          )}
        </div>
      </div>

      {confirmDeploy && (
        <div className="webhook-confirm">
          <FiAlertTriangle />
          <div>
            <p>
              Run the project's deploy command on the server. This is logged in
              the deployments table and to the events feed.
            </p>
            <div className="webhook-confirm-actions">
              <button
                type="button"
                className="ghost-btn"
                onClick={() => setConfirmDeploy(false)}
                disabled={deployPending}
              >
                Cancel
              </button>
              <button
                type="button"
                className="primary-btn"
                onClick={triggerDeploy}
                disabled={deployPending}
              >
                {deployPending ? <Spinner size={14} /> : <FiUploadCloud />}
                Confirm deploy
              </button>
            </div>
          </div>
        </div>
      )}

      {polling && (
        <div className="drawer-polling-banner">
          <Spinner size={12} />
          Watching for deployment status (refreshes every 2s)
        </div>
      )}

      {deploymentsQ.isLoading ? (
        <div className="drawer-loading"><Spinner size={20} /></div>
      ) : deploymentsQ.isError ? (
        <div className="drawer-error">
          {describeError(deploymentsQ.error, 'Failed to load deployments')}
        </div>
      ) : list.length === 0 ? (
        <div className="drawer-empty">No deployments yet.</div>
      ) : (
        <table className="deployments-table">
          <thead>
            <tr>
              <th />
              <th>Triggered</th>
              <th>Triggered by</th>
              <th>Status</th>
              <th>Exit</th>
              <th>Duration</th>
            </tr>
          </thead>
          <tbody>
            {list.map((d) => {
              const expanded = expandedId === d.id
              return (
                <DeploymentRow
                  key={d.id}
                  deployment={d}
                  expanded={expanded}
                  onToggle={() => setExpandedId(expanded ? null : d.id)}
                />
              )
            })}
          </tbody>
        </table>
      )}
    </section>
  )
}

function DeploymentRow({ deployment, expanded, onToggle }) {
  const duration = deploymentDurationLabel(deployment)
  return (
    <>
      <tr
        className={`deployments-row ${expanded ? 'expanded' : ''}`}
        onClick={onToggle}
      >
        <td className="deployments-toggle">
          {expanded ? <FiChevronDown /> : <FiChevronRight />}
        </td>
        <td>
          <RelativeTime value={deployment.triggered_at} />
        </td>
        <td className="deployments-trigger" title={deployment.triggered_by}>
          {deployment.triggered_by || 'unknown'}
        </td>
        <td>
          <DeploymentStatusBadge status={deployment.status} />
        </td>
        <td className="deployments-exit">
          {Number.isFinite(deployment.exit_code) ? deployment.exit_code : '-'}
        </td>
        <td className="deployments-duration">{duration}</td>
      </tr>
      {expanded && (
        <tr className="deployments-detail-row">
          <td colSpan={6}>
            <div className="deployment-detail">
              <div className="deployment-detail-meta">
                <span>
                  <strong>Triggered:</strong>{' '}
                  {formatAbsolute(deployment.triggered_at) || '-'}
                </span>
                <span>
                  <strong>Finished:</strong>{' '}
                  {formatAbsolute(deployment.finished_at) || '-'}
                </span>
                {deployment.remote_ref && (
                  <span>
                    <strong>Ref:</strong> {deployment.remote_ref}
                  </span>
                )}
              </div>
              {deployment.error && (
                <div className="deployment-error">
                  <strong>Error:</strong> {deployment.error}
                </div>
              )}
              <div className="deployment-stream">
                <h5>stdout</h5>
                <pre className="deployment-pre">
                  {deployment.stdout ? deployment.stdout : '(empty)'}
                </pre>
              </div>
              <div className="deployment-stream">
                <h5>stderr</h5>
                <pre className="deployment-pre stderr">
                  {deployment.stderr ? deployment.stderr : '(empty)'}
                </pre>
              </div>
            </div>
          </td>
        </tr>
      )}
    </>
  )
}

function isTerminalStatus(status) {
  return status === 'success' || status === 'failed' || status === 'timeout'
}

function deploymentDurationLabel(d) {
  if (!d || !d.triggered_at) return '-'
  const start = new Date(d.triggered_at).getTime()
  const endStr = d.finished_at && !String(d.finished_at).startsWith('0001-01-01')
    ? d.finished_at
    : null
  if (!endStr) return isTerminalStatus(d.status) ? '-' : 'in progress'
  const end = new Date(endStr).getTime()
  if (!Number.isFinite(start) || !Number.isFinite(end)) return '-'
  const ms = Math.max(0, end - start)
  if (ms < 1000) return `${ms} ms`
  const sec = Math.round(ms / 1000)
  if (sec < 60) return `${sec}s`
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return `${m}m ${s}s`
}

function DrawerField({ label, children }) {
  return (
    <div className="drawer-field">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  )
}
