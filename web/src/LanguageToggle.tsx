import { useTranslation } from 'react-i18next'
import { languages } from './i18n'

/** LanguageToggle switches the UI language; the choice is kept in this browser. */
export function LanguageToggle({ className = 'language' }: { className?: string }) {
  const { t, i18n } = useTranslation()
  return (
    <label className={className}>
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
