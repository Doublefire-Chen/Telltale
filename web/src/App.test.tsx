import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { App } from './App'
import { languageStorageKey, toSupported } from './i18n'

type FakeDashboard = { setupRequired: boolean; password: string; loginError?: 'rate_limited' }

// stubDashboard replaces fetch with an in-memory stand-in for the Dashboard's frontend API.
function stubDashboard(d: FakeDashboard) {
  let authenticated = false
  const json = (status: number, body: unknown) =>
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      if (path === '/api/session' && method === 'GET') {
        return json(200, { setup_required: d.setupRequired, authenticated })
      }
      if (path === '/api/session' && method === 'POST') {
        if (d.loginError) return json(429, { error: d.loginError })
        const { username, password } = JSON.parse(String(init?.body))
        if (username !== 'admin' || password !== d.password) return json(401, { error: 'invalid_credentials' })
        authenticated = true
        return new Response(null, { status: 204 })
      }
      if (path === '/api/session' && method === 'DELETE') {
        authenticated = false
        return new Response(null, { status: 204 })
      }
      if (path === '/api/status') {
        return authenticated ? json(200, { hosts: [] }) : json(401, { error: 'unauthenticated' })
      }
      return json(404, { error: 'not_found' })
    }),
  )
}

describe('App', () => {
  it('shows only the set-password instructions before an admin password exists', async () => {
    stubDashboard({ setupRequired: true, password: '' })
    render(<App />)

    expect(await screen.findByRole('heading', { name: 'Set an admin password' })).toBeTruthy()
    expect(screen.getByText('sudo telltale-dashboard admin set-password')).toBeTruthy()
    expect(screen.queryByLabelText('Password')).toBeNull()
  })

  it('gates the Status page behind login and logs out again', async () => {
    const user = userEvent.setup()
    stubDashboard({ setupRequired: false, password: 'correct horse battery' })
    render(<App />)

    await user.type(await screen.findByLabelText('Password'), 'wrong')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByText('Wrong username or password.')).toBeTruthy()

    await user.clear(screen.getByLabelText('Password'))
    await user.type(screen.getByLabelText('Password'), 'correct horse battery')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByText('No Hosts yet.')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Log out' }))
    expect(await screen.findByLabelText('Password')).toBeTruthy()
  })

  it('explains rate limiting', async () => {
    const user = userEvent.setup()
    stubDashboard({ setupRequired: false, password: 'x', loginError: 'rate_limited' })
    render(<App />)

    await user.type(await screen.findByLabelText('Password'), 'anything')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByText('Too many failed attempts. Try again later.')).toBeTruthy()
  })

  it('switches to Simplified Chinese and remembers the choice', async () => {
    const user = userEvent.setup()
    stubDashboard({ setupRequired: false, password: 'x' })
    render(<App />)

    await user.selectOptions(await screen.findByLabelText('Language'), 'zh-CN')
    expect(await screen.findByRole('heading', { name: '登录' })).toBeTruthy()
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(localStorage.getItem(languageStorageKey)).toBe('zh-CN')
  })
})

describe('language detection', () => {
  it('maps any Chinese locale to Simplified Chinese and everything else to English', () => {
    expect(toSupported('zh')).toBe('zh-CN')
    expect(toSupported('zh-TW')).toBe('zh-CN')
    expect(toSupported('zh-Hans-CN')).toBe('zh-CN')
    expect(toSupported('en-US')).toBe('en')
    expect(toSupported('sv-SE')).toBe('en')
  })
})
