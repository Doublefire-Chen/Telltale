import { type FormEvent, Fragment, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  type AddedHost,
  addHost,
  ApiError,
  getSettings,
  getStatus,
  type Host,
  type HostInfo,
  type LiveMessage,
  liveURL,
  saveSettings,
  setHostOrder,
} from './api'
import { Flags } from './Flag'
import { formatBytes, formatLoad, formatPercent, formatRate, formatUsage, unknown, uptimeParts } from './format'
import { HostDetail } from './HostDetail'
import { HostSettings } from './HostSettings'
import { hostHref, useRoute } from './route'
import { TokenReveal } from './TokenReveal'

const reconnectDelayMs = 2000

type StatusPageProps = {
  /** The admin sees IP addresses, Hidden Hosts and management controls. */
  admin: boolean
  /** Called when the Dashboard refuses access, e.g. the page just went private. */
  onUnauthorized: () => void
}

export function StatusPage({ admin, onUnauthorized }: StatusPageProps) {
  const { t } = useTranslation()
  const [route, navigate] = useRoute()
  const [hosts, setHosts] = useState<Host[] | null>(null)
  const [dashboardVersion, setDashboardVersion] = useState('')
  const [failed, setFailed] = useState(false)

  const load = useCallback(async () => {
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

  useLiveUpdates(load, (msg) => {
    if (msg.type === 'hosts_changed') {
      void load()
      return
    }
    if (msg.type === 'offline') {
      setHosts((prev) => prev?.map((h) => (h.id === msg.host_id ? { ...h, online: false } : h)) ?? prev)
      return
    }
    setHosts((prev) => prev?.map((h) => (h.id === msg.host_id ? { ...h, report: msg.report, online: true } : h)) ?? prev)
  })

  if (failed) return <p role="alert">{t('app.error')}</p>
  if (!hosts) return <p>{t('app.loading')}</p>
  if (route.kind === 'host') {
    const host = hosts.find((h) => h.id === route.id)
    if (host) {
      return <HostDetail host={host} name={displayName(host, t('status.unnamed'))} onBack={() => navigate('/')} />
    }
  }
  return (
    <section className="stack">
      <h1>{t('status.title')}</h1>
      {hosts.length === 0 ? (
        <p className="muted">{t('status.empty')}</p>
      ) : (
        <HostTable
          hosts={hosts}
          dashboardVersion={dashboardVersion}
          onChanged={load}
          admin={admin}
          onOpen={(id) => navigate(hostHref(id))}
        />
      )}
      {admin && <AddHost onAdded={load} />}
      {admin && <DashboardSettings />}
    </section>
  )
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

type HostTableProps = {
  hosts: Host[]
  dashboardVersion: string
  onChanged: () => Promise<void>
  admin: boolean
  onOpen: (id: string) => void
}

/**
 * byGroup orders Hosts for display: groups in the order their first Host
 * appears, each Host keeping the admin's order within its group.
 */
function byGroup(hosts: Host[]): { group: string; hosts: Host[] }[] {
  const groups: { group: string; hosts: Host[] }[] = []
  for (const h of hosts) {
    let g = groups.find((x) => x.group === h.group)
    if (!g) groups.push((g = { group: h.group, hosts: [] }))
    g.hosts.push(h)
  }
  return groups
}

function HostTable({ hosts, dashboardVersion, onChanged, admin, onOpen }: HostTableProps) {
  const { t } = useTranslation()
  const [openSettings, setOpenSettings] = useState<string | null>(null)
  const now = Date.now()
  const groups = byGroup(hosts)
  // Moving swaps a Host with its neighbour as displayed, then saves that order.
  const displayed = groups.flatMap((g) => g.hosts)

  async function move(id: string, direction: -1 | 1) {
    const ids = displayed.map((h) => h.id)
    const i = ids.indexOf(id)
    const j = i + direction
    if (i < 0 || j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
    await setHostOrder(ids)
    await onChanged()
  }

  const editing = displayed.find((h) => h.id === openSettings)
  const editingIndex = editing ? displayed.indexOf(editing) : -1

  return (
    <>
      <div className="table-scroll">
        <table className="hosts">
          <thead>
            <tr>
              <th scope="col">{t('status.name')}</th>
              {admin && <th scope="col">{t('status.ip')}</th>}
              <th scope="col">{t('status.system')}</th>
              <th scope="col">{t('status.uptime')}</th>
              <th scope="col">{t('status.cpu')}</th>
              <th scope="col">{t('status.load')}</th>
              <th scope="col">{t('status.memory')}</th>
              <th scope="col">{t('status.swap')}</th>
              <th scope="col">{t('status.disk')}</th>
              <th scope="col">{t('status.bandwidth')}</th>
              <th scope="col">{t('status.traffic')}</th>
              {admin && (
                <th scope="col">
                  <span className="visually-hidden">{t('hostSettings.save')}</span>
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {groups.map((g) => (
              <Fragment key={g.group}>
                {g.group && (
                  <tr className="group-row">
                    <th scope="colgroup" colSpan={admin ? adminColumns : publicColumns}>
                      {g.group}
                    </th>
                  </tr>
                )}
                {g.hosts.map((h) => (
                  <HostRow
                    key={h.id}
                    host={h}
                    dashboardVersion={dashboardVersion}
                    now={now}
                    admin={admin}
                    onOpen={() => onOpen(h.id)}
                    open={openSettings === h.id}
                    onToggle={() => setOpenSettings(openSettings === h.id ? null : h.id)}
                  />
                ))}
              </Fragment>
            ))}
          </tbody>
        </table>
      </div>
      {editing && (
        <HostSettings
          key={editing.id}
          host={editing}
          displayName={displayName(editing, t('status.unnamed'))}
          canMoveUp={editingIndex > 0}
          canMoveDown={editingIndex < displayed.length - 1}
          onMove={(direction) => move(editing.id, direction)}
          onClose={() => setOpenSettings(null)}
          onChanged={onChanged}
        />
      )}
    </>
  )
}

type HostRowProps = {
  host: Host
  dashboardVersion: string
  now: number
  admin: boolean
  onOpen: () => void
  open: boolean
  onToggle: () => void
}

const adminColumns = 12
const publicColumns = 10

/** displayName is what the Status page calls a Host: its name, else its hostname. */
function displayName(h: Host, unnamed: string): string {
  return h.name || h.host_info?.hostname || unnamed
}

function HostRow({ host: h, dashboardVersion, now, admin, onOpen, open, onToggle }: HostRowProps) {
  const { t, i18n } = useTranslation()
  const lang = i18n.resolvedLanguage ?? 'en'
  const r = h.report
  const info = h.host_info
  const name = displayName(h, t('status.unnamed'))
  const state = h.online ? 'online' : h.report ? 'offline' : 'waiting'
  return (
    <>
      <tr className={state === 'offline' ? 'offline' : undefined}>
        <th scope="row">
          <span className={`dot ${state}`} role="img" aria-label={t(`status.${state}`)} title={t(`status.${state}`)} />
          <Flags countries={h.countries} />
          <a
            className="host-name"
            href={hostHref(h.id)}
            onClick={(e) => {
              if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return
              e.preventDefault()
              onOpen()
            }}
          >
            {name}
          </a>
          {h.hidden && <span className="tag">{t('status.hiddenTag')}</span>}
          {h.agent_outdated && (
            <span
              className="badge"
              title={t('status.outdatedDetail', { agent: info?.agent_version ?? unknown, dashboard: dashboardVersion })}
            >
              {t('status.outdated')}
            </span>
          )}
        </th>
        {admin && (
          <td>
            <span className="in-out">
              <span>{h.ipv4 || unknown}</span>
              <span>{h.ipv6 || unknown}</span>
            </span>
          </td>
        )}
        <td>{systemSummary(info)}</td>
        <td>
          <Uptime bootTime={info?.boot_time} now={now} />
        </td>
        <td>{formatPercent(r?.cpu_percent, lang)}</td>
        <td>{formatLoad([r?.load1, r?.load5, r?.load15], lang)}</td>
        <td>{formatUsage(r?.memory_used, info?.memory_total, lang)}</td>
        <td>{formatUsage(r?.swap_used, info?.swap_total, lang)}</td>
        <td>{formatUsage(r?.disk_used, info?.disk_total, lang)}</td>
        <td>
          <InOut inValue={formatRate(r?.rx_rate, lang)} outValue={formatRate(r?.tx_rate, lang)} />
        </td>
        <td>
          <InOut inValue={formatBytes(r?.net_rx_bytes, lang)} outValue={formatBytes(r?.net_tx_bytes, lang)} />
        </td>
        {admin && (
          <td>
            <button
              type="button"
              className="secondary icon"
              aria-expanded={open}
              aria-label={t('status.settings', { name })}
              onClick={onToggle}
            >
              ⋯
            </button>
          </td>
        )}
      </tr>
    </>
  )
}

/** systemSummary is e.g. "ubuntu 24.04 · x86_64 · kvm". */
function systemSummary(info: HostInfo | null): string {
  if (!info) return unknown
  const platform = [info.platform, info.platform_version].filter(Boolean).join(' ')
  const parts = [platform, info.arch, info.virtualization].filter(Boolean)
  return parts.length ? parts.join(' · ') : unknown
}

function Uptime({ bootTime, now }: { bootTime: number | undefined; now: number }) {
  const { t } = useTranslation()
  const parts = bootTime === undefined ? undefined : uptimeParts(bootTime, now)
  if (!parts) return <>{unknown}</>
  if (parts.d > 0) return <>{t('status.uptimeDays', parts)}</>
  if (parts.h > 0) return <>{t('status.uptimeHours', parts)}</>
  return <>{t('status.uptimeMinutes', parts)}</>
}

/** InOut shows an inbound and an outbound value, labelled for screen readers. */
function InOut({ inValue, outValue }: { inValue: string; outValue: string }) {
  const { t } = useTranslation()
  return (
    <span className="in-out">
      <span>
        <span aria-hidden="true">↓ </span>
        <span className="visually-hidden">{t('status.in')} </span>
        {inValue}
      </span>
      <span>
        <span aria-hidden="true">↑ </span>
        <span className="visually-hidden">{t('status.out')} </span>
        {outValue}
      </span>
    </span>
  )
}

function AddHost({ onAdded }: { onAdded: () => Promise<void> }) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [error, setError] = useState<'invalid_name' | 'error' | null>(null)
  const [added, setAdded] = useState<AddedHost | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      setAdded(await addHost(name))
      setName('')
      await onAdded()
    } catch (err) {
      setError(err instanceof ApiError && err.code === 'invalid_name' ? 'invalid_name' : 'error')
    } finally {
      setBusy(false)
    }
  }

  if (added) {
    return (
      <TokenReveal
        title={t('addHost.addedTitle', { name: added.name || t('status.unnamed') })}
        token={added.token}
        onDone={() => setAdded(null)}
      />
    )
  }
  return (
    <form className="card" onSubmit={(e) => void submit(e)}>
      <h2>{t('addHost.title')}</h2>
      <label>
        {t('addHost.name')}
        <input name="name" value={name} maxLength={64} onChange={(e) => setName(e.target.value)} />
        <span className="muted hint">{t('addHost.nameHint')}</span>
      </label>
      {error && (
        <p role="alert" className="error">
          {error === 'invalid_name' ? t('addHost.invalidName') : t('app.error')}
        </p>
      )}
      <button type="submit" disabled={busy}>
        {t('addHost.submit')}
      </button>
    </form>
  )
}

/** DashboardSettings holds settings for the whole Dashboard, such as public mode. */
function DashboardSettings() {
  const { t } = useTranslation()
  const [statusPublic, setStatusPublic] = useState<boolean | null>(null)
  const [state, setState] = useState<'idle' | 'saved' | 'error'>('idle')

  useEffect(() => {
    getSettings().then(
      (s) => setStatusPublic(s.status_public),
      () => setState('error'),
    )
  }, [])

  async function toggle(value: boolean) {
    setStatusPublic(value)
    try {
      await saveSettings({ status_public: value })
      setState('saved')
    } catch {
      setStatusPublic(!value)
      setState('error')
    }
  }

  return (
    <section className="card">
      <h2>{t('dashboardSettings.title')}</h2>
      <label className="checkbox">
        <input
          type="checkbox"
          checked={statusPublic ?? false}
          disabled={statusPublic === null}
          onChange={(e) => void toggle(e.target.checked)}
        />
        {t('dashboardSettings.public')}
      </label>
      <span className="muted hint">{t('dashboardSettings.publicHint')}</span>
      {state === 'saved' && <span className="muted hint">{t('dashboardSettings.saved')}</span>}
      {state === 'error' && (
        <p role="alert" className="error">
          {t('app.error')}
        </p>
      )}
    </section>
  )
}
