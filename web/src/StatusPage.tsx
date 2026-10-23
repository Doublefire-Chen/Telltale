import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react'
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
import { OsLogo } from './OsLogo'
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
      <h1 className="visually-hidden">{t('status.title')}</h1>
      {hosts.length === 0 ? (
        <p className="muted">{t('status.empty')}</p>
      ) : (
        <HostGroups
          hosts={hosts}
          dashboardVersion={dashboardVersion}
          onChanged={load}
          admin={admin}
          onOpen={(id) => navigate(hostHref(id))}
        />
      )}
      {admin && (
        <div className="admin-panels">
          <AddHost onAdded={load} />
          <DashboardSettings />
        </div>
      )}
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

type HostGroupsProps = {
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

/** HostGroups shows a collapsible panel of Host cards per group. */
function HostGroups({ hosts, dashboardVersion, onChanged, admin, onOpen }: HostGroupsProps) {
  const { t } = useTranslation()
  const [openSettings, setOpenSettings] = useState<string | null>(null)
  const settingsRef = useRef<HTMLDivElement>(null)
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

  useEffect(() => {
    if (openSettings) settingsRef.current?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
  }, [openSettings])

  return (
    <>
      {editing && (
        <div ref={settingsRef} className="settings-anchor">
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
        </div>
      )}
      {groups.map((g) => (
        <details key={g.group} className="group" open>
          <summary>
            <h2>{g.group || t('status.defaultGroup')}</h2>
            <span className="group-count">{g.hosts.length}</span>
          </summary>
          <div className="host-cards">
            {g.hosts.map((h) => (
              <HostCard
                key={h.id}
                host={h}
                dashboardVersion={dashboardVersion}
                now={now}
                admin={admin}
                onOpen={() => onOpen(h.id)}
                settingsOpen={openSettings === h.id}
                onToggleSettings={() => setOpenSettings(openSettings === h.id ? null : h.id)}
              />
            ))}
          </div>
        </details>
      ))}
    </>
  )
}

type HostCardProps = {
  host: Host
  dashboardVersion: string
  now: number
  admin: boolean
  onOpen: () => void
  settingsOpen: boolean
  onToggleSettings: () => void
}

/** displayName is what the Status page calls a Host: its name, else its hostname. */
function displayName(h: Host, unnamed: string): string {
  return h.name || h.host_info?.hostname || unnamed
}

function HostCard({ host: h, dashboardVersion, now, admin, onOpen, settingsOpen, onToggleSettings }: HostCardProps) {
  const { t, i18n } = useTranslation()
  const lang = i18n.resolvedLanguage ?? 'en'
  const [detailsOpen, setDetailsOpen] = useState(false)
  const r = h.report
  const info = h.host_info
  const name = displayName(h, t('status.unnamed'))
  const state = h.online ? 'online' : h.report ? 'offline' : 'waiting'
  const live = state === 'online'
  const nameId = `host-${h.id}`
  const detailsId = `host-details-${h.id}`
  return (
    <article className={`host-card ${state}`} aria-labelledby={nameId}>
      <header className="host-card-header">
        <Flags countries={h.countries} />
        <OsLogo info={info} />
        <a
          id={nameId}
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
        <span className={`state ${state}`} role="img" aria-label={t(`status.${state}`)} title={t(`status.${state}`)}>
          {state !== 'online' && t(`status.${state}Short`)}
        </span>
        <span className="spacer" />
        <button
          type="button"
          className="card-icon"
          aria-expanded={detailsOpen}
          aria-controls={detailsId}
          aria-label={t('status.details', { name })}
          title={t('status.details', { name })}
          onClick={() => setDetailsOpen(!detailsOpen)}
        >
          <InfoIcon />
        </button>
        {admin && (
          <button
            type="button"
            className="card-icon"
            aria-expanded={settingsOpen}
            aria-label={t('status.settings', { name })}
            title={t('status.settings', { name })}
            onClick={onToggleSettings}
          >
            <GearIcon />
          </button>
        )}
      </header>
      {(h.hidden || h.agent_outdated) && (
        <div className="tags">
          {h.hidden && <span className="tag">{t('status.hiddenTag')}</span>}
          {h.agent_outdated && (
            <span
              className="badge"
              title={t('status.outdatedDetail', { agent: info?.agent_version ?? unknown, dashboard: dashboardVersion })}
            >
              {t('status.outdated')}
            </span>
          )}
        </div>
      )}
      {detailsOpen && (
        <dl id={detailsId} className="host-details">
          {admin && (
            <>
              <dt>IPv4</dt>
              <dd>{h.ipv4 || unknown}</dd>
              <dt>IPv6</dt>
              <dd>{h.ipv6 || unknown}</dd>
            </>
          )}
          <dt>{t('status.processes')}</dt>
          <dd>{r?.process_count ?? unknown}</dd>
          <dt>{t('status.connections')}</dt>
          <dd>
            {r?.tcp_connections === undefined && r?.udp_connections === undefined
              ? unknown
              : `TCP ${r?.tcp_connections ?? unknown} · UDP ${r?.udp_connections ?? unknown}`}
          </dd>
          <dt>{t('status.kernel')}</dt>
          <dd>{info?.kernel_version || unknown}</dd>
          <dt>{t('status.cpuModel')}</dt>
          <dd>{info?.cpu_model || unknown}</dd>
          <dt>{t('status.bootTime')}</dt>
          <dd>
            {info?.boot_time === undefined
              ? unknown
              : new Date(info.boot_time * 1000).toLocaleString(lang, { dateStyle: 'medium', timeStyle: 'short' })}
          </dd>
          <dt>{t('status.agentVersion')}</dt>
          <dd>{info?.agent_version || unknown}</dd>
        </dl>
      )}
      <dl className="metrics">
        <dt>{t('status.cpu')}</dt>
        <dd>
          <UsageBar label={t('status.cpu')} percent={r?.cpu_percent} live={live} />
          <span className="usage">{info?.cpu_cores ? t('status.cores', { count: info.cpu_cores }) : ''}</span>
        </dd>
        <dt>{t('status.memory')}</dt>
        <dd>
          <UsageBar label={t('status.memory')} percent={ratio(r?.memory_used, info?.memory_total)} live={live} />
          <span className="usage">{formatUsage(r?.memory_used, info?.memory_total, lang)}</span>
        </dd>
        <dt>{t('status.swap')}</dt>
        <dd>
          {info?.swap_total === 0 ? (
            <>
              <UsageBar label={t('status.swap')} percent={undefined} live={live} text={t('status.off')} />
              <span className="usage" />
            </>
          ) : (
            <>
              <UsageBar label={t('status.swap')} percent={ratio(r?.swap_used, info?.swap_total)} live={live} />
              <span className="usage">{formatUsage(r?.swap_used, info?.swap_total, lang)}</span>
            </>
          )}
        </dd>
        <dt>{t('status.disk')}</dt>
        <dd>
          <UsageBar label={t('status.disk')} percent={ratio(r?.disk_used, info?.disk_total)} live={live} />
          <span className="usage">{formatUsage(r?.disk_used, info?.disk_total, lang)}</span>
        </dd>
        <dt>{t('status.bandwidth')}</dt>
        <dd>
          <InOut inValue={formatRate(r?.rx_rate, lang)} outValue={formatRate(r?.tx_rate, lang)} />
        </dd>
        <dt>{t('status.traffic')}</dt>
        <dd>
          <InOut inValue={formatBytes(r?.net_rx_bytes, lang)} outValue={formatBytes(r?.net_tx_bytes, lang)} />
        </dd>
        <dt>{t('status.system')}</dt>
        <dd className="text">{systemSummary(info)}</dd>
        <dt>{t('status.load')}</dt>
        <dd className="text">{formatLoad([r?.load1, r?.load5, r?.load15], lang)}</dd>
        <dt>{t('status.uptime')}</dt>
        <dd className="text">
          <Uptime bootTime={info?.boot_time} now={now} />
        </dd>
      </dl>
    </article>
  )
}

/** ratio is used as a percentage of total, or undefined while either is unknown. */
function ratio(used: number | undefined, total: number | undefined): number | undefined {
  if (used === undefined || !total) return undefined
  return (used / total) * 100
}

/** level colours a usage bar: blue up to half, orange up to 80%, red above. */
function level(percent: number): 'fine' | 'warning' | 'critical' {
  if (percent < 51) return 'fine'
  if (percent < 81) return 'warning'
  return 'critical'
}

/** UsageBar is a meter that turns orange and red as it fills, and grey while Offline. */
function UsageBar({ label, percent, live, text }: { label: string; percent: number | undefined; live: boolean; text?: string }) {
  const { i18n } = useTranslation()
  const lang = i18n.resolvedLanguage ?? 'en'
  const clamped = percent === undefined ? undefined : Math.min(100, Math.max(0, percent))
  const shown = text ?? (clamped === undefined ? unknown : formatPercent(Math.round(clamped), lang))
  return (
    <div
      className={`bar ${clamped === undefined ? 'empty' : live ? level(clamped) : 'stale'}${clamped !== undefined && clamped >= 85 ? ' full' : ''}`}
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={clamped}
      aria-valuetext={shown}
    >
      <div className="bar-fill" style={{ width: `${clamped ?? 0}%` }} />
      <span className="bar-text">{shown}</span>
    </div>
  )
}

function InfoIcon() {
  return (
    <svg viewBox="0 0 16 16" width={16} height={16} aria-hidden="true">
      <circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" strokeWidth="1.4" />
      <path d="M8 7v4.5M8 4.6v.1" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
    </svg>
  )
}

function GearIcon() {
  return (
    <svg viewBox="0 0 16 16" width={16} height={16} aria-hidden="true">
      <circle cx="8" cy="8" r="2.2" fill="none" stroke="currentColor" strokeWidth="1.4" />
      <path
        d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.4 1.4M11.2 11.2l1.4 1.4M3.4 12.6l1.4-1.4M11.2 4.8l1.4-1.4"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinecap="round"
      />
    </svg>
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
        <span aria-hidden="true" className="arrow">↓</span>
        <span className="visually-hidden">{t('status.in')} </span>
        {inValue}
      </span>
      <span>
        <span aria-hidden="true" className="arrow">↑</span>
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
