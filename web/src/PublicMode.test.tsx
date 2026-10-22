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
}

describe('public Status page', () => {
  it('shows Hosts read-only to anonymous visitors', async () => {
    stubDashboard({ statusPublic: true, hosts: [visible] })
    render(<App />)

    const row = (await screen.findByRole('rowheader', { name: /tokyo/ })).closest('tr')!
    expect(within(row).getByText('12%')).toBeTruthy()
    expect(screen.queryByRole('columnheader', { name: 'IP' })).toBeNull()
    expect(screen.queryByRole('button', { name: /Settings for/ })).toBeNull()
    expect(screen.queryByRole('heading', { name: 'Add Host' })).toBeNull()
    expect(screen.queryByRole('heading', { name: 'Dashboard settings' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Log out' })).toBeNull()
  })

  it('lets an anonymous visitor log in and go back', async () => {
    const user = userEvent.setup()
    stubDashboard({ statusPublic: true, hosts: [visible] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Log in' }))
    expect(screen.getByLabelText('Password')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Back to the Status page' }))
    expect(await screen.findByRole('rowheader', { name: /tokyo/ })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Log in' }))
    await user.type(screen.getByLabelText('Password'), 'correct horse battery')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByRole('columnheader', { name: 'IP' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Settings for tokyo' })).toBeTruthy()
  })

  it('shows the login form when the Status page is private', async () => {
    stubDashboard({ statusPublic: false, hosts: [visible] })
    render(<App />)
    expect(await screen.findByLabelText('Password')).toBeTruthy()
    expect(screen.queryByRole('rowheader', { name: /tokyo/ })).toBeNull()
  })

  it('lets the admin make the Status page public', async () => {
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true, hosts: [visible] })
    render(<App />)

    const toggle = await screen.findByLabelText('Make the Status page public')
    await screen.findByText(/IP addresses and Hidden Hosts are never shown publicly/)
    await user.click(toggle)
    expect(dash.requests.at(-1)).toMatchObject({ method: 'PUT', path: '/api/settings', body: { status_public: true } })
    expect(dash.statusPublic).toBe(true)
    expect(await screen.findByText('Saved')).toBeTruthy()
  })
})
