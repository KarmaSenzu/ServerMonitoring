import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { setBearerToken } from '../api/client.js'
import { auth as authEndpoints } from '../api/endpoints.js'
import { AuthContext } from './context.js'

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null)
  const [status, setStatus] = useState('loading')
  const queryClient = useQueryClient()
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
    }
  }, [])

  const setAuthed = useCallback((u) => {
    if (!mountedRef.current) return
    setUser(u)
    setStatus('authenticated')
  }, [])

  const setAnonymous = useCallback(() => {
    if (!mountedRef.current) return
    setUser(null)
    setStatus('anonymous')
    setBearerToken(null)
  }, [])

  // Bootstrap session by calling /auth/me once on mount.
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      try {
        const me = await authEndpoints.me()
        if (cancelled) return
        setAuthed(me)
      } catch {
        if (cancelled) return
        setAnonymous()
      }
    })()
    return () => {
      cancelled = true
    }
  }, [setAuthed, setAnonymous])

  // React to auth:expired events from the axios interceptor.
  useEffect(() => {
    const onExpired = () => {
      setAnonymous()
      queryClient.clear()
    }
    window.addEventListener('auth:expired', onExpired)
    return () => window.removeEventListener('auth:expired', onExpired)
  }, [queryClient, setAnonymous])

  const login = useCallback(
    async (creds) => {
      const result = await authEndpoints.login(creds)
      // Use cookie auth by default; keep token only for advanced cases.
      // Do NOT persist; bearerToken is in-memory only.
      setBearerToken(null)
      const me = result && result.user ? result.user : await authEndpoints.me()
      setAuthed(me)
      return me
    },
    [setAuthed]
  )

  const logout = useCallback(async () => {
    try {
      await authEndpoints.logout()
    } catch {
      // ignore network errors during logout
    }
    queryClient.clear()
    setAnonymous()
  }, [queryClient, setAnonymous])

  const refresh = useCallback(async () => {
    const result = await authEndpoints.refresh()
    if (result && result.user) {
      setAuthed(result.user)
    }
    return result
  }, [setAuthed])

  const value = useMemo(
    () => ({ user, status, login, logout, refresh }),
    [user, status, login, logout, refresh]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
