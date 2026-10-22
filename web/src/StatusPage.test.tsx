import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Host, HostInfo, Report } from './api'
import { App } from './App'
import { FakeWebSocket, stubDashboard } from './fakeDashboard.test-util'
import i18n from './i18n'

const now = new Date('2026-10-22T12:00:00Z').getTime()

const report: Report = { received_at: 1, time: 1, interval_ms: 2000, cpu_percent: 37.5, memory_used: 1.5 * 1024 ** 3 }

const fullReport: Report = {
  ...report,
  swap_used: 0,
  load1: 0.5,
  load5: 0.25,
  load15: 1,
  disk_used: 40 * 1024 ** 3,
  net_rx_bytes: 3 * 1024 ** 4,
  net_tx_bytes: 512 * 1024 ** 2,
  rx_rate: 2048,
  tx_rate: 0,
}

const hostInfo: HostInfo = {
  hostname: 'vps-tokyo',
  os: 'linux',
  platform: 'ubuntu',
  platform_version: '24.04',
  arch: 'x86_64',
  virtualization: 'kvm',
  memory_total: 2 * 1024 ** 3,
  swap_total: 1024 ** 3,
  disk_total: 50 * 1024 ** 3,
  boot_time: now / 1000 - (3 * 86400 + 4 * 3600 + 5 * 60),
  agent_version: 'v1.1.0',
}

function host(h: Partial<Host> & Pick<Host, 'id'>): Host {
  return { name: '', host_info: null, report: null, agent_outdated: false, countries: [], country_override: '', ...h }
}

function row(name: string) {
  return screen.getByRole('rowheader', { name: new RegExp(name) }).closest('tr')!
}

function cells(name: string) {
  return within(row(name))
    .getAllByRole('cell')
    .map((c) => c.textContent)
}

describe('Status page', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(now)
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows "—" for everything before a Host has reported', async () => {
    stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'new-box' })] })
    render(<App />)
    await screen.findByRole('rowheader', { name: /new-box/ })
    // IPv4/IPv6, System, Uptime, CPU, load, memory, swap, disk, and in/out for Bandwidth and Traffic.
    expect(within(row('new-box')).getAllByText(/^—$|—$/)).toHaveLength(13)
    // No country yet: a neutral globe instead of a flag.
    expect(within(row('new-box')).getByRole('img', { name: '—' })).toBeTruthy()
  })

  it('shows every metric, using Host Info for totals, system and Uptime', async () => {
    stubDashboard({
      authenticated: true,
      hosts: [
        host({
          id: 'h1',
          name: 'tokyo',
          host_info: hostInfo,
          report: fullReport,
          ipv4: '203.0.113.7',
          ipv6: '2001:db8::7',
          countries: ['JP'],
        }),
      ],
    })
    render(<App />)
    await screen.findByRole('rowheader', { name: /tokyo/ })

    expect(cells('tokyo').slice(0, -1)).toEqual([
      '203.0.113.72001:db8::7',
      'ubuntu 24.04 · x86_64 · kvm',
      '3d 4h',
      '37.5%',
      '0.50 0.25 1.00',
      '1.5 GiB / 2.0 GiB',
      '0 B / 1.0 GiB',
      '40.0 GiB / 50.0 GiB',
      '↓ In 2.0 KiB/s↑ Out 0 B/s',
      '↓ In 3.0 TiB↑ Out 512.0 MiB',
    ])
  })

  it('falls back to the hostname when the Host has no display name', async () => {
    stubDashboard({
      authenticated: true,
      hosts: [host({ id: 'h1', host_info: hostInfo }), host({ id: 'h2' })],
    })
    render(<App />)
    expect(await screen.findByRole('rowheader', { name: /vps-tokyo/ })).toBeTruthy()
    expect(screen.getByRole('rowheader', { name: /Unnamed Host/ })).toBeTruthy()
  })

  it('flags Hosts whose Agent is older than the Dashboard', async () => {
    stubDashboard({
      authenticated: true,
      hosts: [
        host({ id: 'h1', name: 'old', host_info: hostInfo, agent_outdated: true }),
        host({ id: 'h2', name: 'current', host_info: { ...hostInfo, agent_version: 'v1.2.0' } }),
      ],
    })
    render(<App />)
    await screen.findByRole('rowheader', { name: /old/ })

    const badge = within(row('old')).getByText('Agent outdated')
    expect(badge.getAttribute('title')).toBe('Agent v1.1.0 is older than Dashboard v1.2.0. Upgrade it on the Host.')
    expect(within(row('current')).queryByText('Agent outdated')).toBeNull()
  })

  it('formats Uptime in hours and minutes for recently booted Hosts', async () => {
    stubDashboard({
      authenticated: true,
      hosts: [
        host({ id: 'h1', name: 'hours', host_info: { boot_time: now / 1000 - (2 * 3600 + 7 * 60) } }),
        host({ id: 'h2', name: 'minutes', host_info: { boot_time: now / 1000 - 9 * 60 } }),
      ],
    })
    render(<App />)
    await screen.findByRole('rowheader', { name: /hours/ })
    expect(cells('hours')[2]).toBe('2h 7m')
    expect(cells('minutes')[2]).toBe('9m')
  })

  it('shows SVG flags: one per country, with the ISO code as alt text', async () => {
    stubDashboard({
      authenticated: true,
      hosts: [
        host({ id: 'h1', name: 'one', countries: ['JP'] }),
        host({ id: 'h2', name: 'two', countries: ['US', 'DE'] }),
      ],
    })
    render(<App />)
    await screen.findByRole('rowheader', { name: /one/ })

    const jp = within(row('one')).getByRole('img', { name: 'JP' })
    expect(jp.tagName).toBe('IMG')
    expect(jp.getAttribute('src')).toMatch(/jp\.svg/)
    expect(jp.getAttribute('title')).toBe('JP')
    expect(within(row('two')).getAllByRole('img').map((i) => i.getAttribute('alt'))).toEqual(['US', 'DE'])
  })

  it('sets and clears a country override from the Host settings', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'anycast', countries: ['AU'] })] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Settings for anycast' }))
    const input = screen.getByLabelText(/Country override/)
    await user.type(input, 'jp')
    expect((input as HTMLInputElement).value).toBe('JP')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(dash.countryRequests).toEqual([{ id: 'h1', country: 'JP' }])
    expect(await within(row('anycast')).findByRole('img', { name: 'JP' })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Settings for anycast' }))
    await user.clear(screen.getByLabelText(/Country override/))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(dash.countryRequests.at(-1)).toEqual({ id: 'h1', country: '' })
  })

  it('updates live from the WebSocket without reloading', async () => {
    stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'tokyo', report })] })
    render(<App />)
    await screen.findByText('37.5%')

    const socket = FakeWebSocket.instances.at(-1)!
    expect(socket.url).toMatch(/\/api\/live$/)
    act(() => socket.emit({ type: 'report', host_id: 'h1', report: { ...report, cpu_percent: 81 } }))
    expect(await screen.findByText('81%')).toBeTruthy()
  })

  it('adds a Host and shows its token exactly once as an Agent config', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    stubDashboard({ authenticated: true })
    render(<App />)

    await user.type(await screen.findByLabelText(/Display name/), 'Tokyo-Vultr')
    await user.click(screen.getByRole('button', { name: 'Add' }))

    expect(await screen.findByRole('heading', { name: 'Host “Tokyo-Vultr” added' })).toBeTruthy()
    const config = screen.getByText(/token = "tt_A+"/)
    expect(config.textContent).toContain(`dashboard_url = "${location.origin}"`)
    expect(await screen.findByRole('rowheader', { name: /Tokyo-Vultr/ })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Done' }))
    expect(screen.queryByText(/tt_A+/)).toBeNull()
  })

  it('adds a Host without a name', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    stubDashboard({ authenticated: true })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Add' }))
    expect(await screen.findByRole('heading', { name: 'Host “Unnamed Host” added' })).toBeTruthy()
  })

  it('renders in Simplified Chinese', async () => {
    stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'tokyo', host_info: hostInfo, report })] })
    await i18n.changeLanguage('zh-CN')
    render(<App />)
    expect(await screen.findByRole('columnheader', { name: '内存' })).toBeTruthy()
    expect(screen.getByRole('heading', { name: '添加主机' })).toBeTruthy()
    expect(cells('tokyo')[2]).toBe('3天4小时')
  })
})
