import { unknown } from './format'

// Vendored flag-icons SVGs (MIT), bundled at build time; no CDN at runtime.
const flagUrls = import.meta.glob<string>('/node_modules/flag-icons/flags/4x3/*.svg', {
  query: '?url',
  import: 'default',
  eager: true,
})

function flagUrl(code: string): string | undefined {
  return flagUrls[`/node_modules/flag-icons/flags/4x3/${code.toLowerCase()}.svg`]
}

/** Flags shows one SVG flag per ISO country code, or a neutral globe when there are none. */
export function Flags({ countries }: { countries: string[] }) {
  const known = countries.filter((c) => flagUrl(c))
  if (known.length === 0) return <Globe />
  return (
    <span className="flags">
      {known.map((c) => (
        <img key={c} className="flag" src={flagUrl(c)} alt={c} title={c} width={20} height={15} />
      ))}
    </span>
  )
}

function Globe() {
  return (
    <svg className="flag globe" viewBox="0 0 20 15" width={20} height={15} role="img" aria-label={unknown}>
      <circle cx="10" cy="7.5" r="6.5" fill="none" stroke="currentColor" />
      <ellipse cx="10" cy="7.5" rx="2.8" ry="6.5" fill="none" stroke="currentColor" />
      <path d="M3.5 7.5h13" stroke="currentColor" />
    </svg>
  )
}
