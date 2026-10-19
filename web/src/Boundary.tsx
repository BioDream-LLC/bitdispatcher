import { Component, type ReactNode } from 'react'

// Boundary keeps one failing panel from blanking the dispatcher's screen: the panel says it failed and
// offers a retry, and every other panel keeps working.
export default class Boundary extends Component<{ name: string; children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null }
  static getDerivedStateFromError(error: Error) {
    return { error }
  }
  render() {
    if (!this.state.error) return this.props.children
    return (
      <div role="alert" className="panel flex min-h-0 flex-1 flex-col items-start gap-2 p-3 text-xs text-rose-200">
        <strong>The {this.props.name} panel failed to draw.</strong>
        <span className="font-mono text-[11px] text-rose-300/80">{this.state.error.message}</span>
        <span className="text-slate-400">Every other panel and the command line still work.</span>
        <button className="btn" onClick={() => this.setState({ error: null })}>Try again</button>
      </div>
    )
  }
}
