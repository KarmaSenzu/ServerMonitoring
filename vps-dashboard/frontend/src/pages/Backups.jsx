import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  FiDatabase,
  FiRefreshCw,
  FiPlay,
  FiDownload,
  FiTrash2,
  FiCopy,
  FiCheckCircle,
  FiXCircle,
} from 'react-icons/fi'
import { backups as backupsApi } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import RelativeTime from '../ui/RelativeTime.jsx'
import { humanizeBytes } from '../ui/format.js'
import { describeError } from '../ui/errors.js'
import { Modal } from './Projects.jsx'
import './Backups.css'

export default function BackupsPage() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const queryClient = useQueryClient()
  const toast = useToast()
  const [confirmDelete, setConfirmDelete] = useState(null)

  const backupsQ = useQuery({
    queryKey: ['backups'],
    queryFn: backupsApi.list,
    refetchInterval: 60_000,
  })

  const runM = useMutation({
    mutationFn: () => backupsApi.run(),
    onSuccess: (data) => {
      const ok = data?.ok !== false
      toast.push({
        type: ok ? 'success' : 'error',
        message: ok
          ? 'Backup created'
          : `Backup failed: ${data?.error || 'unknown error'}`,
      })
      queryClient.invalidateQueries({ queryKey: ['backups'] })
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Backup failed') })
    },
  })

  const deleteM = useMutation({
    mutationFn: (id) => backupsApi.remove(id),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Backup deleted' })
      queryClient.invalidateQueries({ queryKey: ['backups'] })
      setConfirmDelete(null)
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Delete failed') })
    },
  })

  const list = Array.isArray(backupsQ.data) ? backupsQ.data : []

  const refreshing = backupsQ.isFetching

  return (
    <div className="backups-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>Backups</h1>
            <p>SQLite database snapshots</p>
          </div>
          <div className="header-actions">
            <button
              type="button"
              className="ghost-btn"
              onClick={() =>
                queryClient.invalidateQueries({ queryKey: ['backups'] })
              }
              disabled={refreshing}
            >
              <FiRefreshCw className={refreshing ? 'spinning' : ''} />
              Refresh
            </button>
            {isAdmin && (
              <button
                type="button"
                className="primary-btn"
                onClick={() => runM.mutate()}
                disabled={runM.isPending}
              >
                {runM.isPending ? <Spinner size={14} /> : <FiPlay />}
                Run backup now
              </button>
            )}
          </div>
        </div>
      </div>

      {backupsQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : backupsQ.isError ? (
        <EmptyState
          icon={<FiDatabase size={40} />}
          title="Failed to load backups"
          description={describeError(backupsQ.error)}
        />
      ) : list.length === 0 ? (
        <EmptyState
          icon={<FiDatabase size={40} />}
          title="No backups yet"
          description={
            isAdmin
              ? 'Run a backup to create the first snapshot'
              : 'No snapshots have been recorded yet'
          }
        />
      ) : (
        <div className="backups-card glass">
          <table className="backups-table">
            <thead>
              <tr>
                <th>Timestamp</th>
                <th>Trigger</th>
                <th>Size</th>
                <th>Status</th>
                <th>Path</th>
                {isAdmin && <th className="backups-actions-col">Actions</th>}
              </tr>
            </thead>
            <tbody>
              {list.map((b) => (
                <BackupRow
                  key={b.id}
                  backup={b}
                  isAdmin={isAdmin}
                  onDelete={() => setConfirmDelete(b)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {confirmDelete && isAdmin && (
        <ConfirmModal
          title="Delete backup?"
          message={`This will remove the backup recorded at ${new Date(confirmDelete.ts).toLocaleString()}. The on-disk file is also deleted.`}
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

function BackupRow({ backup, isAdmin, onDelete }) {
  const toast = useToast()
  const path = backup.path || ''
  const truncatedPath = path
    ? path.length > 48
      ? `…${path.slice(-46)}`
      : path
    : '—'

  const handleCopy = async () => {
    if (!path) return
    try {
      await navigator.clipboard.writeText(path)
      toast.push({ type: 'success', message: 'Path copied' })
    } catch {
      toast.push({ type: 'error', message: 'Could not copy path' })
    }
  }

  return (
    <tr className="backups-row">
      <td>
        <RelativeTime value={backup.ts} />
      </td>
      <td className="backups-trigger" title={backup.trigger || ''}>
        {backup.trigger || 'unknown'}
      </td>
      <td>{humanizeBytes(backup.size_bytes)}</td>
      <td>
        {backup.ok ? (
          <span className="backup-status ok">
            <FiCheckCircle />
            OK
          </span>
        ) : (
          <span className="backup-status fail" title={backup.error || ''}>
            <FiXCircle />
            FAIL
          </span>
        )}
        {!backup.ok && backup.error && (
          <div className="backup-error">{backup.error}</div>
        )}
      </td>
      <td>
        <div className="backups-path">
          <code className="backups-path-text" title={path}>
            {truncatedPath}
          </code>
          {path && (
            <button
              type="button"
              className="icon-btn"
              onClick={handleCopy}
              title="Copy full path"
            >
              <FiCopy />
            </button>
          )}
        </div>
      </td>
      {isAdmin && (
        <td className="backups-actions">
          <a
            href={backup.ok ? backupsApi.downloadUrl(backup.id) : undefined}
            className={`action-btn ${!backup.ok ? 'disabled' : ''}`}
            aria-disabled={!backup.ok}
            onClick={(e) => {
              if (!backup.ok) e.preventDefault()
            }}
            title={backup.ok ? 'Download .db file' : 'Failed backups cannot be downloaded'}
            // The browser handles streaming; cookies travel with the request.
          >
            <FiDownload />
            Download
          </a>
          <button
            type="button"
            className="action-btn danger"
            onClick={onDelete}
          >
            <FiTrash2 />
            Delete
          </button>
        </td>
      )}
    </tr>
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
