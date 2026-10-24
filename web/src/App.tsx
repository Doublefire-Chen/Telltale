import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AdminPanel } from './admin/AdminPanel'
import { Login } from './admin/Login'
import { getSession, logout, type Session } from './api'
import { Icon } from './Icon'
import { LanguageToggle } from './LanguageToggle'
import { adminHref, isAdminRoute, useRoute } from './route'
import { StatusPage } from './StatusPage'

type View = { kind: 'loading' } | { kind: 'error' } | { kind: 'ready'; session: Session }

export function App() {
  const { t } = useTranslation()
  const [route, navigate] = useRoute()
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
  const adminRoute = isAdminRoute(route)
  const publicView = !!session && !session.setup_required && !authenticated && session.status_public && !wantsLogin && !adminRoute
  const loginView = !!session && !session.setup_required && !authenticated && !publicView

  if (authenticated && adminRoute) {
    return (
      <AdminPanel
        onUnauthorized={refresh}
        onLoggedOut={async () => {
          navigate('/')
          await refresh()
        }}
      />
    )
  }
  if (loginView) {
    return (
      <Login
        onLoggedIn={async () => {
          setWantsLogin(false)
          await refresh()
        }}
        onBack={
          session?.status_public
            ? () => {
                setWantsLogin(false)
                if (adminRoute) navigate('/')
              }
            : undefined
        }
      />
    )
  }

  return (
    <>
      <header className="header">
        <span className="brand">{t('app.name')}</span>
        <div className="header-actions">
          <LanguageToggle />
          {authenticated && (
            <>
              <button type="button" className="secondary" onClick={() => navigate(adminHref)}>
                <Icon name="gear-fill" />
                {t('admin.panel')}
              </button>
              <button type="button" className="secondary" onClick={() => void logout().then(refresh)}>
                <Icon name="box-arrow-right" />
                {t('status.logout')}
              </button>
            </>
          )}
          {publicView && (
            <button type="button" className="secondary" onClick={() => setWantsLogin(true)}>
              <Icon name="box-arrow-in-right" />
              {t('login.open')}
            </button>
          )}
        </div>
      </header>
      <main className="main">
        {view.kind === 'loading' && <p>{t('app.loading')}</p>}
        {view.kind === 'error' && <p role="alert">{t('app.error')}</p>}
        {view.kind === 'ready' && view.session.setup_required && <SetupRequired />}
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
