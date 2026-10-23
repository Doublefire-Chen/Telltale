import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getSession, login, logout, type LoginResult, type Session } from './api'
import { languages } from './i18n'
import { Icon } from './Icon'
import { StatusPage } from './StatusPage'

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

  // On a public Status page, anonymous visitors see it read-only and can
  // choose to open the login form.
  const [wantsLogin, setWantsLogin] = useState(false)
  const session = view.kind === 'ready' ? view.session : null
  const authenticated = session?.authenticated ?? false
  const publicView = !!session && !session.setup_required && !authenticated && session.status_public && !wantsLogin
  const loginView = !!session && !session.setup_required && !authenticated && !publicView

  return (
    <>
      <header className="header">
        <span className="brand">{t('app.name')}</span>
        <div className="header-actions">
          <LanguageToggle />
          {authenticated && (
            <button type="button" className="secondary" onClick={() => void logout().then(refresh)}>
              <Icon name="box-arrow-right" />
              {t('status.logout')}
            </button>
          )}
          {publicView && (
            <button type="button" className="secondary" onClick={() => setWantsLogin(true)}>
              <Icon name="box-arrow-in-right" />
              {t('login.open')}
            </button>
          )}
          {loginView && session?.status_public && (
            <button type="button" className="secondary" onClick={() => setWantsLogin(false)}>
              {t('login.back')}
            </button>
          )}
        </div>
      </header>
      <main className="main">
        {view.kind === 'loading' && <p>{t('app.loading')}</p>}
        {view.kind === 'error' && <p role="alert">{t('app.error')}</p>}
        {view.kind === 'ready' && view.session.setup_required && <SetupRequired />}
        {loginView && (
          <Login
            onLoggedIn={async () => {
              setWantsLogin(false)
              await refresh()
            }}
          />
        )}
        {(authenticated || publicView) && <StatusPage key={String(authenticated)} admin={authenticated} onUnauthorized={refresh} />}
      </main>
      <footer className="footer">
        <a href="https://github.com/Doublefire-Chen/Telltale" target="_blank" rel="noreferrer">
          {t('footer.poweredBy')}
        </a>
        {' · '}
        <a href="https://db-ip.com" target="_blank" rel="noreferrer">
          {t('footer.geoip')}
        </a>
        {' · '}
        <a href="https://yujie.pro" target="_blank" rel="noreferrer">
          {t('footer.theme')}
        </a>
      </footer>
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
        <code>sudo -u telltale telltale-dashboard admin set-password</code>
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
