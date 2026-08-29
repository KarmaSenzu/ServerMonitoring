import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import {
  FiPlus,
  FiRefreshCw,
  FiEdit2,
  FiTrash2,
  FiSend,
  FiBell,
  FiActivity,
  FiAlertTriangle,
} from 'react-icons/fi'
import { channels as channelsApi, alerts as alertsApi, projects as projectsApi } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import { describeError } from '../ui/errors.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import SeverityBadge from '../ui/SeverityBadge.jsx'
import RelativeTime from '../ui/RelativeTime.jsx'
import { humanizeDuration } from '../ui/format.js'
import { Modal } from './Projects.jsx'
import './Notifications.css'

const TABS = [
  { id: 'channels', label: 'Channels' },
  { id: 'rules', label: 'Alert Rules' },
]

const ALERT_TYPES = [
  { value: 'system_cpu', label: 'System CPU' },
  { value: 'system_memory', label: 'System memory' },
  { value: 'system_disk', label: 'System disk' },
  { value: 'project_health', label: 'Project health' },
  { value: 'container_state', label: 'Container state' },
  { value: 'tunnel_state', label: 'Tunnel state' },
]

const COMPARATORS = [
  { value: 'gte', label: '>=' },
  { value: 'lte', label: '<=' },
  { value: 'eq', label: '=' },
  { value: 'neq', label: '!=' },
]

const SEVERITIES = ['info', 'warning', 'error', 'critical']

const SYSTEM_TYPES = ['system_cpu', 'system_memory', 'system_disk']
const STATE_TYPES = ['project_health', 'container_state', 'tunnel_state']

export default function NotificationsPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const tab = TABS.some((t) => t.id === searchParams.get('tab'))
    ? searchParams.get('tab')
    : 'channels'

  const setTab = (id) => {
    const next = new URLSearchParams(searchParams)
    next.set('tab', id)
    setSearchParams(next, { replace: true })
  }

  return (
    <div className="notifications-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>Notifications</h1>
            <p>Outbound channels and alert rules</p>
          </div>
        </div>
        <div className="tabs">
          {TABS.map((t) => (
            <button
              key={t.id}
              type="button"
              className={`tab ${tab === t.id ? 'active' : ''}`}
              onClick={() => setTab(t.id)}
            >
              {t.label}
            </button>
          ))}
        </div>
      </div>

      {tab === 'channels' ? <ChannelsTab /> : <RulesTab />}
    </div>
  )
}

// ---------- Channels tab ------------------------------------------------

function ChannelsTab() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const queryClient = useQueryClient()
  const toast = useToast()

  const [editing, setEditing] = useState(null) // null | "new" | channel
  const [confirmDelete, setConfirmDelete] = useState(null)
  const [testingId, setTestingId] = useState(null)

  const channelsQ = useQuery({
    queryKey: ['notifications', 'channels'],
    queryFn: channelsApi.list,
  })

  const createM = useMutation({
    mutationFn: (payload) => channelsApi.create(payload),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Channel created' })
      queryClient.invalidateQueries({ queryKey: ['notifications', 'channels'] })
      setEditing(null)
    },
  })

  const patchM = useMutation({
    mutationFn: ({ id, payload }) => channelsApi.patch(id, payload),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Channel updated' })
      queryClient.invalidateQueries({ queryKey: ['notifications', 'channels'] })
      setEditing(null)
    },
  })

  const deleteM = useMutation({
    mutationFn: (id) => channelsApi.remove(id),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Channel deleted' })
      queryClient.invalidateQueries({ queryKey: ['notifications', 'channels'] })
      setConfirmDelete(null)
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Delete failed') })
    },
  })

  const list = Array.isArray(channelsQ.data) ? channelsQ.data : []

  const handleTest = async (channel) => {
    setTestingId(channel.id)
    try {
      const res = await channelsApi.test(channel.id)
      if (res?.delivered) {
        toast.push({ type: 'success', message: `Test message delivered to ${channel.name}` })
      } else {
        toast.push({
          type: 'error',
          message: res?.error
            ? `Test failed: ${res.error}`
            : `Test message could not be delivered to ${channel.name}`,
        })
      }
    } catch (err) {
      toast.push({ type: 'error', message: describeError(err, 'Test failed') })
    } finally {
      setTestingId(null)
    }
  }

  const handleToggle = (channel, enabled) => {
    patchM.mutate({ id: channel.id, payload: { enabled } })
  }

  return (
    <div className="tab-panel">
      <div className="tab-toolbar">
        <button
          type="button"
          className="refresh-btn glass"
          onClick={() => queryClient.invalidateQueries({ queryKey: ['notifications', 'channels'] })}
          disabled={channelsQ.isFetching}
        >
          <FiRefreshCw className={channelsQ.isFetching ? 'spinning' : ''} />
          Refresh
        </button>
        {isAdmin && (
          <button type="button" className="primary-btn" onClick={() => setEditing('new')}>
            <FiPlus />
            New channel
          </button>
        )}
      </div>

      {channelsQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : channelsQ.isError ? (
        <EmptyState
          icon={<FiAlertTriangle size={40} />}
          title="Failed to load channels"
          description={describeError(channelsQ.error)}
        />
      ) : list.length === 0 ? (
        <EmptyState
          icon={<FiBell size={40} />}
          title="No channels yet"
          description={
            isAdmin
              ? 'Create your first notification channel with the New channel button'
              : 'Ask an administrator to configure a notification channel'
          }
        />
      ) : (
        <div className="channels-grid">
          {list.map((ch) => (
            <ChannelCard
              key={ch.id}
              channel={ch}
              isAdmin={isAdmin}
              testing={testingId === ch.id}
              togglePending={patchM.isPending}
              onEdit={() => setEditing(ch)}
              onDelete={() => setConfirmDelete(ch)}
              onTest={() => handleTest(ch)}
              onToggle={(v) => handleToggle(ch, v)}
            />
          ))}
        </div>
      )}

      {editing && isAdmin && (
        <ChannelFormModal
          mode={editing === 'new' ? 'create' : 'edit'}
          initial={editing === 'new' ? null : editing}
          submitting={createM.isPending || patchM.isPending}
          error={
            (createM.isError && createM.error) ||
            (patchM.isError && patchM.error) ||
            null
          }
          onSubmit={(payload) => {
            if (editing === 'new') {
              createM.mutate(payload)
            } else {
              patchM.mutate({ id: editing.id, payload })
            }
          }}
          onClose={() => {
            createM.reset()
            patchM.reset()
            setEditing(null)
          }}
        />
      )}

      {confirmDelete && isAdmin && (
        <ConfirmModal
          title="Delete channel?"
          message={`This will remove "${confirmDelete.name}" from the notification system. Existing alert rules referencing it will keep the id but stop delivering.`}
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

function ChannelCard({ channel, isAdmin, testing, togglePending, onEdit, onDelete, onTest, onToggle }) {
  const chatId = channel.config?.chat_id
  const parseMode = channel.config?.parse_mode
  return (
    <div className="channel-card glass animate-in">
      <div className="channel-card-head">
        <div className="channel-card-title">
          <span className="channel-type-badge">{channel.type}</span>
          <h3>{channel.name}</h3>
        </div>
        {isAdmin ? (
          <label className="switch" title={channel.enabled ? 'Disable channel' : 'Enable channel'}>
            <input
              type="checkbox"
              checked={channel.enabled}
              disabled={togglePending}
              onChange={(e) => onToggle(e.target.checked)}
            />
            <span className="slider" />
          </label>
        ) : (
          <span className={`enabled-badge ${channel.enabled ? 'on' : 'off'}`}>
            {channel.enabled ? 'Enabled' : 'Disabled'}
          </span>
        )}
      </div>

      <dl className="channel-fields">
        <div className="field-row">
          <dt>Token</dt>
          <dd>
            {channel.bot_token_present ? (
              <span className="token-state set">Token set</span>
            ) : (
              <span className="token-state missing">Token missing</span>
            )}
          </dd>
        </div>
        {chatId != null && (
          <div className="field-row">
            <dt>chat_id</dt>
            <dd>{String(chatId)}</dd>
          </div>
        )}
        {parseMode && (
          <div className="field-row">
            <dt>parse_mode</dt>
            <dd>{String(parseMode)}</dd>
          </div>
        )}
        <div className="field-row">
          <dt>Updated</dt>
          <dd><RelativeTime value={channel.updated_at} /></dd>
        </div>
      </dl>

      {isAdmin && (
        <div className="channel-card-actions">
          <button type="button" className="action-btn" onClick={onEdit}>
            <FiEdit2 />
            Edit
          </button>
          <button
            type="button"
            className="action-btn"
            onClick={onTest}
            disabled={testing}
          >
            {testing ? <FiRefreshCw className="spinning" /> : <FiSend />}
            Test
          </button>
          <button type="button" className="action-btn danger" onClick={onDelete}>
            <FiTrash2 />
            Delete
          </button>
        </div>
      )}
    </div>
  )
}

function ChannelFormModal({ mode, initial, submitting, error, onSubmit, onClose }) {
  const isCreate = mode === 'create'
  const [name, setName] = useState(initial?.name || '')
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)
  const [botToken, setBotToken] = useState('')
  const [chatId, setChatId] = useState(initial?.config?.chat_id ?? '')
  const [parseMode, setParseMode] = useState(initial?.config?.parse_mode || '')

  const errorText = error ? describeError(error, 'Save failed') : ''

  const handleSubmit = (e) => {
    e.preventDefault()
    if (submitting) return
    const config = {}
    if (chatId !== '' && chatId != null) config.chat_id = chatId
    if (parseMode) config.parse_mode = parseMode
    if (botToken.trim() !== '') config.bot_token = botToken.trim()

    if (isCreate) {
      onSubmit({
        type: 'telegram',
        name: name.trim(),
        enabled,
        config,
      })
    } else {
      const payload = {
        name: name.trim(),
        enabled,
      }
      // Only include config when at least one field is being set so we
      // don't accidentally clear chat_id/parse_mode by sending a sparse map.
      if (Object.keys(config).length > 0) {
        payload.config = config
      }
      onSubmit(payload)
    }
  }

  return (
    <Modal title={isCreate ? 'New channel' : `Edit ${initial.name}`} onClose={onClose} size="small">
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="modal-grid">
          <div className="form-group full">
            <label>Type</label>
            <input type="text" value="telegram" disabled />
          </div>
          <div className="form-group full">
            <label>Name *</label>
            <input
              type="text"
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>
          <div className="form-group full">
            <label>{isCreate ? 'Bot token *' : 'Bot token (leave empty to keep existing)'}</label>
            <input
              type="password"
              autoComplete="new-password"
              value={botToken}
              onChange={(e) => setBotToken(e.target.value)}
              placeholder={isCreate ? '123456:ABC-...' : initial?.bot_token_present ? 'Token set, leave empty to keep' : 'Token missing'}
              required={isCreate}
            />
          </div>
          <div className="form-group full">
            <label>Chat ID *</label>
            <input
              type="text"
              value={chatId}
              onChange={(e) => setChatId(e.target.value)}
              required
            />
          </div>
          <div className="form-group full">
            <label>Parse mode</label>
            <select value={parseMode} onChange={(e) => setParseMode(e.target.value)}>
              <option value="">none</option>
              <option value="HTML">HTML</option>
              <option value="Markdown">Markdown</option>
              <option value="MarkdownV2">MarkdownV2</option>
            </select>
          </div>
          <div className="form-group full checkbox-group">
            <label>
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />
              Enabled
            </label>
          </div>
        </div>

        {errorText && <div className="modal-error">{errorText}</div>}

        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button
            type="submit"
            className="primary-btn"
            disabled={submitting || !name.trim() || (isCreate && !botToken.trim()) || chatId === ''}
          >
            {submitting ? <Spinner size={14} /> : null}
            {isCreate ? 'Create' : 'Save'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

// ---------- Alert Rules tab --------------------------------------------

function RulesTab() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const queryClient = useQueryClient()
  const toast = useToast()

  const [editing, setEditing] = useState(null)
  const [confirmDelete, setConfirmDelete] = useState(null)
  const [testingId, setTestingId] = useState(null)
  const [testStrips, setTestStrips] = useState({}) // ruleId -> { delivered, errors, expiresAt }

  const rulesQ = useQuery({
    queryKey: ['notifications', 'rules'],
    queryFn: alertsApi.list,
  })

  const channelsQ = useQuery({
    queryKey: ['notifications', 'channels'],
    queryFn: channelsApi.list,
    staleTime: 60_000,
  })

  const createM = useMutation({
    mutationFn: (payload) => alertsApi.create(payload),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Rule created' })
      queryClient.invalidateQueries({ queryKey: ['notifications', 'rules'] })
      setEditing(null)
    },
  })

  const patchM = useMutation({
    mutationFn: ({ id, payload }) => alertsApi.patch(id, payload),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Rule updated' })
      queryClient.invalidateQueries({ queryKey: ['notifications', 'rules'] })
      setEditing(null)
    },
  })

  const deleteM = useMutation({
    mutationFn: (id) => alertsApi.remove(id),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Rule deleted' })
      queryClient.invalidateQueries({ queryKey: ['notifications', 'rules'] })
      setConfirmDelete(null)
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Delete failed') })
    },
  })

  // Cull expired test strips so they disappear after ~8 seconds.
  useEffect(() => {
    if (Object.keys(testStrips).length === 0) return undefined
    const id = setInterval(() => {
      const now = Date.now()
      setTestStrips((prev) => {
        const next = {}
        let changed = false
        for (const [k, v] of Object.entries(prev)) {
          if (v.expiresAt > now) {
            next[k] = v
          } else {
            changed = true
          }
        }
        return changed ? next : prev
      })
    }, 1000)
    return () => clearInterval(id)
  }, [testStrips])

  const channels = useMemo(
    () => (Array.isArray(channelsQ.data) ? channelsQ.data : []),
    [channelsQ.data]
  )
  const channelById = useMemo(() => {
    const m = new Map()
    for (const c of channels) m.set(c.id, c)
    return m
  }, [channels])

  const list = Array.isArray(rulesQ.data) ? rulesQ.data : []

  const handleTest = async (rule) => {
    setTestingId(rule.id)
    try {
      const res = await alertsApi.test(rule.id)
      const delivered = Array.isArray(res?.delivered) ? res.delivered.length : 0
      const errorCount = res?.errors ? Object.keys(res.errors).length : 0
      toast.push({
        type: errorCount > 0 ? 'warning' : 'success',
        message: `Delivered: ${delivered}, errors: ${errorCount}`,
      })
      setTestStrips((prev) => ({
        ...prev,
        [rule.id]: {
          delivered,
          errors: res?.errors || {},
          expiresAt: Date.now() + 8000,
        },
      }))
    } catch (err) {
      toast.push({ type: 'error', message: describeError(err, 'Test failed') })
    } finally {
      setTestingId(null)
    }
  }

  const handleToggle = (rule, enabled) => {
    patchM.mutate({ id: rule.id, payload: { enabled } })
  }

  return (
    <div className="tab-panel">
      <div className="tab-toolbar">
        <button
          type="button"
          className="refresh-btn glass"
          onClick={() => queryClient.invalidateQueries({ queryKey: ['notifications', 'rules'] })}
          disabled={rulesQ.isFetching}
        >
          <FiRefreshCw className={rulesQ.isFetching ? 'spinning' : ''} />
          Refresh
        </button>
        {isAdmin && (
          <button type="button" className="primary-btn" onClick={() => setEditing('new')}>
            <FiPlus />
            New rule
          </button>
        )}
      </div>

      {rulesQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : rulesQ.isError ? (
        <EmptyState
          icon={<FiAlertTriangle size={40} />}
          title="Failed to load rules"
          description={describeError(rulesQ.error)}
        />
      ) : list.length === 0 ? (
        <EmptyState
          icon={<FiActivity size={40} />}
          title="No alert rules yet"
          description={
            isAdmin
              ? 'Create your first rule with the New rule button'
              : 'Ask an administrator to configure alerting'
          }
        />
      ) : (
        <div className="rules-grid">
          {list.map((rule) => (
            <RuleCard
              key={rule.id}
              rule={rule}
              isAdmin={isAdmin}
              testing={testingId === rule.id}
              togglePending={patchM.isPending}
              channelById={channelById}
              onEdit={() => setEditing(rule)}
              onDelete={() => setConfirmDelete(rule)}
              onTest={() => handleTest(rule)}
              onToggle={(v) => handleToggle(rule, v)}
              testStrip={testStrips[rule.id] || null}
            />
          ))}
        </div>
      )}

      {editing && isAdmin && (
        <RuleFormModal
          mode={editing === 'new' ? 'create' : 'edit'}
          initial={editing === 'new' ? null : editing}
          channels={channels}
          submitting={createM.isPending || patchM.isPending}
          error={
            (createM.isError && createM.error) ||
            (patchM.isError && patchM.error) ||
            null
          }
          onSubmit={(payload) => {
            if (editing === 'new') {
              createM.mutate(payload)
            } else {
              patchM.mutate({ id: editing.id, payload })
            }
          }}
          onClose={() => {
            createM.reset()
            patchM.reset()
            setEditing(null)
          }}
        />
      )}

      {confirmDelete && isAdmin && (
        <ConfirmModal
          title="Delete rule?"
          message={`This will remove "${confirmDelete.name}".`}
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

function RuleCard({ rule, isAdmin, testing, togglePending, channelById, onEdit, onDelete, onTest, onToggle, testStrip }) {
  const isSystem = SYSTEM_TYPES.includes(rule.type)
  const isState = STATE_TYPES.includes(rule.type)

  return (
    <div className="rule-card glass animate-in">
      <div className="rule-card-head">
        <div className="rule-card-title">
          <h3>{rule.name}</h3>
          <SeverityBadge severity={rule.severity} />
        </div>
        {isAdmin ? (
          <label className="switch" title={rule.enabled ? 'Disable rule' : 'Enable rule'}>
            <input
              type="checkbox"
              checked={rule.enabled}
              disabled={togglePending}
              onChange={(e) => onToggle(e.target.checked)}
            />
            <span className="slider" />
          </label>
        ) : (
          <span className={`enabled-badge ${rule.enabled ? 'on' : 'off'}`}>
            {rule.enabled ? 'Enabled' : 'Disabled'}
          </span>
        )}
      </div>

      <dl className="rule-fields">
        <div className="field-row">
          <dt>Type</dt>
          <dd>{rule.type}</dd>
        </div>
        <div className="field-row">
          <dt>Condition</dt>
          <dd>
            {isSystem
              ? `${comparatorSymbol(rule.comparator)} ${rule.threshold}%`
              : isState
                ? 'state = down'
                : `${comparatorSymbol(rule.comparator)} ${rule.threshold}`}
          </dd>
        </div>
        <div className="field-row">
          <dt>For / Cooldown</dt>
          <dd>
            {humanizeDuration(rule.for_seconds)} / {humanizeDuration(rule.cooldown_seconds)}
          </dd>
        </div>
        {rule.scope && Object.keys(rule.scope).length > 0 && (
          <div className="field-row">
            <dt>Scope</dt>
            <dd className="scope-cell">
              {Object.entries(rule.scope).map(([k, v]) => (
                <span key={k} className="scope-chip">{k}: {String(v)}</span>
              ))}
            </dd>
          </div>
        )}
        <div className="field-row">
          <dt>Channels</dt>
          <dd className="channel-chips">
            {rule.channels && rule.channels.length > 0 ? (
              rule.channels.map((id) => {
                const ch = channelById.get(id)
                return (
                  <span key={id} className={`channel-chip ${ch ? '' : 'missing'}`}>
                    {ch ? ch.name : `${id.slice(0, 8)} (missing)`}
                  </span>
                )
              })
            ) : (
              <span className="dim">none</span>
            )}
          </dd>
        </div>
        <div className="field-row">
          <dt>Last triggered</dt>
          <dd><RelativeTime value={rule.last_triggered_at} /></dd>
        </div>
      </dl>

      {testStrip && (
        <div className="rule-test-strip">
          <span className="strip-label">Last test</span>
          <span className="strip-stat">Delivered: {testStrip.delivered}</span>
          <span className="strip-stat">Errors: {Object.keys(testStrip.errors).length}</span>
          {Object.entries(testStrip.errors).map(([id, msg]) => (
            <span key={id} className="strip-error" title={msg}>
              {(channelById.get(id)?.name || id.slice(0, 8))}: {String(msg).slice(0, 80)}
            </span>
          ))}
        </div>
      )}

      {isAdmin && (
        <div className="rule-card-actions">
          <button type="button" className="action-btn" onClick={onEdit}>
            <FiEdit2 />
            Edit
          </button>
          <button
            type="button"
            className="action-btn"
            onClick={onTest}
            disabled={testing}
          >
            {testing ? <FiRefreshCw className="spinning" /> : <FiSend />}
            Test
          </button>
          <button type="button" className="action-btn danger" onClick={onDelete}>
            <FiTrash2 />
            Delete
          </button>
        </div>
      )}
    </div>
  )
}

function comparatorSymbol(c) {
  switch (c) {
    case 'gte': return '>='
    case 'lte': return '<='
    case 'eq': return '='
    case 'neq': return '!='
    default: return c || ''
  }
}

const DURATION_UNITS = [
  { value: 1, label: 'seconds' },
  { value: 60, label: 'minutes' },
  { value: 3600, label: 'hours' },
]

// pickUnit returns the largest unit that divides the input cleanly,
// falling back to seconds. Used to seed the duration inputs from a
// rule's stored seconds value.
function pickUnit(seconds) {
  const s = Number(seconds) || 0
  if (s === 0) return { amount: 0, unit: 1 }
  if (s % 3600 === 0) return { amount: s / 3600, unit: 3600 }
  if (s % 60 === 0) return { amount: s / 60, unit: 60 }
  return { amount: s, unit: 1 }
}

function RuleFormModal({ mode, initial, channels, submitting, error, onSubmit, onClose }) {
  const isCreate = mode === 'create'

  const [name, setName] = useState(initial?.name || '')
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)
  const [severity, setSeverity] = useState(initial?.severity || 'warning')
  const [type, setType] = useState(initial?.type || 'system_cpu')
  const [comparator, setComparator] = useState(initial?.comparator || 'gte')
  const [threshold, setThreshold] = useState(
    initial?.threshold != null ? String(initial.threshold) : '80'
  )
  const initialFor = pickUnit(initial?.for_seconds || 0)
  const [forAmount, setForAmount] = useState(String(initialFor.amount))
  const [forUnit, setForUnit] = useState(initialFor.unit)

  const initialCool = pickUnit(initial?.cooldown_seconds || 0)
  const [cooldownAmount, setCooldownAmount] = useState(String(initialCool.amount))
  const [cooldownUnit, setCooldownUnit] = useState(initialCool.unit)

  const [selectedChannels, setSelectedChannels] = useState(initial?.channels || [])
  const [scopeProject, setScopeProject] = useState(
    initial?.scope?.project_id ? String(initial.scope.project_id) : ''
  )
  const [scopeContainer, setScopeContainer] = useState(
    initial?.scope?.container ? String(initial.scope.container) : ''
  )
  const [scopeTunnel, setScopeTunnel] = useState(
    initial?.scope?.tunnel_service ? String(initial.scope.tunnel_service) : ''
  )
  const [validationError, setValidationError] = useState('')

  const projectsQ = useQuery({
    queryKey: ['projects', { all: true }],
    queryFn: () => projectsApi.list(),
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
  })
  const projectList = Array.isArray(projectsQ.data) ? projectsQ.data : []

  const isSystem = SYSTEM_TYPES.includes(type)
  const showProject = type === 'project_health'
  const showContainer = type === 'container_state'
  const showTunnel = type === 'tunnel_state'

  const errorText = error ? describeError(error, 'Save failed') : validationError

  const handleSubmit = (e) => {
    e.preventDefault()
    if (submitting) return
    setValidationError('')

    if (!name.trim()) {
      setValidationError('Name is required')
      return
    }
    if (selectedChannels.length === 0) {
      setValidationError('Select at least one channel')
      return
    }
    const forSec = Math.floor(Number(forAmount || 0) * forUnit)
    const cooldownSec = Math.floor(Number(cooldownAmount || 0) * cooldownUnit)
    if (forSec < 0 || forSec > 86400) {
      setValidationError('For duration must be between 0 and 24h')
      return
    }
    if (cooldownSec < 0 || cooldownSec > 86400) {
      setValidationError('Cooldown duration must be between 0 and 24h')
      return
    }
    let thresholdNum = Number(threshold || 0)
    if (isSystem) {
      if (thresholdNum < 0 || thresholdNum > 100) {
        setValidationError('Threshold must be 0..100 for system_* rules')
        return
      }
    }

    const scope = {}
    if (showProject && scopeProject) scope.project_id = scopeProject
    if (showContainer && scopeContainer.trim()) scope.container = scopeContainer.trim()
    if (showTunnel && scopeTunnel.trim()) scope.tunnel_service = scopeTunnel.trim()

    onSubmit({
      name: name.trim(),
      enabled,
      type,
      threshold: isSystem ? thresholdNum : 0,
      comparator,
      for_seconds: forSec,
      cooldown_seconds: cooldownSec,
      severity,
      channels: selectedChannels,
      scope,
    })
  }

  const toggleChannel = (id) => {
    setSelectedChannels((prev) =>
      prev.includes(id) ? prev.filter((c) => c !== id) : [...prev, id]
    )
  }

  return (
    <Modal title={isCreate ? 'New rule' : `Edit ${initial.name}`} onClose={onClose}>
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="modal-grid">
          <div className="form-group full">
            <label>Name *</label>
            <input
              type="text"
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>
          <div className="form-group">
            <label>Type</label>
            <select value={type} onChange={(e) => setType(e.target.value)}>
              {ALERT_TYPES.map((t) => (
                <option key={t.value} value={t.value}>{t.label}</option>
              ))}
            </select>
          </div>
          <div className="form-group">
            <label>Severity</label>
            <select value={severity} onChange={(e) => setSeverity(e.target.value)}>
              {SEVERITIES.map((s) => (
                <option key={s} value={s}>{s}</option>
              ))}
            </select>
          </div>
          <div className="form-group">
            <label>Comparator</label>
            <select value={comparator} onChange={(e) => setComparator(e.target.value)}>
              {COMPARATORS.map((c) => (
                <option key={c.value} value={c.value}>{c.label}</option>
              ))}
            </select>
          </div>
          {isSystem && (
            <div className="form-group">
              <label>Threshold (%)</label>
              <input
                type="number"
                min="0"
                max="100"
                value={threshold}
                onChange={(e) => setThreshold(e.target.value)}
              />
            </div>
          )}

          <div className="form-group">
            <label>For (sustained)</label>
            <div className="duration-input">
              <input
                type="number"
                min="0"
                value={forAmount}
                onChange={(e) => setForAmount(e.target.value)}
              />
              <select value={forUnit} onChange={(e) => setForUnit(Number(e.target.value))}>
                {DURATION_UNITS.map((u) => (
                  <option key={u.value} value={u.value}>{u.label}</option>
                ))}
              </select>
            </div>
          </div>
          <div className="form-group">
            <label>Cooldown</label>
            <div className="duration-input">
              <input
                type="number"
                min="0"
                value={cooldownAmount}
                onChange={(e) => setCooldownAmount(e.target.value)}
              />
              <select value={cooldownUnit} onChange={(e) => setCooldownUnit(Number(e.target.value))}>
                {DURATION_UNITS.map((u) => (
                  <option key={u.value} value={u.value}>{u.label}</option>
                ))}
              </select>
            </div>
          </div>

          {showProject && (
            <div className="form-group full">
              <label>Project</label>
              <select value={scopeProject} onChange={(e) => setScopeProject(e.target.value)}>
                <option value="">Any project</option>
                {projectList.map((p) => (
                  <option key={p.id} value={p.id}>{p.name}{p.domain ? ` (${p.domain})` : ''}</option>
                ))}
              </select>
            </div>
          )}
          {showContainer && (
            <div className="form-group full">
              <label>Container name</label>
              <input
                type="text"
                value={scopeContainer}
                onChange={(e) => setScopeContainer(e.target.value)}
                placeholder="my-app"
              />
            </div>
          )}
          {showTunnel && (
            <div className="form-group full">
              <label>Tunnel service</label>
              <input
                type="text"
                value={scopeTunnel}
                onChange={(e) => setScopeTunnel(e.target.value)}
                placeholder="cloudflared-myapp"
              />
            </div>
          )}

          <div className="form-group full">
            <label>Channels *</label>
            {channels.length === 0 ? (
              <div className="form-hint">No channels configured. Create one in the Channels tab first.</div>
            ) : (
              <div className="channels-checklist">
                {channels.map((ch) => (
                  <label key={ch.id} className={`channel-check ${selectedChannels.includes(ch.id) ? 'on' : ''}`}>
                    <input
                      type="checkbox"
                      checked={selectedChannels.includes(ch.id)}
                      onChange={() => toggleChannel(ch.id)}
                    />
                    <span>{ch.name}</span>
                    {!ch.enabled && <span className="muted-label">disabled</span>}
                  </label>
                ))}
              </div>
            )}
          </div>

          <div className="form-group full checkbox-group">
            <label>
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />
              Enabled
            </label>
          </div>
        </div>

        {errorText && <div className="modal-error">{errorText}</div>}

        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button
            type="submit"
            className="primary-btn"
            disabled={submitting || !name.trim() || selectedChannels.length === 0}
          >
            {submitting ? <Spinner size={14} /> : null}
            {isCreate ? 'Create' : 'Save'}
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
