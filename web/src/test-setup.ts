import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'
import i18n from './i18n'

afterEach(async () => {
  cleanup()
  vi.unstubAllGlobals()
  await i18n.changeLanguage('en')
  localStorage.clear()
})
