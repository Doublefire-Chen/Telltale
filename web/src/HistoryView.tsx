import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getHistory, type History, type HistoryRange, type Host } from './api'
import { Chart, type ChartSeries } from './Chart'
import { Flags } from './Flag'
import { formatBytes, formatLoad, formatPercent } from './format'
import { Icon } from './Icon'
import { OsLogo } from './OsLogo'
import { useRoute } from './route'

const ranges: HistoryRange[] = ['1h', '24h', '7d', '30d']
const refreshMs = 30_000

type ChartSpec = {
  title: string
  series: ChartSeries[]
  max?: number
  formatValue: (v: number) => string
  /** False to leave the chart out, e.g. Swap on a Host without swap. */
  show?: boolean
}

/**
 * HistoryView charts a Host's history for a chosen time range. The admin panel
 * shows it for every Host, the Status page for Hosts with public history;
 * each styles it in its own look.
 */
export function HistoryView({ host, name, backHref }: { host: Host; name: string; backHref: string }) {
  const { t, i18n } = useTranslation()
  const [, navigate] = useRoute()
  const lang = i18n.resolvedLanguage ?? 'en'
  const [range, setRange] = useState<HistoryRange>('1h')
  const [history, setHistory] = useState<History | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let stopped = false
    const load = () =>
      getHistory(host.id, range).then(
        (h) => {
          if (!stopped) {
            setHistory(h)
            setFailed(false)
          }
        },
        () => !stopped && setFailed(true),
      )
    setHistory(null)
    void load()
    const timer = setInterval(() => void load(), refreshMs)
    return () => {
      stopped = true
      clearInterval(timer)
    }
  }, [host.id, range])

  const info = host.host_info
  const specs: ChartSpec[] =
    history && history.t.length > 0
      ? [
        {
          title: t('status.cpu'),
          max: 100,
          series: [{ label: t('status.cpu'), values: history.cpu, color: '#2357d9' }],
          formatValue: (v) => formatPercent(v, lang),
        },
        {
          title: t('status.memory'),
          max: info?.memory_total,
          series: [{ label: t('status.memory'), values: history.memory, color: '#7048e8' }],
          formatValue: (v) => formatBytes(v, lang),
        },
        {
          title: t('status.swap'),
          max: info?.swap_total,
          show: !!info?.swap_total,
          series: [{ label: t('status.swap'), values: history.swap, color: '#ae3ec9' }],
          formatValue: (v) => formatBytes(v, lang),
        },
        {
          title: t('status.disk'),
          max: info?.disk_total,
          series: [{ label: t('status.disk'), values: history.disk, color: '#e8590c' }],
          formatValue: (v) => formatBytes(v, lang),
        },
        {
          title: t('status.bandwidth'),
          series: [
            { label: t('status.in'), values: history.rx_rate, color: '#2f9e44' },
            { label: t('status.out'), values: history.tx_rate, color: '#1c7ed6' },
          ],
          formatValue: (v) => `${formatBytes(v, lang)}/s`,
        },
        {
          title: t('status.load'),
          series: [{ label: t('status.load'), values: history.load1, color: '#0c8599' }],
          formatValue: (v) => formatLoad([v], lang),
        },
      ]
    : []
  // Nor does a metric the Host never sends, such as Load on Windows.
  const charts = specs.filter((c) => c.show !== false && c.series.some((s) => s.values?.some((v) => v !== null)))

  return (
    <section className="history-page">
      <div className="history-head">
        <a
          className="back"
          href={backHref}
          onClick={(e) => {
            if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return
            e.preventDefault()
            navigate(backHref)
          }}
        >
          <Icon name="arrow-left" />
          {t('detail.back')}
        </a>
        <h1 className="history-title">
          <Flags countries={host.countries} />
          <OsLogo info={info} />
          {t('detail.title', { name })}
        </h1>
        <div className="ui buttons range" role="group" aria-label={t('detail.range')}>
          {ranges.map((r) => (
            <button
              key={r}
              type="button"
              className={`ui button${r === range ? ' primary' : ''}`}
              aria-pressed={r === range}
              onClick={() => setRange(r)}
            >
              {r}
            </button>
          ))}
        </div>
      </div>
      {failed && (
        <div className="ui error message" role="alert">
          {t('app.error')}
        </div>
      )}
      {!history && !failed && <p>{t('app.loading')}</p>}
      {history && history.t.length === 0 && <div className="ui message">{t('detail.noData')}</div>}
      {history && history.t.length > 0 && (
        <div className="charts">
          {charts.map((c) => (
            <Chart key={c.title} title={c.title} t={history.t} max={c.max} series={c.series} formatValue={c.formatValue} />
          ))}
        </div>
      )}
    </section>
  )
}
