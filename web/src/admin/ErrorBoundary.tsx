import { Component, type ReactNode } from 'react'

type Props = { fallback: ReactNode; children: ReactNode }

/**
 * ErrorBoundary shows fallback instead of a blank page when rendering its
 * children throws.
 */
export class ErrorBoundary extends Component<Props, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children
  }
}
