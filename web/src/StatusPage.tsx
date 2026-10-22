import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { type AddedHost, addHost, ApiError, getStatus, type Host, type LiveMessage, liveURL } from './api'
import { formatBytes, formatPercent } from './format'

const reconnectDelayMs = 2000

export function StatusPage() {
  const { t } = useTranslation()
  const [hosts, setHosts] = useState<Host[] | null>(null)
  const [failed, setFailed] = useState(false)

  const load = useCallback(async () => {
    try {
      setHosts((await getStatus()).hosts)
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
      {hosts.length === 0 ? <p className="muted">{t('status.empty')}</p> : <HostTable hosts={hosts} />}
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

function HostTable({ hosts }: { hosts: Host[] }) {
  const { t, i18n } = useTranslation()
  const lang = i18n.resolvedLanguage ?? 'en'
  return (
    <table className="hosts">
      <thead>
        <tr>
          <th scope="col">{t('status.name')}</th>
          <th scope="col">{t('status.cpu')}</th>
          <th scope="col">{t('status.memory')}</th>
        </tr>
      </thead>
      <tbody>
        {hosts.map((h) => (
          <tr key={h.id}>
            <th scope="row">{h.name}</th>
            <td>{formatPercent(h.report?.cpu_percent, lang)}</td>
            <td>{formatBytes(h.report?.memory_used, lang)}</td>
          </tr>
        ))}
      </tbody>
    </table>
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
        <input name="name" value={name} maxLength={64} onChange={(e) => setName(e.target.value)} required />
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
      <h2>{t('addHost.addedTitle', { name: host.name })}</h2>
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
