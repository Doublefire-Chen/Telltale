// Bootstrap Icons (MIT), the set Nezha's themes use, bundled at build
// time. Each is drawn as a CSS mask, so it takes the colour of its class.
const iconUrls = import.meta.glob<string>(
  [
    '/node_modules/bootstrap-icons/icons/arrow-down-circle.svg',
    '/node_modules/bootstrap-icons/icons/arrow-up-circle.svg',
    '/node_modules/bootstrap-icons/icons/arrow-down-circle-fill.svg',
    '/node_modules/bootstrap-icons/icons/arrow-up-circle-fill.svg',
    '/node_modules/bootstrap-icons/icons/cpu-fill.svg',
    '/node_modules/bootstrap-icons/icons/memory.svg',
    '/node_modules/bootstrap-icons/icons/hdd-rack-fill.svg',
    '/node_modules/bootstrap-icons/icons/clock-fill.svg',
    '/node_modules/bootstrap-icons/icons/info-circle-fill.svg',
    '/node_modules/bootstrap-icons/icons/gear-fill.svg',
    '/node_modules/bootstrap-icons/icons/box-arrow-in-right.svg',
    '/node_modules/bootstrap-icons/icons/box-arrow-right.svg',
    '/node_modules/bootstrap-icons/icons/server.svg',
    '/node_modules/bootstrap-icons/icons/gear.svg',
    '/node_modules/bootstrap-icons/icons/pencil-square.svg',
    '/node_modules/bootstrap-icons/icons/trash3.svg',
    '/node_modules/bootstrap-icons/icons/plus-lg.svg',
    '/node_modules/bootstrap-icons/icons/key.svg',
    '/node_modules/bootstrap-icons/icons/arrow-up.svg',
    '/node_modules/bootstrap-icons/icons/arrow-down.svg',
    '/node_modules/bootstrap-icons/icons/clipboard.svg',
    '/node_modules/bootstrap-icons/icons/check-lg.svg',
    '/node_modules/bootstrap-icons/icons/house.svg',
    '/node_modules/bootstrap-icons/icons/graph-up.svg',
    '/node_modules/bootstrap-icons/icons/arrow-left.svg',
    '/node_modules/bootstrap-icons/icons/box-arrow-left.svg',
    '/node_modules/bootstrap-icons/icons/person-circle.svg',
    '/node_modules/bootstrap-icons/icons/collection.svg',
    '/node_modules/bootstrap-icons/icons/chevron-down.svg',
    '/node_modules/bootstrap-icons/icons/x-lg.svg',
    '/node_modules/bootstrap-icons/icons/ubuntu.svg',
    '/node_modules/bootstrap-icons/icons/windows.svg',
    '/node_modules/bootstrap-icons/icons/apple.svg',
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
  | 'hdd-rack-fill'
  | 'clock-fill'
  | 'info-circle-fill'
  | 'gear-fill'
  | 'box-arrow-in-right'
  | 'box-arrow-right'
  | 'server'
  | 'gear'
  | 'pencil-square'
  | 'trash3'
  | 'plus-lg'
  | 'key'
  | 'arrow-up'
  | 'arrow-down'
  | 'clipboard'
  | 'check-lg'
  | 'house'
  | 'graph-up'
  | 'arrow-left'
  | 'box-arrow-left'
  | 'person-circle'
  | 'collection'
  | 'chevron-down'
  | 'x-lg'
  | 'ubuntu'
  | 'windows'
  | 'apple'

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
