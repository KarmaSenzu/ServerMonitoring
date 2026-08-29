import { createContext } from 'react'

// AuthContext exposes {user, status, login, logout, refresh}.
// status: "loading" | "anonymous" | "authenticated"
export const AuthContext = createContext(null)
