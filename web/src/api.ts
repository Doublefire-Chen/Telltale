// Client for the Dashboard's frontend API. Responses carry codes and raw
// values only; all wording lives in the i18n resources.

export type Session = {
  setup_required: boolean
  authenticated: boolean
}

export type LoginResult = 'ok' | 'invalid_credentials' | 'rate_limited'

export type Status = {
  hosts: unknown[]
}

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
