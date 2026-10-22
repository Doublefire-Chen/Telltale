// Formatting helpers. Missing values render as an em dash in every language.

export const unknown = '—'

export function formatPercent(value: number | undefined, lang: string): string {
  if (value === undefined) return unknown
  return new Intl.NumberFormat(lang, { style: 'percent', maximumFractionDigits: 1 }).format(value / 100)
}

const byteUnits = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']

export function formatBytes(value: number | undefined, lang: string): string {
  if (value === undefined) return unknown
  let unit = 0
  let n = value
  while (n >= 1024 && unit < byteUnits.length - 1) {
    n /= 1024
    unit++
  }
  const digits = unit === 0 ? 0 : 1
  return `${new Intl.NumberFormat(lang, { maximumFractionDigits: digits, minimumFractionDigits: digits }).format(n)} ${byteUnits[unit]}`
}
