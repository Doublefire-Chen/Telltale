import { type ReactNode, useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { logout } from '../api'
import { useHosts } from '../hosts'
import { Icon, type IconName } from '../Icon'
import { adminHref, adminSettingsHref, type Route, useRoute } from '../route'
import { HostsPage } from './HostsPage'
import { Confirm, Toast, type ToastMessage } from './Modal'
import { SettingsPage } from './SettingsPage'
import './admin.css'

/**
 * AdminPanel is where the admin manages Hosts and the Dashboard, laid out
 * like Nezha V0's admin pages: a fixed top menu, the page, a dark footer.
 */
export function AdminPanel({ onLoggedOut, onUnauthorized }: { onLoggedOut: () => Promise<void>; onUnauthorized: () => void }) {
  const { t } = useTranslation()
  const [route] = useRoute()
  const { hosts, dashboardVersion, failed, reload } = useHosts(onUnauthorized)
  const [toast, setToast] = useState<ToastMessage | null>(null)
  const [confirmingLogout, setConfirmingLogout] = useState(false)
  const showToast = useCallback((m: ToastMessage) => setToast(m), [])

  return (
    <AdminChrome
      dashboardVersion={dashboardVersion}
      menu={
        <>
          <MenuItem route={route} kind="admin-hosts" href={adminHref} icon="server" label={t('admin.hosts')} />
          <MenuItem route={route} kind="admin-settings" href={adminSettingsHref} icon="gear" label={t('admin.settings')} />
        </>
      }
      right={<AccountMenu onLogout={() => setConfirmingLogout(true)} />}
    >
      {failed && <div className="ui error message" role="alert">{t('app.error')}</div>}
      {!failed && !hosts && <p>{t('app.loading')}</p>}
      {hosts && route.kind === 'admin-hosts' && (
        <HostsPage hosts={hosts} dashboardVersion={dashboardVersion} reload={reload} toast={showToast} />
      )}
      {route.kind === 'admin-settings' && <SettingsPage dashboardVersion={dashboardVersion} toast={showToast} />}
      <Toast toast={toast} onDone={() => setToast(null)} />
      {confirmingLogout && (
        <Confirm
          title={t('admin.logoutTitle')}
          body={t('admin.logoutBody')}
          onCancel={() => setConfirmingLogout(false)}
          onConfirm={() => void logout().then(onLoggedOut)}
        />
      )}
    </AdminChrome>
  )
}

type AdminChromeProps = {
  dashboardVersion?: string
  menu?: ReactNode
  right?: ReactNode
  className?: string
  children: ReactNode
}

/** AdminChrome is Nezha's frame: top fixed menu, container, inverted footer. */
export function AdminChrome({ dashboardVersion, menu, right, className = '', children }: AdminChromeProps) {
  const { t } = useTranslation()
  useEffect(() => {
    document.body.classList.add('nz-body')
    return () => document.body.classList.remove('nz-body')
  }, [])
  return (
    <div className="nz">
      <nav className="ui large top fixed menu" aria-label={t('admin.panel')}>
        <div className="ui container">
          <a className="item nz-brand" href="/">
            <Icon name="speedometer2" className="logo" />
            {t('app.name')}
          </a>
          {menu}
          {right && <div className="right menu">{right}</div>}
        </div>
      </nav>
      <main className={`nb-container ${className}`}>
        <div className="ui container">{children}</div>
      </main>
      <footer className="ui inverted vertical footer segment">
        <div className="ui center aligned container">
          <b>
            © <a href="/">{t('app.name')}</a>
          </b>{' '}
          |{' '}
          <small>
            <a href="https://github.com/Doublefire-Chen/Telltale" target="_blank" rel="noreferrer">
              {t('footer.poweredBy')}
            </a>{' '}
            {dashboardVersion} ·{' '}
            <a href="https://db-ip.com" target="_blank" rel="noreferrer">
              {t('footer.geoip')}
            </a>
          </small>
        </div>
      </footer>
    </div>
  )
}

function MenuItem({ route, kind, href, icon, label }: { route: Route; kind: Route['kind']; href: string; icon: IconName; label: string }) {
  const [, navigate] = useRoute()
  const active = route.kind === kind
  return (
    <a
      className={`item${active ? ' active' : ''}`}
      href={href}
      aria-current={active ? 'page' : undefined}
      onClick={(e) => {
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return
        e.preventDefault()
        navigate(href)
      }}
    >
      <Icon name={icon} />
      {label}
    </a>
  )
}

/** AccountMenu is Nezha's avatar dropdown: back to the Status page, log out. */
function AccountMenu({ onLogout }: { onLogout: () => void }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <div className={`item ui simple dropdown${open ? ' open' : ''}`} onMouseLeave={() => setOpen(false)}>
      <button type="button" className="text" aria-haspopup="menu" aria-expanded={open} aria-label={t('admin.account')} onClick={() => setOpen(!open)}>
        <Icon name="person-circle" className="avatar" />
        admin
        <Icon name="chevron-down" className="dropdown-icon" />
      </button>
      <div className="menu" role="menu">
        <a className="item" role="menuitem" href="/">
          <Icon name="house" />
          {t('admin.backHome')}
        </a>
        <button type="button" className="item" role="menuitem" onClick={onLogout}>
          <Icon name="box-arrow-left" />
          {t('admin.logout')}
        </button>
      </div>
    </div>
  )
}
