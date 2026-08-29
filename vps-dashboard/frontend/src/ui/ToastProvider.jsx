import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { FiCheckCircle, FiAlertTriangle, FiXCircle, FiInfo, FiX } from 'react-icons/fi'
import { ToastContext } from './toastContext.js'
import './Toast.css'

let nextId = 1
const TOAST_TTL = 4000

export function ToastProvider({ children }) {
  const [toasts, setToasts] = useState([])
  const timers = useRef(new Map())

  const remove = useCallback((id) => {
    setToasts((current) => current.filter((t) => t.id !== id))
    const timer = timers.current.get(id)
    if (timer) {
      clearTimeout(timer)
      timers.current.delete(id)
    }
  }, [])

  const push = useCallback(
    ({ type = 'info', message, ttl }) => {
      const id = nextId++
      const toast = { id, type, message }
      setToasts((current) => [...current, toast])
      const lifetime = ttl == null ? TOAST_TTL : ttl
      if (lifetime > 0) {
        const timer = setTimeout(() => remove(id), lifetime)
        timers.current.set(id, timer)
      }
      return id
    },
    [remove]
  )

  useEffect(() => {
    const cleanup = timers.current
    return () => {
      cleanup.forEach((t) => clearTimeout(t))
      cleanup.clear()
    }
  }, [])

  const value = useMemo(() => ({ push, remove }), [push, remove])

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="toast-stack" role="status" aria-live="polite">
        {toasts.map((t) => (
          <ToastItem key={t.id} toast={t} onDismiss={() => remove(t.id)} />
        ))}
      </div>
    </ToastContext.Provider>
  )
}

function ToastItem({ toast, onDismiss }) {
  return (
    <div className={`toast toast-${toast.type}`}>
      <div className="toast-icon">
        <ToastIcon type={toast.type} />
      </div>
      <div className="toast-message">{toast.message}</div>
      <button
        type="button"
        className="toast-dismiss"
        onClick={onDismiss}
        aria-label="Dismiss"
      >
        <FiX />
      </button>
    </div>
  )
}

function ToastIcon({ type }) {
  switch (type) {
    case 'success':
      return <FiCheckCircle />
    case 'error':
      return <FiXCircle />
    case 'warning':
      return <FiAlertTriangle />
    default:
      return <FiInfo />
  }
}
