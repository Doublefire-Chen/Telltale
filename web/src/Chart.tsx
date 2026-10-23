import { useEffect, useRef } from 'react'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'

export type ChartSeries = { label: string; values: (number | null)[]; color: string }

type Props = {
  title: string
  /** Sample times in Unix seconds. */
  t: number[]
  series: ChartSeries[]
  formatValue: (v: number) => string
  /** Upper bound of the y axis, e.g. 100 for percentages. */
  max?: number
}

const height = 180

/** Chart draws a time series with uPlot, sized to its container. */
export function Chart({ title, t, series, formatValue, max }: Props) {
  const container = useRef<HTMLDivElement>(null)
  const plot = useRef<uPlot | null>(null)
  const data = [t, ...series.map((s) => s.values)] as uPlot.AlignedData

  // Rebuild when the series' shape or formatting changes; otherwise just set data.
  const shape = series.map((s) => `${s.label}:${s.color}`).join('|') + `|${max}`
  useEffect(() => {
    const el = container.current
    if (!el) return
    const muted = getComputedStyle(el).getPropertyValue('--label').trim() || '#787878'
    const axis = { stroke: muted, grid: { stroke: 'rgba(128,128,128,0.15)' }, ticks: { show: false } }
    plot.current = new uPlot(
      {
        width: el.clientWidth || 600,
        height,
        scales: { x: { time: true }, y: max === undefined ? { range: (_u, _min, hi) => [0, hi || 1] } : { range: [0, max] } },
        axes: [axis, { ...axis, size: 70, values: (_u, ticks) => ticks.map(formatValue) }],
        series: [
          {},
          ...series.map((s) => ({
            label: s.label,
            stroke: s.color,
            width: 1.5,
            value: (_u: uPlot, v: number | null) => (v === null ? '—' : formatValue(v)),
          })),
        ],
        legend: { show: series.length > 1 },
      },
      data,
      el,
    )
    const resize = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => {
      plot.current?.setSize({ width: el.clientWidth, height })
    })
    resize?.observe(el)
    return () => {
      resize?.disconnect()
      plot.current?.destroy()
      plot.current = null
    }
    // Deliberately keyed on shape only: formatValue and data are new on every
    // render, and data updates go through setData below.
  }, [shape])

  useEffect(() => {
    plot.current?.setData(data)
  })

  return (
    <figure className="chart">
      <figcaption>{title}</figcaption>
      <div ref={container} />
    </figure>
  )
}
