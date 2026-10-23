import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Host } from './api'
import { App } from './App'
import { stubDashboard } from './fakeDashboard.test-util'

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
}

const live = () => plots.filter((p) => !p.destroyed)

describe('Host detail page', () => {
  afterEach(() => {
    plots.length = 0
    history.replaceState(null, '', '/')
  })

  it('opens from the Status page and charts the last hour', async () => {
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true, hosts: [host] })
    render(<App />)

    await user.click(await screen.findByRole('link', { name: 'tokyo' }))
    expect(location.pathname).toBe('/hosts/h1')
    expect(await screen.findByRole('heading', { name: /tokyo/ })).toBeTruthy()
    expect(screen.getByRole('button', { name: '1h' }).getAttribute('aria-pressed')).toBe('true')
    expect(dash.requests.at(-1)?.path).toBe('/api/hosts/h1/history?range=1h')

    await screen.findByText('Bandwidth')
    expect(screen.getAllByRole('figure').map((f) => f.querySelector('figcaption')?.textContent)).toEqual([
      'CPU',
      'Memory',
      'Disk',
      'Bandwidth',
    ])
    expect(live()).toHaveLength(4)
    const bandwidth = live()[3]
    expect(bandwidth.series.map((s) => s.label)).toEqual([undefined, 'In', 'Out'])
    expect(bandwidth.data[1]).toEqual([10, 10, 10])
  })

  it('switches between 1h, 24h, 7d and 30d', async () => {
    const user = userEvent.setup()
    const dash = stubDashboard({ authenticated: true, hosts: [host] })
    history.replaceState(null, '', '/hosts/h1')
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
    stubDashboard({ authenticated: true, hosts: [host], historyPoints: 0 })
    history.replaceState(null, '', '/hosts/h1')
    render(<App />)
    expect(await screen.findByText('No history for this range yet.')).toBeTruthy()
    expect(live()).toHaveLength(0)
  })

  it('goes back to the Status page with the back button and browser history', async () => {
    const user = userEvent.setup()
    stubDashboard({ authenticated: true, hosts: [host] })
    render(<App />)

    await user.click(await screen.findByRole('link', { name: 'tokyo' }))
    await user.click(await screen.findByRole('button', { name: /All Hosts/ }))
    expect(location.pathname).toBe('/')
    expect(await screen.findByRole('article', { name: /tokyo/ })).toBeTruthy()

    await user.click(screen.getByRole('link', { name: 'tokyo' }))
    await screen.findByRole('button', { name: /All Hosts/ })
    history.back()
    expect(await screen.findByRole('article', { name: /tokyo/ })).toBeTruthy()
  })
})
