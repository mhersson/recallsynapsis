import { useEffect, useId, type ReactNode } from 'react'
import './Dialog.css'

type Props = { title: ReactNode; onClose: () => void; children: ReactNode }

// The scrim ignores clicks: a stray one must not lose what was typed.
export default function Dialog({ title, onClose, children }: Props) {
  const titleId = useId()
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])
  return (
    <div className="scrim">
      <div className="dialog" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <h2 id={titleId}>{title}</h2>
        {children}
      </div>
    </div>
  )
}
