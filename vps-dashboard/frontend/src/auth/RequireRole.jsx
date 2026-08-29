import { Link } from 'react-router-dom'
import { FiLock } from 'react-icons/fi'
import { useAuth } from './useAuth.js'

export default function RequireRole({ role, children }) {
  const { user } = useAuth()
  if (!user || user.role !== role) {
    return (
      <div className="forbidden glass">
        <FiLock size={32} />
        <h2>Forbidden</h2>
        <p>
          This page requires the <strong>{role}</strong> role.
        </p>
        <Link to="/" className="forbidden-link">
          Back to Dashboard
        </Link>
      </div>
    )
  }
  return children
}
