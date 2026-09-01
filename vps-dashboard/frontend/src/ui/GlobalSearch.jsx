import { useEffect, useState, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import {
  FiServer,
  FiTerminal,
  FiShuffle,
  FiTag,
  FiSearch,
} from 'react-icons/fi'
import { searchApi } from '../api/endpoints.js'
import './GlobalSearch.css'

const KIND_ICONS = {
  server: <FiServer />,
  command: <FiTerminal />,
  tunnel: <FiShuffle />,
  tag: <FiTag />,
}

const KIND_LABELS = {
  server: 'Servers',
  command: 'Commands',
  tunnel: 'Tunnels',
  tag: 'Tags',
}

export default function GlobalSearch() {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [highlightIdx, setHighlightIdx] = useState(0)
  const inputRef = useRef(null)
  const navigate = useNavigate()

  // Ctrl+K / Cmd+K shortcut.
  useEffect(() => {
    const handler = (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault()
        setOpen((prev) => !prev)
      }
      if (e.key === 'Escape') {
        setOpen(false)
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  // Focus input when opened.
  useEffect(() => {
    if (open && inputRef.current) {
      setTimeout(() => inputRef.current.focus(), 50)
    }
  }, [open])

  // Reset state when closed — deferred to avoid cascading renders.
  useEffect(() => {
    if (!open) {
      const timer = setTimeout(() => {
        setQuery('')
        setHighlightIdx(0)
      }, 100)
      return () => clearTimeout(timer)
    }
    return undefined
  }, [open])

  const searchQ = useQuery({
    queryKey: ['global-search', query],
    queryFn: () => searchApi.search(query),
    enabled: open && query.trim().length > 0,
    staleTime: 5000,
  })

  const data = searchQ.data || {}
  const flatResults = [
    ...(data.servers || []).map((r) => ({ ...r, kindKey: 'server' })),
    ...(data.commands || []).map((r) => ({ ...r, kindKey: 'command' })),
    ...(data.tunnels || []).map((r) => ({ ...r, kindKey: 'tunnel' })),
    ...(data.tags || []).map((r) => ({ ...r, kindKey: 'tag' })),
  ]

  const handleKeyDown = (e) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setHighlightIdx((i) => Math.min(i + 1, flatResults.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setHighlightIdx((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter' && flatResults[highlightIdx]) {
      e.preventDefault()
      const result = flatResults[highlightIdx]
      if (result.link) {
        navigate(result.link)
        setOpen(false)
      }
    }
  }

  const handleResultClick = (result) => {
    if (result.link) {
      navigate(result.link)
      setOpen(false)
    }
  }

  if (!open) {
    return (
      <button
        type="button"
        className="global-search-trigger"
        onClick={() => setOpen(true)}
        title="Search (Ctrl+K)"
      >
        <FiSearch />
        <span className="search-trigger-text">Search…</span>
        <kbd className="search-kbd">⌘K</kbd>
      </button>
    )
  }

  return (
    <div className="global-search-overlay" onClick={() => setOpen(false)}>
      <div className="global-search-modal glass" onClick={(e) => e.stopPropagation()}>
        <div className="global-search-input-row">
          <FiSearch className="search-input-icon" />
          <input
            ref={inputRef}
            type="text"
            className="global-search-input"
            placeholder="Search infrastructure…"
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setHighlightIdx(0)
            }}
            onKeyDown={handleKeyDown}
          />
          <kbd className="search-kbd">ESC</kbd>
        </div>

        {query.trim() && (
          <div className="global-search-results">
            {searchQ.isLoading ? (
              <div className="search-loading">Searching…</div>
            ) : flatResults.length === 0 ? (
              <div className="search-empty">No results for &quot;{query}&quot;</div>
            ) : (
              <>
                {['server', 'command', 'tunnel', 'tag'].map((kind) => {
                  const items = flatResults.filter((r) => r.kindKey === kind)
                  if (items.length === 0) return null
                  return (
                    <div key={kind} className="search-group">
                      <div className="search-group-label">{KIND_LABELS[kind]}</div>
                      {items.map((r) => {
                        const idx = flatResults.indexOf(r)
                        return (
                          <button
                            key={`${kind}-${r.id}`}
                            type="button"
                            className={`search-result ${idx === highlightIdx ? 'highlighted' : ''}`}
                            onClick={() => handleResultClick(r)}
                            onMouseEnter={() => setHighlightIdx(idx)}
                          >
                            <span className="search-result-icon">{KIND_ICONS[kind]}</span>
                            <span className="search-result-name">{r.name}</span>
                            {r.detail && <span className="search-result-detail mono">{r.detail}</span>}
                          </button>
                        )
                      })}
                    </div>
                  )
                })}
              </>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
