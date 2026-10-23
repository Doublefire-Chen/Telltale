import type { HostInfo } from './api'

// Vendored font-logos SVGs (public domain), bundled at build time; no CDN at
// runtime. They are drawn as CSS masks so they take the text colour, in light
// and dark mode alike.
const logoUrls = import.meta.glob<string>('/node_modules/font-logos/vectors/*.svg', {
  query: '?url',
  import: 'default',
  eager: true,
})

function logoUrl(name: string): string | undefined {
  return logoUrls[`/node_modules/font-logos/vectors/${name}.svg`]
}

// Platform IDs (from /etc/os-release, as the Agent reports them) whose logo
// has a different name in font-logos.
const aliases: Record<string, string> = {
  darwin: 'apple',
  arch: 'archlinux',
  rocky: 'rocky-linux',
  kali: 'kali-linux',
  pop: 'pop-os',
  raspbian: 'raspberry-pi',
  endeavouros: 'endeavour',
  amzn: 'redhat',
  amazon: 'redhat',
  rhel: 'redhat',
  'opensuse-leap': 'opensuse',
  'opensuse-tumbleweed': 'tumbleweed',
  mx: 'mxlinux',
}

/** logoFor picks the font-logos name for a Host's system, or undefined for none. */
export function logoFor(info: HostInfo | null): string | undefined {
  if (!info) return undefined
  const platform = (info.platform ?? '').toLowerCase()
  const name = aliases[platform] ?? platform
  if (name && logoUrl(name)) return name
  if (info.os === 'darwin') return 'apple'
  if (info.os === 'linux') return 'tux'
  return undefined
}

/** OsLogo shows the distribution's or OS's logo, named by its platform. */
export function OsLogo({ info }: { info: HostInfo | null }) {
  const label = [info?.platform, info?.platform_version].filter(Boolean).join(' ') || info?.os || ''
  if (info?.os === 'windows') {
    return (
      <svg className="os-logo" viewBox="0 0 16 16" role="img" aria-label={label}>
        <title>{label}</title>
        <path d="M1 2.7 6.6 2v5.4H1zM7.4 1.9 15 1v6.4H7.4zM1 8.3h5.6v5.4L1 13zM7.4 8.3H15V15l-7.6-1z" fill="currentColor" />
      </svg>
    )
  }
  const name = logoFor(info)
  if (!name) return null
  return (
    <span
      className="os-logo"
      role="img"
      aria-label={label}
      title={label}
      // React sets styles through the CSSOM, which the Dashboard's CSP allows.
      style={{ maskImage: `url("${logoUrl(name)}")`, WebkitMaskImage: `url("${logoUrl(name)}")` }}
    />
  )
}
