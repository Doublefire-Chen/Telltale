import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  type AddedHost,
  addHost,
  ApiError,
  getStatus,
  type Host,
  type HostInfo,
  type LiveMessage,
  liveURL,
  setCountryOverride,
} from './api'
import { Flags } from './Flag'
import { formatBytes, formatLoad, formatPercent, formatRate, formatUsage, unknown, uptimeParts } from './format'

const reconnectDelayMs = 2000

export function StatusPage() {
  const { t } = useTranslation()
  const [hosts, setHosts] = useState<Host[] | null>(null)
  const [dashboardVersion, setDashboardVersion] = useState('')
  const [failed, setFailed] = useState(false)

  const load = useCallback(async () => {
    try {
      const status = await getStatus()
      setHosts(status.hosts)
      setDashboardVersion(status.dashboard_version)
      setFailed(false)
    } catch {
      setFailed(true)
    }
  }, [])

  useLiveUpdates(load, (msg) => {
    if (msg.type === 'hosts_changed') {
      void load()
      return
    }
    setHosts((prev) => prev?.map((h) => (h.id === msg.host_id ? { ...h, report: msg.report } : h)) ?? prev)
  })

  if (failed) return <p role="alert">{t('app.error')}</p>
  if (!hosts) return <p>{t('app.loading')}</p>
  return (
    <section className="stack">
      <h1>{t('status.title')}</h1>
      {hosts.length === 0 ? (
        <p className="muted">{t('status.empty')}</p>
      ) : (
        <HostTable hosts={hosts} dashboardVersion={dashboardVersion} onChanged={load} />
      )}
      <AddHost onAdded={load} />
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

type HostTableProps = { hosts: Host[]; dashboardVersion: string; onChanged: () => Promise<void> }

function HostTable({ hosts, dashboardVersion, onChanged }: HostTableProps) {
  const { t } = useTranslation()
  const [openSettings, setOpenSettings] = useState<string | null>(null)
  const now = Date.now()
  return (
    <div className="table-scroll">
      <table className="hosts">
        <thead>
          <tr>
            <th scope="col">{t('status.name')}</th>
            <th scope="col">{t('status.ip')}</th>
            <th scope="col">{t('status.system')}</th>
            <th scope="col">{t('status.uptime')}</th>
            <th scope="col">{t('status.cpu')}</th>
            <th scope="col">{t('status.load')}</th>
            <th scope="col">{t('status.memory')}</th>
            <th scope="col">{t('status.swap')}</th>
            <th scope="col">{t('status.disk')}</th>
            <th scope="col">{t('status.bandwidth')}</th>
            <th scope="col">{t('status.traffic')}</th>
            <th scope="col">
              <span className="visually-hidden">{t('hostSettings.save')}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {hosts.map((h) => (
            <HostRow
              key={h.id}
              host={h}
              dashboardVersion={dashboardVersion}
              now={now}
              open={openSettings === h.id}
              onToggle={() => setOpenSettings(openSettings === h.id ? null : h.id)}
              onChanged={onChanged}
            />
          ))}
        </tbody>
      </table>
    </div>
  )
}

type HostRowProps = {
  host: Host
  dashboardVersion: string
  now: number
  open: boolean
  onToggle: () => void
  onChanged: () => Promise<void>
}

const columnCount = 13

function HostRow({ host: h, dashboardVersion, now, open, onToggle, onChanged }: HostRowProps) {
  const { t, i18n } = useTranslation()
  const lang = i18n.resolvedLanguage ?? 'en'
  const r = h.report
  const info = h.host_info
  const name = h.name || info?.hostname || t('status.unnamed')
  return (
    <>
      <tr>
        <th scope="row">
          <Flags countries={h.countries} />
          <span className="host-name">{name}</span>
          {h.agent_outdated && (
            <span
              className="badge"
              title={t('status.outdatedDetail', { agent: info?.agent_version ?? unknown, dashboard: dashboardVersion })}
            >
              {t('status.outdated')}
            </span>
          )}
        </th>
        <td>
          <span className="in-out">
            <span>{h.ipv4 || unknown}</span>
            <span>{h.ipv6 || unknown}</span>
          </span>
        </td>
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
      </tr>
      {open && (
        <tr className="settings-row">
          <td colSpan={columnCount}>
            <HostSettings host={h} onClose={onToggle} onSaved={onChanged} />
          </td>
        </tr>
      )}
    </>
  )
}

/** HostSettings edits what the Dashboard stores about a Host; nothing is sent to its Agent. */
type HostSettingsProps = { host: Host; onClose: () => void; onSaved: () => Promise<void> }

function HostSettings({ host, onClose, onSaved }: HostSettingsProps) {
  const { t } = useTranslation()
  const [country, setCountry] = useState(host.country_override)
  const [error, setError] = useState<'invalid_country' | 'error' | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await setCountryOverride(host.id, country.trim())
      onClose()
      await onSaved()
    } catch (err) {
      setError(err instanceof ApiError && err.code === 'invalid_country' ? 'invalid_country' : 'error')
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="settings" onSubmit={(e) => void submit(e)}>
      <label>
        {t('hostSettings.country')}
        <input
          name="country"
          value={country}
          maxLength={2}
          placeholder="GeoIP"
          autoCapitalize="characters"
          onChange={(e) => setCountry(e.target.value.toUpperCase())}
        />
        <span className="muted hint">{t('hostSettings.countryHint')}</span>
      </label>
      {error && (
        <p role="alert" className="error">
          {error === 'invalid_country' ? t('hostSettings.invalidCountry') : t('app.error')}
        </p>
      )}
      <div className="row">
        <button type="button" className="secondary" onClick={onClose}>
          {t('hostSettings.close')}
        </button>
        <button type="submit" disabled={busy}>
          {t('hostSettings.save')}
        </button>
      </div>
    </form>
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

  if (added) return <HostAdded host={added} onDone={() => setAdded(null)} />
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

/** HostAdded shows the new Host's token once, as a ready-to-save Agent config. */
function HostAdded({ host, onDone }: { host: AddedHost; onDone: () => void }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const config = `dashboard_url = "${location.origin}"\ntoken = "${host.token}"\n`

  return (
    <section className="card" aria-live="polite">
      <h2>{t('addHost.addedTitle', { name: host.name || t('status.unnamed') })}</h2>
      <p>{t('addHost.addedBody')}</p>
      <pre>
        <code>{config}</code>
      </pre>
      <div className="row">
        <button
          type="button"
          className="secondary"
          onClick={() => void navigator.clipboard?.writeText(config).then(() => setCopied(true))}
        >
          {copied ? t('addHost.copied') : t('addHost.copy')}
        </button>
        <button type="button" onClick={onDone}>
          {t('addHost.done')}
        </button>
      </div>
    </section>
  )
}
