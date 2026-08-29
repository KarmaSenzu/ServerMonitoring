import { Link } from 'react-router-dom'
import { FiAlertTriangle } from 'react-icons/fi'

export default function NotFoundPage() {
  return (
    <div className="forbidden glass">
      <FiAlertTriangle size={32} />
      <h2>Page Not Found</h2>
      <p>The page you are looking for does not exist.</p>
      <Link to="/" className="forbidden-link">
        Back to Dashboard
      </Link>
    </div>
  )
}
