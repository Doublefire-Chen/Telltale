import { useState } from 'react'
import { useTranslation } from 'react-i18next'

/**
 * TokenReveal shows a Host token once, as a ready-to-save Agent config. The
 * Dashboard keeps only the token's hash, so this is the only chance to copy it.
 */
export function TokenReveal({ title, token, onDone }: { title: string; token: string; onDone: () => void }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const config = `dashboard_url = "${location.origin}"\ntoken = "${token}"\n`

  return (
    <section className="card" aria-live="polite">
      <h2>{title}</h2>
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
