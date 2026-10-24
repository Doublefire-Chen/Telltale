import { type MouseEvent, useCallback, useEffect, useState } from 'react'

// A small router: the Status page at "/" with a Host's public history at
// "/hosts/{id}", and the admin panel under "/admin", with any Host's history
// at "/admin/hosts/{id}". The Dashboard serves index.html for all of them.

export type Route =
  | { kind: 'status' }
  | { kind: 'host'; id: string }
  | { kind: 'admin-hosts' }
  | { kind: 'admin-host'; id: string }
  | { kind: 'admin-settings' }

export function parseRoute(pathname: string): Route {
  const m = pathname.match(/^\/admin\/hosts\/([^/]+)\/?$/)
  if (m) return { kind: 'admin-host', id: decodeURIComponent(m[1]) }
  const p = pathname.match(/^\/hosts\/([^/]+)\/?$/)
  if (p) return { kind: 'host', id: decodeURIComponent(p[1]) }
  if (/^\/admin\/?$/.test(pathname)) return { kind: 'admin-hosts' }
  if (/^\/admin\/settings\/?$/.test(pathname)) return { kind: 'admin-settings' }
  return { kind: 'status' }
}

export function isAdminRoute(route: Route): boolean {
  return route.kind.startsWith('admin-')
}

/** publicHistoryHref is a Host's history on the Status page. */
export function publicHistoryHref(id: string): string {
  return `/hosts/${encodeURIComponent(id)}`
}

/** hostHistoryHref is where the admin sees a Host's history. */
export function hostHistoryHref(id: string): string {
  return `/admin/hosts/${encodeURIComponent(id)}`
}

export const adminHref = '/admin'
export const adminSettingsHref = '/admin/settings'

// pushState fires no event, so navigate announces itself to every useRoute.
const navigateEvent = 'telltale:navigate'

/** navigate moves to href within the app. */
export function navigate(href: string) {
  history.pushState(null, '', href)
  dispatchEvent(new Event(navigateEvent))
}

/**
 * followLink handles a click on an in-app link: a plain click navigates
 * within the app, while modified clicks (new tab, etc.) are left to the browser.
 */
export function followLink(e: MouseEvent<HTMLAnchorElement>, go: (href: string) => void) {
  if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return
  e.preventDefault()
  go(e.currentTarget.getAttribute('href')!)
}

/** useRoute returns the current route and a function to navigate within the app. */
export function useRoute(): [Route, (href: string) => void] {
  const [route, setRoute] = useState(() => parseRoute(location.pathname))

  useEffect(() => {
    const update = () => setRoute(parseRoute(location.pathname))
    addEventListener('popstate', update)
    addEventListener(navigateEvent, update)
    return () => {
      removeEventListener('popstate', update)
      removeEventListener(navigateEvent, update)
    }
  }, [])

  return [route, useCallback((href: string) => navigate(href), [])]
}
