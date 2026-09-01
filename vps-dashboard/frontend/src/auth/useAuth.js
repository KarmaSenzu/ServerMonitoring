import { useContext } from 'react'
import { AuthContext } from './context.js'

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used inside an AuthProvider')
  }
  return ctx
}

// useRequireRole returns {allowed, role} for a single role check.
// Pass undefined/null to skip the check.
export function useRequireRole(role) {
  const { user } = useAuth()
  const current = user?.role || null
  if (!role) {
    return { allowed: true, role: current }
  }
  return { allowed: current === role, role: current }
}

// canMutate returns true for roles that can perform infrastructure
// operations (restart containers, run commands, deploy, backup, SSH
// terminal/files). Per §32: ADMIN and OPERATOR can mutate.
export function canMutate(role) {
  return role === 'admin' || role === 'operator'
}

// canManage returns true for roles that can manage system configuration
// (users, credentials, providers, tunnels, snippets). Per §32: only
// ADMIN can manage.
export function canManage(role) {
  return role === 'admin'
}
