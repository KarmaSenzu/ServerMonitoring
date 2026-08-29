import { request } from './client'

// Helper that unwraps the {data: ...} envelope used by most endpoints.
async function getData(url, params) {
  const body = await request('get', url, undefined, params)
  return body && Object.prototype.hasOwnProperty.call(body, 'data')
    ? body.data
    : body
}

export const auth = {
  login: (creds) => request('post', '/auth/login', creds),
  logout: () => request('post', '/auth/logout'),
  me: async () => {
    const body = await request('get', '/auth/me')
    return body && body.user ? body.user : body
  },
  refresh: () => request('post', '/auth/refresh'),
}

export const system = {
  stats: () => getData('/system/stats'),
  cpu: () => getData('/system/cpu'),
  memory: () => getData('/system/memory'),
  disk: () => getData('/system/disk'),
  host: () => getData('/system/host'),
  network: () => getData('/system/network'),
  history: ({ window = '1h' } = {}) => getData('/system/history', { window }),
}

export const tunnels = {
  list: () => getData('/system/tunnels'),
  get: () => getData('/system/tunnel'),
  restart: (service) =>
    request('post', `/system/tunnels/${encodeURIComponent(service)}/restart`).then(unwrap),
}

export const docker = {
  containers: () => getData('/docker/containers'),
  start: (name) => request('post', `/docker/containers/${encodeURIComponent(name)}/start`),
  stop: (name, timeoutSeconds) =>
    request('post', `/docker/containers/${encodeURIComponent(name)}/stop`,
      timeoutSeconds ? { timeout_seconds: timeoutSeconds } : {}),
  restart: (name, timeoutSeconds) =>
    request('post', `/docker/containers/${encodeURIComponent(name)}/restart`,
      timeoutSeconds ? { timeout_seconds: timeoutSeconds } : {}),
  logs: (name, { tail = 200, since } = {}) => {
    const params = { tail }
    if (since) params.since = since
    return getData(`/docker/containers/${encodeURIComponent(name)}/logs`, params)
  },
  // streamLogsPath returns the relative SSE URL. Use openSSE() from api/sse.js
  // to actually open the stream — EventSource is the right primitive.
  streamLogsPath: (name, { tail = 100 } = {}) => {
    const qs = new URLSearchParams()
    if (tail != null) qs.set('tail', String(tail))
    const suffix = qs.toString() ? `?${qs.toString()}` : ''
    return `/docker/containers/${encodeURIComponent(name)}/logs/stream${suffix}`
  },
}

export const pm2 = {
  list: () => getData('/pm2/processes'),
  logs: (name, lines = 200) =>
    getData(`/pm2/processes/${encodeURIComponent(name)}/logs`, { lines }),
  action: (name, action) =>
    request('post', `/pm2/processes/${encodeURIComponent(name)}/${encodeURIComponent(action)}`),
  remove: (name) =>
    request('delete', `/pm2/processes/${encodeURIComponent(name)}`),
}

export const projects = {
  list: (filter) => getData('/projects', filter),
  get: (id) => getData(`/projects/${encodeURIComponent(id)}`),
  getByName: (name) => getData(`/projects/by-name/${encodeURIComponent(name)}`),
  create: (payload) => request('post', '/projects', payload).then(unwrap),
  update: (id, payload) => request('put', `/projects/${encodeURIComponent(id)}`, payload).then(unwrap),
  patch: (id, payload) => request('patch', `/projects/${encodeURIComponent(id)}`, payload).then(unwrap),
  remove: (id) => request('delete', `/projects/${encodeURIComponent(id)}`),
  health: (id) => getData(`/projects/${encodeURIComponent(id)}/health`),
  healthHistory: (id, { since, limit } = {}) => {
    const params = {}
    if (since) params.since = since
    if (limit != null) params.limit = limit
    return getData(`/projects/${encodeURIComponent(id)}/health-history`, params)
  },
  action: (id, action) =>
    request('post', `/projects/${encodeURIComponent(id)}/${encodeURIComponent(action)}`).then(unwrap),

  // Wave 4 — deployments and webhook secret.
  deploy: (id, { wait = false, remoteRef = 'manual' } = {}) =>
    request(
      'post',
      `/projects/${encodeURIComponent(id)}/deploy`,
      { remote_ref: remoteRef },
      wait ? { wait: 'true' } : undefined,
    ).then(unwrap),
  deployments: (id, { limit = 20 } = {}) =>
    getData(`/projects/${encodeURIComponent(id)}/deployments`, { limit }),
  deployment: (id, deploymentId) =>
    getData(
      `/projects/${encodeURIComponent(id)}/deployments/${encodeURIComponent(deploymentId)}`,
    ),
  regenerateWebhookSecret: (id) =>
    request(
      'post',
      `/projects/${encodeURIComponent(id)}/webhook-secret/regenerate`,
    ).then(unwrap),
}

// Wave 4 — database backups (admin can run/download/delete; both roles can list).
export const backups = {
  list: () => getData('/backups'),
  get: (id) => getData(`/backups/${encodeURIComponent(id)}`),
  run: () => request('post', '/backups/run').then(unwrap),
  remove: (id) => request('delete', `/backups/${encodeURIComponent(id)}`),
  // Direct browser-driven download. Returns a relative URL the caller
  // can stick into an <a href=...> or window.location to trigger the
  // native download flow with cookie auth.
  downloadUrl: (id) => `/backups/${encodeURIComponent(id)}/download`,
}

// Wave 4 — per-environment overrides.
export const environments = {
  list: () => getData('/environments'),
  update: (env, payload) =>
    request('put', `/environments/${encodeURIComponent(env)}`, payload).then(unwrap),
  defaults: () => getData('/environments/defaults'),
}

export const discovery = {
  snapshot: () => getData('/discovery/snapshot'),
  adopt: (candidate, overrides) =>
    request('post', '/discovery/adopt', { candidate, overrides: overrides || {} }).then(unwrap),
  adoptMany: (candidates) =>
    request('post', '/discovery/adopt-many', { candidates }).then(unwrap),
}

export const generator = {
  docker: (opts) => request('post', '/generator/docker', opts),
  pm2: (opts) => request('post', '/generator/pm2', opts),
  compose: (opts) => request('post', '/generator/compose', opts),
  nginx: (opts) => request('post', '/generator/nginx', opts),
}

export const users = {
  list: () => getData('/users'),
  create: (payload) => request('post', '/users', payload).then(unwrap),
  patch: (id, payload) => request('patch', `/users/${encodeURIComponent(id)}`, payload).then(unwrap),
  remove: (id) => request('delete', `/users/${encodeURIComponent(id)}`),
}

// Events: returns the raw envelope `{data, total}` because pagination
// callers need both pieces.
export const events = {
  list: ({ since, until, category, severity, projectId, q, limit, offset } = {}) => {
    const params = {}
    if (since) params.since = since
    if (until) params.until = until
    if (category && category !== 'all') params.category = category
    if (severity && severity !== 'all') params.severity = severity
    if (projectId) params.project_id = projectId
    if (q) params.q = q
    if (limit != null) params.limit = limit
    if (offset != null) params.offset = offset
    return request('get', '/events', undefined, params)
  },
}

// Notification channels.
export const channels = {
  list: () => getData('/notifications/channels'),
  get: (id) => getData(`/notifications/channels/${encodeURIComponent(id)}`),
  create: (payload) => request('post', '/notifications/channels', payload).then(unwrap),
  patch: (id, payload) =>
    request('patch', `/notifications/channels/${encodeURIComponent(id)}`, payload).then(unwrap),
  remove: (id) => request('delete', `/notifications/channels/${encodeURIComponent(id)}`),
  test: (id) =>
    request('post', `/notifications/channels/${encodeURIComponent(id)}/test`).then(unwrap),
}

// Alert rules.
export const alerts = {
  list: () => getData('/alerts/rules'),
  get: (id) => getData(`/alerts/rules/${encodeURIComponent(id)}`),
  create: (payload) => request('post', '/alerts/rules', payload).then(unwrap),
  patch: (id, payload) =>
    request('patch', `/alerts/rules/${encodeURIComponent(id)}`, payload).then(unwrap),
  remove: (id) => request('delete', `/alerts/rules/${encodeURIComponent(id)}`),
  test: (id) => request('post', `/alerts/rules/${encodeURIComponent(id)}/test`).then(unwrap),
}

function unwrap(body) {
  if (body && Object.prototype.hasOwnProperty.call(body, 'data')) {
    return body.data
  }
  return body
}
