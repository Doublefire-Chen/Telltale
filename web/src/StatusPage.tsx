import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { type AddedHost, addHost, ApiError, getStatus, type Host, type HostInfo, type LiveMessage, liveURL } from './api'
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
        <HostTable hosts={hosts} dashboardVersion={dashboardVersion} />
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

function HostTable({ hosts, dashboardVersion }: { hosts: Host[]; dashboardVersion: string }) {
  const { t, i18n } = useTranslation()
  const lang = i18n.resolvedLanguage ?? 'en'
  const now = Date.now()
  return (
    <div className="table-scroll">
      <table className="hosts">
        <thead>
          <tr>
            <th scope="col">{t('status.name')}</th>
            <th scope="col">{t('status.system')}</th>
            <th scope="col">{t('status.uptime')}</th>
            <th scope="col">{t('status.cpu')}</th>
            <th scope="col">{t('status.load')}</th>
            <th scope="col">{t('status.memory')}</th>
            <th scope="col">{t('status.swap')}</th>
            <th scope="col">{t('status.disk')}</th>
            <th scope="col">{t('status.bandwidth')}</th>
            <th scope="col">{t('status.traffic')}</th>
          </tr>
        </thead>
        <tbody>
          {hosts.map((h) => {
            const r = h.report
            const info = h.host_info
            return (
              <tr key={h.id}>
                <th scope="row">
                  <span className="host-name">{h.name || info?.hostname || t('status.unnamed')}</span>
                  {h.agent_outdated && (
                    <span
                      className="badge"
                      title={t('status.outdatedDetail', { agent: info?.agent_version ?? unknown, dashboard: dashboardVersion })}
                    >
                      {t('status.outdated')}
                    </span>
                  )}
                </th>
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
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
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
