import { Component } from 'react'

// ErrorBoundary catches any uncaught render/lifecycle error in its subtree
// and shows a friendly recovery screen instead of a blank/black page.
// This is critical for a monitoring dashboard: a single bad data shape
// (e.g. null metrics from a remote server) must never unmount the whole app.
export default class ErrorBoundary extends Component {
  constructor(props) {
    super(props)
    this.state = { error: null, hasError: false }
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, error }
  }

  componentDidCatch(error, info) {
    // eslint-disable-next-line no-console
    console.error('[ErrorBoundary]', error, info?.componentStack)
  }

  handleReload = () => {
    this.setState({ hasError: false, error: null })
    if (this.props.onReset) this.props.onReset()
  }

  handleReloadPage = () => {
    window.location.reload()
  }

  render() {
    if (!this.state.hasError) {
      return this.props.children
    }

    const message = this.state.error?.message || 'An unexpected error occurred'

    return (
      <div className="error-boundary">
        <div className="error-boundary-card">
          <div className="error-boundary-icon">⚠</div>
          <h1 className="error-boundary-title">Something went wrong</h1>
          <p className="error-boundary-subtitle">
            The dashboard hit an unexpected error. Your monitoring data is safe.
          </p>
          <pre className="error-boundary-detail">{message}</pre>
          <div className="error-boundary-actions">
            <button className="error-boundary-btn primary" onClick={this.handleReload}>
              Try Again
            </button>
            <button className="error-boundary-btn ghost" onClick={this.handleReloadPage}>
              Reload Page
            </button>
          </div>
        </div>
      </div>
    )
  }
}
