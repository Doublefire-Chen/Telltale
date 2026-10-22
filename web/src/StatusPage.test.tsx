import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from './App'
import { FakeWebSocket, stubDashboard } from './fakeDashboard.test-util'
import i18n from './i18n'

const report = { received_at: 1, time: 1, interval_ms: 2000, cpu_percent: 37.5, memory_used: 1.5 * 1024 ** 3 }

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
    expect(within(fresh).getAllByText('—')).toHaveLength(2)
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
