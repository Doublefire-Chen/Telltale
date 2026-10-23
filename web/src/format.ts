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

/** formatCapacity is a size without needless decimals, e.g. "2 GiB" or "3.9 TiB". */
export function formatCapacity(value: number | undefined, lang: string): string {
  if (value === undefined) return unknown
  let unit = 0
  let n = value
  while (n >= 1024 && unit < byteUnits.length - 1) {
    n /= 1024
    unit++
  }
  return `${new Intl.NumberFormat(lang, { maximumFractionDigits: n < 10 ? 1 : 0 }).format(n)} ${byteUnits[unit]}`
}

export function formatRate(bytesPerSecond: number | undefined, lang: string): string {
  if (bytesPerSecond === undefined) return unknown
  return `${formatBytes(bytesPerSecond, lang)}/s`
}

export function formatLoad(values: (number | undefined)[], lang: string): string {
  if (values.some((v) => v === undefined)) return unknown
  const nf = new Intl.NumberFormat(lang, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  return values.map((v) => nf.format(v!)).join(' ')
}

/** formatUsage shows "used / total", or just used while the total is unknown. */
export function formatUsage(used: number | undefined, total: number | undefined, lang: string): string {
  if (used === undefined) return unknown
  if (total === undefined) return formatBytes(used, lang)
  return `${formatBytes(used, lang)} / ${formatBytes(total, lang)}`
}

/** uptimeParts splits the time since bootTime (Unix seconds) into days, hours and minutes. */
export function uptimeParts(bootTime: number, nowMs: number): { d: number; h: number; m: number } | undefined {
  const seconds = Math.floor(nowMs / 1000) - bootTime
  if (!Number.isFinite(seconds) || seconds < 0) return undefined
  return { d: Math.floor(seconds / 86400), h: Math.floor((seconds % 86400) / 3600), m: Math.floor((seconds % 3600) / 60) }
}
