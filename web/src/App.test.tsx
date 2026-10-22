import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { App } from './App'
import { stubDashboard } from './fakeDashboard.test-util'
import { languageStorageKey, toSupported } from './i18n'

describe('App', () => {
  it('shows only the set-password instructions before an admin password exists', async () => {
    stubDashboard({ setupRequired: true })
    render(<App />)

    expect(await screen.findByRole('heading', { name: 'Set an admin password' })).toBeTruthy()
    expect(screen.getByText('sudo telltale-dashboard admin set-password')).toBeTruthy()
    expect(screen.queryByLabelText('Password')).toBeNull()
  })

  it('gates the Status page behind login and logs out again', async () => {
    const user = userEvent.setup()
    stubDashboard()
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
    stubDashboard({ loginError: 'rate_limited' })
    render(<App />)

    await user.type(await screen.findByLabelText('Password'), 'anything')
    await user.click(screen.getByRole('button', { name: 'Log in' }))
    expect(await screen.findByText('Too many failed attempts. Try again later.')).toBeTruthy()
  })

  it('switches to Simplified Chinese and remembers the choice', async () => {
    const user = userEvent.setup()
    stubDashboard()
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
