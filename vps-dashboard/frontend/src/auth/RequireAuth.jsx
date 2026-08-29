import { Navigate, useLocation } from 'react-router-dom'
import { useAuth } from './useAuth.js'
import Spinner from '../ui/Spinner.jsx'

export default function RequireAuth({ children }) {
  const { status } = useAuth()
  const location = useLocation()

  if (status === 'loading') {
    return (
      <div className="route-loading">
        <Spinner size={28} />
        <p>Checking session...</p>
      </div>
    )
  }
  if (status === 'anonymous') {
    return <Navigate to="/login" replace state={{ from: location }} />
  }
  return children
}
