import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { login, type LoginResult } from '../api'
import { LanguageToggle } from '../LanguageToggle'
import { AdminChrome } from './AdminPanel'

type LoginProps = {
  onLoggedIn: () => Promise<void>
  /** Set when there is a public Status page to go back to. */
  onBack?: () => void
}

/** Login is Nezha V0's centred login column, with Telltale's password form. */
export function Login({ onLoggedIn, onBack }: LoginProps) {
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
    <AdminChrome className="login" right={<LanguageToggle className="item language" />}>
      <div className="login-column">
        <h1 className="ui image header">
          <img className="logo" src="/favicon.svg" alt="" />
          <span className="content">{t('login.title')}</span>
        </h1>
        <form className={`ui large form${result ? ' error' : ''}`} onSubmit={(e) => void submit(e)}>
          <div className="ui stacked segment">
            <div className="field">
              <label htmlFor="login-username">{t('login.username')}</label>
              <input
                id="login-username"
                name="username"
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </div>
            <div className="field">
              <label htmlFor="login-password">{t('login.password')}</label>
              <input
                id="login-password"
                name="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
            </div>
            <button type="submit" className={`ui fluid large primary button${busy ? ' loading' : ''}`} disabled={busy}>
              {t('login.submit')}
            </button>
          </div>
          {result && (
            <div className="ui error message" role="alert">
              {result === 'invalid_credentials'
                ? t('login.invalid')
                : result === 'rate_limited'
                  ? t('login.rateLimited')
                  : t('app.error')}
            </div>
          )}
        </form>
        {onBack && (
          <div className="ui message">
            <button type="button" className="link" onClick={onBack}>
              {t('login.back')}
            </button>
          </div>
        )}
      </div>
    </AdminChrome>
  )
}
