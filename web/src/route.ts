import { useCallback, useEffect, useState } from 'react'

// A small router: the Status page at "/", a Host's detail page at
// "/hosts/{id}", and the admin panel under "/admin". The Dashboard serves
// index.html for all of them.

export type Route =
  | { kind: 'status' }
  | { kind: 'host'; id: string }
  | { kind: 'admin-hosts' }
  | { kind: 'admin-settings' }

export function parseRoute(pathname: string): Route {
  const m = pathname.match(/^\/hosts\/([^/]+)\/?$/)
  if (m) return { kind: 'host', id: decodeURIComponent(m[1]) }
  if (/^\/admin\/?$/.test(pathname)) return { kind: 'admin-hosts' }
  if (/^\/admin\/settings\/?$/.test(pathname)) return { kind: 'admin-settings' }
  return { kind: 'status' }
}

export function isAdminRoute(route: Route): boolean {
  return route.kind === 'admin-hosts' || route.kind === 'admin-settings'
}

export function hostHref(id: string): string {
  return `/hosts/${encodeURIComponent(id)}`
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
