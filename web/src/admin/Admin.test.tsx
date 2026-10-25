import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import type { Host } from '../api'
import { App } from '../App'
import { agentSHA256, stubDashboard } from '../fakeDashboard.test-util'
import i18n from '../i18n'

function host(h: Partial<Host> & Pick<Host, 'id'>): Host {
  return {
    name: '',
    host_info: null,
    report: null,
    online: false,
    agent_outdated: false,
    countries: [],
    country_override: '',
    group: '',
    hidden: false,
    public_history: false,
    ...h,
  }
}

/** row is a Host's row in the admin table. */
function row(name: string) {
  return screen.getByRole('checkbox', { name: `Select ${name}` }).closest('tr')!
}

function names() {
  return screen.getAllByRole('checkbox', { name: /^Select / }).map((c) => c.getAttribute('aria-label')!.slice('Select '.length))
}

describe('admin panel', () => {
  beforeEach(() => {
    history.replaceState(null, '', '/admin')
  })

  it('opens from the Status page and goes back', async () => {
    history.replaceState(null, '', '/')
    const user = userEvent.setup()
    stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'tokyo' })] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Admin panel' }))
    expect(location.pathname).toBe('/admin')
    expect(await screen.findByRole('link', { name: 'Hosts', current: 'page' })).toBeTruthy()
    expect(row('tokyo')).toBeTruthy()

    await user.click(screen.getByRole('link', { name: 'Settings' }))
    expect(location.pathname).toBe('/admin/settings')
    expect(await screen.findByLabelText('Make the Status page public')).toBeTruthy()
  })

  it('asks anonymous visitors to log in first', async () => {
    const user = userEvent.setup()
    stubDashboard({ hosts: [host({ id: 'h1', name: 'tokyo' })] })
    render(<App />)

    await user.type(await screen.findByLabelText('Password'), 'correct horse battery')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByRole('checkbox', { name: 'Select tokyo' })).toBeTruthy()
  })

  it('lists Hosts in the order the Status page shows them, with IPs and Agent versions', async () => {
    stubDashboard({
      authenticated: true,
      hosts: [
        host({ id: 'a', name: 'alpha', group: 'Asia', ipv4: '192.0.2.1', ipv6: '2001:db8::1', online: true, report: { received_at: 1, time: 1, interval_ms: 2000 } }),
        host({ id: 'b', name: 'bravo', group: 'Europe', agent_outdated: true, host_info: { agent_version: 'v0.0.9' } }),
        host({ id: 'c', name: 'charlie', group: 'Asia', hidden: true }),
      ],
    })
    render(<App />)

    await screen.findByRole('checkbox', { name: 'Select alpha' })
    expect(names()).toEqual(['alpha', 'charlie', 'bravo'])
    expect(row('alpha').textContent).toContain('192.0.2.1')
    expect(row('alpha').textContent).toContain('2001:db8::1')
    expect(row('alpha').textContent).toContain('Online')
    expect(row('bravo').textContent).toContain('v0.0.9')
    expect(row('bravo').textContent).toContain('Outdated')
    expect(row('charlie').textContent).toContain('Yes')
  })

  it('adds a Host and shows its token once, with an install command for this release', async () => {
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Add Host' }))
    const dialog = screen.getByRole('dialog', { name: 'Add Host' })
    await user.type(within(dialog).getByLabelText('Display name'), 'Tokyo-Vultr')
    await user.type(within(dialog).getByLabelText('Group'), 'Asia')
    await user.click(within(dialog).getByRole('button', { name: 'Add Host' }))

    const reveal = await screen.findByRole('dialog', { name: 'Host “Tokyo-Vultr” added' })
    expect(within(reveal).getByText(/^tt_A+$/)).toBeTruthy()
    expect(within(reveal).queryByText(/dashboard_url/)).toBeNull()
    const command = within(reveal).getByText(/install\.sh install/).textContent!
    expect(command).toContain('/Telltale/v1.2.0/deploy/install.sh')
    expect(command).toContain('install.sh && sh install.sh install')
    expect(command).not.toContain('sudo')
    expect(command).toContain(`--dashboard-url ${location.origin} --version v1.2.0`)
    expect(command).toContain(`--sha256 linux-amd64=${agentSHA256['linux-amd64']} --sha256 linux-arm64=${agentSHA256['linux-arm64']}`)
    expect(command).not.toContain('tt_')
    expect(dash.requests.at(-1)).toMatchObject({ method: 'PATCH', body: { group: 'Asia' } })
    expect(row('Tokyo-Vultr')).toBeTruthy()

    await user.click(within(reveal).getByRole('button', { name: 'Done' }))
    expect(screen.queryByText(/tt_A+/)).toBeNull()
  })

  it('offers an install command for macOS and Windows too, pinned to their Agents', async () => {
    const user = userEvent.setup()
    stubDashboard({ authenticated: true })
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Add Host' }))
    await user.click(within(screen.getByRole('dialog', { name: 'Add Host' })).getByRole('button', { name: 'Add Host' }))
    const reveal = await screen.findByRole('dialog', { name: /added/ })

    await user.click(within(reveal).getByRole('button', { name: 'macOS' }))
    expect(within(reveal).getByRole('button', { name: 'macOS' }).getAttribute('aria-pressed')).toBe('true')
    const mac = within(reveal).getByText(/install\.sh install/).textContent!
    expect(mac).toContain(`--sha256 darwin-amd64=${agentSHA256['darwin-amd64']} --sha256 darwin-arm64=${agentSHA256['darwin-arm64']}`)
    expect(mac).not.toContain('linux-')

    await user.click(within(reveal).getByRole('button', { name: 'Windows' }))
    const windows = within(reveal).getByText(/scriptblock/).textContent!
    expect(windows).toBe(
      "& ([scriptblock]::Create((irm https://raw.githubusercontent.com/Doublefire-Chen/Telltale/v1.2.0/deploy/install.ps1))) " +
        `-DashboardUrl ${location.origin} -Version v1.2.0 ` +
        `-Sha256 'windows-amd64=${agentSHA256['windows-amd64']}','windows-arm64=${agentSHA256['windows-arm64']}'`,
    )
    expect(windows).not.toContain('tt_')
  })

  it('adds a Host without a name', async () => {
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Add Host' }))
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Add Host' }))
    expect(await screen.findByRole('dialog', { name: 'Host “Unnamed Host” added' })).toBeTruthy()
    // Nothing to set beyond the name, so no follow-up update.
    expect(dash.requests.some((r) => r.method === 'PATCH')).toBe(false)
  })

  it('edits a Host’s name, group, country override and visibility', async () => {
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'web-1', countries: ['AU'] })] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Edit web-1' }))
    const dialog = screen.getByRole('dialog', { name: 'Edit web-1' })
    const name = within(dialog).getByLabelText('Display name')
    await user.clear(name)
    await user.type(name, 'Tokyo')
    await user.type(within(dialog).getByLabelText('Group'), 'Asia')
    const country = within(dialog).getByLabelText('Country override')
    await user.type(country, 'jp')
    expect((country as HTMLInputElement).value).toBe('JP')
    await user.click(within(dialog).getByLabelText('Hide from the public Status page'))
    const publicHistory = within(dialog).getByLabelText('Show its history on the public Status page') as HTMLInputElement
    expect(publicHistory.checked).toBe(false)
    await user.click(publicHistory)
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))

    expect(dash.requests.at(-1)).toMatchObject({
      method: 'PATCH',
      path: '/api/hosts/h1',
      body: { name: 'Tokyo', group: 'Asia', country_override: 'JP', hidden: true, public_history: true },
    })
    expect(await screen.findByRole('status')).toHaveProperty('textContent', 'Saved')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(row('Tokyo').textContent).toContain('Asia')
  })

  it('shows the Dashboard’s reason when it rejects an edit', async () => {
    const user = userEvent.setup()
    stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'web-1' })] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Edit web-1' }))
    const dialog = screen.getByRole('dialog')
    await user.type(within(dialog).getByLabelText('Country override'), '1')
    await user.click(within(dialog).getByRole('button', { name: 'Save' }))
    expect((await within(dialog).findByRole('alert')).textContent).toBe('Enter a two-letter country code, or leave it blank.')
  })

  it('reorders Hosts', async () => {
    const user = userEvent.setup()
    const dash = stubDashboard({
      authenticated: true,
      hosts: [host({ id: 'a', name: 'alpha' }), host({ id: 'b', name: 'bravo' }), host({ id: 'c', name: 'charlie' })],
    })
    render(<App />)

    await screen.findByRole('checkbox', { name: 'Select charlie' })
    expect((screen.getByRole('button', { name: 'Move charlie down' }) as HTMLButtonElement).disabled).toBe(true)
    await user.click(screen.getByRole('button', { name: 'Move charlie up' }))
    expect(dash.requests.at(-1)).toMatchObject({ method: 'PUT', body: { ids: ['a', 'c', 'b'] } })
    await waitFor(() => expect(names()).toEqual(['alpha', 'charlie', 'bravo']))
  })

  it('regenerates a token only after confirming, and shows the new one once', async () => {
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'web-1' })] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Regenerate the token of web-1' }))
    expect(screen.getByRole('dialog', { name: 'Regenerate token' }).textContent).toContain('The current token of web-1 stops working at once')
    expect(dash.requests.some((r) => r.path.endsWith('/token'))).toBe(false)
    await user.click(screen.getByRole('button', { name: 'Confirm' }))

    expect(dash.requests.at(-1)).toMatchObject({ method: 'POST', path: '/api/hosts/h1/token' })
    const reveal = await screen.findByRole('dialog', { name: 'New token for web-1' })
    expect(within(reveal).getByText(/^tt_N+$/)).toBeTruthy()
    await user.click(within(reveal).getByRole('button', { name: 'Done' }))
    expect(screen.queryByText(/tt_N+/)).toBeNull()
  })

  it('deletes a Host only after confirming', async () => {
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'old-box' }), host({ id: 'h2', name: 'keeper' })] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Delete old-box' }))
    expect(screen.getByRole('dialog', { name: 'Delete Host' }).textContent).toContain('Delete old-box and everything stored about it?')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(dash.requests.some((r) => r.method === 'DELETE')).toBe(false)

    await user.click(screen.getByRole('button', { name: 'Delete old-box' }))
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(dash.requests.at(-1)).toMatchObject({ method: 'DELETE', path: '/api/hosts/h1' })
    await waitFor(() => expect(names()).toEqual(['keeper']))
  })

  it('closes a dialog with Escape', async () => {
    const user = userEvent.setup()
    stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'web-1' })] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Delete web-1' }))
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('warns when a batch action has nothing selected', async () => {
    const user = userEvent.setup()
    stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'web-1' })] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Batch delete' }))
    expect(screen.getByRole('alert').textContent).toBe('No Host selected')
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('moves the selected Hosts to a group', async () => {
    const user = userEvent.setup()
    const dash = stubDashboard({
      authenticated: true,
      hosts: [host({ id: 'a', name: 'alpha' }), host({ id: 'b', name: 'bravo' }), host({ id: 'c', name: 'charlie' })],
    })
    render(<App />)

    await user.click(await screen.findByRole('checkbox', { name: 'Select alpha' }))
    await user.click(screen.getByRole('checkbox', { name: 'Select charlie' }))
    await user.click(screen.getByRole('button', { name: 'Batch edit group' }))
    const dialog = screen.getByRole('dialog', { name: 'Set the group of 2 Hosts' })
    await user.type(within(dialog).getByLabelText('Group'), 'Asia')
    await user.click(within(dialog).getByRole('button', { name: 'Confirm' }))

    await waitFor(() => expect(dash.requests.filter((r) => r.method === 'PATCH')).toHaveLength(2))
    expect(dash.requests.filter((r) => r.method === 'PATCH').map((r) => [r.path, r.body])).toEqual([
      ['/api/hosts/a', { group: 'Asia' }],
      ['/api/hosts/c', { group: 'Asia' }],
    ])
  })

  it('deletes every selected Host after confirming', async () => {
    const user = userEvent.setup()
    stubDashboard({
      authenticated: true,
      hosts: [host({ id: 'a', name: 'alpha' }), host({ id: 'b', name: 'bravo' }), host({ id: 'c', name: 'charlie' })],
    })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Select all' }))
    await user.click(screen.getByRole('checkbox', { name: 'Select bravo' }))
    await user.click(screen.getByRole('button', { name: 'Batch delete' }))
    expect(screen.getByRole('dialog').textContent).toContain('Delete 2 Hosts')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(names()).toEqual(['bravo']))
  })

  it('makes the Status page public from Settings', async () => {
    history.replaceState(null, '', '/admin/settings')
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true })
    render(<App />)

    const toggle = await screen.findByLabelText('Make the Status page public')
    await waitFor(() => expect((toggle as HTMLInputElement).disabled).toBe(false))
    expect(screen.getByText(/IP addresses and Hidden Hosts are never shown publicly/)).toBeTruthy()
    await user.click(toggle)
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(dash.requests.at(-1)).toMatchObject({ method: 'PUT', path: '/api/settings', body: { status_public: true } })
    expect(dash.statusPublic).toBe(true)
    expect((await screen.findByRole('status')).textContent).toBe('Saved')
  })

  it('logs out only after confirming', async () => {
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Account menu' }))
    await user.click(screen.getByRole('menuitem', { name: 'Log out' }))
    expect(dash.authenticated).toBe(true)
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() => expect(dash.authenticated).toBe(false))
    expect(location.pathname).toBe('/')
    expect(await screen.findByLabelText('Password')).toBeTruthy()
  })

  it('renders in Simplified Chinese', async () => {
    stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'tokyo' })] })
    await i18n.changeLanguage('zh-CN')
    render(<App />)
    expect(await screen.findByRole('button', { name: '批量修改分组' })).toBeTruthy()
    expect(screen.getByRole('columnheader', { name: '管理' })).toBeTruthy()
    expect(screen.getByRole('button', { name: '编辑 tokyo' })).toBeTruthy()
  })
})
