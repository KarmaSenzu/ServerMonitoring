import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  FiActivity,
  FiPlay,
  FiSquare,
  FiRefreshCw,
  FiRotateCw,
  FiTrash2,
  FiTerminal,
  FiAlertTriangle,
  FiFileText,
} from 'react-icons/fi'
import { pm2 } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import { describeError } from '../ui/errors.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import { humanizeUptime } from '../ui/format.js'
import LogsDrawer from '../components/LogsDrawer.jsx'
import { Modal } from './Projects.jsx'
import './PM2.css'

const AUTO_REFRESH_MS = 10000

function humanizeBytes(bytes) {
  const n = Number(bytes)
  if (!Number.isFinite(n) || n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = n
  let i = 0
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i += 1
  }
  return `${value.toFixed(value >= 100 || i === 0 ? 0 : value >= 10 ? 1 : 2)} ${units[i]}`
}

function statusClass(status) {
  const s = (status || '').toLowerCase()
  if (s === 'online') return 'online'
  if (s === 'stopped' || s === 'stopping') return 'stopped'
  if (s === 'errored' || s === 'error') return 'errored'
  return 'other'
}

export default function PM2Page() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const queryClient = useQueryClient()
  const toast = useToast()

  const [autoRefresh, setAutoRefresh] = useState(false)
  const [logsTarget, setLogsTarget] = useState(null)
  const [confirmDelete, setConfirmDelete] = useState(null)
  const [actionPending, setActionPending] = useState(null)

  const processesQ = useQuery({
    queryKey: ['pm2', 'processes'],
    queryFn: pm2.list,
    refetchInterval: autoRefresh ? AUTO_REFRESH_MS : false,
    retry: (count, err) => {
      if (err?.code === 'pm2_unavailable') return false
      return count < 1
    },
  })

  const actionM = useMutation({
    mutationFn: ({ name, action }) => pm2.action(name, action),
    onSuccess: (_data, { name, action }) => {
      toast.push({ type: 'success', message: `pm2 ${action} → ${name}` })
      queryClient.invalidateQueries({ queryKey: ['pm2', 'processes'] })
    },
    onError: (err, { name, action }) => {
      toast.push({
        type: 'error',
        message: describeError(err, `Failed to ${action} ${name}`),
      })
    },
    onSettled: () => setActionPending(null),
  })

  const deleteM = useMutation({
    mutationFn: (name) => pm2.remove(name),
    onSuccess: (_data, name) => {
      toast.push({ type: 'success', message: `pm2 deleted → ${name}` })
      queryClient.invalidateQueries({ queryKey: ['pm2', 'processes'] })
      setConfirmDelete(null)
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Delete failed') })
    },
  })

  const handleAction = (name, action) => {
    if (actionPending) return
    setActionPending(`${name}:${action}`)
    actionM.mutate({ name, action })
  }

  const list = useMemo(
    () => (Array.isArray(processesQ.data) ? processesQ.data : []),
    [processesQ.data]
  )

  const unavailable = processesQ.error?.code === 'pm2_unavailable'

  const handleManualRefresh = () => {
    queryClient.invalidateQueries({ queryKey: ['pm2', 'processes'] })
  }

  return (
    <div className="pm2-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>PM2 Processes</h1>
            <p>Node.js processes managed by pm2</p>
          </div>
          <div className="header-actions">
            <label className="auto-refresh-toggle">
              <input
                type="checkbox"
                checked={autoRefresh}
                onChange={(e) => setAutoRefresh(e.target.checked)}
              />
              Auto-refresh (10s)
            </label>
            <button
              type="button"
              className="refresh-btn glass"
              onClick={handleManualRefresh}
              disabled={processesQ.isFetching}
            >
              <FiRefreshCw className={processesQ.isFetching ? 'spinning' : ''} />
              Refresh
            </button>
          </div>
        </div>
      </div>

      {processesQ.isLoading ? (
        <div className="loading-state">
          <Spinner size={24} />
          <p>Loading PM2 processes...</p>
        </div>
      ) : unavailable ? (
        <EmptyState
          icon={<FiAlertTriangle size={40} />}
          title="PM2 is not available"
          description="Install pm2 globally on the server to manage Node.js processes (npm install -g pm2)."
        />
      ) : processesQ.isError ? (
        <EmptyState
          icon={<FiAlertTriangle size={40} />}
          title="Failed to load PM2 processes"
          description={describeError(processesQ.error)}
        />
      ) : list.length === 0 ? (
        <EmptyState
          icon={<FiActivity size={40} />}
          title="No PM2 processes"
          description="Start a process with pm2 and it will appear here."
        />
      ) : (
        <div className="pm2-table glass">
          <div className="pm2-table-row pm2-table-head">
            <div>Name</div>
            <div>Status</div>
            <div>PID</div>
            <div>Uptime</div>
            <div>CPU</div>
            <div>Memory</div>
            <div>Restarts</div>
            <div className="pm2-actions-head">Actions</div>
          </div>
          {list.map((p) => {
            const cls = statusClass(p.status)
            const acting = actionPending && actionPending.startsWith(`${p.name}:`)
            return (
              <div className="pm2-table-row" key={p.name}>
                <div className="pm2-name-cell">
                  <span className="pm2-name">{p.name}</span>
                  {p.script_path && (
                    <span className="pm2-script">{p.script_path}</span>
                  )}
                  {p.cwd && <span className="pm2-cwd">{p.cwd}</span>}
                </div>
                <div>
                  <span className={`pm2-status ${cls}`}>
                    <span className={`pm2-status-dot ${cls}`} />
                    {p.status || 'unknown'}
                  </span>
                </div>
                <div className="pm2-mono">{p.pid > 0 ? p.pid : '-'}</div>
                <div className="pm2-mono">
                  {humanizeUptime(p.uptime)}
                </div>
                <div className="pm2-mono">{Number(p.cpu_percent ?? 0).toFixed(1)}%</div>
                <div className="pm2-mono">{humanizeBytes(p.memory_bytes)}</div>
                <div className="pm2-mono">{p.restarts ?? 0}</div>
                <div className="pm2-actions-cell">
                  <button
                    type="button"
                    className="pm2-action-btn"
                    onClick={() => setLogsTarget(p.name)}
                    title="View logs"
                  >
                    <FiFileText />
                    Logs
                  </button>
                  {isAdmin && (
                    <>
                      <button
                        type="button"
                        className="pm2-action-btn start"
                        onClick={() => handleAction(p.name, 'start')}
                        disabled={acting}
                        title="Start"
                      >
                        {acting && actionPending === `${p.name}:start`
                          ? <FiRefreshCw className="spinning" />
                          : <FiPlay />}
                      </button>
                      <button
                        type="button"
                        className="pm2-action-btn stop"
                        onClick={() => handleAction(p.name, 'stop')}
                        disabled={acting}
                        title="Stop"
                      >
                        {acting && actionPending === `${p.name}:stop`
                          ? <FiRefreshCw className="spinning" />
                          : <FiSquare />}
                      </button>
                      <button
                        type="button"
                        className="pm2-action-btn restart"
                        onClick={() => handleAction(p.name, 'restart')}
                        disabled={acting}
                        title="Restart"
                      >
                        {acting && actionPending === `${p.name}:restart`
                          ? <FiRefreshCw className="spinning" />
                          : <FiRotateCw />}
                      </button>
                      <button
                        type="button"
                        className="pm2-action-btn reload"
                        onClick={() => handleAction(p.name, 'reload')}
                        disabled={acting}
                        title="Reload"
                      >
                        {acting && actionPending === `${p.name}:reload`
                          ? <FiRefreshCw className="spinning" />
                          : <FiTerminal />}
                      </button>
                      <button
                        type="button"
                        className="pm2-action-btn danger"
                        onClick={() => setConfirmDelete(p)}
                        disabled={acting}
                        title="Delete"
                      >
                        <FiTrash2 />
                      </button>
                    </>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}

      <LogsDrawer
        open={Boolean(logsTarget)}
        source="pm2"
        name={logsTarget || ''}
        onClose={() => setLogsTarget(null)}
      />

      {confirmDelete && isAdmin && (
        <Modal
          title="Delete pm2 process?"
          onClose={() => !deleteM.isPending && setConfirmDelete(null)}
          size="small"
        >
          <p className="modal-message">
            This removes <strong>{confirmDelete.name}</strong> from pm2 (equivalent to
            <span className="modal-mono"> pm2 delete</span>).
          </p>
          <div className="modal-actions">
            <button
              type="button"
              className="ghost-btn"
              onClick={() => setConfirmDelete(null)}
              disabled={deleteM.isPending}
            >
              Cancel
            </button>
            <button
              type="button"
              className="danger-btn"
              onClick={() => deleteM.mutate(confirmDelete.name)}
              disabled={deleteM.isPending}
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
