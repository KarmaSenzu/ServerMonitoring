import axios from 'axios'

// In dev mode the Vite proxy forwards /auth, /system, /docker, /projects,
// /discovery, /generator, /users, /health to http://localhost:3001 with
// cookies preserved. In production the SPA is served by Nginx on the same
// origin as the API, so a relative baseURL works in both cases.
export const apiClient = axios.create({
  baseURL: '/',
  withCredentials: true,
  timeout: 15000,
  headers: {
    'Content-Type': 'application/json',
  },
})

// Optional bearer token kept in memory only. Cookie auth is the default,
// so this stays empty unless explicitly set.
let bearerToken = null

export function setBearerToken(token) {
  bearerToken = token || null
}

export function getBearerToken() {
  return bearerToken
}

apiClient.interceptors.request.use((config) => {
  if (bearerToken) {
    config.headers.Authorization = `Bearer ${bearerToken}`
  }
  return config
})

apiClient.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      // Notify the app that the session expired so AuthProvider can
      // flip to anonymous and the router can bounce to /login.
      try {
        window.dispatchEvent(new CustomEvent('auth:expired'))
      } catch {
        // ignore (non-browser environments)
      }
    }
    return Promise.reject(normalizeError(error))
  }
)

function normalizeError(error) {
  // Axios network or timeout error (no response).
  if (!error.response) {
    const wrapped = new Error(error.message || 'network_error')
    wrapped.status = 0
    wrapped.code = 'network_error'
    wrapped.detail = error.message
    wrapped.raw = null
    return wrapped
  }
  const { status, data } = error.response
  const code =
    (data && typeof data === 'object' && data.error) ||
    `http_${status}`
  const detail =
    (data && typeof data === 'object' && (data.detail || data.details)) ||
    error.message
  const wrapped = new Error(typeof detail === 'string' ? detail : code)
  wrapped.status = status
  wrapped.code = code
  wrapped.detail = detail
  wrapped.raw = data
  return wrapped
}

// request runs an axios call and unwraps the JSON body. The error path
// throws a normalized error (see normalizeError) so callers can rely on
// `.status`, `.code`, `.detail`, `.raw`.
export async function request(method, url, data, params) {
  const res = await apiClient.request({
    method,
    url,
    data,
    params,
  })
  return res.data
}
