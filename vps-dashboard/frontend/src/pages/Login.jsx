import { useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { FiLogIn } from 'react-icons/fi'
import { useAuth } from '../auth/useAuth.js'
import Spinner from '../ui/Spinner.jsx'
import { describeError } from '../ui/errors.js'
import './Login.css'

export default function LoginPage() {
  const { login, status } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState(null)
  const [submitting, setSubmitting] = useState(false)

  const redirectTo = location.state?.from?.pathname || '/'

  // If we are already authenticated, bounce to the destination.
  useEffect(() => {
    if (status === 'authenticated') {
      navigate(redirectTo, { replace: true })
    }
  }, [status, navigate, redirectTo])

  const onSubmit = async (e) => {
    e.preventDefault()
    if (submitting) return
    setError(null)
    setSubmitting(true)
    try {
      await login({ username: username.trim(), password })
      navigate(redirectTo, { replace: true })
    } catch (err) {
      if (err.status === 401) {
        setError('Invalid username or password')
      } else {
        setError(describeError(err, 'Sign-in failed'))
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="login-shell">
      <form className="login-card glass" onSubmit={onSubmit}>
        <div className="login-brand">
          <div className="brand-icon">V</div>
          <div>
            <h1>VPS Dashboard</h1>
            <p>Sign in to continue</p>
          </div>
        </div>

        <div className="login-field">
          <label htmlFor="login-username">Username</label>
          <input
            id="login-username"
            type="text"
            autoComplete="username"
            autoFocus
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            disabled={submitting}
            required
          />
        </div>

        <div className="login-field">
          <label htmlFor="login-password">Password</label>
          <input
            id="login-password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            disabled={submitting}
            required
          />
        </div>

        {error && <div className="login-error">{error}</div>}

        <button
          type="submit"
          className="login-submit"
          disabled={submitting || !username || !password}
        >
          {submitting ? <Spinner size={16} /> : <FiLogIn />}
          {submitting ? 'Signing in...' : 'Sign in'}
        </button>
      </form>
    </div>
  )
}
