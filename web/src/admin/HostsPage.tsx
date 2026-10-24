import { type FormEvent, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { deleteHost, type Host, regenerateToken, setHostOrder, updateHost } from '../api'
import { Flags } from '../Flag'
import { byGroup, displayName } from '../hosts'
import { Icon, type IconName } from '../Icon'
import { OsLogo } from '../OsLogo'
import { hostHistoryHref, useRoute } from '../route'
import { HostForm, TokenModal } from './HostForm'
import { Confirm, Modal, type ToastMessage } from './Modal'

type Dialog =
  | { kind: 'add' }
  | { kind: 'edit'; host: Host }
  | { kind: 'token'; title: string; token: string }
  | { kind: 'delete'; ids: string[] }
  | { kind: 'regenerate'; host: Host }
  | { kind: 'group'; ids: string[] }

type HostsPageProps = {
  hosts: Host[]
  dashboardVersion: string
  reload: () => Promise<void>
  toast: (t: ToastMessage) => void
}

/** HostsPage is the admin's table of Hosts, after Nezha V0's server page. */
export function HostsPage({ hosts, dashboardVersion, reload, toast }: HostsPageProps) {
  const { t } = useTranslation()
  const [, navigate] = useRoute()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [dialog, setDialog] = useState<Dialog | null>(null)
  const [busy, setBusy] = useState(false)
  const unnamed = t('status.unnamed')
  // The same order as the Status page shows them, so # matches what visitors see.
  const ordered = byGroup(hosts).flatMap((g) => g.hosts)
  const groups = [...new Set(hosts.map((h) => h.group).filter(Boolean))]
  const chosen = ordered.filter((h) => selected.has(h.id)).map((h) => h.id)
  const allSelected = ordered.length > 0 && chosen.length === ordered.length

  function toggle(id: string) {
    const next = new Set(selected)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setSelected(next)
  }

  function batch(kind: 'delete' | 'group') {
    if (chosen.length === 0) {
      toast({ kind: 'warning', text: t('admin.noneSelected') })
      return
    }
    setDialog({ kind, ids: chosen })
  }

  async function run(action: () => Promise<void>, success?: string) {
    setBusy(true)
    try {
      await action()
      if (success) toast({ kind: 'success', text: success })
    } catch {
      toast({ kind: 'error', text: t('app.error') })
    } finally {
      setBusy(false)
      await reload()
    }
  }

  async function move(id: string, direction: -1 | 1) {
    const ids = ordered.map((h) => h.id)
    const i = ids.indexOf(id)
    const j = i + direction
    if (i < 0 || j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
    await run(() => setHostOrder(ids))
  }

  return (
    <>
      <div className="toolbar">
        <button type="button" className="ui primary right labeled button" onClick={() => batch('group')}>
          {t('admin.batchGroup')}
          <Icon name="collection" />
        </button>
        <button type="button" className="ui primary right labeled button" onClick={() => batch('delete')}>
          {t('admin.batchDelete')}
          <Icon name="trash3" />
        </button>
        <button type="button" className="ui primary right labeled button" onClick={() => setDialog({ kind: 'add' })}>
          {t('admin.add')}
          <Icon name="plus-lg" />
        </button>
      </div>

      <div className="table-scroll">
        <table className="ui basic table">
          <thead>
            <tr>
              <th>
                <button
                  type="button"
                  className="ui mini primary button"
                  aria-pressed={allSelected}
                  onClick={() => setSelected(allSelected ? new Set() : new Set(ordered.map((h) => h.id)))}
                >
                  {t('admin.selectAll')}
                </button>
              </th>
              <th>{t('admin.order')}</th>
              <th>{t('admin.name')}</th>
              <th>{t('admin.group')}</th>
              <th>{t('admin.ip')}</th>
              <th>{t('admin.country')}</th>
              <th>{t('admin.agent')}</th>
              <th>{t('admin.state')}</th>
              <th>{t('admin.hidden')}</th>
              <th>{t('admin.publicHistory')}</th>
              <th>{t('admin.manage')}</th>
            </tr>
          </thead>
          <tbody>
            {ordered.length === 0 && (
              <tr>
                <td colSpan={11} className="empty">
                  {t('admin.empty')}
                </td>
              </tr>
            )}
            {ordered.map((h, i) => {
              const name = displayName(h, unnamed)
              const state = h.online ? 'online' : h.report ? 'offline' : 'waiting'
              return (
                <tr key={h.id} className={selected.has(h.id) ? 'active' : undefined}>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={t('admin.select', { name })}
                      checked={selected.has(h.id)}
                      onChange={() => toggle(h.id)}
                    />
                  </td>
                  <td>{i + 1}</td>
                  <td>
                    <span className="name-cell">
                      <OsLogo info={h.host_info} />
                      <span>
                        {name}
                        {h.name && h.host_info?.hostname && h.host_info.hostname !== h.name && (
                          <small className="sub">{h.host_info.hostname}</small>
                        )}
                      </span>
                    </span>
                  </td>
                  <td>{h.group || <span className="sub">{t('status.defaultGroup')}</span>}</td>
                  <td className="ips">
                    {h.ipv4 && <span>{h.ipv4}</span>}
                    {h.ipv6 && <span>{h.ipv6}</span>}
                    {!h.ipv4 && !h.ipv6 && '—'}
                  </td>
                  <td>
                    <span className="country-cell">
                      <Flags countries={h.countries} />
                      <span>{h.countries.join(' / ') || '—'}</span>
                      {!h.country_override && h.countries.length > 0 && <small className="sub">{t('admin.geoip')}</small>}
                    </span>
                  </td>
                  <td>
                    {h.host_info?.agent_version ?? '—'}
                    {h.agent_outdated && <span className="ui mini red label">{t('admin.outdated')}</span>}
                  </td>
                  <td>
                    <span className={`ui mini state label ${state === 'online' ? 'green' : state === 'offline' ? 'red' : 'grey'}`}>
                      {t(`status.${state}Short`)}
                    </span>
                  </td>
                  <td>{h.hidden ? t('admin.yes') : t('admin.no')}</td>
                  <td>{h.public_history ? t('admin.yes') : t('admin.no')}</td>
                  <td>
                    <div className="ui mini buttons">
                      <IconButton icon="graph-up" label={t('admin.history', { name })} onClick={() => navigate(hostHistoryHref(h.id))} />
                      <IconButton icon="arrow-up" label={t('admin.moveUp', { name })} disabled={busy || i === 0} onClick={() => void move(h.id, -1)} />
                      <IconButton
                        icon="arrow-down"
                        label={t('admin.moveDown', { name })}
                        disabled={busy || i === ordered.length - 1}
                        onClick={() => void move(h.id, 1)}
                      />
                      <IconButton icon="pencil-square" label={t('admin.edit', { name })} onClick={() => setDialog({ kind: 'edit', host: h })} />
                      <IconButton icon="key" label={t('admin.regenerate', { name })} onClick={() => setDialog({ kind: 'regenerate', host: h })} />
                      <IconButton icon="trash3" label={t('admin.delete', { name })} onClick={() => setDialog({ kind: 'delete', ids: [h.id] })} />
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      {dialog?.kind === 'add' && (
        <HostForm
          groups={groups}
          onClose={() => setDialog(null)}
          onSaved={async (added) => {
            await reload()
            if (added) {
              setDialog({ kind: 'token', title: t('token.addedTitle', { name: added.name || unnamed }), token: added.token })
            }
          }}
        />
      )}
      {dialog?.kind === 'edit' && (
        <HostForm
          host={dialog.host}
          displayName={displayName(dialog.host, unnamed)}
          groups={groups}
          onClose={() => setDialog(null)}
          onSaved={async () => {
            setDialog(null)
            toast({ kind: 'success', text: t('admin.saved') })
            await reload()
          }}
        />
      )}
      {dialog?.kind === 'token' && (
        <TokenModal title={dialog.title} token={dialog.token} dashboardVersion={dashboardVersion} onDone={() => setDialog(null)} />
      )}
      {dialog?.kind === 'regenerate' && (
        <Confirm
          title={t('admin.regenerateTitle')}
          body={t('admin.regenerateBody', { name: displayName(dialog.host, unnamed) })}
          busy={busy}
          onCancel={() => setDialog(null)}
          onConfirm={() => {
            const h = dialog.host
            void run(async () => {
              const token = await regenerateToken(h.id)
              setDialog({ kind: 'token', title: t('token.regeneratedTitle', { name: displayName(h, unnamed) }), token })
            })
          }}
        />
      )}
      {dialog?.kind === 'delete' && (
        <Confirm
          title={t('admin.deleteTitle')}
          body={
            dialog.ids.length === 1
              ? t('admin.deleteBody', { name: displayName(hosts.find((h) => h.id === dialog.ids[0])!, unnamed) })
              : t('admin.batchDeleteBody', { count: dialog.ids.length })
          }
          busy={busy}
          onCancel={() => setDialog(null)}
          onConfirm={() => {
            const ids = dialog.ids
            void run(async () => {
              for (const id of ids) await deleteHost(id)
              setSelected(new Set([...selected].filter((id) => !ids.includes(id))))
              setDialog(null)
            }, t('admin.deleted'))
          }}
        />
      )}
      {dialog?.kind === 'group' && (
        <GroupModal
          count={dialog.ids.length}
          groups={groups}
          busy={busy}
          onCancel={() => setDialog(null)}
          onSave={(group) => {
            const ids = dialog.ids
            void run(async () => {
              for (const id of ids) await updateHost(id, { group })
              setDialog(null)
            }, t('admin.saved'))
          }}
        />
      )}
    </>
  )
}

function IconButton({ icon, label, disabled, onClick }: { icon: IconName; label: string; disabled?: boolean; onClick: () => void }) {
  return (
    <button type="button" className="ui button" aria-label={label} title={label} disabled={disabled} onClick={onClick}>
      <Icon name={icon} />
    </button>
  )
}

/** GroupModal asks for the group to give the selected Hosts. */
function GroupModal({ count, groups, busy, onCancel, onSave }: { count: number; groups: string[]; busy: boolean; onCancel: () => void; onSave: (group: string) => void }) {
  const { t } = useTranslation()
  const formId = useId()
  const [group, setGroup] = useState('')
  function submit(e: FormEvent) {
    e.preventDefault()
    onSave(group.trim())
  }
  return (
    <Modal
      title={t('admin.batchGroupTitle', { count })}
      size="mini"
      onClose={onCancel}
      actions={
        <>
          <button type="button" className="ui negative button" onClick={onCancel}>
            {t('admin.cancel')}
          </button>
          <button type="submit" form={formId} className={`ui positive primary right labeled button${busy ? ' loading' : ''}`} disabled={busy}>
            {t('admin.confirm')}
            <Icon name="check-lg" />
          </button>
        </>
      }
    >
      <form id={formId} className="ui form" onSubmit={submit}>
        <div className="field">
          <label htmlFor={`${formId}-group`}>{t('hostForm.group')}</label>
          <input id={`${formId}-group`} list={`${formId}-groups`} value={group} maxLength={32} onChange={(e) => setGroup(e.target.value)} />
          <datalist id={`${formId}-groups`}>
            {groups.map((g) => (
              <option key={g} value={g} />
            ))}
          </datalist>
          <small className="hint">{t('hostForm.groupHint')}</small>
        </div>
      </form>
    </Modal>
  )
}
