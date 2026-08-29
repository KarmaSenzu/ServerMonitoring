import { useState } from 'react'
import { Link } from 'react-router-dom'
import {
  FiPlay,
  FiSquare,
  FiRefreshCw,
  FiBox,
  FiFileText,
  FiLink,
} from 'react-icons/fi'
import { docker } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import { describeError } from '../ui/errors.js'
import LogsDrawer from './LogsDrawer.jsx'
import './DockerCard.css'

export default function DockerCard({ container, project, onRefresh }) {
  const [loading, setLoading] = useState(null)
  const [logsOpen, setLogsOpen] = useState(false)
  const { user } = useAuth()
  const toast = useToast()
  const isAdmin = user?.role === 'admin'

  const state = container.state || ''
  const isRunning = state === 'running'

  const handleAction = async (action) => {
    if (!isAdmin || loading) return
    setLoading(action)
    try {
      if (action === 'start') await docker.start(container.name)
      if (action === 'stop') await docker.stop(container.name)
      if (action === 'restart') await docker.restart(container.name)
      toast.push({ type: 'success', message: `Container "${container.name}" ${action}ed` })
      if (onRefresh) onRefresh()
    } catch (err) {
      toast.push({ type: 'error', message: describeError(err, `Failed to ${action} container`) })
    } finally {
      setLoading(null)
    }
  }

  const shortId = container.short_id || (container.id ? container.id.slice(0, 12) : '')

  return (
    <div className="docker-card glass animate-in">
      <div className="docker-card-header">
        <div className="docker-card-info">
          <div className="docker-card-icon">
            <FiBox />
          </div>
          <div>
            <h3 className="docker-card-name">{container.name}</h3>
            <span className="docker-card-image">{container.image}</span>
          </div>
        </div>
        <div className={`docker-status-badge ${state}`}>
          <span className={`status-dot ${state}`}></span>
          {state || 'unknown'}
        </div>
      </div>

      <div className="docker-card-details">
        <div className="detail-row">
          <span className="detail-label">ID</span>
          <span className="detail-value">{shortId}</span>
        </div>
        <div className="detail-row">
          <span className="detail-label">Status</span>
          <span className="detail-value">{container.status || '-'}</span>
        </div>
        {container.ports && (
          <div className="detail-row">
            <span className="detail-label">Ports</span>
            <span className="detail-value">{container.ports}</span>
          </div>
        )}
        {project && (
          <div className="detail-row">
            <span className="detail-label">Project</span>
            <span className="detail-value">
              <Link to="/projects" className="docker-project-link">
                <FiLink size={11} />
                {project.name}
              </Link>
            </span>
          </div>
        )}
      </div>

      <div className="docker-card-actions">
        <button
          type="button"
          className="action-btn logs"
          onClick={() => setLogsOpen(true)}
          disabled={loading !== null}
        >
          <FiFileText />
          Logs
        </button>
        {isAdmin && !isRunning && (
          <button
            type="button"
            className="action-btn start"
            onClick={() => handleAction('start')}
            disabled={loading !== null}
          >
            {loading === 'start' ? <FiRefreshCw className="spinning" /> : <FiPlay />}
            Start
          </button>
        )}
        {isAdmin && isRunning && (
          <button
            type="button"
            className="action-btn stop"
            onClick={() => handleAction('stop')}
            disabled={loading !== null}
          >
            {loading === 'stop' ? <FiRefreshCw className="spinning" /> : <FiSquare />}
            Stop
          </button>
        )}
        {isAdmin && (
          <button
            type="button"
            className="action-btn restart"
            onClick={() => handleAction('restart')}
            disabled={loading !== null}
          >
            {loading === 'restart' ? <FiRefreshCw className="spinning" /> : <FiRefreshCw />}
            Restart
          </button>
        )}
      </div>

      <LogsDrawer
        open={logsOpen}
        source="docker"
        name={container.name}
        onClose={() => setLogsOpen(false)}
      />
    </div>
  )
}
