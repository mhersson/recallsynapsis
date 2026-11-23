import { useEffect } from 'react'
import { useBlocker } from 'react-router'
import Dialog from './Dialog.tsx'

// While typed is true, leaving what is edited inside the app asks first, and closing the tab gets the browser's own
// question; what names it in the question. A change of the query alone, such as a list's filters, asks nothing. It gives
// the question to show.
export function useUnsaved(typed: boolean, what: string) {
  const leaving = useBlocker(({ currentLocation, nextLocation }) => typed && currentLocation.pathname !== nextLocation.pathname)
  // A save that lands while the way out is held leaves nothing to lose, so the way out goes on.
  useEffect(() => {
    if (!typed && leaving.state === 'blocked') leaving.proceed()
  }, [typed, leaving])
  useEffect(() => {
    if (!typed) return
    const ask = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', ask)
    return () => window.removeEventListener('beforeunload', ask)
  }, [typed])
  return (
    leaving.state === 'blocked' && (
      <Dialog title="Give up what you typed?" onClose={() => leaving.reset()}>
        <p className="note">It is not saved, and leaving {what} loses it.</p>
        <div className="acts">
          <button className="btn" onClick={() => leaving.reset()}>
            Keep editing
          </button>
          <button className="btn danger" onClick={() => leaving.proceed()}>
            Give up mine
          </button>
        </div>
      </Dialog>
    )
  )
}
