// Client for the Dashboard's frontend API. Responses carry codes and raw
// values only; all wording lives in the i18n resources.

export type Session = {
  setup_required: boolean
  authenticated: boolean
}

export type LoginResult = 'ok' | 'invalid_credentials' | 'rate_limited'

export type Report = {
  /** When the Dashboard received the Report, Unix ms. */
  received_at: number
  time: number
  interval_ms: number
  cpu_percent?: number
  memory_used?: number
  swap_used?: number
  load1?: number
  load5?: number
  load15?: number
  disk_used?: number
  /** Traffic: bytes since the Host's last boot. */
  net_rx_bytes?: number
  net_tx_bytes?: number
  /** Bandwidth in bytes per second, derived by the Dashboard. */
  rx_rate?: number
  tx_rate?: number
}

export type HostInfo = {
  hostname?: string
  os?: string
  platform?: string
  platform_version?: string
  kernel_version?: string
  arch?: string
  cpu_model?: string
  cpu_cores?: number
  memory_total?: number
  swap_total?: number
  disk_total?: number
  virtualization?: string
  /** Unix seconds. */
  boot_time?: number
  agent_version?: string
}

export type Host = {
  id: string
  /** Display name; empty means show the hostname. */
  name: string
  host_info: HostInfo | null
  report: Report | null
  agent_outdated: boolean
}

export type Status = {
  dashboard_version: string
  hosts: Host[]
}

export type AddedHost = {
  id: string
  name: string
  /** Shown once; the Dashboard keeps only its hash. */
  token: string
}

export type LiveMessage =
  | { type: 'report'; host_id: string; report: Report }
  | { type: 'hosts_changed' }

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(`API ${status}: ${code}`)
  }
}

async function request(method: string, path: string, body?: unknown): Promise<Response> {
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    let code = 'unknown'
    try {
      code = ((await res.json()) as { error?: string }).error ?? code
    } catch {
      // Non-JSON error body; keep the generic code.
    }
    throw new ApiError(res.status, code)
  }
  return res
}

export async function getSession(): Promise<Session> {
  return (await request('GET', '/api/session')).json()
}

export async function login(username: string, password: string): Promise<LoginResult> {
  try {
    await request('POST', '/api/session', { username, password })
    return 'ok'
  } catch (err) {
    if (err instanceof ApiError && (err.code === 'invalid_credentials' || err.code === 'rate_limited')) {
      return err.code
    }
    throw err
  }
}

export async function logout(): Promise<void> {
  await request('DELETE', '/api/session')
}

export async function getStatus(): Promise<Status> {
  return (await request('GET', '/api/status')).json()
}

export async function addHost(name: string): Promise<AddedHost> {
  return (await request('POST', '/api/hosts', { name })).json()
}

/** liveURL is the WebSocket that pushes LiveMessages to the Status page. */
export function liveURL(): string {
  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${scheme}//${location.host}/api/live`
}
