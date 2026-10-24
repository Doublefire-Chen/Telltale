import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Host } from '../api'
import { App } from '../App'
import { stubDashboard } from '../fakeDashboard.test-util'

// uPlot needs a real canvas; record what it is asked to draw instead.
const plots = vi.hoisted(() => [] as { series: { label?: string }[]; data: unknown[][]; destroyed: boolean }[])
vi.mock('uplot', () => ({
  default: class {
    p: (typeof plots)[number]
    constructor(opts: { series: { label?: string }[] }, data: unknown[][]) {
      this.p = { series: opts.series, data, destroyed: false }
      plots.push(this.p)
    }
    setData(data: unknown[][]) {
      this.p.data = data
    }
    setSize() {}
    destroy() {
      this.p.destroyed = true
    }
  },
}))

const host: Host = {
  id: 'h1',
  name: 'tokyo',
  host_info: { memory_total: 4096, disk_total: 8192 },
  report: null,
  online: true,
  agent_outdated: false,
  countries: ['JP'],
  country_override: '',
  group: '',
  hidden: false,
  public_history: false,
}

const live = () => plots.filter((p) => !p.destroyed)

describe('history', () => {
  afterEach(() => {
    plots.length = 0
  })

  it('opens from the admin table and charts the last hour', async () => {
    history.replaceState(null, '', '/admin')
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true, hosts: [host] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'History of tokyo' }))
    expect(location.pathname).toBe('/admin/hosts/h1')
    expect(await screen.findByRole('heading', { name: /History of tokyo/ })).toBeTruthy()
    expect(screen.getByRole('button', { name: '1h' }).getAttribute('aria-pressed')).toBe('true')
    expect(dash.requests.at(-1)?.path).toBe('/api/hosts/h1/history?range=1h')

    await screen.findByText('Bandwidth')
    // The Host has no swap total, so Swap is left out.
    expect(screen.getAllByRole('figure').map((f) => f.querySelector('figcaption')?.textContent)).toEqual([
      'CPU',
      'Memory',
      'Disk',
      'Bandwidth',
      'Load',
    ])
    expect(live()).toHaveLength(5)
    const bandwidth = live()[3]
    expect(bandwidth.series.map((s) => s.label)).toEqual([undefined, 'In', 'Out'])
    expect(bandwidth.data[1]).toEqual([10, 10, 10])
  })

  it('charts Swap when the Host has swap', async () => {
    history.replaceState(null, '', '/admin/hosts/h1')
    stubDashboard({ authenticated: true, hosts: [{ ...host, host_info: { ...host.host_info, swap_total: 1024 } }] })
    render(<App />)
    await screen.findByText('Bandwidth')
    expect(screen.getAllByRole('figure').map((f) => f.querySelector('figcaption')?.textContent)).toContain('Swap')
  })

  it('switches between 1h, 24h, 7d and 30d', async () => {
    history.replaceState(null, '', '/admin/hosts/h1')
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true, hosts: [host] })
    render(<App />)
    await screen.findByText('Bandwidth')

    for (const range of ['24h', '7d', '30d']) {
      await user.click(screen.getByRole('button', { name: range }))
      expect(screen.getByRole('button', { name: range }).getAttribute('aria-pressed')).toBe('true')
      await screen.findByText('Bandwidth')
      expect(dash.requests.at(-1)?.path).toBe(`/api/hosts/h1/history?range=${range}`)
    }
  })

  it('says so when a range has no history', async () => {
    history.replaceState(null, '', '/admin/hosts/h1')
    stubDashboard({ authenticated: true, hosts: [host], historyPoints: 0 })
    render(<App />)
    expect(await screen.findByText('No history for this range yet.')).toBeTruthy()
    expect(live()).toHaveLength(0)
  })

  it('copes with an empty range whose metrics come back as null', async () => {
    history.replaceState(null, '', '/admin/hosts/h1')
    stubDashboard({ authenticated: true, hosts: [host], historyPoints: 0, historyNulls: true })
    render(<App />)
    expect(await screen.findByText('No history for this range yet.')).toBeTruthy()
    expect(screen.getByRole('button', { name: '30d' })).toBeTruthy()
  })

  it('goes back to the Hosts table with the back link and browser history', async () => {
    history.replaceState(null, '', '/admin')
    const user = userEvent.setup()
    stubDashboard({ authenticated: true, hosts: [host] })
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'History of tokyo' }))
    await user.click(await screen.findByRole('link', { name: 'All Hosts' }))
    expect(location.pathname).toBe('/admin')
    expect(await screen.findByRole('checkbox', { name: 'Select tokyo' })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'History of tokyo' }))
    await screen.findByRole('link', { name: 'All Hosts' })
    history.back()
    expect(await screen.findByRole('checkbox', { name: 'Select tokyo' })).toBeTruthy()
  })

  it('opens from the public Status page for a Host whose history is public', async () => {
    history.replaceState(null, '', '/')
    const user = userEvent.setup()
    const dash = stubDashboard({
      statusPublic: true,
      hosts: [{ ...host, public_history: true }, { ...host, id: 'h2', name: 'osaka' }],
    })
    render(<App />)

    const osaka = await screen.findByRole('article', { name: /osaka/ })
    expect(within(osaka).queryByRole('link', { name: 'osaka' })).toBeNull()
    await user.click(within(screen.getByRole('article', { name: /tokyo/ })).getByRole('link', { name: 'tokyo' }))
    expect(location.pathname).toBe('/hosts/h1')
    expect(await screen.findByRole('heading', { name: /History of tokyo/ })).toBeTruthy()
    await screen.findByText('Bandwidth')
    expect(dash.requests.at(-1)?.path).toBe('/api/hosts/h1/history?range=1h')

    await user.click(screen.getByRole('link', { name: 'All Hosts' }))
    expect(location.pathname).toBe('/')
    expect(await screen.findByRole('article', { name: /osaka/ })).toBeTruthy()
  })

  it('keeps visitors off the history of a Host that is not public', async () => {
    history.replaceState(null, '', '/hosts/h1')
    const dash = stubDashboard({ statusPublic: true, hosts: [host] })
    render(<App />)
    expect(await screen.findByRole('article', { name: /tokyo/ })).toBeTruthy()
    expect(screen.queryByRole('figure')).toBeNull()
    expect(dash.requests.some((r) => r.path.includes('/history'))).toBe(false)
  })

  it('opens from the Status page for the admin, public or not', async () => {
    history.replaceState(null, '', '/')
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true, hosts: [host] })
    render(<App />)

    const card = await screen.findByRole('article', { name: /tokyo/ })
    await user.click(within(card).getByRole('link', { name: 'tokyo' }))
    expect(location.pathname).toBe('/hosts/h1')
    await screen.findByText('Bandwidth')
    expect(dash.requests.at(-1)?.path).toBe('/api/hosts/h1/history?range=1h')
    await user.click(screen.getByRole('link', { name: 'All Hosts' }))
    expect(await screen.findByRole('article', { name: /tokyo/ })).toBeTruthy()
  })
})
