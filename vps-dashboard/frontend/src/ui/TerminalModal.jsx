import { useEffect, useRef, useState } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import './TerminalModal.css'

// TerminalModal opens a WebSocket connection to the backend SSH PTY
// and renders an interactive terminal (Phase 5: React → WebSocket →
// Go → SSH PTY).
//
// Auth: the browser automatically sends the vpsd_token cookie with
// same-origin WebSocket requests, so no explicit token is needed.
export default function TerminalModal({ server, onClose }) {
  const termRef = useRef(null)
  const wsRef = useRef(null)
  const fitRef = useRef(null)
  const [status, setStatus] = useState('connecting')
  const [error, setError] = useState('')

  useEffect(() => {
    if (!server) return

    const term = new Terminal({
      cols: 80,
      rows: 24,
      fontSize: 13,
      fontFamily: "'SF Mono', 'Fira Code', 'Cascadia Code', monospace",
      theme: {
        background: '#0a0a14',
        foreground: '#e0e0e0',
        cursor: '#00e676',
        selectionBackground: 'rgba(0, 230, 118, 0.2)',
      },
      cursorBlink: true,
      scrollback: 5000,
      allowProposedApi: true,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(termRef.current)
    fit.fit()
    fitRef.current = fit

    // Build WebSocket URL (cookie auth is sent automatically).
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${proto}//${window.location.host}/servers/${encodeURIComponent(server.id)}/terminal`

    let ws
    try {
      ws = new WebSocket(wsUrl)
      wsRef.current = ws
    } catch (err) {
      // Defer state updates to avoid cascading renders in effect body.
      const msg = `WebSocket failed: ${err.message}`
      requestAnimationFrame(() => {
        setStatus('error')
        setError(msg)
      })
      return
    }

    ws.onopen = () => {
      setStatus('connected')
      term.focus()
      // Send initial terminal size.
      ws.send(JSON.stringify({ type: 'resize', rows: term.rows, cols: term.cols }))
    }

    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data)
        if (msg.type === 'output') {
          term.write(msg.data)
        } else if (msg.type === 'error') {
          setStatus('error')
          setError(msg.data)
          term.write(`\r\n\x1b[31m${msg.data}\x1b[0m\r\n`)
        }
      } catch {
        term.write(ev.data)
      }
    }

    ws.onerror = () => {
      setStatus('error')
      setError('WebSocket connection error')
    }

    ws.onclose = () => {
      setStatus('closed')
    }

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'input', data }))
      }
    })

    const onResize = () => {
      if (fitRef.current) fitRef.current.fit()
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', rows: term.rows, cols: term.cols }))
      }
    }
    window.addEventListener('resize', onResize)

    return () => {
      window.removeEventListener('resize', onResize)
      if (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING) {
        try { ws.send(JSON.stringify({ type: 'close' })) } catch { /* ignore */ }
        ws.close()
      }
      term.dispose()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [server?.id])

  const handleClose = () => {
    if (wsRef.current) {
      try { wsRef.current.send(JSON.stringify({ type: 'close' })) } catch { /* ignore */ }
      wsRef.current.close()
    }
    onClose()
  }

  return (
    <div className="terminal-modal-overlay" onClick={handleClose}>
      <div className="terminal-modal glass" onClick={(e) => e.stopPropagation()}>
        <div className="terminal-modal-header">
          <div>
            <span className="terminal-title">
              {server?.name} — {server?.hostname}:{server?.ssh_port}
            </span>
          </div>
          <div className="terminal-status">
            {status === 'connecting' && <span className="status-badge status-unknown">Connecting…</span>}
            {status === 'connected' && <span className="status-badge status-online">Live</span>}
            {status === 'closed' && <span className="status-badge status-offline">Disconnected</span>}
            {status === 'error' && <span className="status-badge status-offline">Error</span>}
            <button type="button" className="ghost-btn terminal-close-btn" onClick={handleClose}>
              Close
            </button>
          </div>
        </div>
        <div className="terminal-container" ref={termRef} />
        {error && <div className="terminal-error mono">{error}</div>}
      </div>
    </div>
  )
}
