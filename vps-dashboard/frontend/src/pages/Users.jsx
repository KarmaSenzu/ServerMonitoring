import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FiUsers, FiUserPlus, FiKey, FiTrash2, FiShield } from 'react-icons/fi'
import { users as usersApi } from '../api/endpoints.js'
import { useAuth } from '../auth/useAuth.js'
import { useToast } from '../ui/useToast.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import { describeError } from '../ui/errors.js'
import { Modal } from './Projects.jsx'
import './Users.css'

function formatDate(s) {
  if (!s) return '-'
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return s
  return d.toLocaleString()
}

export default function UsersPage() {
  const { user: currentUser } = useAuth()
  const queryClient = useQueryClient()
  const toast = useToast()
  const [creating, setCreating] = useState(false)
  const [resetting, setResetting] = useState(null) // user
  const [confirmDelete, setConfirmDelete] = useState(null)
  const [confirmRole, setConfirmRole] = useState(null)

  const usersQ = useQuery({
    queryKey: ['users'],
    queryFn: usersApi.list,
  })

  const createM = useMutation({
    mutationFn: (payload) => usersApi.create(payload),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'User created' })
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setCreating(false)
    },
  })

  const patchM = useMutation({
    mutationFn: ({ id, payload }) => usersApi.patch(id, payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Update failed') })
    },
  })

  const deleteM = useMutation({
    mutationFn: (id) => usersApi.remove(id),
    onSuccess: () => {
      toast.push({ type: 'success', message: 'User deleted' })
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setConfirmDelete(null)
    },
    onError: (err) => {
      toast.push({ type: 'error', message: describeError(err, 'Delete failed') })
    },
  })

  const list = Array.isArray(usersQ.data) ? usersQ.data : []
  const adminCount = list.filter((u) => u.role === 'admin').length

  return (
    <div className="users-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>Users</h1>
            <p>Manage who can sign in and what they can do</p>
          </div>
          <button type="button" className="primary-btn" onClick={() => setCreating(true)}>
            <FiUserPlus />
            Add user
          </button>
        </div>
      </div>

      {usersQ.isLoading ? (
        <div className="loading-state"><Spinner size={24} /></div>
      ) : usersQ.isError ? (
        <EmptyState
          icon={<FiUsers size={40} />}
          title="Failed to load users"
          description={describeError(usersQ.error)}
        />
      ) : list.length === 0 ? (
        <EmptyState
          icon={<FiUsers size={40} />}
          title="No users"
          description="Create the first user with the button above"
        />
      ) : (
        <div className="users-list glass">
          <div className="users-row users-head">
            <div>Username</div>
            <div>Role</div>
            <div>Created</div>
            <div className="actions-col">Actions</div>
          </div>
          {list.map((u) => {
            const isSelf = u.id === currentUser?.id
            const isLastAdmin = u.role === 'admin' && adminCount <= 1
            const cannotDemote = isLastAdmin
            const cannotDelete = isSelf || isLastAdmin
            return (
              <div key={u.id} className="users-row">
                <div className="user-name">
                  {u.username}
                  {isSelf && <span className="self-badge">you</span>}
                </div>
                <div>
                  <span className={`role-badge role-${u.role}`}>{u.role}</span>
                </div>
                <div className="user-date">{formatDate(u.createdAt || u.created_at)}</div>
                <div className="actions-col">
                  <button
                    type="button"
                    className="action-btn small"
                    title={cannotDemote ? 'Cannot demote the last admin' : 'Toggle role'}
                    disabled={patchM.isPending || cannotDemote}
                    onClick={() => setConfirmRole(u)}
                  >
                    <FiShield />
                    {u.role === 'admin' ? 'Demote' : 'Set role'}
                  </button>
                  <button
                    type="button"
                    className="action-btn small"
                    onClick={() => setResetting(u)}
                  >
                    <FiKey />
                    Reset password
                  </button>
                  <button
                    type="button"
                    className="action-btn small danger"
                    disabled={cannotDelete || deleteM.isPending}
                    title={
                      isSelf
                        ? 'You cannot delete your own account'
                        : isLastAdmin
                        ? 'Cannot delete the last admin'
                        : 'Delete user'
                    }
                    onClick={() => setConfirmDelete(u)}
                  >
                    <FiTrash2 />
                    Delete
                  </button>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {creating && (
        <CreateUserModal
          submitting={createM.isPending}
          error={createM.isError ? createM.error : null}
          onSubmit={(payload) => createM.mutate(payload)}
          onClose={() => {
            createM.reset()
            setCreating(false)
          }}
        />
      )}

      {resetting && (
        <ResetPasswordModal
          target={resetting}
          submitting={patchM.isPending}
          error={patchM.isError ? patchM.error : null}
          onSubmit={(password) =>
            patchM.mutate(
              { id: resetting.id, payload: { password } },
              {
                onSuccess: () => {
                  toast.push({ type: 'success', message: `Password reset for ${resetting.username}` })
                  setResetting(null)
                },
              }
            )
          }
          onClose={() => {
            patchM.reset()
            setResetting(null)
          }}
        />
      )}

      {confirmRole && (
        <ConfirmModal
          title={`Change role for ${confirmRole.username}?`}
          message={`Current role: ${confirmRole.role}. Select a new role below.`}
          confirmLabel="Change role"
          submitting={patchM.isPending}
          onCancel={() => setConfirmRole(null)}
          onConfirm={() => {
            // Cycle: admin → operator → viewer → admin
            const cycle = { admin: 'operator', operator: 'viewer', viewer: 'admin' }
            const newRole = cycle[confirmRole.role] || 'viewer'
            patchM.mutate(
              { id: confirmRole.id, payload: { role: newRole } },
              {
                onSuccess: () => {
                  toast.push({ type: 'success', message: `Role updated to ${newRole} for ${confirmRole.username}` })
                  setConfirmRole(null)
                },
              }
            )
          }}
        />
      )}

      {confirmDelete && (
        <ConfirmModal
          title="Delete user?"
          message={`This will permanently remove "${confirmDelete.username}".`}
          confirmLabel="Delete"
          variant="danger"
          submitting={deleteM.isPending}
          onCancel={() => setConfirmDelete(null)}
          onConfirm={() => deleteM.mutate(confirmDelete.id)}
        />
      )}
    </div>
  )
}

function CreateUserModal({ submitting, error, onSubmit, onClose }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState('viewer')
  const errorText = error ? describeError(error, 'Create failed') : ''

  const handleSubmit = (e) => {
    e.preventDefault()
    if (submitting) return
    onSubmit({ username: username.trim(), password, role })
  }

  return (
    <Modal title="Add user" onClose={onClose} size="small">
      <form className="modal-form" onSubmit={handleSubmit}>
        <div className="modal-grid">
          <div className="form-group full">
            <label>Username</label>
            <input
              type="text"
              autoFocus
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
            />
          </div>
          <div className="form-group full">
            <label>Password</label>
            <input
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              minLength={8}
            />
          </div>
          <div className="form-group full">
            <label>Role</label>
            <select value={role} onChange={(e) => setRole(e.target.value)}>
              <option value="viewer">Viewer</option>
              <option value="admin">Admin</option>
            </select>
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
            disabled={submitting || !username.trim() || password.length < 8}
          >
            {submitting ? <Spinner size={14} /> : null}
            Create
          </button>
        </div>
      </form>
    </Modal>
  )
}

function ResetPasswordModal({ target, submitting, error, onSubmit, onClose }) {
  const [password, setPassword] = useState('')
  const errorText = error ? describeError(error, 'Reset failed') : ''

  return (
    <Modal title={`Reset password for ${target.username}`} onClose={onClose} size="small">
      <form
        className="modal-form"
        onSubmit={(e) => {
          e.preventDefault()
          if (submitting) return
          onSubmit(password)
        }}
      >
        <div className="modal-grid">
          <div className="form-group full">
            <label>New password</label>
            <input
              type="password"
              autoComplete="new-password"
              autoFocus
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              minLength={8}
            />
          </div>
        </div>
        {errorText && <div className="modal-error">{errorText}</div>}
        <div className="modal-actions">
          <button type="button" className="ghost-btn" onClick={onClose} disabled={submitting}>
            Cancel
          </button>
          <button type="submit" className="primary-btn" disabled={submitting || password.length < 8}>
            {submitting ? <Spinner size={14} /> : null}
            Set password
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
