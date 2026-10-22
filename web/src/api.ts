// Client for the Dashboard's frontend API. Responses carry codes and raw
// values only; all wording lives in the i18n resources.

export type Session = {
  setup_required: boolean
  authenticated: boolean
  /** The Status page may be shown without logging in. */
  status_public: boolean
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
  /** False once the Host missed three intervals (Offline), or before its first Report. */
  online: boolean
  agent_outdated: boolean
  /** ISO country codes to show as flags (override, else IPv4/IPv6 countries). */
  countries: string[]
  /** Admin only. */
  ipv4?: string
  ipv6?: string
  country_override: string
  group: string
  /** Left off the public Status page. */
  hidden: boolean
}

export type HostUpdate = Partial<Pick<Host, 'name' | 'group' | 'hidden' | 'country_override'>>

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
  | { type: 'offline'; host_id: string }
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

const hostPath = (id: string) => `/api/hosts/${encodeURIComponent(id)}`

export async function updateHost(id: string, update: HostUpdate): Promise<void> {
  await request('PATCH', hostPath(id), update)
}

export async function setHostOrder(ids: string[]): Promise<void> {
  await request('PUT', '/api/hosts/order', { ids })
}

/** regenerateToken revokes a Host's token and returns the new one, shown once. */
export async function regenerateToken(id: string): Promise<string> {
  return ((await (await request('POST', `${hostPath(id)}/token`)).json()) as { token: string }).token
}

export async function deleteHost(id: string): Promise<void> {
  await request('DELETE', hostPath(id))
}

export type Settings = {
  status_public: boolean
}

export async function getSettings(): Promise<Settings> {
  return (await request('GET', '/api/settings')).json()
}

export async function saveSettings(settings: Settings): Promise<void> {
  await request('PUT', '/api/settings', settings)
}

export type HistoryRange = '1h' | '24h' | '7d' | '30d'

/** History is a Host's charted metrics, column by column; times are Unix seconds. */
export type History = {
  range: HistoryRange
  t: number[]
  cpu: (number | null)[]
  memory: (number | null)[]
  swap: (number | null)[]
  disk: (number | null)[]
  rx_rate: (number | null)[]
  tx_rate: (number | null)[]
  load1: (number | null)[]
}

export async function getHistory(id: string, range: HistoryRange): Promise<History> {
  return (await request('GET', `${hostPath(id)}/history?range=${range}`)).json()
}
