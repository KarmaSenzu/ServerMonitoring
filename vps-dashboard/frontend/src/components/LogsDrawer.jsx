import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  FiX,
  FiRefreshCw,
  FiPlay,
  FiPause,
  FiArrowDown,
  FiAlertTriangle,
  FiZap,
} from 'react-icons/fi'
import { docker, pm2 } from '../api/endpoints.js'
import { openSSE } from '../api/sse.js'
import Spinner from '../ui/Spinner.jsx'
import { describeError } from '../ui/errors.js'
import './LogsDrawer.css'

const TAIL_OPTIONS = [50, 200, 500, 1000]
const STREAM_BUFFER_LIMIT = 5000

// LogsDrawer renders a side-drawer log viewer. Two modes:
//   source="docker" → snapshot via /docker/containers/:name/logs
//                     plus optional SSE streaming via /logs/stream
//   source="pm2"    → snapshot only via /pm2/processes/:name/logs
//
// Snapshot lines come back as a single string (stdout) or an object with
// {stdout, stderr, truncated}. We render them in a monospace <pre> with
// auto-scroll to bottom. Stream mode tags each line with stdout|stderr.
export default function LogsDrawer({ open, source = 'docker', name, onClose }) {
  const [tail, setTail] = useState(200)
  const [snapshot, setSnapshot] = useState(null)
  const [snapshotLoading, setSnapshotLoading] = useState(false)
  const [snapshotError, setSnapshotError] = useState(null)

  const [streaming, setStreaming] = useState(false)
  const [paused, setPaused] = useState(false)
  const [streamLines, setStreamLines] = useState([])
  const [streamFooter, setStreamFooter] = useState(null)
  const [streamError, setStreamError] = useState(null)
  const [autoScroll, setAutoScroll] = useState(true)

  const streamHandleRef = useRef(null)
  const pendingLinesRef = useRef([])
  const pausedRef = useRef(false)
  const scrollRef = useRef(null)
  const flushTimerRef = useRef(null)

  const supportsStream = source === 'docker'

  // ---- snapshot fetcher -------------------------------------------------
  const loadSnapshot = useCallback(async () => {
    if (!name) return
    setSnapshotLoading(true)
    setSnapshotError(null)
    try {
      let data
      if (source === 'pm2') {
        data = await pm2.logs(name, tail)
      } else {
        data = await docker.logs(name, { tail })
      }
      setSnapshot(data)
    } catch (err) {
      setSnapshotError(err)
    } finally {
      setSnapshotLoading(false)
    }
  }, [name, source, tail])

  // ---- streaming --------------------------------------------------------
  const flushPending = useCallback(() => {
    if (pendingLinesRef.current.length === 0) return
    const batch = pendingLinesRef.current
    pendingLinesRef.current = []
    setStreamLines((prev) => {
      const next = prev.concat(batch)
      if (next.length > STREAM_BUFFER_LIMIT) {
        return next.slice(next.length - STREAM_BUFFER_LIMIT)
      }
      return next
    })
  }, [])

  const scheduleFlush = useCallback(() => {
    if (pausedRef.current) return
    if (flushTimerRef.current) return
    flushTimerRef.current = setTimeout(() => {
      flushTimerRef.current = null
      flushPending()
    }, 80)
  }, [flushPending])

  const stopStream = useCallback(() => {
    if (streamHandleRef.current) {
      streamHandleRef.current.close()
      streamHandleRef.current = null
    }
    if (flushTimerRef.current) {
      clearTimeout(flushTimerRef.current)
      flushTimerRef.current = null
    }
    pendingLinesRef.current = []
  }, [])

  const startStream = useCallback(() => {
    if (!name || !supportsStream) return
    stopStream()
    pausedRef.current = false

    const path = docker.streamLogsPath(name, { tail: 100 })
    streamHandleRef.current = openSSE(path, {
      onEvent: ({ type, data }) => {
        if (type === 'end') {
          const reason = (data && data.reason) || 'closed'
          const exit = data && typeof data.exit_code === 'number' ? data.exit_code : null
          const text = exit != null
            ? `Stream ended (${reason}, exit ${exit})`
            : `Stream ended (${reason})`
          setStreamFooter(text)
          return
        }
        if (type === 'stdout' || type === 'stderr') {
          const line = data && typeof data === 'object' && typeof data.line === 'string'
            ? data.line
            : typeof data === 'string'
              ? data
              : ''
          pendingLinesRef.current.push({ stream: type, line })
          scheduleFlush()
        }
      },
      onError: (err) => {
        setStreamError(err && err.message ? err.message : 'Stream error')
      },
      onClose: (reason) => {
        if (reason === 'manual' || reason === 'abort') return
        // ensure any pending lines flush even after close
        flushPending()
      },
    })
  }, [name, supportsStream, stopStream, scheduleFlush, flushPending])

  // The streaming toggle is imperative: clear stream state and open / close
  // the EventSource directly. Doing this here rather than in an effect keeps
  // the side-effects readable and avoids state-mutation-inside-effect lint.
  const handleToggleStreaming = () => {
    if (streaming) {
      stopStream()
      setStreaming(false)
      return
    }
    setStreamLines([])
    setStreamFooter(null)
    setStreamError(null)
    setPaused(false)
    pausedRef.current = false
    startStream()
    setStreaming(true)
  }

  // Drive snapshot fetching as a side-effect against the API. The fetch
  // helper itself owns the state mutations.
  useEffect(() => {
    if (!open || streaming) return undefined
    let cancelled = false
    ;(async () => {
      if (cancelled) return
      await loadSnapshot()
    })()
    return () => {
      cancelled = true
    }
  }, [open, streaming, loadSnapshot])

  // Tear down the SSE subscription when the drawer is closed or the
  // component unmounts. We only need to stop the subscription here; state
  // resets happen in handleClose so we never mutate state inside an effect
  // body.
  useEffect(() => {
    if (open) return undefined
    return () => {
      stopStream()
    }
  }, [open, stopStream])

  useEffect(() => {
    return () => {
      stopStream()
    }
  }, [stopStream])

  // When the drawer closes, reset to a clean slate so reopening for a
  // different target doesn't show stale lines. We mirror this in onClose
  // explicitly to avoid a setState-in-effect pattern.
  const handleClose = () => {
    stopStream()
    setStreaming(false)
    setStreamLines([])
    setStreamFooter(null)
    setStreamError(null)
    setSnapshot(null)
    setSnapshotError(null)
    setPaused(false)
    pausedRef.current = false
    if (onClose) onClose()
  }

  useEffect(() => {
    pausedRef.current = paused
    if (!paused) {
      flushPending()
    }
  }, [paused, flushPending])

  // ---- auto-scroll handling --------------------------------------------
  useEffect(() => {
    if (!autoScroll) return
    const el = scrollRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
  }, [streamLines, snapshot, autoScroll])

  const handleScroll = () => {
    const el = scrollRef.current
    if (!el) return
    const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
    const atBottom = distanceFromBottom < 24
    if (atBottom !== autoScroll) {
      setAutoScroll(atBottom)
    }
  }

  const jumpToBottom = () => {
    const el = scrollRef.current
    if (!el) return
    el.scrollTop = el.scrollHeight
    setAutoScroll(true)
  }

  // ---- rendering -------------------------------------------------------
  const snapshotBody = useMemo(() => {
    if (!snapshot) return null
    const out = typeof snapshot.stdout === 'string' ? snapshot.stdout : ''
    const err = typeof snapshot.stderr === 'string' ? snapshot.stderr : ''
    return { out, err, truncated: !!snapshot.truncated }
  }, [snapshot])

  if (!open) return null

  return (
    <div className="logs-drawer-backdrop" onMouseDown={handleClose}>
      <aside
        className="logs-drawer glass"
        onMouseDown={(e) => e.stopPropagation()}
        role="dialog"
        aria-label={`Logs: ${name}`}
      >
        <header className="logs-drawer-header">
          <div className="logs-drawer-title">
            <h3>{source === 'pm2' ? 'PM2 logs' : 'Container logs'}</h3>
            <span className="logs-drawer-subject">{name}</span>
          </div>
          <button type="button" className="logs-drawer-close" onClick={handleClose} aria-label="Close">
            <FiX />
          </button>
        </header>

        <div className="logs-drawer-toolbar">
          {!streaming && (
            <div className="logs-toolbar-group">
              <span className="logs-toolbar-label">Tail</span>
              <div className="logs-tail-toggle">
                {TAIL_OPTIONS.map((n) => (
                  <button
                    key={n}
                    type="button"
                    className={`tail-btn ${tail === n ? 'active' : ''}`}
                    onClick={() => setTail(n)}
                  >
                    {n}
                  </button>
                ))}
              </div>
            </div>
          )}

          <div className="logs-toolbar-actions">
            {!streaming && (
              <button
                type="button"
                className="logs-btn"
                onClick={loadSnapshot}
                disabled={snapshotLoading}
              >
                {snapshotLoading ? <Spinner size={13} /> : <FiRefreshCw />}
                Refresh
              </button>
            )}
            {supportsStream && (
              <button
                type="button"
                className={`logs-btn ${streaming ? 'active' : ''}`}
                onClick={handleToggleStreaming}
              >
                <FiZap />
                {streaming ? 'Stop stream' : 'Stream'}
              </button>
            )}
            {streaming && (
              <button
                type="button"
                className={`logs-btn ${paused ? 'active' : ''}`}
                onClick={() => setPaused((v) => !v)}
              >
                {paused ? <FiPlay /> : <FiPause />}
                {paused ? 'Resume' : 'Pause'}
              </button>
            )}
          </div>
        </div>

        <div className="logs-drawer-body">
          {snapshotBody?.truncated && !streaming && (
            <div className="logs-truncated">
              <FiAlertTriangle />
              Output truncated. Increase tail to see more.
            </div>
          )}

          {streamError && streaming && (
            <div className="logs-error">
              <FiAlertTriangle />
              {streamError}
            </div>
          )}

          {!streaming && snapshotError && (
            <div className="logs-error">
              <FiAlertTriangle />
              {describeError(snapshotError, 'Failed to load logs')}
            </div>
          )}

          <div
            ref={scrollRef}
            className="logs-scroll"
            onScroll={handleScroll}
          >
            {!streaming && snapshotBody && (
              <pre className="logs-pre">
                {snapshotBody.out}
                {snapshotBody.err && (
                  <span className="logs-line stderr">{`\n--- stderr ---\n${snapshotBody.err}`}</span>
                )}
              </pre>
            )}

            {streaming && (
              <pre className="logs-pre">
                {streamLines.map((entry, i) => (
                  <span key={i} className={`logs-line ${entry.stream}`}>
                    {entry.line}
                    {'\n'}
                  </span>
                ))}
                {streamFooter && (
                  <span className="logs-line footer">{`\n${streamFooter}\n`}</span>
                )}
              </pre>
            )}

            {!streaming && !snapshotBody && !snapshotLoading && !snapshotError && (
              <div className="logs-empty">No log output</div>
            )}
          </div>

          {!autoScroll && (
            <button type="button" className="logs-jump-btn" onClick={jumpToBottom}>
              <FiArrowDown />
              Jump to bottom
            </button>
          )}
        </div>
      </aside>
    </div>
  )
}
