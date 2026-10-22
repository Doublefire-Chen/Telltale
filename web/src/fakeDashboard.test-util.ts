import { vi } from 'vitest'
import type { Host, LiveMessage } from './api'

export type FakeDashboardOptions = {
  setupRequired?: boolean
  password?: string
  loginError?: 'rate_limited'
  authenticated?: boolean
  statusPublic?: boolean
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
    statusPublic: opts.statusPublic ?? false,
    hosts: opts.hosts ?? [],
    requests: [] as { method: string; path: string; body: Record<string, unknown> }[],
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
        return json(200, {
          setup_required: state.setupRequired,
          authenticated: state.authenticated,
          status_public: state.statusPublic,
        })
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
      if (path === '/api/status' && (state.authenticated || state.statusPublic)) {
        return json(200, { dashboard_version: 'v1.2.0', hosts: state.hosts })
      }
      if (!state.authenticated) return json(401, { error: 'unauthenticated' })
      if (path === '/api/settings' && method === 'GET') return json(200, { status_public: state.statusPublic })
      if (path === '/api/settings' && method === 'PUT') {
        state.statusPublic = body.status_public
        state.requests.push({ method, path, body })
        return new Response(null, { status: 204 })
      }
      if (path === '/api/hosts' && method === 'POST') {
        const name = String(body.name).trim()
        if (name.length > 64) return json(400, { error: 'invalid_name' })
        const host: Host = {
          id: `h${state.hosts.length + 1}`,
          name,
          host_info: null,
          report: null,
          online: false,
          agent_outdated: false,
          countries: [],
          country_override: '',
          group: '',
          hidden: false,
        }
        state.hosts = [...state.hosts, host]
        return json(201, { id: host.id, name, token: 'tt_' + 'A'.repeat(43) })
      }
      if (path === '/api/hosts/order' && method === 'PUT') {
        const ids: string[] = body.ids
        state.hosts = ids.map((id) => state.hosts.find((h) => h.id === id)!)
        state.requests.push({ method, path, body })
        return new Response(null, { status: 204 })
      }
      const one = path.match(/^\/api\/hosts\/([^/]+)(\/token)?$/)
      const target = one && state.hosts.find((h) => h.id === one[1])
      if (one && !target) return json(404, { error: 'not_found' })
      if (one && target) {
        state.requests.push({ method, path, body })
        if (one[2] && method === 'POST') return json(200, { token: 'tt_' + 'N'.repeat(43) })
        if (method === 'DELETE') {
          state.hosts = state.hosts.filter((h) => h !== target)
          return new Response(null, { status: 204 })
        }
        if (method === 'PATCH') {
          if (body.country_override && !/^[A-Z]{2}$/.test(body.country_override)) {
            return json(400, { error: 'invalid_country' })
          }
          const updated = { ...target, ...body }
          updated.countries = updated.country_override ? [updated.country_override] : target.countries
          state.hosts = state.hosts.map((h) => (h === target ? updated : h))
          return new Response(null, { status: 204 })
        }
      }
      return json(404, { error: 'not_found' })
    }),
  )
  return state
}
