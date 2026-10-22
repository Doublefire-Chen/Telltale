import { useCallback, useEffect, useState } from 'react'

// A two-route router: the Status page at "/" and a Host's detail page at
// "/hosts/{id}". The Dashboard serves index.html for both.

export type Route = { kind: 'status' } | { kind: 'host'; id: string }

export function parseRoute(pathname: string): Route {
  const m = pathname.match(/^\/hosts\/([^/]+)\/?$/)
  return m ? { kind: 'host', id: decodeURIComponent(m[1]) } : { kind: 'status' }
}

export function hostHref(id: string): string {
  return `/hosts/${encodeURIComponent(id)}`
}

/** useRoute returns the current route and a function to navigate within the app. */
export function useRoute(): [Route, (href: string) => void] {
  const [route, setRoute] = useState(() => parseRoute(location.pathname))

  useEffect(() => {
    const onPop = () => setRoute(parseRoute(location.pathname))
    addEventListener('popstate', onPop)
    return () => removeEventListener('popstate', onPop)
  }, [])

  const navigate = useCallback((href: string) => {
    history.pushState(null, '', href)
    setRoute(parseRoute(location.pathname))
  }, [])

  return [route, navigate]
}
