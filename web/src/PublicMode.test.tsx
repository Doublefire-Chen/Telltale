import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { Host } from './api'
import { App } from './App'
import { stubDashboard } from './fakeDashboard.test-util'

// The fake Dashboard returns Hosts as given; the real one strips IPs and
// Hidden Hosts for anonymous viewers (tested on the server). These tests
// cover what the web app shows and hides by itself.
const visible: Host = {
  id: 'h1',
  name: 'tokyo',
  host_info: null,
  report: { received_at: 1, time: 1, interval_ms: 2000, cpu_percent: 12 },
  online: true,
  agent_outdated: false,
  countries: ['JP'],
  country_override: '',
  group: '',
  hidden: false,
  public_history: false,
}

describe('public Status page', () => {
  it('shows Hosts read-only to anonymous visitors', async () => {
    const user = userEvent.setup()
    stubDashboard({ statusPublic: true, hosts: [visible] })
    render(<App />)

    const card = await screen.findByRole('article', { name: /tokyo/ })
    expect(within(card).getByRole('meter', { name: 'CPU' }).getAttribute('aria-valuetext')).toBe('12%')
    await user.click(within(card).getByRole('button', { name: 'Details for tokyo' }))
    expect(within(card).queryByText('IPv4')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Admin panel' })).toBeNull()
    // History is admin-only, so the name is not a link to it.
    expect(within(card).queryByRole('link', { name: 'tokyo' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Log out' })).toBeNull()
  })

  it('lets an anonymous visitor log in and go back', async () => {
    const user = userEvent.setup()
    stubDashboard({ statusPublic: true, hosts: [visible] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Log in' }))
    expect(screen.getByLabelText('Password')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Back to the Status page' }))
    expect(await screen.findByRole('article', { name: /tokyo/ })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Log in' }))
    await user.type(screen.getByLabelText('Password'), 'correct horse battery')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByRole('button', { name: 'Admin panel' })).toBeTruthy()
    expect(screen.getByRole('article', { name: /tokyo/ })).toBeTruthy()
  })

  it('shows the login form when the Status page is private', async () => {
    stubDashboard({ statusPublic: false, hosts: [visible] })
    render(<App />)
    expect(await screen.findByLabelText('Password')).toBeTruthy()
    expect(screen.queryByRole('article', { name: /tokyo/ })).toBeNull()
  })

  it('keeps anonymous visitors off a Host’s history page', async () => {
    history.replaceState(null, '', '/hosts/h1')
    const dash = stubDashboard({ statusPublic: true, hosts: [visible] })
    render(<App />)

    expect(await screen.findByRole('article', { name: /tokyo/ })).toBeTruthy()
    expect(screen.queryByRole('figure')).toBeNull()
    expect(dash.requests.some((r) => r.path.includes('/history'))).toBe(false)
  })
})
