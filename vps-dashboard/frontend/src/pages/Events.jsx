import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useSearchParams } from 'react-router-dom'
import {
  FiRefreshCw,
  FiSearch,
  FiX,
  FiList,
  FiAlertTriangle,
  FiChevronLeft,
  FiChevronRight,
  FiChevronDown,
  FiChevronUp,
  FiExternalLink,
} from 'react-icons/fi'
import { events, projects } from '../api/endpoints.js'
import EmptyState from '../ui/EmptyState.jsx'
import Spinner from '../ui/Spinner.jsx'
import SeverityBadge from '../ui/SeverityBadge.jsx'
import RelativeTime from '../ui/RelativeTime.jsx'
import { describeError } from '../ui/errors.js'
import './Events.css'

const TIME_PRESETS = [
  { value: '15m', label: '15m', seconds: 15 * 60 },
  { value: '1h', label: '1h', seconds: 3600 },
  { value: '6h', label: '6h', seconds: 6 * 3600 },
  { value: '24h', label: '24h', seconds: 24 * 3600 },
  { value: '7d', label: '7d', seconds: 7 * 86400 },
]

const CATEGORY_OPTIONS = [
  'all',
  'health',
  'system',
  'docker',
  'pm2',
  'tunnel',
  'auth',
  'alert',
]

const SEVERITY_OPTIONS = ['all', 'info', 'warning', 'error', 'critical']

const PAGE_SIZES = [50, 100, 200]

// Convert a "datetime-local" input value into an ISO string. The
// browser returns a tz-naive YYYY-MM-DDTHH:mm; we treat it as the
// user's local zone and emit RFC 3339 in UTC.
function localToISO(local) {
  if (!local) return ''
  const d = new Date(local)
  if (Number.isNaN(d.getTime())) return ''
  return d.toISOString()
}

function isoToLocal(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export default function EventsPage() {
  const queryClient = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()

  const [preset, setPreset] = useState('1h')
  const [customSince, setCustomSince] = useState('')
  const [customUntil, setCustomUntil] = useState('')
  const [category, setCategory] = useState(searchParams.get('category') || 'all')
  const [severity, setSeverity] = useState(searchParams.get('severity') || 'all')
  const [projectId, setProjectId] = useState(searchParams.get('project_id') || '')
  const [projectQuery, setProjectQuery] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [search, setSearch] = useState('')
  const [autoRefresh, setAutoRefresh] = useState(false)
  const [pageSize, setPageSize] = useState(100)
  const [page, setPage] = useState(0)
  const [expanded, setExpanded] = useState(null)
  // refreshTick advances every 30s while a preset range is active so
  // the rolling window slides forward without breaking the purity rule
  // (Date.now is read in an effect, not directly during render).
  const [nowRef, setNowRef] = useState(() => Date.now())

  useEffect(() => {
    if (preset === 'custom') return undefined
    const id = setInterval(() => setNowRef(Date.now()), 30_000)
    return () => clearInterval(id)
  }, [preset])

  // Mirror category/severity/projectId into the URL so links from
  // other pages (e.g. Dashboard "Recent alerts") preselect correctly.
  useEffect(() => {
    const next = new URLSearchParams(searchParams)
    if (category !== 'all') next.set('category', category)
    else next.delete('category')
    if (severity !== 'all') next.set('severity', severity)
    else next.delete('severity')
    if (projectId) next.set('project_id', projectId)
    else next.delete('project_id')
    setSearchParams(next, { replace: true })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [category, severity, projectId])

  // Debounce search input.
  useEffect(() => {
    const t = setTimeout(() => {
      setSearch(searchInput.trim())
      setPage(0)
    }, 300)
    return () => clearTimeout(t)
  }, [searchInput])

  const bounds = useMemo(() => {
    if (preset === 'custom') {
      return {
        since: localToISO(customSince) || '',
        until: localToISO(customUntil) || '',
      }
    }
    const found = TIME_PRESETS.find((p) => p.value === preset)
    const sec = found ? found.seconds : 3600
    return { since: new Date(nowRef - sec * 1000).toISOString(), until: '' }
  }, [preset, customSince, customUntil, nowRef])

  const projectsQ = useQuery({
    queryKey: ['projects', { all: true }],
    queryFn: () => projects.list(),
    staleTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
  })

  const eventsQ = useQuery({
    queryKey: [
      'events',
      {
        since: bounds.since,
        until: bounds.until,
        category,
        severity,
        projectId,
        q: search,
        limit: pageSize,
        offset: page * pageSize,
      },
    ],
    queryFn: () =>
      events.list({
        since: bounds.since || undefined,
        until: bounds.until || undefined,
        category,
        severity,
        projectId: projectId || undefined,
        q: search || undefined,
        limit: pageSize,
        offset: page * pageSize,
      }),
    refetchInterval: autoRefresh ? 10000 : false,
    keepPreviousData: true,
  })

  const list = Array.isArray(eventsQ.data?.data) ? eventsQ.data.data : []
  const total = Number(eventsQ.data?.total ?? 0)
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  const projectIndex = useMemo(() => {
    const m = new Map()
    if (Array.isArray(projectsQ.data)) {
      for (const p of projectsQ.data) {
        if (p && p.id) m.set(p.id, p)
      }
    }
    return m
  }, [projectsQ.data])

  const projectMatches = useMemo(() => {
    const list2 = Array.isArray(projectsQ.data) ? projectsQ.data : []
    const q = projectQuery.trim().toLowerCase()
    if (!q) return list2.slice(0, 8)
    return list2
      .filter((p) =>
        [p.name, p.domain, p.id].some((v) =>
          String(v || '').toLowerCase().includes(q)
        )
      )
      .slice(0, 8)
  }, [projectsQ.data, projectQuery])

  const handlePresetChange = (value) => {
    setPreset(value)
    setPage(0)
    if (value !== 'custom') {
      setCustomSince('')
      setCustomUntil('')
    }
  }

  const handleRefresh = () => {
    queryClient.invalidateQueries({ queryKey: ['events'] })
  }

  const showFrom = total === 0 ? 0 : page * pageSize + 1
  const showTo = Math.min((page + 1) * pageSize, total)

  return (
    <div className="events-page">
      <div className="page-header">
        <div className="page-header-row">
          <div>
            <h1>Events</h1>
            <p>Recent activity, alerts, and audit history</p>
          </div>
          <div className="header-actions">
            <label className="auto-refresh-toggle">
              <input
                type="checkbox"
                checked={autoRefresh}
                onChange={(e) => setAutoRefresh(e.target.checked)}
              />
              Auto-refresh (10s)
            </label>
            <button
              type="button"
              className="refresh-btn glass"
              onClick={handleRefresh}
              disabled={eventsQ.isFetching}
            >
              <FiRefreshCw className={eventsQ.isFetching ? 'spinning' : ''} />
              Refresh
            </button>
          </div>
        </div>
      </div>

      <div className="events-toolbar glass">
        <div className="toolbar-row">
          <div className="time-presets">
            {TIME_PRESETS.map((p) => (
              <button
                key={p.value}
                type="button"
                className={`time-chip ${preset === p.value ? 'active' : ''}`}
                onClick={() => handlePresetChange(p.value)}
              >
                {p.label}
              </button>
            ))}
            <button
              type="button"
              className={`time-chip ${preset === 'custom' ? 'active' : ''}`}
              onClick={() => handlePresetChange('custom')}
            >
              Custom
            </button>
          </div>

          {preset === 'custom' && (
            <div className="time-custom">
              <label>
                <span>From</span>
                <input
                  type="datetime-local"
                  value={customSince || isoToLocal(bounds.since)}
                  onChange={(e) => setCustomSince(e.target.value)}
                />
              </label>
              <label>
                <span>To</span>
                <input
                  type="datetime-local"
                  value={customUntil || isoToLocal(bounds.until)}
                  onChange={(e) => setCustomUntil(e.target.value)}
                />
              </label>
            </div>
          )}
        </div>

        <div className="toolbar-row">
          <div className="filter-group">
            <label className="filter-label">Category</label>
            <select
              className="filter-select"
              value={category}
              onChange={(e) => {
                setCategory(e.target.value)
                setPage(0)
              }}
            >
              {CATEGORY_OPTIONS.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </div>

          <div className="filter-group">
            <label className="filter-label">Severity</label>
            <select
              className="filter-select"
              value={severity}
              onChange={(e) => {
                setSeverity(e.target.value)
                setPage(0)
              }}
            >
              {SEVERITY_OPTIONS.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </div>

          <div className="filter-group filter-grow">
            <label className="filter-label">Project</label>
            <ProjectAutocomplete
              value={projectId}
              query={projectQuery}
              onQueryChange={setProjectQuery}
              onSelect={(id) => {
                setProjectId(id)
                setPage(0)
              }}
              matches={projectMatches}
              currentProject={projectId ? projectIndex.get(projectId) : null}
            />
          </div>

          <div className="filter-group filter-grow">
            <label className="filter-label">Search</label>
            <div className="search-wrap">
              <FiSearch />
              <input
                type="text"
                placeholder="Search messages"
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
              />
              {searchInput && (
                <button
                  type="button"
                  className="search-clear"
                  onClick={() => setSearchInput('')}
                  aria-label="Clear search"
                >
                  <FiX />
                </button>
              )}
            </div>
          </div>
        </div>
      </div>

      {eventsQ.isLoading ? (
        <div className="loading-state">
          <Spinner size={24} />
          <p>Loading events...</p>
        </div>
      ) : eventsQ.isError ? (
        <EmptyState
          icon={<FiAlertTriangle size={40} />}
          title="Failed to load events"
          description={describeError(eventsQ.error)}
        />
      ) : list.length === 0 ? (
        <EmptyState
          icon={<FiList size={40} />}
          title="No events match"
          description="Try widening the time range or clearing filters"
        />
      ) : (
        <>
          <div className="events-table glass">
            <div className="events-row events-head">
              <div>Time</div>
              <div>Severity</div>
              <div>Category</div>
              <div>Source</div>
              <div>Project</div>
              <div>Message</div>
              <div className="expand-col" aria-hidden="true" />
            </div>
            {list.map((ev) => {
              const isOpen = expanded === ev.id
              const project = ev.project_id ? projectIndex.get(ev.project_id) : null
              return (
                <div key={ev.id} className="events-row-wrap">
                  <button
                    type="button"
                    className={`events-row events-data-row ${isOpen ? 'open' : ''}`}
                    onClick={() => setExpanded(isOpen ? null : ev.id)}
                  >
                    <div>
                      <RelativeTime value={ev.ts} />
                    </div>
                    <div>
                      <SeverityBadge severity={ev.severity} />
                    </div>
                    <div className="cell-mono">{ev.category || '-'}</div>
                    <div className="cell-mono">{ev.source || '-'}</div>
                    <div className="cell-truncate">
                      {ev.project_id ? (
                        <Link
                          to={`/projects?focus=${encodeURIComponent(ev.project_id)}`}
                          onClick={(e) => e.stopPropagation()}
                          className="project-link"
                        >
                          {project?.name || ev.project_id}
                          <FiExternalLink size={11} />
                        </Link>
                      ) : (
                        <span className="dim">-</span>
                      )}
                    </div>
                    <div className="cell-message">{ev.message || ''}</div>
                    <div className="expand-col">
                      {isOpen ? <FiChevronUp /> : <FiChevronDown />}
                    </div>
                  </button>
                  {isOpen && (
                    <div className="event-detail">
                      <div className="event-detail-meta">
                        <span>id</span>
                        <code>{ev.id}</code>
                      </div>
                      <pre className="event-detail-json">
                        {JSON.stringify(ev.data || {}, null, 2)}
                      </pre>
                    </div>
                  )}
                </div>
              )
            })}
          </div>

          <div className="events-pagination">
            <span className="page-info">
              Showing {showFrom}-{showTo} of {total}
            </span>
            <div className="page-controls">
              <select
                className="page-size-select"
                value={pageSize}
                onChange={(e) => {
                  setPageSize(Number(e.target.value))
                  setPage(0)
                }}
              >
                {PAGE_SIZES.map((s) => (
                  <option key={s} value={s}>
                    {s} / page
                  </option>
                ))}
              </select>
              <button
                type="button"
                className="page-btn"
                onClick={() => setPage((p) => Math.max(0, p - 1))}
                disabled={page === 0 || eventsQ.isFetching}
              >
                <FiChevronLeft />
                Prev
              </button>
              <span className="page-indicator">
                Page {page + 1} / {totalPages}
              </span>
              <button
                type="button"
                className="page-btn"
                onClick={() => setPage((p) => p + 1)}
                disabled={page + 1 >= totalPages || eventsQ.isFetching}
              >
                Next
                <FiChevronRight />
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

function ProjectAutocomplete({ value, query, onQueryChange, onSelect, matches, currentProject }) {
  const [open, setOpen] = useState(false)
  const display = currentProject
    ? `${currentProject.name}${currentProject.domain ? ` (${currentProject.domain})` : ''}`
    : ''

  return (
    <div className="autocomplete">
      {value ? (
        <div className="autocomplete-chip">
          <span>{display || value}</span>
          <button
            type="button"
            className="chip-clear"
            onClick={() => {
              onSelect('')
              onQueryChange('')
            }}
            aria-label="Clear project filter"
          >
            <FiX />
          </button>
        </div>
      ) : (
        <input
          type="text"
          className="autocomplete-input"
          placeholder="Any project"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          onFocus={() => setOpen(true)}
          onBlur={() => setTimeout(() => setOpen(false), 150)}
        />
      )}
      {!value && open && matches.length > 0 && (
        <div className="autocomplete-list">
          {matches.map((p) => (
            <button
              key={p.id}
              type="button"
              className="autocomplete-item"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => {
                onSelect(p.id)
                onQueryChange('')
                setOpen(false)
              }}
            >
              <span className="autocomplete-name">{p.name}</span>
              {p.domain && <span className="autocomplete-meta">{p.domain}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
