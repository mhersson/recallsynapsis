import { useEffect } from 'react'
import { createPortal } from 'react-dom'
import { useBlocker } from 'react-router'
import Dialog from './Dialog.tsx'

// While changed is true, leaving what is edited inside the app asks first, and closing the tab gets the browser's own
// question; what names it in the question. A change of the query alone, such as a list's filters, asks nothing. It gives
// the question to show.
export function useUnsaved(changed: boolean, what: string) {
  const leaving = useBlocker(({ currentLocation, nextLocation }) => changed && currentLocation.pathname !== nextLocation.pathname)
  // A save that lands while the way out is held leaves nothing to lose, so the way out goes on.
  useEffect(() => {
    if (!changed && leaving.state === 'blocked') leaving.proceed()
  }, [changed, leaving])
  useEffect(() => {
    if (!changed) return
    const ask = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', ask)
    return () => window.removeEventListener('beforeunload', ask)
  }, [changed])
  // The question is the page's, so it goes to the body, where a folded part of a page cannot hide it.
  return (
    leaving.state === 'blocked' &&
    createPortal(
      <Dialog title="Give up your changes?" onClose={() => leaving.reset()}>
        <p className="note">They are not saved, and leaving {what} loses them.</p>
        <div className="acts">
          <button className="btn" onClick={() => leaving.reset()}>
            Keep editing
          </button>
          <button className="btn danger" onClick={() => leaving.proceed()}>
            Give up mine
          </button>
        </div>
      </Dialog>,
      document.body,
    )
  )
}
