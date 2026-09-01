import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  FiKey,
  FiPlus,
  FiTrash2,
  FiCopy,
  FiRefreshCw,
  FiUpload,
} from 'react-icons/fi'
import { sshApi } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import { describeError } from '../ui/errors.js'
import { Modal } from './Projects.jsx'
import './SSHKeys.css'

function formatDate(s) {
  if (!s) return '-'
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return s
  return d.toLocaleString()
}

export default function SSHKeysPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const toast = useToast()

  const isAdmin = user?.role === 'admin'

  const [generating, setGenerating] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(null)
  const [viewing, setViewing] = useState(null) // { name, public_key }
  const [lastGenerated, setLastGenerated] = useState(null) // { name, public_key }

  const keysQ = useQuery({
    queryKey: ['ssh-keys'],
    queryFn: () => sshApi.keys(),
  })

  const deleteM = useMutation({
    mutationFn: (name) => sshApi.removeKey(name),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'Key deleted' })
      queryClient.invalidateQueries({ queryKey: ['ssh-keys'] })
      setConfirmDelete(null)
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Delete failed') })
    },
  })

  const keys = useMemo(
    () => (Array.isArray(keysQ.data) ? keysQ.data : []),
    [keysQ.data]
  )

  const copyText = async (text, label) => {
    try {
      await navigator.clipboard.writeText(text)
      toast.push({ type: 'success', message: `${label} copied to clipboard` })
    } catch {
      toast.push({ type: 'error', message: 'Clipboard unavailable' })
    }
  }

  return (
    <div className="sshkeys-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>SSH Keys</h1>
            <p>
              Credential store for the SSH engine — private keys never leave
              the backend; only metadata is exposed
            </p>
          </div>
          {isAdmin && (
            <div className="header-actions">
              <button type="button" className="ghost-btn" onClick={() => setUploading(true)}>
                <FiUpload />
                Upload key
              </button>
              <button type="button" className="primary-btn" onClick={() => setGenerating(true)}>
                <FiPlus />
                Generate key
              </button>
            </div>
          )}
        </div>
      </div>

      {keysQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : keysQ.isError ? (
        <EmptyState
          icon={<FiKey size={40} />}
          title="Failed to load keys"
          description={describeError(keysQ.error)}
        />
      ) : keys.length === 0 ? (
        <EmptyState
          icon={<FiKey size={40} />}
          title="No SSH keys"
          description={
            isAdmin
              ? 'Generate an Ed25519 key pair, then add its public line to the remote server authorized_keys'
              : 'An administrator has not registered any SSH keys yet'
          }
        />
      ) : (
        <div className="sshkeys-list glass">
          <div className="sshkeys-row sshkeys-head">
            <div>Name</div>
            <div>Type</div>
            <div>Fingerprint</div>
            <div>Comment</div>
            <div>Created</div>
            <div className="actions-col">Actions</div>
          </div>
          {keys.map((k) => (
            <div key={k.name} className="sshkeys-row">
              <div className="key-name mono">{k.name}</div>
              <div>
                <span className="key-type-badge">{k.type}</span>
              </div>
              <div className="key-fingerprint mono" title={k.fingerprint}>
                {k.fingerprint}
              </div>
              <div className="key-comment">{k.comment || '-'}</div>
              <div className="key-date">{formatDate(k.created_at)}</div>
              <div className="actions-col">
                <button
                  type="button"
                  className="action-btn small"
                  title="Copy fingerprint"
                  onClick={() => copyText(k.fingerprint, 'Fingerprint')}
                >
                  <FiCopy />
                  Fingerprint
                </button>
                <button
                  type="button"
                  className="action-btn small"
                  title="View public key"
                  onClick={async () => {
                    try {
                      const res = await sshApi.publicKey(k.name)
                      setViewing({ name: k.name, public_key: res.public_key })
                    } catch (err) {
                      toast.push({ type: 'error', message: describeError(err, 'Load failed') })
                    }
                  }}
                >
                  <FiKey />
                  Public
                </button>
                {isAdmin && (
                  <button
                    type="button"
                    className="action-btn small danger"
                    disabled={deleteM.isPending}
                    onClick={() => setConfirmDelete(k)}
                    title="Delete key"
                  >
                    <FiTrash2 />
                    Delete
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {generating && (
        <GenerateKeyModal
          onSubmit={async (payload) => {
            const res = await sshApi.generateKey(payload)
            toast.push({ type: 'success', message: `Key ${payload.name} generated` })
            queryClient.invalidateQueries({ queryKey: ['ssh-keys'] })
            setGenerating(false)
            if (res && res.public_key) {
              setLastGenerated({ name: payload.name, public_key: res.public_key })
            }
          }}
          onClose={() => setGenerating(false)}
        />
      )}

      {uploading && (
        <UploadKeyModal
          onSubmit={async (payload) => {
            await sshApi.addKey(payload)
            toast.push({ type: 'success', message: `Key ${payload.name} added` })
            queryClient.invalidateQueries({ queryKey: ['ssh-keys'] })
            setUploading(false)
          }}
          onClose={() => setUploading(false)}
        />
      )}

      {lastGenerated && (
        <Modal title={`Public key for ${lastGenerated.name}`} onClose={() => setLastGenerated(null)} size="normal">
          <p className="modal-message">
            Add this public line to the remote server&apos;s
            <code className="inline-code"> ~/.ssh/authorized_keys</code>. The
            private half stays in the backend key store.
          </p>
          <textarea readOnly rows={3} className="mono public-key-box" value={lastGenerated.public_key} />
          <div className="modal-actions">
            <button type="button" className="ghost-btn" onClick={() => setLastGenerated(null)}>
              Close
            </button>
            <button
              type="button"
              className="primary-btn"
              onClick={() => copyText(lastGenerated.public_key, 'Public key')}
            >
              <FiCopy />
              Copy
            </button>
          </div>
        </Modal>
      )}

      {viewing && (
        <Modal title={`Public key for ${viewing.name}`} onClose={() => setViewing(null)} size="normal">
          <textarea readOnly rows={3} className="mono public-key-box" value={viewing.public_key} />
          <div className="modal-actions">
            <button type="button" className="ghost-btn" onClick={() => setViewing(null)}>
              Close
            </button>
            <button
              type="button"
              className="primary-btn"
              onClick={() => copyText(viewing.public_key, 'Public key')}
            >
              <FiCopy />
              Copy
            </button>
          </div>
        </Modal>
      )}

      {confirmDelete && (
        <Modal title="Delete SSH key?" onClose={() => setConfirmDelete(null)} size="small">
          <p className="modal-message">
            This removes &quot;{confirmDelete.name}&quot; from the credential
            store. Servers referencing this key will no longer be able to
            connect until a replacement is configured.
          </p>
          <div className="modal-actions">
            <button type="button" className="ghost-btn" onClick={() => setConfirmDelete(null)} disabled={deleteM.isPending}>
              Cancel
            </button>
            <button
              type="button"
              className="danger-btn"
              disabled={deleteM.isPending}
              onClick={() => deleteM.mutate(confirmDelete.name)}
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

function GenerateKeyModal({ onSubmit, onClose }) {
  const [name, setName] = useState('')
  const [comment, setComment] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (submitting) return
    setSubmitting(true)
    setError('')
    try {
      await onSubmit({ name: name.trim(), comment: comment.trim() })
    } catch (err) {
      setError(describeError(err, 'Generate failed'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal title="Generate SSH key" onClose={onClose} size="small">
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="modal-grid">
          <div className="form-group full">
            <label>Key name</label>
            <input
              type="text"
              className="mono"
              autoFocus
              placeholder="production-key"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>
          <div className="form-group full">
            <label>Comment (optional)</label>
            <input
              type="text"
              placeholder="dashboard@server-01"
              value={comment}
              onChange={(e) => setComment(e.target.value)}
            />
          </div>
        </div>
        <p className="modal-hint">
          Generates an Ed25519 key pair. The private half is stored with 0600
          permissions; the public line is shown once for distribution.
        </p>
        {error && <div className="modal-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button type="submit" className="primary-btn" disabled={submitting || !name.trim()}>
            {submitting ? <Spinner size={14} /> : <FiRefreshCw />}
            Generate
          </button>
        </div>
      </form>
    </Modal>
  )
}

function UploadKeyModal({ onSubmit, onClose }) {
  const [name, setName] = useState('')
  const [privateKey, setPrivateKey] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (submitting) return
    setSubmitting(true)
    setError('')
    try {
      await onSubmit({ name: name.trim(), private_key: privateKey.trim() })
    } catch (err) {
      setError(describeError(err, 'Upload failed'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal title="Upload private key" onClose={onClose} size="normal">
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="modal-grid">
          <div className="form-group full">
            <label>Key name</label>
            <input
              type="text"
              className="mono"
              autoFocus
              placeholder="production-key"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>
          <div className="form-group full">
            <label>Private key (PEM / OpenSSH)</label>
            <textarea
              rows={6}
              className="mono"
              placeholder={'-----BEGIN OPENSSH PRIVATE KEY-----\n...\n-----END OPENSSH PRIVATE KEY-----'}
              value={privateKey}
              onChange={(e) => setPrivateKey(e.target.value)}
              required
            />
          </div>
        </div>
        <p className="modal-hint">
          Passphrase-protected keys are rejected — the platform runs
          unattended. The key is stored once and never returned by the API.
        </p>
        {error && <div className="modal-error">{error}</div>}
        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button
            type="submit"
            className="primary-btn"
            disabled={submitting || !name.trim() || !privateKey.trim()}
          >
            {submitting ? <Spinner size={14} /> : <FiUpload />}
            Store key
          </button>
        </div>
      </form>
    </Modal>
  )
}
