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
  /** SHA-256 of each Agent released with this Dashboard, by "os-arch". */
  agentSHA256: Record<string, string>
  onDone: () => void
}

type InstallOS = 'linux' | 'darwin' | 'windows'

const installOSes: { os: InstallOS; icon: 'ubuntu' | 'apple' | 'windows' }[] = [
  { os: 'linux', icon: 'ubuntu' },
  { os: 'darwin', icon: 'apple' },
  { os: 'windows', icon: 'windows' },
]

/**
 * installCommand is the one-line install for os, from this Dashboard's own
 * release tag. It asks for the token, so the token stays out of the command
 * and shell history, and pins every CPU type's Agent by hash when the
 * Dashboard knows them, so the Host needs no cosign. Empty for a dev build.
 */
function installCommand(os: InstallOS, release: string, agentSHA256: Record<string, string>): string {
  if (!release) return ''
  const pins = ['amd64', 'arm64'].map((arch) => `${os}-${arch}`).filter((p) => agentSHA256[p]).map((p) => `${p}=${agentSHA256[p]}`)
  const raw = `https://raw.githubusercontent.com/Doublefire-Chen/Telltale/${release}/deploy`
  if (os === 'windows') {
    const sha = pins.length === 2 ? ` -Sha256 ${pins.map((p) => `'${p}'`).join(',')}` : ''
    return `& ([scriptblock]::Create((irm ${raw}/install.ps1))) -DashboardUrl ${location.origin} -Version ${release}${sha}`
  }
  const sha = pins.length === 2 ? pins.map((p) => ` --sha256 ${p}`).join('') : ''
  return `curl -fsSLO ${raw}/install.sh && sudo sh install.sh install --dashboard-url ${location.origin} --version ${release}${sha}`
}

/**
 * TokenModal shows a Host token once, with an install command for Linux,
 * macOS or Windows that asks for it. The Dashboard keeps only the token's hash.
 */
export function TokenModal({ title, token, dashboardVersion, agentSHA256, onDone }: TokenModalProps) {
  const { t } = useTranslation()
  const [os, setOS] = useState<InstallOS>('linux')
  const release = /^v\d+\.\d+\.\d+$/.test(dashboardVersion) ? dashboardVersion : ''
  const command = installCommand(os, release, agentSHA256)
  const pinned = command.includes('sha256') || command.includes('Sha256')

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
      {release && (
        <section className="copy-block">
          <div className="ui mini buttons install-os" role="group" aria-label={t('token.install')}>
            {installOSes.map(({ os: o, icon }) => (
              <button
                key={o}
                type="button"
                className={`ui button${o === os ? ' primary' : ''}`}
                aria-pressed={o === os}
                onClick={() => setOS(o)}
              >
                <Icon name={icon} /> {t(`token.os.${o}`)}
              </button>
            ))}
          </div>
          <p>
            {t(`token.installBody.${os}`)} {pinned ? t('token.pinned') : t('token.needsCosign')}
          </p>
          <CopyField label={t('token.install')} value={command} />
        </section>
      )}
      <section className="copy-block">
        <h3>
          <Icon name="key" /> {t('token.tokenLabel')}
        </h3>
        <CopyField label={t('token.tokenLabel')} value={token} />
        <p>
          {t('token.manual')}{' '}
          <a href="https://github.com/Doublefire-Chen/Telltale#3-the-agent" target="_blank" rel="noreferrer">
            README
          </a>
        </p>
      </section>
    </Modal>
  )
}

/** CopyField shows a value as code, with a green copy button like Nezha's. */
function CopyField({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  return (
    <div className="copy-field">
      <code className="one-line">{value}</code>
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
