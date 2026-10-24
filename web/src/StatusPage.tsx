import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Host, HostInfo } from './api'
import { Flags } from './Flag'
import { formatBytes, formatCapacity, formatLoad, formatPercent, formatRate, formatUsage, unknown, uptimeParts } from './format'
import { HistoryView } from './HistoryView'
import { byGroup, displayName, useHosts } from './hosts'
import { Icon, type IconName } from './Icon'
import { OsLogo } from './OsLogo'
import { followLink, publicHistoryHref, useRoute } from './route'

type StatusPageProps = {
  /** The admin sees IP addresses and Hidden Hosts. */
  admin: boolean
  /** Called when the Dashboard refuses access, e.g. the page just went private. */
  onUnauthorized: () => void
}

export function StatusPage({ admin, onUnauthorized }: StatusPageProps) {
  const { t } = useTranslation()
  const [route] = useRoute()
  const { hosts, dashboardVersion, failed } = useHosts(onUnauthorized)

  if (failed) return <p role="alert">{t('app.error')}</p>
  if (!hosts) return <p>{t('app.loading')}</p>
  if (route.kind === 'host') {
    // Every Host for the admin; for visitors only Hosts whose history is
    // public. The Dashboard enforces the same rule, so this just avoids
    // offering a page that would fail.
    const host = hosts.find((h) => h.id === route.id && (admin || h.public_history))
    if (host) return <HistoryView host={host} name={displayName(host, t('status.unnamed'))} backHref="/" />
  }
  return (
    <section className="stack">
      <h1 className="visually-hidden">{t('status.title')}</h1>
      {hosts.length === 0 ? (
        <p className="muted">{t('status.empty')}</p>
      ) : (
        <HostGroups hosts={hosts} dashboardVersion={dashboardVersion} admin={admin} />
      )}
    </section>
  )
}

type HostGroupsProps = {
  hosts: Host[]
  dashboardVersion: string
  admin: boolean
}

/** HostGroups shows a collapsible panel of Host cards per group. */
function HostGroups({ hosts, dashboardVersion, admin }: HostGroupsProps) {
  const { t } = useTranslation()
  const now = Date.now()
  return (
    <>
      {byGroup(hosts).map((g) => (
        <details key={g.group} className="group" open>
          <summary>
            <h2>{g.group || t('status.defaultGroup')}</h2>
            <span className="group-count">{g.hosts.length}</span>
          </summary>
          <div className="host-cards">
            {g.hosts.map((h) => (
              <HostCard
                key={h.id}
                host={h}
                dashboardVersion={dashboardVersion}
                now={now}
                admin={admin}
              />
            ))}
          </div>
        </details>
      ))}
    </>
  )
}

type HostCardProps = {
  host: Host
  dashboardVersion: string
  now: number
  admin: boolean
}

function HostCard({ host: h, dashboardVersion, now, admin }: HostCardProps) {
  const { t, i18n } = useTranslation()
  const [, navigate] = useRoute()
  const lang = i18n.resolvedLanguage ?? 'en'
  // Like Nezha's, the details popover opens on hover; a click pins it open.
  const [pinned, setPinned] = useState(false)
  const [hovered, setHovered] = useState(false)
  const detailsOpen = pinned || hovered
  const r = h.report
  const info = h.host_info
  const name = displayName(h, t('status.unnamed'))
  const state = h.online ? 'online' : h.report ? 'offline' : 'waiting'
  const live = state === 'online'
  const nameId = `host-${h.id}`
  const detailsId = `host-details-${h.id}`
  return (
    <article className={`host-card ${state}`} aria-labelledby={nameId}>
      <header className="host-card-header">
        <h3 className="host-title">
          <Flags countries={h.countries} round />
          <OsLogo info={info} />
          {admin || h.public_history ? (
            <a id={nameId} className="host-name" href={publicHistoryHref(h.id)} onClick={(e) => followLink(e, navigate)}>
              {name}
            </a>
          ) : (
            <span id={nameId} className="host-name">
              {name}
            </span>
          )}
          <span className={`state ${state}`} role="img" aria-label={t(`status.${state}`)} title={t(`status.${state}`)}>
            {state !== 'online' && `[${t(`status.${state}Short`)}]`}
          </span>
        </h3>
        <div className="card-actions" onMouseLeave={() => setHovered(false)}>
          <span className="info-anchor">
            <button
              type="button"
              className="card-icon info"
              aria-expanded={detailsOpen}
              aria-controls={detailsId}
              aria-label={t('status.details', { name })}
              title={t('status.details', { name })}
              onMouseEnter={() => setHovered(true)}
              onClick={() => setPinned(!pinned)}
            >
              <Icon name="info-circle-fill" />
            </button>
          {detailsOpen && (
            <div id={detailsId} className="host-popover" role="group" aria-label={t('status.details', { name })}>
              <dl className="host-details">
                {admin && (
                  <>
                    <dt>IPv4</dt>
                    <dd>{h.ipv4 || unknown}</dd>
                    <dt>IPv6</dt>
                    <dd>{h.ipv6 || unknown}</dd>
                  </>
                )}
                <dt>{t('status.system')}</dt>
                <dd>{systemSummary(info)}</dd>
                <dt>{t('status.kernel')}</dt>
                <dd>{info?.kernel_version || unknown}</dd>
                <dt>{t('status.cpuModel')}</dt>
                <dd>{info?.cpu_model || unknown}</dd>
                <dt>{t('status.memory')}</dt>
                <dd>{formatUsage(r?.memory_used, info?.memory_total, lang)}</dd>
                <dt>{t('status.swap')}</dt>
                <dd>{info?.swap_total === 0 ? t('status.off') : formatUsage(r?.swap_used, info?.swap_total, lang)}</dd>
                <dt>{t('status.disk')}</dt>
                <dd>{formatUsage(r?.disk_used, info?.disk_total, lang)}</dd>
                <dt>{t('status.load')}</dt>
                <dd>{formatLoad([r?.load1, r?.load5, r?.load15], lang)}</dd>
                <dt>{t('status.processes')}</dt>
                <dd>{r?.process_count ?? unknown}</dd>
                <dt>{t('status.connections')}</dt>
                <dd>
                  {r?.tcp_connections === undefined && r?.udp_connections === undefined
                    ? unknown
                    : `TCP ${r?.tcp_connections ?? unknown} · UDP ${r?.udp_connections ?? unknown}`}
                </dd>
                <dt>{t('status.bootTime')}</dt>
                <dd>
                  {info?.boot_time === undefined
                    ? unknown
                    : new Date(info.boot_time * 1000).toLocaleString(lang, { dateStyle: 'medium', timeStyle: 'short' })}
                </dd>
                <dt>{t('status.agentVersion')}</dt>
                <dd>{info?.agent_version || unknown}</dd>
              </dl>
            </div>
          )}
          </span>
        </div>
      </header>
      {(h.hidden || h.agent_outdated) && (
        <div className="tags">
          {h.hidden && <span className="tag">{t('status.hiddenTag')}</span>}
          {h.agent_outdated && (
            <span
              className="badge"
              title={t('status.outdatedDetail', { agent: info?.agent_version ?? unknown, dashboard: dashboardVersion })}
            >
              {t('status.outdated')}
            </span>
          )}
        </div>
      )}
      <dl className="metrics">
        <dt>{t('status.cpu')}</dt>
        <dd>
          <UsageBar label={t('status.cpu')} percent={r?.cpu_percent} live={live} />
        </dd>
        <dt>{t('status.memory')}</dt>
        <dd>
          <UsageBar label={t('status.memory')} percent={ratio(r?.memory_used, info?.memory_total)} live={live} />
        </dd>
        <dt>{t('status.swap')}</dt>
        <dd>
          <UsageBar
            label={t('status.swap')}
            percent={info?.swap_total === 0 ? undefined : ratio(r?.swap_used, info?.swap_total)}
            live={live}
            text={info?.swap_total === 0 ? t('status.off') : undefined}
          />
        </dd>
        <dt>{t('status.bandwidth')}</dt>
        <dd className="text">
          <InOut inValue={formatRate(r?.rx_rate, lang)} outValue={formatRate(r?.tx_rate, lang)} />
        </dd>
        <dt>{t('status.traffic')}</dt>
        <dd className="text">
          <InOut inValue={formatBytes(r?.net_rx_bytes, lang)} outValue={formatBytes(r?.net_tx_bytes, lang)} filled />
        </dd>
        <dt>{t('status.disk')}</dt>
        <dd>
          <UsageBar label={t('status.disk')} percent={ratio(r?.disk_used, info?.disk_total)} live={live} />
        </dd>
        <dt>{t('status.specs')}</dt>
        <dd className="text specs">
          <span>
            <Icon name="cpu-fill" className="cpu" />
            {info?.cpu_cores ? t('status.cores', { count: info.cpu_cores }) : unknown}
          </span>
          <span>
            <Icon name="memory" className="ram" />
            <span className="visually-hidden">{t('status.memory')} </span>
            {formatCapacity(info?.memory_total, lang)}
          </span>
          <span>
            <Icon name="hdd-rack-fill" className="hdd" />
            <span className="visually-hidden">{t('status.disk')} </span>
            {formatCapacity(info?.disk_total, lang)}
          </span>
        </dd>
        <dt>{t('status.uptime')}</dt>
        <dd className="text">
          <Icon name="clock-fill" className="muted-icon" />
          <Uptime bootTime={info?.boot_time} now={now} />
        </dd>
      </dl>
    </article>
  )
}

/** ratio is used as a percentage of total, or undefined while either is unknown. */
function ratio(used: number | undefined, total: number | undefined): number | undefined {
  if (used === undefined || !total) return undefined
  return (used / total) * 100
}

/** level colours a usage bar: blue up to half, orange up to 80%, red above. */
function level(percent: number): 'fine' | 'warning' | 'critical' {
  if (percent < 51) return 'fine'
  if (percent < 81) return 'warning'
  return 'critical'
}

/** UsageBar is a pill-shaped meter that turns orange and red as it fills, and grey while Offline. */
function UsageBar({ label, percent, live, text }: { label: string; percent: number | undefined; live: boolean; text?: string }) {
  const { i18n } = useTranslation()
  const lang = i18n.resolvedLanguage ?? 'en'
  const clamped = percent === undefined ? undefined : Math.min(100, Math.max(0, percent))
  const shown = text ?? (clamped === undefined ? unknown : formatPercent(Math.round(clamped), lang))
  return (
    <div
      className={`bar ${clamped === undefined ? 'empty' : live ? level(clamped) : 'stale'}`}
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={clamped}
      aria-valuetext={shown}
    >
      {clamped === undefined ? (
        <span className="bar-text">{shown}</span>
      ) : (
        <div className="bar-fill" style={{ width: `${clamped}%` }}>
          <span className="bar-text">{shown}</span>
        </div>
      )}
    </div>
  )
}

/** systemSummary is e.g. "ubuntu 24.04 · x86_64 · kvm". */
function systemSummary(info: HostInfo | null): string {
  if (!info) return unknown
  const platform = [info.platform, info.platform_version].filter(Boolean).join(' ')
  const parts = [platform, info.arch, info.virtualization].filter(Boolean)
  return parts.length ? parts.join(' · ') : unknown
}

function Uptime({ bootTime, now }: { bootTime: number | undefined; now: number }) {
  const { t } = useTranslation()
  const parts = bootTime === undefined ? undefined : uptimeParts(bootTime, now)
  if (!parts) return <>{unknown}</>
  if (parts.d > 0) return <>{t('status.uptimeDays', parts)}</>
  if (parts.h > 0) return <>{t('status.uptimeHours', parts)}</>
  return <>{t('status.uptimeMinutes', parts)}</>
}

/** InOut shows an inbound and an outbound value, labelled for screen readers. */
function InOut({ inValue, outValue, filled = false }: { inValue: string; outValue: string; filled?: boolean }) {
  const { t } = useTranslation()
  const down: IconName = filled ? 'arrow-down-circle-fill' : 'arrow-down-circle'
  const up: IconName = filled ? 'arrow-up-circle-fill' : 'arrow-up-circle'
  return (
    <span className="in-out">
      <span>
        <Icon name={down} className="muted-icon" />
        <span className="visually-hidden">{t('status.in')} </span>
        {inValue}
      </span>
      <span>
        <Icon name={up} className="muted-icon" />
        <span className="visually-hidden">{t('status.out')} </span>
        {outValue}
      </span>
    </span>
  )
}
