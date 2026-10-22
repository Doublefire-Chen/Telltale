import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from './App'
import { FakeWebSocket, stubDashboard } from './fakeDashboard.test-util'
import i18n from './i18n'

const report = { received_at: 1, time: 1, interval_ms: 2000, cpu_percent: 37.5, memory_used: 1.5 * 1024 ** 3 }

const fullReport = {
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

describe('Status page', () => {
  it('lists Hosts with CPU and memory, and "—" before the first Report', async () => {
    stubDashboard({
      authenticated: true,
      hosts: [
        { id: 'h1', name: 'tokyo', report },
        { id: 'h2', name: 'new-box', report: null },
      ],
    })
    render(<App />)

    const tokyo = (await screen.findByRole('rowheader', { name: 'tokyo' })).closest('tr')!
    expect(within(tokyo).getByText('37.5%')).toBeTruthy()
    expect(within(tokyo).getByText('1.5 GiB')).toBeTruthy()
    const fresh = screen.getByRole('rowheader', { name: 'new-box' }).closest('tr')!
    // CPU, load, memory, swap, disk, and in/out for Bandwidth and Traffic.
    expect(within(fresh).getAllByText(/—/)).toHaveLength(9)
  })

  it('shows load, swap, disk, Bandwidth and Traffic', async () => {
    stubDashboard({ authenticated: true, hosts: [{ id: 'h1', name: 'tokyo', report: fullReport }] })
    render(<App />)

    const row = (await screen.findByRole('rowheader', { name: 'tokyo' })).closest('tr')!
    const cells = within(row).getAllByRole('cell').map((c) => c.textContent)
    expect(cells).toEqual([
      '37.5%',
      '0.50 0.25 1.00',
      '1.5 GiB',
      '0 B',
      '40.0 GiB',
      '↓ In 2.0 KiB/s↑ Out 0 B/s',
      '↓ In 3.0 TiB↑ Out 512.0 MiB',
    ])
    expect(within(row).getAllByText('In', { exact: false })).toHaveLength(2)
  })

  it('updates live from the WebSocket without reloading', async () => {
    stubDashboard({ authenticated: true, hosts: [{ id: 'h1', name: 'tokyo', report }] })
    render(<App />)
    await screen.findByText('37.5%')

    const socket = FakeWebSocket.instances.at(-1)!
    expect(socket.url).toMatch(/\/api\/live$/)
    act(() => socket.emit({ type: 'report', host_id: 'h1', report: { ...report, cpu_percent: 81 } }))
    expect(await screen.findByText('81%')).toBeTruthy()
  })

  it('adds a Host and shows its token exactly once as an Agent config', async () => {
    const user = userEvent.setup()
    stubDashboard({ authenticated: true })
    render(<App />)

    await user.type(await screen.findByLabelText('Display name'), 'Tokyo-Vultr')
    await user.click(screen.getByRole('button', { name: 'Add' }))

    expect(await screen.findByRole('heading', { name: 'Host “Tokyo-Vultr” added' })).toBeTruthy()
    const config = screen.getByText(/token = "tt_A+"/)
    expect(config.textContent).toContain(`dashboard_url = "${location.origin}"`)
    expect(await screen.findByRole('rowheader', { name: 'Tokyo-Vultr' })).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Done' }))
    expect(screen.queryByText(/tt_A+/)).toBeNull()
  })

  it('renders in Simplified Chinese', async () => {
    stubDashboard({ authenticated: true, hosts: [{ id: 'h1', name: 'tokyo', report }] })
    await i18n.changeLanguage('zh-CN')
    render(<App />)
    expect(await screen.findByRole('columnheader', { name: '内存' })).toBeTruthy()
    expect(screen.getByRole('heading', { name: '添加主机' })).toBeTruthy()
  })
})
