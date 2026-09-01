import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  FiBox,
  FiRefreshCw,
  FiPlay,
  FiPause,
  FiRewind,
  FiChevronDown,
  FiChevronRight,
  FiAlertCircle,
} from 'react-icons/fi'
import { containerApi } from '../api/endpoints.js'
import { useAuth, canMutate } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import { describeError } from '../ui/errors.js'
import './Containers.css'

export default function ContainersPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const toast = useToast()

  const isAdmin = canMutate(user?.role)

  const [expanded, setExpanded] = useState(new Set())

  const fleetQ = useQuery({
    queryKey: ['containers-fleet'],
    queryFn: () => containerApi.fleet(),
    refetchInterval: 30000,
  })

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['containers-fleet'] })
  }

  const toggle = (serverId) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(serverId)) {
        next.delete(serverId)
      } else {
        next.add(serverId)
      }
      return next
    })
  }

  const servers = useMemo(
    () => (Array.isArray(fleetQ.data) ? fleetQ.data : []),
    [fleetQ.data]
  )

  const totalContainers = servers.reduce((sum, s) => sum + (s.containers?.length || 0), 0)
  const runningContainers = servers.reduce(
    (sum, s) => sum + (s.containers?.filter((c) => c.state === 'running').length || 0),
    0
  )

  return (
    <div className="containers-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>Containers</h1>
            <p>
              Fleet overview — containers across all registered servers
            </p>
          </div>
          <div className="header-actions">
            <button type="button" className="ghost-btn" onClick={refresh}>
              <FiRefreshCw />
              Refresh
            </button>
          </div>
        </div>
      </div>

      <div className="containers-summary">
        <div className="summary-chip">
          <span className="chip-value">{servers.length}</span>
          <span className="chip-label">Servers</span>
        </div>
        <div className="summary-chip status-online">
          <span className="chip-value">{runningContainers}</span>
          <span className="chip-label">Running</span>
        </div>
        <div className="summary-chip status-unknown">
          <span className="chip-value">{totalContainers}</span>
          <span className="chip-label">Total</span>
        </div>
      </div>

      {fleetQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : fleetQ.isError ? (
        <EmptyState
          icon={<FiBox size={40} />}
          title="Failed to load containers"
          description={describeError(fleetQ.error)}
        />
      ) : servers.length === 0 ? (
        <EmptyState
          icon={<FiBox size={40} />}
          title="No servers registered"
          description="Register servers first, then their containers will appear here"
        />
      ) : (
        <div className="fleet-list">
          {servers.map((s) => {
            const isExpanded = expanded.has(s.server_id)
            const containerCount = s.containers?.length || 0
            const runningCount = s.containers?.filter((c) => c.state === 'running').length || 0
            const hasError = !!s.error

            return (
              <div key={s.server_id} className="fleet-server glass">
                <button
                  type="button"
                  className="fleet-server-header"
                  onClick={() => toggle(s.server_id)}
                >
                  <span className="fleet-chevron">
                    {isExpanded ? <FiChevronDown /> : <FiChevronRight />}
                  </span>
                  <span className="fleet-server-name">{s.server_name}</span>
                  <span className={`status-badge status-${s.status}`}>
                    <span className="status-dot" />
                    {s.status}
                  </span>
                  {s.engine && (
                    <span className="engine-badge">{s.engine}</span>
                  )}
                  <span className="fleet-counts">
                    {hasError ? (
                      <span className="fleet-error">
                        <FiAlertCircle /> Error
                      </span>
                    ) : (
                      <>
                        <span className="muted">{runningCount} running</span>
                        <span className="muted">/ {containerCount} total</span>
                      </>
                    )}
                  </span>
                </button>

                {isExpanded && (
                  <div className="fleet-server-body">
                    {hasError ? (
                      <div className="fleet-error-detail mono">{s.error}</div>
                    ) : containerCount === 0 ? (
                      <p className="fleet-empty muted">No containers on this server.</p>
                    ) : (
                      <div className="container-grid">
                        {s.containers.map((c) => (
                          <ContainerCard
                            key={c.id}
                            container={c}
                            isAdmin={isAdmin}
                            onAction={async (action) => {
                              try {
                                if (action === 'start') {
                                  await containerApi.start(s.server_id, c.name)
                                } else if (action === 'stop') {
                                  await containerApi.stop(s.server_id, c.name)
                                } else if (action === 'restart') {
                                  await containerApi.restart(s.server_id, c.name)
                                }
                                toast.push({ type: 'success', message: `${action} ${c.name}` })
                                refresh()
                              } catch (err) {
                                toast.push({ type: 'error', message: describeError(err, `${action} failed`) })
                              }
                            }}
                          />
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

function ContainerCard({ container, isAdmin, onAction }) {
  const [busy, setBusy] = useState(false)

  const handleAction = async (action) => {
    if (busy) return
    setBusy(true)
    try {
      await onAction(action)
    } finally {
      setBusy(false)
    }
  }

  const isRunning = container.state === 'running'

  return (
    <div className="container-card">
      <div className="container-card-header">
        <span className={`container-state-dot state-${container.state}`} />
        <span className="container-name mono">{container.name}</span>
        <span className="container-state-badge">{container.state}</span>
      </div>
      <div className="container-card-body">
        <div className="container-meta">
          <span className="muted mono">{container.image}</span>
          {container.ports && <span className="muted mono ports">{container.ports}</span>}
        </div>
        <div className="container-status muted">{container.status}</div>
      </div>
      {isAdmin && (
        <div className="container-card-actions">
          {isRunning ? (
            <button
              type="button"
              className="action-btn small"
              disabled={busy}
              onClick={() => handleAction('stop')}
              title="Stop container"
            >
              <FiPause /> Stop
            </button>
          ) : (
            <button
              type="button"
              className="action-btn small"
              disabled={busy}
              onClick={() => handleAction('start')}
              title="Start container"
            >
              <FiPlay /> Start
            </button>
          )}
          <button
            type="button"
            className="action-btn small"
            disabled={busy}
            onClick={() => handleAction('restart')}
            title="Restart container"
          >
            <FiRewind /> Restart
          </button>
        </div>
      )}
    </div>
  )
}
