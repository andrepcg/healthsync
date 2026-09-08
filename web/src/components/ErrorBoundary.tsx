import { Component, type ErrorInfo, type ReactNode } from 'react'

interface State {
  error: Error | null
}

/** Keeps one broken widget from blanking the whole dashboard. */
export class ErrorBoundary extends Component<{ children: ReactNode; label?: string }, State> {
  state: State = { error: null }
  static getDerivedStateFromError(error: Error): State {
    return { error }
  }
  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[healthsync]', this.props.label ?? 'component', error, info.componentStack)
  }
  componentDidUpdate(prev: { children: ReactNode }) {
    if (prev.children !== this.props.children && this.state.error) this.setState({ error: null })
  }
  render() {
    if (this.state.error) {
      return (
        <div className="card">
          <div className="err" style={{ fontWeight: 600 }}>Something went wrong{this.props.label ? ` in ${this.props.label}` : ''}.</div>
          <div className="muted small mono" style={{ marginTop: 6 }}>{this.state.error.message}</div>
          <button className="btn sm" style={{ marginTop: 10 }} onClick={() => this.setState({ error: null })}>Retry</button>
        </div>
      )
    }
    return this.props.children
  }
}
