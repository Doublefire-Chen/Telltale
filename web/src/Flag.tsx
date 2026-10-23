import { unknown } from './format'

// Vendored flag-icons SVGs (MIT), bundled at build time; no CDN at runtime.
const flagUrls = import.meta.glob<string>(['/node_modules/flag-icons/flags/4x3/*.svg', '/node_modules/flag-icons/flags/1x1/*.svg'], {
  query: '?url',
  import: 'default',
  eager: true,
})

type Shape = '4x3' | '1x1'

function flagUrl(code: string, shape: Shape = '4x3'): string | undefined {
  return flagUrls[`/node_modules/flag-icons/flags/${shape}/${code.toLowerCase()}.svg`]
}

/**
 * Flags shows one SVG flag per ISO country code, or a neutral globe when
 * there are none. Round flags (on the Status page's cards) use the square
 * artwork, clipped to a circle.
 */
export function Flags({ countries, round = false }: { countries: string[]; round?: boolean }) {
  const shape: Shape = round ? '1x1' : '4x3'
  const known = countries.filter((c) => flagUrl(c, shape))
  if (known.length === 0) return <Globe />
  return (
    <span className="flags">
      {known.map((c) => (
        <img
          key={c}
          className={round ? 'flag round' : 'flag'}
          src={flagUrl(c, shape)}
          alt={c}
          title={c}
          width={round ? 19 : 20}
          height={round ? 19 : 15}
        />
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
