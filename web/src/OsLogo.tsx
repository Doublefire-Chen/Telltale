import type { HostInfo } from './api'

// Vendored font-logos SVGs (public domain), bundled at build time; no CDN at
// runtime. They are drawn as CSS masks filled with each brand's colour.
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

// Brand colours, tuned where needed to stay readable on light and dark
// backgrounds. Logos not listed (Tux, Apple) take the text colour.
const colors: Record<string, string> = {
  almalinux: '#4d90e0',
  alpine: '#2d84c4',
  archlinux: '#1793d1',
  centos: '#a14f99',
  coreos: '#3a9ad9',
  debian: '#d70751',
  deepin: '#007cff',
  devuan: '#7b8ca2',
  elementary: '#64baff',
  endeavour: '#7f7fff',
  fedora: '#51a2da',
  freebsd: '#c8302b',
  gentoo: '#7a6fb0',
  'kali-linux': '#367bf0',
  linuxmint: '#87cf3e',
  mageia: '#2397d4',
  manjaro: '#35bf5c',
  mxlinux: '#6b9ac4',
  nixos: '#5277c3',
  openbsd: '#e8b400',
  opensuse: '#73ba25',
  openwrt: '#00b5e2',
  'pop-os': '#48b9c7',
  'raspberry-pi': '#c51a4a',
  redhat: '#ee0000',
  'rocky-linux': '#10b981',
  slackware: '#6a7fc1',
  tumbleweed: '#73ba25',
  ubuntu: '#e95420',
  void: '#478061',
  zorin: '#15a6f0',
}

const windowsBlue = '#0078d4'

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
        <path d="M1 2.7 6.6 2v5.4H1zM7.4 1.9 15 1v6.4H7.4zM1 8.3h5.6v5.4L1 13zM7.4 8.3H15V15l-7.6-1z" fill={windowsBlue} />
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
      style={{
        maskImage: `url("${logoUrl(name)}")`,
        WebkitMaskImage: `url("${logoUrl(name)}")`,
        backgroundColor: colors[name],
      }}
    />
  )
}
