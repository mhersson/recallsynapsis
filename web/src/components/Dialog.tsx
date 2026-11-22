import { useId, useLayoutEffect, useRef, type ReactNode } from 'react'
import './Dialog.css'

type Props = { title: ReactNode; onClose: () => void; children: ReactNode }

// The browser's modal dialog: the page behind it is inert, and closing it returns the focus to what opened it. Escape
// only asks the owner to close it, so it stays open as long as the owner shows it. The focus starts on its first control,
// as an autoFocus inside it does nothing. A click on the backdrop does nothing: a stray one must not lose what was typed.
export default function Dialog({ title, onClose, children }: Props) {
  const titleId = useId()
  const ref = useRef<HTMLDialogElement>(null)
  // Closed in a layout effect, which runs before it leaves the page: one removed while open returns no focus.
  useLayoutEffect(() => {
    const dialog = ref.current!
    dialog.showModal()
    return () => dialog.close()
  }, [])
  return (
    <dialog
      ref={ref}
      className="dialog"
      aria-labelledby={titleId}
      onCancel={(e) => {
        e.preventDefault()
        onClose()
      }}
    >
      <h2 id={titleId}>{title}</h2>
      {children}
    </dialog>
  )
}
