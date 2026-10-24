import { type FormEvent, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getSettings, saveSettings } from '../api'
import { type Language, languages } from '../i18n'
import type { ToastMessage } from './Modal'

/**
 * SettingsPage holds settings for the whole Dashboard, after Nezha V0's
 * settings form. The language is this browser's own choice.
 */
export function SettingsPage({ dashboardVersion, toast }: { dashboardVersion: string; toast: (t: ToastMessage) => void }) {
  const { t, i18n } = useTranslation()
  const [statusPublic, setStatusPublic] = useState<boolean | null>(null)
  const [language, setLanguage] = useState(i18n.resolvedLanguage ?? 'en')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    getSettings().then(
      (s) => setStatusPublic(s.status_public),
      () => toast({ kind: 'error', text: t('app.error') }),
    )
  }, [t, toast])

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (statusPublic === null) return
    setBusy(true)
    try {
      await saveSettings({ status_public: statusPublic })
      await i18n.changeLanguage(language as Language)
      toast({ kind: 'success', text: t('settings.saved') })
    } catch {
      toast({ kind: 'error', text: t('app.error') })
    } finally {
      setBusy(false)
    }
  }

  return (
    <form className="ui large form settings-form" onSubmit={(e) => void submit(e)}>
      <h1 className="visually-hidden">{t('settings.title')}</h1>
      <div className="field">
        <label className="ui checkbox">
          <input
            type="checkbox"
            name="status_public"
            checked={statusPublic ?? false}
            disabled={statusPublic === null}
            onChange={(e) => setStatusPublic(e.target.checked)}
          />
          <span>{t('settings.public')}</span>
        </label>
        <small className="hint">{t('settings.publicHint')}</small>
      </div>
      <div className="field">
        <label htmlFor="settings-language">{t('settings.language')}</label>
        <select id="settings-language" value={language} onChange={(e) => setLanguage(e.target.value)}>
          {languages.map((l) => (
            <option key={l.code} value={l.code} lang={l.code}>
              {l.label}
            </option>
          ))}
        </select>
      </div>
      <div className="field">
        <label>{t('settings.password')}</label>
        <small className="hint">{t('settings.passwordHint')}</small>
        <code className="one-line">sudo -u telltale telltale-dashboard admin set-password</code>
      </div>
      <div className="field">
        <label>{t('settings.version')}</label>
        <span>{dashboardVersion || '—'}</span>
      </div>
      <button type="submit" className={`ui primary button${busy ? ' loading' : ''}`} disabled={busy || statusPublic === null}>
        {t('settings.save')}
      </button>
    </form>
  )
}
