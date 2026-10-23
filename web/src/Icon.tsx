// Bootstrap Icons (MIT), the set Nezha's default theme uses, bundled at build
// time. Each is drawn as a CSS mask, so it takes the colour of its class.
const iconUrls = import.meta.glob<string>(
  [
    '/node_modules/bootstrap-icons/icons/arrow-down-circle.svg',
    '/node_modules/bootstrap-icons/icons/arrow-up-circle.svg',
    '/node_modules/bootstrap-icons/icons/arrow-down-circle-fill.svg',
    '/node_modules/bootstrap-icons/icons/arrow-up-circle-fill.svg',
    '/node_modules/bootstrap-icons/icons/cpu-fill.svg',
    '/node_modules/bootstrap-icons/icons/memory.svg',
    '/node_modules/bootstrap-icons/icons/hdd-fill.svg',
    '/node_modules/bootstrap-icons/icons/clock-fill.svg',
    '/node_modules/bootstrap-icons/icons/info-circle-fill.svg',
    '/node_modules/bootstrap-icons/icons/gear-fill.svg',
  ],
  { query: '?url', import: 'default', eager: true },
)

export type IconName =
  | 'arrow-down-circle'
  | 'arrow-up-circle'
  | 'arrow-down-circle-fill'
  | 'arrow-up-circle-fill'
  | 'cpu-fill'
  | 'memory'
  | 'hdd-fill'
  | 'clock-fill'
  | 'info-circle-fill'
  | 'gear-fill'

/** Icon is decorative: whatever it stands for must also be said in text. */
export function Icon({ name, className = '' }: { name: IconName; className?: string }) {
  const url = iconUrls[`/node_modules/bootstrap-icons/icons/${name}.svg`]
  return (
    <span
      className={`icon ${className}`}
      aria-hidden="true"
      // React sets styles through the CSSOM, which the Dashboard's CSP allows.
      style={{ maskImage: `url("${url}")`, WebkitMaskImage: `url("${url}")` }}
    />
  )
}
