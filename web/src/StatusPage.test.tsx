import { act, render, screen, waitFor, within } from '@testing-library/react'
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
  cpu_cores: 2,
  memory_total: 2 * 1024 ** 3,
  swap_total: 1024 ** 3,
  disk_total: 50 * 1024 ** 3,
  boot_time: now / 1000 - (3 * 86400 + 4 * 3600 + 5 * 60),
  agent_version: 'v1.1.0',
}

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

function card(name: string) {
  return screen.getByRole('article', { name: new RegExp(name) })
}

/** metric is the text shown for a labelled metric on a Host's card. */
function metric(name: string, label: string) {
  const dt = within(card(name)).getAllByText(label).find((e) => e.tagName === 'DT')!
  return dt.nextElementSibling!.textContent
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
    await screen.findByRole('article', { name: /new-box/ })
    for (const label of ['CPU', 'Memory', 'Swap', 'Disk']) {
      expect(within(card('new-box')).getByRole('meter', { name: label }).getAttribute('aria-valuetext')).toBe('—')
    }
    expect(metric('new-box', 'Bandwidth')).toBe('In —Out —')
    expect(metric('new-box', 'Info')).toBe('—Memory —Disk —')
    expect(metric('new-box', 'Uptime')).toBe('—')
    // No country yet: a neutral globe instead of a flag.
    expect(within(card('new-box')).getByRole('img', { name: '—' })).toBeTruthy()
    expect(within(card('new-box')).getByRole('img', { name: 'Waiting for the first Report' })).toBeTruthy()
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
          online: true,
          ipv4: '203.0.113.7',
          ipv6: '2001:db8::7',
          countries: ['JP'],
        }),
      ],
    })
    render(<App />)
    await screen.findByRole('article', { name: /tokyo/ })

    const meter = (label: string) => within(card('tokyo')).getByRole('meter', { name: label })
    expect(meter('CPU').getAttribute('aria-valuetext')).toBe('38%')
    expect(meter('Memory').getAttribute('aria-valuenow')).toBe('75')
    expect(meter('Memory').className).toContain('warning')
    expect(meter('Swap').getAttribute('aria-valuetext')).toBe('0%')
    expect(meter('Disk').className).toContain('warning')
    expect(metric('tokyo', 'Disk')).toBe('80%')
    expect(metric('tokyo', 'Bandwidth')).toBe('In 2.0 KiB/sOut 0 B/s')
    expect(metric('tokyo', 'Traffic')).toBe('In 3.0 TiBOut 512.0 MiB')
    expect(metric('tokyo', 'Info')).toBe('2 coresMemory 2 GiBDisk 50 GiB')
    expect(metric('tokyo', 'Uptime')).toBe('3d 4h')
    expect(within(card('tokyo')).getByRole('img', { name: 'ubuntu 24.04' })).toBeTruthy()
  })

  it('shows IPs, processes and connections in the details panel', async () => {
    vi.useRealTimers()
    const user = userEvent.setup()
    stubDashboard({
      authenticated: true,
      hosts: [
        host({
          id: 'h1',
          name: 'tokyo',
          host_info: hostInfo,
          report: { ...fullReport, process_count: 187, tcp_connections: 42, udp_connections: 9 },
          ipv4: '203.0.113.7',
          ipv6: '2001:db8::7',
        }),
      ],
    })
    render(<App />)
    await user.click(await screen.findByRole('button', { name: 'Details for tokyo' }))
    expect(metric('tokyo', 'IPv4')).toBe('203.0.113.7')
    expect(metric('tokyo', 'System')).toBe('ubuntu 24.04 · x86_64 · kvm')
    expect(metric('tokyo', 'Load')).toBe('0.50 0.25 1.00')
    expect(metric('tokyo', 'IPv6')).toBe('2001:db8::7')
    expect(metric('tokyo', 'Processes')).toBe('187')
    expect(metric('tokyo', 'Connections')).toBe('TCP 42 · UDP 9')
    expect(metric('tokyo', 'Agent')).toBe('v1.1.0')
  })

  it('shows Swap as off when the Host has none, and greys bars while Offline', async () => {
    stubDashboard({
      authenticated: true,
      hosts: [host({ id: 'h1', name: 'tokyo', host_info: { ...hostInfo, swap_total: 0 }, report: fullReport })],
    })
    render(<App />)
    await screen.findByRole('article', { name: /tokyo/ })
    expect(within(card('tokyo')).getByRole('meter', { name: 'Swap' }).getAttribute('aria-valuetext')).toBe('Off')
    expect(within(card('tokyo')).getByRole('meter', { name: 'CPU' }).className).toContain('stale')
  })

  it('falls back to the hostname when the Host has no display name', async () => {
    stubDashboard({
      authenticated: true,
      hosts: [host({ id: 'h1', host_info: hostInfo }), host({ id: 'h2' })],
    })
    render(<App />)
    expect(await screen.findByRole('article', { name: /vps-tokyo/ })).toBeTruthy()
    expect(screen.getByRole('article', { name: /Unnamed Host/ })).toBeTruthy()
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
    await screen.findByRole('article', { name: /old/ })

    const badge = within(card('old')).getByText('Agent outdated')
    expect(badge.getAttribute('title')).toBe('Agent v1.1.0 is older than Dashboard v1.2.0. Upgrade it on the Host.')
    expect(within(card('current')).queryByText('Agent outdated')).toBeNull()
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
    await screen.findByRole('article', { name: /hours/ })
    expect(metric('hours', 'Uptime')).toBe('2h 7m')
    expect(metric('minutes', 'Uptime')).toBe('9m')
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
    await screen.findByRole('article', { name: /one/ })

    const jp = within(card('one')).getByRole('img', { name: 'JP' })
    expect(jp.tagName).toBe('IMG')
    expect(jp.getAttribute('src')).toMatch(/jp\.svg/)
    expect(jp.getAttribute('title')).toBe('JP')
    const flags = [...card('two').querySelectorAll('img.flag')].map((i) => i.getAttribute('alt'))
    expect(flags).toEqual(['US', 'DE'])
  })

  it('updates live from the WebSocket without reloading', async () => {
    stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'tokyo', report, online: true })] })
    render(<App />)
    await screen.findByRole('article', { name: /tokyo/ })
    expect(within(card('tokyo')).getByRole('img', { name: 'Online' })).toBeTruthy()

    const socket = FakeWebSocket.instances.at(-1)!
    expect(socket.url).toMatch(/\/api\/live$/)
    act(() => socket.emit({ type: 'report', host_id: 'h1', report: { ...report, cpu_percent: 81 } }))
    await waitFor(() =>
      expect(within(card('tokyo')).getByRole('meter', { name: 'CPU' }).getAttribute('aria-valuetext')).toBe('81%'),
    )
  })

  it('shows a Host going Offline and coming back, live', async () => {
    stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'tokyo', report, online: true })] })
    render(<App />)
    await screen.findByRole('article', { name: /tokyo/ })
    const socket = FakeWebSocket.instances.at(-1)!

    act(() => socket.emit({ type: 'offline', host_id: 'h1' }))
    expect(within(card('tokyo')).getByRole('img', { name: 'Offline' })).toBeTruthy()
    expect(card('tokyo').className).toContain('offline')
    // The last known values stay visible, dimmed.
    expect(within(card('tokyo')).getByRole('meter', { name: 'CPU' }).getAttribute('aria-valuetext')).toBe('38%')

    act(() => socket.emit({ type: 'report', host_id: 'h1', report }))
    expect(within(card('tokyo')).getByRole('img', { name: 'Online' })).toBeTruthy()
  })

  it('renders in Simplified Chinese', async () => {
    stubDashboard({ authenticated: true, hosts: [host({ id: 'h1', name: 'tokyo', host_info: hostInfo, report })] })
    await i18n.changeLanguage('zh-CN')
    render(<App />)
    await screen.findByRole('article', { name: /tokyo/ })
    expect(within(card('tokyo')).getByRole('meter', { name: '内存' })).toBeTruthy()
    expect(metric('tokyo', '运行时间')).toBe('3天4小时')
  })
})
