// Thin wrapper around the native EventSource API that:
//   - opens with credentials so the backend's auth cookie is sent
//   - subscribes to named events ("stdout", "stderr", "end") and the
//     generic "message" event
//   - JSON-parses each frame's data when valid (falls back to the raw string)
//   - closes when the AbortSignal is aborted, when an "end" frame arrives,
//     or on a hard transport error
//
// Returns an object exposing `close()` for explicit teardown.
//
//   const handle = openSSE('/docker/containers/foo/logs/stream?tail=100', {
//     onEvent: ({ type, data }) => console.log(type, data),
//     onError: (err) => console.error(err),
//     onClose: (reason) => console.log('closed', reason),
//   })
//   // ...later
//   handle.close()

const NAMED_EVENTS = ['stdout', 'stderr', 'end']

export function openSSE(path, { onEvent, onError, onClose, signal } = {}) {
  if (typeof EventSource === 'undefined') {
    const err = new Error('EventSource is not available in this environment')
    if (onError) onError(err)
    return { close() {} }
  }

  let closed = false
  const source = new EventSource(path, { withCredentials: true })

  const dispatch = (type) => (event) => {
    if (closed) return
    let data = event?.data
    if (typeof data === 'string' && data.length > 0) {
      try {
        data = JSON.parse(data)
      } catch {
        // leave raw string
      }
    }
    if (onEvent) {
      try {
        onEvent({ type, data })
      } catch {
        // swallow listener errors so the stream stays open
      }
    }
    if (type === 'end') {
      finalize('end')
    }
  }

  const finalize = (reason) => {
    if (closed) return
    closed = true
    try {
      source.close()
    } catch {
      // ignore
    }
    if (signal && abortListener) {
      try {
        signal.removeEventListener('abort', abortListener)
      } catch {
        // ignore
      }
    }
    if (onClose) {
      try {
        onClose(reason)
      } catch {
        // ignore
      }
    }
  }

  source.addEventListener('message', dispatch('message'))
  for (const evt of NAMED_EVENTS) {
    source.addEventListener(evt, dispatch(evt))
  }

  source.onerror = (event) => {
    if (closed) return
    // EventSource fires onerror both on transient hiccups and hard failures.
    // readyState === CLOSED means the browser gave up reconnecting.
    if (source.readyState === EventSource.CLOSED) {
      if (onError) {
        try {
          onError(event)
        } catch {
          // ignore
        }
      }
      finalize('error')
    }
  }

  let abortListener = null
  if (signal) {
    if (signal.aborted) {
      finalize('abort')
    } else {
      abortListener = () => finalize('abort')
      signal.addEventListener('abort', abortListener, { once: true })
    }
  }

  return {
    close() {
      finalize('manual')
    },
  }
}
