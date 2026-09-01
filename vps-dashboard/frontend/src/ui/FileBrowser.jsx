import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  FiFolder,
  FiFile,
  FiArrowLeft,
  FiDownload,
  FiTrash2,
  FiUpload,
  FiFolderPlus,
  FiEdit2,
} from 'react-icons/fi'
import { fileApi } from '../api/endpoints.js'
import { useToast } from './useToast.js'
import Spinner from './Spinner.jsx'
import { describeError } from './errors.js'
import { Modal } from '../pages/Projects.jsx'
import './FileBrowser.css'

function formatSize(bytes) {
  if (!bytes || bytes <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let val = bytes
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024
    i++
  }
  return `${Math.round(val * 10) / 10} ${units[i]}`
}

function formatDate(s) {
  if (!s) return '-'
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? s : d.toLocaleDateString()
}

export default function FileBrowser({ server, onClose }) {
  const toast = useToast()
  const queryClient = useQueryClient()
  const [currentPath, setCurrentPath] = useState('/')
  const [renaming, setRenaming] = useState(null)
  const [mkdirModal, setMkdirModal] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(null)

  const browseQ = useQuery({
    queryKey: ['files-browse', server.id, currentPath],
    queryFn: () => fileApi.browse(server.id, currentPath),
  })

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['files-browse', server.id, currentPath] })
  }

  const deleteM = useMutation({
    mutationFn: ({ serverId, path }) => fileApi.remove(serverId, path),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Deleted' })
      refresh()
      setConfirmDelete(null)
    },
    onError: (err) => toast.push({ type: 'error', message: describeError(err, 'Delete failed') }),
  })

  const navigateTo = (path) => {
    setCurrentPath(path)
  }

  const goUp = () => {
    if (currentPath === '/') return
    const parts = currentPath.split('/').filter(Boolean)
    parts.pop()
    navigateTo('/' + parts.join('/'))
  }

  const entries = browseQ.data?.entries || []
  const displayPath = browseQ.data?.path || currentPath

  const breadcrumbs = displayPath.split('/').filter(Boolean)

  return (
    <Modal title={`Files — ${server.name}`} onClose={onClose} size="large">
      <div className="file-browser">
        {/* Toolbar */}
        <div className="file-toolbar">
          <button
            type="button"
            className="ghost-btn small"
            onClick={goUp}
            disabled={currentPath === '/'}
          >
            <FiArrowLeft />
            Up
          </button>
          <div className="file-path mono">
            <button type="button" className="breadcrumb-btn" onClick={() => navigateTo('/')}>
              /
            </button>
            {breadcrumbs.map((part, i) => (
              <span key={i} className="breadcrumb-segment">
                <button
                  type="button"
                  className="breadcrumb-btn"
                  onClick={() => navigateTo('/' + breadcrumbs.slice(0, i + 1).join('/'))}
                >
                  {part}
                </button>
                {i < breadcrumbs.length - 1 && <span className="breadcrumb-sep">/</span>}
              </span>
            ))}
          </div>
          <div className="file-actions">
            <button type="button" className="ghost-btn small" onClick={() => setMkdirModal(true)}>
              <FiFolderPlus />
              New folder
            </button>
            <button type="button" className="ghost-btn small" onClick={() => setUploading(true)}>
              <FiUpload />
              Upload
            </button>
          </div>
        </div>

        {/* Listing */}
        <div className="file-list">
          {browseQ.isLoading ? (
            <div className="file-loading"><Spinner size={20} /></div>
          ) : browseQ.isError ? (
            <div className="file-error">{describeError(browseQ.error, 'Failed to browse files')}</div>
          ) : entries.length === 0 ? (
            <div className="file-empty muted">This directory is empty.</div>
          ) : (
            <div className="file-entries">
              {entries.map((e) => (
                <div key={e.path} className="file-entry">
                  <button
                    type="button"
                    className="file-entry-main"
                    onClick={() => {
                      if (e.is_dir) navigateTo(e.path)
                    }}
                  >
                    <span className={`file-icon ${e.is_dir ? 'dir' : 'file'}`}>
                      {e.is_dir ? <FiFolder /> : <FiFile />}
                    </span>
                    <span className="file-name">{e.name}</span>
                    <span className="file-size muted">{e.is_dir ? '' : formatSize(e.size)}</span>
                    <span className="file-date muted">{formatDate(e.mod_time)}</span>
                  </button>
                  <div className="file-entry-actions">
                    {!e.is_dir && (
                      <a
                        href={fileApi.downloadUrl(server.id, e.path)}
                        className="action-btn small"
                        title="Download"
                        download
                      >
                        <FiDownload />
                      </a>
                    )}
                    <button
                      type="button"
                      className="action-btn small"
                      title="Rename"
                      onClick={() => setRenaming(e)}
                    >
                      <FiEdit2 />
                    </button>
                    <button
                      type="button"
                      className="action-btn small danger"
                      title="Delete"
                      disabled={deleteM.isPending}
                      onClick={() => setConfirmDelete(e)}
                    >
                      <FiTrash2 />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Rename modal */}
      {renaming && (
        <RenameModal
          entry={renaming}
          currentPath={displayPath}
          serverId={server.id}
          onSubmit={async (newName) => {
            await fileApi.rename(server.id, renaming.path, displayPath === '/' ? '/' + newName : displayPath + '/' + newName)
            toast.push({ type: 'success', message: 'Renamed' })
            refresh()
            setRenaming(null)
          }}
          onClose={() => setRenaming(null)}
        />
      )}

      {/* Mkdir modal */}
      {mkdirModal && (
        <MkdirModal
          currentPath={displayPath}
          serverId={server.id}
          onSubmit={async (name) => {
            const newPath = displayPath === '/' ? '/' + name : displayPath + '/' + name
            await fileApi.mkdir(server.id, newPath)
            toast.push({ type: 'success', message: 'Folder created' })
            refresh()
            setMkdirModal(false)
          }}
          onClose={() => setMkdirModal(false)}
        />
      )}

      {/* Upload modal */}
      {uploading && (
        <UploadModal
          currentPath={displayPath}
          serverId={server.id}
          onSubmit={async (file) => {
            const newPath = displayPath === '/' ? '/' + file.name : displayPath + '/' + file.name
            await fileApi.upload(server.id, newPath, file)
            toast.push({ type: 'success', message: 'Uploaded' })
            refresh()
            setUploading(false)
          }}
          onClose={() => setUploading(false)}
        />
      )}

      {/* Delete confirmation */}
      {confirmDelete && (
        <Modal title="Delete?" onClose={() => setConfirmDelete(null)} size="small">
          <p className="modal-message">
            Delete &quot;{confirmDelete.name}&quot;{confirmDelete.is_dir ? ' and all its contents' : ''}?
          </p>
          <div className="modal-actions">
            <button type="button" className="ghost-btn" onClick={() => setConfirmDelete(null)} disabled={deleteM.isPending}>
              Cancel
            </button>
            <button
              type="button"
              className="danger-btn"
              disabled={deleteM.isPending}
              onClick={() => deleteM.mutate({ serverId: server.id, path: confirmDelete.path })}
            >
              {deleteM.isPending ? <Spinner size={14} /> : null}
              Delete
            </button>
          </div>
        </Modal>
      )}
    </Modal>
  )
}

function RenameModal({ entry, onSubmit, onClose }) {
  const [name, setName] = useState(entry.name)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (submitting || !name.trim()) return
    setSubmitting(true)
    setError('')
    try {
      await onSubmit(name.trim())
    } catch (err) {
      setError(describeError(err, 'Rename failed'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal title="Rename" onClose={onClose} size="small">
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="form-group full">
          <label>New name</label>
          <input type="text" autoFocus value={name} onChange={(e) => setName(e.target.value)} required />
        </div>
        {error && <div className="modal-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>Cancel</button>
          <button type="submit" className="primary-btn" disabled={submitting || !name.trim()}>
            Rename
          </button>
        </div>
      </form>
    </Modal>
  )
}

function MkdirModal({ onSubmit, onClose }) {
  const [name, setName] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (submitting || !name.trim()) return
    setSubmitting(true)
    setError('')
    try {
      await onSubmit(name.trim())
    } catch (err) {
      setError(describeError(err, 'Create failed'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal title="New folder" onClose={onClose} size="small">
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="form-group full">
          <label>Folder name</label>
          <input type="text" autoFocus value={name} onChange={(e) => setName(e.target.value)} required />
        </div>
        {error && <div className="modal-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>Cancel</button>
          <button type="submit" className="primary-btn" disabled={submitting || !name.trim()}>
            Create
          </button>
        </div>
      </form>
    </Modal>
  )
}

function UploadModal({ onSubmit, onClose }) {
  const [file, setFile] = useState(null)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (submitting || !file) return
    setSubmitting(true)
    setError('')
    try {
      await onSubmit(file)
    } catch (err) {
      setError(describeError(err, 'Upload failed'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal title="Upload file" onClose={onClose} size="small">
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="form-group full">
          <label>File</label>
          <input
            type="file"
            onChange={(e) => setFile(e.target.files?.[0])}
            required
          />
        </div>
        {error && <div className="modal-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>Cancel</button>
          <button type="submit" className="primary-btn" disabled={submitting || !file}>
            {submitting ? <Spinner size={14} /> : null}
            Upload
          </button>
        </div>
      </form>
    </Modal>
  )
}
