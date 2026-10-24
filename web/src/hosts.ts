import { useCallback, useEffect, useRef, useState } from 'react'
import { ApiError, getStatus, type Host, type LiveMessage, liveURL } from './api'

const reconnectDelayMs = 2000

export function displayName(h: Host, unnamed: string): string {
  return h.name || h.host_info?.hostname || unnamed
}

/**
 * byGroup orders Hosts for display: groups in the order their first Host
 * appears, each Host keeping the admin's order within its group.
 */
export function byGroup(hosts: Host[]): { group: string; hosts: Host[] }[] {
  const groups: { group: string; hosts: Host[] }[] = []
  for (const h of hosts) {
    let g = groups.find((x) => x.group === h.group)
    if (!g) groups.push((g = { group: h.group, hosts: [] }))
    g.hosts.push(h)
  }
  return groups
}

export type HostsState = {
  hosts: Host[] | null
  dashboardVersion: string
  failed: boolean
  reload: () => Promise<void>
}

/**
 * useHosts loads every Host and keeps them current from the live WebSocket.
 * onUnauthorized is called when the Dashboard refuses access, e.g. because
 * the page just went private.
 */
export function useHosts(onUnauthorized: () => void): HostsState {
  const [hosts, setHosts] = useState<Host[] | null>(null)
  const [dashboardVersion, setDashboardVersion] = useState('')
  const [failed, setFailed] = useState(false)

  const reload = useCallback(async () => {
    try {
      const status = await getStatus()
      setHosts(status.hosts)
      setDashboardVersion(status.dashboard_version)
      setFailed(false)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        onUnauthorized()
        return
      }
      setFailed(true)
    }
  }, [onUnauthorized])

  useLiveUpdates(reload, (msg) => {
    if (msg.type === 'hosts_changed') {
      void reload()
      return
    }
    if (msg.type === 'offline') {
      setHosts((prev) => prev?.map((h) => (h.id === msg.host_id ? { ...h, online: false } : h)) ?? prev)
      return
    }
    setHosts((prev) => prev?.map((h) => (h.id === msg.host_id ? { ...h, report: msg.report, online: true } : h)) ?? prev)
  })

  return { hosts, dashboardVersion, failed, reload }
}

/**
 * useLiveUpdates loads the full state, then applies messages from the live
 * WebSocket. After a disconnect it reconnects and reloads, so nothing missed
 * while offline goes stale.
 */
function useLiveUpdates(load: () => Promise<void>, onMessage: (msg: LiveMessage) => void) {
  const handler = useRef(onMessage)
  handler.current = onMessage

  useEffect(() => {
    let socket: WebSocket | null = null
    let timer: ReturnType<typeof setTimeout> | undefined
    let stopped = false

    function connect() {
      void load()
      socket = new WebSocket(liveURL())
      socket.onmessage = (e) => handler.current(JSON.parse(String(e.data)) as LiveMessage)
      socket.onclose = () => {
        if (!stopped) timer = setTimeout(connect, reconnectDelayMs)
      }
    }
    connect()
    return () => {
      stopped = true
      clearTimeout(timer)
      socket?.close()
    }
  }, [load])
}
