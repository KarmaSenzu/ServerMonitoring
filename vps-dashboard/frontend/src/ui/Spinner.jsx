import { FiRefreshCw } from 'react-icons/fi'

export default function Spinner({ size = 18, className = '' }) {
  return (
    <FiRefreshCw
      className={`spinning ${className}`.trim()}
      size={size}
      aria-label="Loading"
    />
  )
}
