import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getSession, getStatus, login, logout, type LoginResult, type Session, type Status } from './api'
import { languages } from './i18n'

type View = { kind: 'loading' } | { kind: 'error' } | { kind: 'ready'; session: Session }

export function App() {
  const { t } = useTranslation()
  const [view, setView] = useState<View>({ kind: 'loading' })

  const refresh = useCallback(async () => {
    try {
      setView({ kind: 'ready', session: await getSession() })
    } catch {
      setView({ kind: 'error' })
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const authenticated = view.kind === 'ready' && view.session.authenticated

  return (
    <>
      <header className="header">
        <span className="brand">{t('app.name')}</span>
        <div className="header-actions">
          <LanguageToggle />
          {authenticated && (
            <button type="button" className="secondary" onClick={() => void logout().then(refresh)}>
              {t('status.logout')}
            </button>
          )}
        </div>
      </header>
      <main className="main">
        {view.kind === 'loading' && <p>{t('app.loading')}</p>}
        {view.kind === 'error' && <p role="alert">{t('app.error')}</p>}
        {view.kind === 'ready' && view.session.setup_required && <SetupRequired />}
        {view.kind === 'ready' && !view.session.setup_required && !authenticated && <Login onLoggedIn={refresh} />}
        {authenticated && <StatusPage />}
      </main>
    </>
  )
}

function LanguageToggle() {
  const { t, i18n } = useTranslation()
  return (
    <label className="language">
      <span className="visually-hidden">{t('app.language')}</span>
      <select value={i18n.resolvedLanguage} onChange={(e) => void i18n.changeLanguage(e.target.value)}>
        {languages.map((l) => (
          <option key={l.code} value={l.code} lang={l.code}>
            {l.label}
          </option>
        ))}
      </select>
    </label>
  )
}

function SetupRequired() {
  const { t } = useTranslation()
  return (
    <section className="card">
      <h1>{t('setup.title')}</h1>
      <p>{t('setup.body')}</p>
      <pre>
        <code>sudo telltale-dashboard admin set-password</code>
      </pre>
      <p>{t('setup.after')}</p>
    </section>
  )
}

function Login({ onLoggedIn }: { onLoggedIn: () => Promise<void> }) {
  const { t } = useTranslation()
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [result, setResult] = useState<Exclude<LoginResult, 'ok'> | 'error' | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    try {
      const outcome = await login(username, password)
      if (outcome === 'ok') {
        await onLoggedIn()
        return
      }
      setResult(outcome)
    } catch {
      setResult('error')
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="card" onSubmit={(e) => void submit(e)}>
      <h1>{t('login.title')}</h1>
      <label>
        {t('login.username')}
        <input name="username" autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} />
      </label>
      <label>
        {t('login.password')}
        <input
          name="password"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
        />
      </label>
      {result && (
        <p role="alert" className="error">
          {result === 'invalid_credentials'
            ? t('login.invalid')
            : result === 'rate_limited'
              ? t('login.rateLimited')
              : t('app.error')}
        </p>
      )}
      <button type="submit" disabled={busy}>
        {t('login.submit')}
      </button>
    </form>
  )
}

function StatusPage() {
  const { t } = useTranslation()
  const [status, setStatus] = useState<Status | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    getStatus().then(setStatus, () => setFailed(true))
  }, [])

  if (failed) return <p role="alert">{t('app.error')}</p>
  if (!status) return <p>{t('app.loading')}</p>
  return (
    <section>
      <h1>{t('status.title')}</h1>
      {status.hosts.length === 0 && <p className="muted">{t('status.empty')}</p>}
    </section>
  )
}
