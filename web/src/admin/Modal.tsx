import { type ReactNode, useEffect, useId, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../Icon'

type ModalProps = {
  title: string
  /** "tiny" for forms, "mini" for confirmations, as in Semantic UI. */
  size: 'tiny' | 'mini'
  onClose: () => void
  children: ReactNode
  actions: ReactNode
}

/**
 * Modal is a Semantic UI modal over a dark dimmer: Escape or a click on the
 * dimmer closes it, and it takes focus while open.
 */
export function Modal({ title, size, onClose, children, actions }: ModalProps) {
  const titleId = useId()
  const box = useRef<HTMLDivElement>(null)
  const close = useRef(onClose)
  close.current = onClose

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    const first = box.current?.querySelector<HTMLElement>('input, textarea, select, button.positive, button')
    first?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close.current()
    }
    addEventListener('keydown', onKey)
    document.body.classList.add('dimmed')
    return () => {
      removeEventListener('keydown', onKey)
      document.body.classList.remove('dimmed')
      previous?.focus?.()
    }
  }, [])

  return (
    <div className="ui dimmer" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div ref={box} className={`ui ${size} modal`} role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <h2 id={titleId} className="header">
          {title}
        </h2>
        <div className="content">{children}</div>
        <div className="actions">{actions}</div>
      </div>
    </div>
  )
}

type ConfirmProps = {
  title: string
  body: string
  busy?: boolean
  onConfirm: () => void
  onCancel: () => void
}

/** Confirm is Nezha's mini confirmation modal: Cancel, or Confirm ✓. */
export function Confirm({ title, body, busy = false, onConfirm, onCancel }: ConfirmProps) {
  const { t } = useTranslation()
  return (
    <Modal
      title={title}
      size="mini"
      onClose={onCancel}
      actions={
        <>
          <button type="button" className="ui negative button" onClick={onCancel}>
            {t('admin.cancel')}
          </button>
          <button
            type="button"
            className={`ui positive primary right labeled button${busy ? ' loading' : ''}`}
            disabled={busy}
            onClick={onConfirm}
          >
            {t('admin.confirm')}
            <Icon name="check-lg" />
          </button>
        </>
      }
    >
      <p>{body}</p>
    </Modal>
  )
}

export type ToastMessage = { kind: 'success' | 'warning' | 'error'; text: string }

/** Toast is a short-lived top-centre alert, like Nezha's suiAlert. */
export function Toast({ toast, onDone }: { toast: ToastMessage | null; onDone: () => void }) {
  const done = useRef(onDone)
  done.current = onDone
  useEffect(() => {
    if (!toast) return
    const timer = setTimeout(() => done.current(), 3000)
    return () => clearTimeout(timer)
  }, [toast])
  if (!toast) return null
  return (
    <div className={`ui toast ${toast.kind}`} role={toast.kind === 'success' ? 'status' : 'alert'}>
      {toast.text}
    </div>
  )
}
