import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getHistory, type History, type HistoryRange, type Host } from './api'
import { Chart } from './Chart'
import { Flags } from './Flag'
import { formatBytes, formatPercent } from './format'

const ranges: HistoryRange[] = ['1h', '24h', '7d', '30d']
const refreshMs = 30_000

type Props = {
  host: Host
  name: string
  onBack: () => void
}

/** HostDetail shows a Host's history charts for a chosen time range. */
export function HostDetail({ host, name, onBack }: Props) {
  const { t, i18n } = useTranslation()
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

  return (
    <section className="stack">
      <div>
        <button type="button" className="link" onClick={onBack}>
          ← {t('detail.back')}
        </button>
      </div>
      <h1>
        <Flags countries={host.countries} />
        {name}
      </h1>
      <div className="segmented" role="group" aria-label={t('detail.range')}>
        {ranges.map((r) => (
          <button key={r} type="button" aria-pressed={r === range} onClick={() => setRange(r)}>
            {r}
          </button>
        ))}
      </div>
      {failed && <p role="alert">{t('app.error')}</p>}
      {!history && !failed && <p>{t('app.loading')}</p>}
      {history && history.t.length === 0 && <p className="muted">{t('detail.noData')}</p>}
      {history && history.t.length > 0 && (
        <div className="charts">
          <Chart
            title={t('status.cpu')}
            t={history.t}
            max={100}
            series={[{ label: t('status.cpu'), values: history.cpu, color: '#2357d9' }]}
            formatValue={(v) => formatPercent(v, lang)}
          />
          <Chart
            title={t('status.memory')}
            t={history.t}
            max={host.host_info?.memory_total}
            series={[{ label: t('status.memory'), values: history.memory, color: '#7048e8' }]}
            formatValue={(v) => formatBytes(v, lang)}
          />
          <Chart
            title={t('status.disk')}
            t={history.t}
            max={host.host_info?.disk_total}
            series={[{ label: t('status.disk'), values: history.disk, color: '#e8590c' }]}
            formatValue={(v) => formatBytes(v, lang)}
          />
          <Chart
            title={t('status.bandwidth')}
            t={history.t}
            series={[
              { label: t('status.in'), values: history.rx_rate, color: '#2f9e44' },
              { label: t('status.out'), values: history.tx_rate, color: '#1c7ed6' },
            ]}
            formatValue={(v) => `${formatBytes(v, lang)}/s`}
          />
        </div>
      )}
    </section>
  )
}
