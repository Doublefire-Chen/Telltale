import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'
import i18n from './i18n'

// jsdom lacks matchMedia, which uPlot calls when it is imported.
window.matchMedia ??= (query: string) =>
  ({
    matches: false,
    media: query,
    addEventListener() {},
    removeEventListener() {},
    addListener() {},
    removeListener() {},
    dispatchEvent: () => false,
    onchange: null,
  }) as MediaQueryList

afterEach(async () => {
  cleanup()
  vi.unstubAllGlobals()
  await i18n.changeLanguage('en')
  localStorage.clear()
  history.replaceState(null, '', '/')
})
