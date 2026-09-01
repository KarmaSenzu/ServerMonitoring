import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const apiPrefixes = [
  '/auth',
  '/system',
  '/docker',
  '/pm2',
  '/projects',
  '/servers',
  '/ssh',
  '/containers',
  '/commands',
  '/tunnels',
  '/cloud',
  '/search',
  '/mcp',
  '/discovery',
  '/generator',
  '/users',
  '/health',
  '/events',
  '/notifications',
  '/alerts',
  '/backups',
  '/environments',
  '/webhooks',
]

// Some API routes share a prefix with SPA routes (e.g. /projects). The
// bypass hook lets browser-initiated HTML requests fall through to Vite
// so the SPA can render, while XHR / fetch / JSON requests still hit
// the backend.
function isHtmlRequest(req) {
  if (req.method !== 'GET') return false
  const accept = req.headers.accept || ''
  if (!accept.includes('text/html')) return false
  // Treat anything an XHR sets as not-HTML.
  if (req.headers['x-requested-with']) return false
  return true
}

// Vite reads its config in Node, so process.env is available. The
// fallback keeps the default proxy target backwards-compatible while
// allowing smoke-test runs to point Vite at a non-default backend port.
// eslint-disable-next-line no-undef
const apiTarget = (typeof process !== 'undefined' && process.env && process.env.VITE_API_TARGET) || 'http://localhost:3001'

const proxy = Object.fromEntries(
  apiPrefixes.map((p) => [
    p,
    {
      target: apiTarget,
      changeOrigin: true,
      secure: false,
      ws: p === '/servers', // WebSocket only for terminal routes
      bypass(req) {
        if (isHtmlRequest(req)) {
          // Returning the URL tells Vite to serve it as a static asset,
          // which then falls through to index.html via the SPA fallback.
          return req.url
        }
      },
    },
  ])
)

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy,
  },
})
