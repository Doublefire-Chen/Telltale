import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { type AddedHost, addHost, ApiError, type Host, updateHost } from '../api'
import { Icon } from '../Icon'
import { Modal } from './Modal'

type FieldError = 'invalid_name' | 'invalid_group' | 'invalid_country' | 'error'

type HostFormProps = {
  /** The Host to edit; absent to add one. */
  host?: Host
  /** The name shown for the Host being edited. */
  displayName?: string
  /** Existing groups, offered as suggestions. */
  groups: string[]
  onClose: () => void
  /** Called after saving; added is set when a Host was just added. */
  onSaved: (added?: AddedHost) => Promise<void>
}

/**
 * HostForm adds or edits a Host in a modal, like Nezha's server modal. It edits
 * only what the Dashboard stores; nothing here reaches the Agent (ADR 0001).
 */
export function HostForm({ host, displayName, groups, onClose, onSaved }: HostFormProps) {
  const { t } = useTranslation()
  const formId = useId()
  const listId = useId()
  const [name, setName] = useState(host?.name ?? '')
  const [group, setGroup] = useState(host?.group ?? '')
  const [country, setCountry] = useState(host?.country_override ?? '')
  const [hidden, setHidden] = useState(host?.hidden ?? false)
  const [publicHistory, setPublicHistory] = useState(host?.public_history ?? false)
  const [error, setError] = useState<FieldError | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    const update = { name: name.trim(), group: group.trim(), hidden, public_history: publicHistory, country_override: country.trim() }
    try {
      if (host) {
        await updateHost(host.id, update)
        await onSaved()
      } else {
        const added = await addHost(update.name)
        if (update.group || update.hidden || update.public_history || update.country_override) {
          await updateHost(added.id, {
            group: update.group,
            hidden: update.hidden,
            public_history: update.public_history,
            country_override: update.country_override,
          })
        }
        await onSaved(added)
      }
    } catch (err) {
      const code = err instanceof ApiError ? err.code : ''
      setError(code === 'invalid_name' || code === 'invalid_group' || code === 'invalid_country' ? code : 'error')
      setBusy(false)
    }
  }

  return (
    <Modal
      title={host ? t('hostForm.editTitle', { name: displayName ?? '' }) : t('hostForm.addTitle')}
      size="tiny"
      onClose={onClose}
      actions={
        <>
          <button type="button" className="ui negative button" onClick={onClose}>
            {t('admin.cancel')}
          </button>
          <button
            type="submit"
            form={formId}
            className={`ui positive primary right labeled button${busy ? ' loading' : ''}`}
            disabled={busy}
          >
            {host ? t('admin.save') : t('admin.add')}
            <Icon name={host ? 'pencil-square' : 'plus-lg'} />
          </button>
        </>
      }
    >
      <form id={formId} className={`ui form${error ? ' error' : ''}`} onSubmit={(e) => void submit(e)}>
        <div className="field">
          <label htmlFor={`${formId}-name`}>{t('hostForm.name')}</label>
          <input
            id={`${formId}-name`}
            name="name"
            value={name}
            maxLength={64}
            placeholder={host?.host_info?.hostname ?? ''}
            onChange={(e) => setName(e.target.value)}
          />
          <small className="hint">{t('hostForm.nameHint')}</small>
        </div>
        <div className="field">
          <label htmlFor={`${formId}-group`}>{t('hostForm.group')}</label>
          <input
            id={`${formId}-group`}
            name="group"
            list={listId}
            value={group}
            maxLength={32}
            onChange={(e) => setGroup(e.target.value)}
          />
          <datalist id={listId}>
            {groups.map((g) => (
              <option key={g} value={g} />
            ))}
          </datalist>
          <small className="hint">{t('hostForm.groupHint')}</small>
        </div>
        <div className="field">
          <label htmlFor={`${formId}-country`}>{t('hostForm.country')}</label>
          <input
            id={`${formId}-country`}
            name="country"
            className="short"
            value={country}
            maxLength={2}
            placeholder="GeoIP"
            autoCapitalize="characters"
            onChange={(e) => setCountry(e.target.value.toUpperCase())}
          />
          <small className="hint">{t('hostForm.countryHint')}</small>
        </div>
        <div className="field">
          <label className="ui checkbox">
            <input type="checkbox" name="hidden" checked={hidden} onChange={(e) => setHidden(e.target.checked)} />
            <span>{t('hostForm.hidden')}</span>
          </label>
        </div>
        <div className="field">
          <label className="ui checkbox">
            <input
              type="checkbox"
              name="public_history"
              checked={publicHistory}
              onChange={(e) => setPublicHistory(e.target.checked)}
            />
            <span>{t('hostForm.publicHistory')}</span>
          </label>
          <small className="hint">{t('hostForm.publicHistoryHint')}</small>
        </div>
        {error && (
          <div className="ui error message" role="alert">
            {error === 'error' ? t('app.error') : t(`hostForm.${error === 'invalid_name' ? 'invalidName' : error === 'invalid_group' ? 'invalidGroup' : 'invalidCountry'}`)}
          </div>
        )}
      </form>
    </Modal>
  )
}

type TokenModalProps = {
  title: string
  token: string
  dashboardVersion: string
  onDone: () => void
}

/**
 * TokenModal shows a Host token once, with a Linux install command and a
 * ready-to-save Agent config. The Dashboard keeps only the token's hash.
 */
export function TokenModal({ title, token, dashboardVersion, onDone }: TokenModalProps) {
  const { t } = useTranslation()
  const config = `dashboard_url = "${location.origin}"\ntoken = "${token}"\n`
  // The installer from this Dashboard's own release tag; it asks for the
  // token, so the token stays out of the command and shell history.
  const release = /^v\d+\.\d+\.\d+$/.test(dashboardVersion) ? dashboardVersion : ''
  const command = release
    ? `curl -fsSLO https://raw.githubusercontent.com/Doublefire-Chen/Telltale/${release}/deploy/install.sh && sudo sh install.sh install --dashboard-url ${location.origin} --version ${release}`
    : ''

  return (
    <Modal
      title={title}
      size="tiny"
      onClose={onDone}
      actions={
        <button type="button" className="ui positive primary right labeled button" onClick={onDone}>
          {t('token.done')}
          <Icon name="check-lg" />
        </button>
      }
    >
      <div className="ui warning message">{t('token.once')}</div>
      {command && (
        <section className="copy-block">
          <h3>
            <Icon name="ubuntu" /> {t('token.linux')}
          </h3>
          <p>{t('token.linuxBody')}</p>
          <CopyField label={t('token.linux')} value={command} />
        </section>
      )}
      <section className="copy-block">
        <h3>
          <Icon name="key" /> {t('token.tokenLabel')}
        </h3>
        <CopyField label={t('token.tokenLabel')} value={token} />
      </section>
      <section className="copy-block">
        <h3>
          <Icon name="clipboard" /> {t('token.config')}
        </h3>
        <p>{t('token.configBody')}</p>
        <CopyField label={t('token.config')} value={config} multiline />
      </section>
    </Modal>
  )
}

/** CopyField shows a value as code, with a green copy button like Nezha's. */
function CopyField({ label, value, multiline = false }: { label: string; value: string; multiline?: boolean }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  return (
    <div className="copy-field">
      {multiline ? (
        <pre>
          <code>{value}</code>
        </pre>
      ) : (
        <code className="one-line">{value}</code>
      )}
      <button
        type="button"
        className="ui green mini icon-only button"
        aria-label={`${copied ? t('token.copied') : t('token.copy')}: ${label}`}
        title={copied ? t('token.copied') : t('token.copy')}
        onClick={() => void navigator.clipboard?.writeText(value).then(() => setCopied(true))}
      >
        <Icon name={copied ? 'check-lg' : 'clipboard'} />
      </button>
    </div>
  )
}
