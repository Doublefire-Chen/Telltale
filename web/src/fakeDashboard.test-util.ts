import { vi } from 'vitest'
import type { Host, LiveMessage } from './api'

export type FakeDashboardOptions = {
  setupRequired?: boolean
  password?: string
  loginError?: 'rate_limited'
  authenticated?: boolean
  hosts?: Host[]
}

/** FakeWebSocket stands in for the live feed; tests push messages with emit. */
export class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onmessage: ((e: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  closed = false

  constructor(readonly url: string) {
    FakeWebSocket.instances.push(this)
  }

  emit(msg: LiveMessage) {
    this.onmessage?.({ data: JSON.stringify(msg) })
  }

  close() {
    this.closed = true
  }
}

/**
 * stubDashboard replaces fetch and WebSocket with an in-memory stand-in for
 * the Dashboard's frontend API, and returns its mutable state.
 */
export function stubDashboard(opts: FakeDashboardOptions = {}) {
  const state = {
    setupRequired: opts.setupRequired ?? false,
    password: opts.password ?? 'correct horse battery',
    authenticated: opts.authenticated ?? false,
    hosts: opts.hosts ?? [],
  }
  const json = (status: number, body: unknown) =>
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

  FakeWebSocket.instances = []
  vi.stubGlobal('WebSocket', FakeWebSocket)
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      const body = init?.body ? JSON.parse(String(init.body)) : undefined
      if (path === '/api/session' && method === 'GET') {
        return json(200, { setup_required: state.setupRequired, authenticated: state.authenticated })
      }
      if (path === '/api/session' && method === 'POST') {
        if (opts.loginError) return json(429, { error: opts.loginError })
        if (body.username !== 'admin' || body.password !== state.password) {
          return json(401, { error: 'invalid_credentials' })
        }
        state.authenticated = true
        return new Response(null, { status: 204 })
      }
      if (path === '/api/session' && method === 'DELETE') {
        state.authenticated = false
        return new Response(null, { status: 204 })
      }
      if (!state.authenticated) return json(401, { error: 'unauthenticated' })
      if (path === '/api/status') return json(200, { dashboard_version: 'v1.2.0', hosts: state.hosts })
      if (path === '/api/hosts' && method === 'POST') {
        const name = String(body.name).trim()
        if (name.length > 64) return json(400, { error: 'invalid_name' })
        const host: Host = { id: `h${state.hosts.length + 1}`, name, host_info: null, report: null, agent_outdated: false }
        state.hosts = [...state.hosts, host]
        return json(201, { id: host.id, name, token: 'tt_' + 'A'.repeat(43) })
      }
      return json(404, { error: 'not_found' })
    }),
  )
  return state
}
