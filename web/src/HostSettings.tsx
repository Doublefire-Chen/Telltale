import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ApiError, deleteHost, type Host, regenerateToken, updateHost } from './api'
import { TokenReveal } from './TokenReveal'

type Props = {
  host: Host
  /** The name shown on the Status page (display name, hostname or placeholder). */
  displayName: string
  canMoveUp: boolean
  canMoveDown: boolean
  onMove: (direction: -1 | 1) => Promise<void>
  onClose: () => void
  onChanged: () => Promise<void>
}

type FieldError = 'invalid_name' | 'invalid_group' | 'invalid_country' | 'error'

/**
 * HostSettings edits what the Dashboard stores about a Host. Nothing here is
 * sent to the Host's Agent (ADR 0001).
 */
export function HostSettings({ host, displayName, canMoveUp, canMoveDown, onMove, onClose, onChanged }: Props) {
  const { t } = useTranslation()
  const [name, setName] = useState(host.name)
  const [group, setGroup] = useState(host.group)
  const [hidden, setHidden] = useState(host.hidden)
  const [country, setCountry] = useState(host.country_override)
  const [error, setError] = useState<FieldError | null>(null)
  const [busy, setBusy] = useState(false)
  const [newToken, setNewToken] = useState<string | null>(null)
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  async function run(action: () => Promise<void>) {
    setBusy(true)
    setError(null)
    try {
      await action()
    } catch (err) {
      const code = err instanceof ApiError ? err.code : ''
      setError(code === 'invalid_name' || code === 'invalid_group' || code === 'invalid_country' ? code : 'error')
    } finally {
      setBusy(false)
    }
  }

  function save(e: FormEvent) {
    e.preventDefault()
    void run(async () => {
      await updateHost(host.id, { name: name.trim(), group: group.trim(), hidden, country_override: country.trim() })
      onClose()
      await onChanged()
    })
  }

  if (newToken) {
    return (
      <TokenReveal
        title={t('hostSettings.regeneratedTitle', { name: displayName })}
        token={newToken}
        onDone={() => setNewToken(null)}
      />
    )
  }

  return (
    <form className="settings" onSubmit={save}>
      <div className="settings-fields">
        <label>
          {t('hostSettings.name')}
          <input name="name" value={name} maxLength={64} onChange={(e) => setName(e.target.value)} />
          <span className="muted hint">{t('addHost.nameHint')}</span>
        </label>
        <label>
          {t('hostSettings.group')}
          <input name="group" value={group} maxLength={32} onChange={(e) => setGroup(e.target.value)} />
          <span className="muted hint">{t('hostSettings.groupHint')}</span>
        </label>
        <label>
          {t('hostSettings.country')}
          <input
            name="country"
            className="short"
            value={country}
            maxLength={2}
            placeholder="GeoIP"
            autoCapitalize="characters"
            onChange={(e) => setCountry(e.target.value.toUpperCase())}
          />
          <span className="muted hint">{t('hostSettings.countryHint')}</span>
        </label>
        <label className="checkbox">
          <input type="checkbox" name="hidden" checked={hidden} onChange={(e) => setHidden(e.target.checked)} />
          {t('hostSettings.hidden')}
        </label>
      </div>

      {error && (
        <p role="alert" className="error">
          {error === 'invalid_name'
            ? t('addHost.invalidName')
            : error === 'invalid_group'
              ? t('hostSettings.invalidGroup')
              : error === 'invalid_country'
                ? t('hostSettings.invalidCountry')
                : t('app.error')}
        </p>
      )}

      <div className="row">
        <button type="button" className="secondary" disabled={busy || !canMoveUp} onClick={() => void run(() => onMove(-1))}>
          {t('hostSettings.moveUp')}
        </button>
        <button
          type="button"
          className="secondary"
          disabled={busy || !canMoveDown}
          onClick={() => void run(() => onMove(1))}
        >
          {t('hostSettings.moveDown')}
        </button>
        <span className="spacer" />
        <button type="button" className="secondary" onClick={onClose}>
          {t('hostSettings.close')}
        </button>
        <button type="submit" disabled={busy}>
          {t('hostSettings.save')}
        </button>
      </div>

      <div className="danger-zone">
        <div>
          <button
            type="button"
            className="secondary"
            disabled={busy}
            onClick={() => void run(async () => setNewToken(await regenerateToken(host.id)))}
          >
            {t('hostSettings.regenerate')}
          </button>
          <span className="muted hint">{t('hostSettings.regenerateHint')}</span>
        </div>
        {confirmingDelete ? (
          <div role="alertdialog" aria-label={t('hostSettings.delete')}>
            <p>{t('hostSettings.deleteConfirm', { name: displayName })}</p>
            <div className="row">
              <button type="button" className="secondary" onClick={() => setConfirmingDelete(false)}>
                {t('hostSettings.cancel')}
              </button>
              <button
                type="button"
                className="danger"
                disabled={busy}
                onClick={() =>
                  void run(async () => {
                    await deleteHost(host.id)
                    onClose()
                    await onChanged()
                  })
                }
              >
                {t('hostSettings.deleteConfirmButton')}
              </button>
            </div>
          </div>
        ) : (
          <button type="button" className="secondary danger-text" onClick={() => setConfirmingDelete(true)}>
            {t('hostSettings.delete')}
          </button>
        )}
      </div>
    </form>
  )
}
