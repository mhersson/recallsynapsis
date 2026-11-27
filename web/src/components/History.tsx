import { useEffect, useRef, useState, type ReactNode } from 'react'
import { from, type Entry } from './history.ts'
import { HistoryIcon } from './icons.tsx'
import { clock } from './time.ts'
import { useNarrow } from './useNarrow.ts'
import './History.css'

type RowsProps = { list: Entry[]; current: number; fresh?: number; onRead: (revision: number) => void }

// The revisions, the newest first, each with Read but the current one; fresh is the one a restore just wrote.
export function Revisions({ list, current, fresh, onRead }: RowsProps) {
  return list.map((r) => (
    <div key={r.revision} className={r.revision === fresh ? 'hrow fresh' : 'hrow'}>
      <span className="n">revision {r.revision}</span>
      <span>{clock(r.at)}</span>
      <span className="src">{from(r.by)}</span>
      {r.revision === current ? (
        <span className="tag">current</span>
      ) : (
        <button className="btn link" onClick={() => onRead(r.revision)}>
          Read
        </button>
      )}
      {r.restored_from && <span className="src">restored revision {r.restored_from}</span>}
      {r.said && <p className="said">You said: “{r.said}”</p>}
    </div>
  ))
}

type FrameProps = {
  revision: number
  of: number
  got?: Entry
  says: string
  failed: string
  onRestore?: () => Promise<string>
  onBack: () => void
  children: ReactNode
}

// An earlier revision, once got, read in a frame that is clearly not the current text, under a notice that says what
// restoring it does; of is the current revision. onRestore gives the refusal's line, or '', and is absent where nothing
// is restored.
export function RevisionFrame({ revision, of, got, says, failed, onRestore, onBack, children }: FrameProps) {
  const narrow = useNarrow()
  const [sending, setSending] = useState(false)
  const [refused, setRefused] = useState('')
  // Read from the foot of a page, the revision opens where it begins.
  const top = useRef<HTMLDivElement>(null)
  useEffect(() => top.current?.scrollIntoView?.({ block: 'start' }), [])
  const restore = async (send: () => Promise<string>) => {
    setSending(true)
    setRefused(await send())
    setSending(false)
  }
  const acts = (
    <div className={narrow ? 'acts stack' : 'acts'}>
      {got && onRestore && (
        <button className="btn" disabled={sending} onClick={() => void restore(onRestore)}>
          Restore this revision
        </button>
      )}
      <button className="btn" onClick={onBack}>
        Back to the current text
      </button>
    </div>
  )
  return (
    <div className="revision" ref={top}>
      <div className="notice">
        <div className="nt">
          <HistoryIcon />
          Revision {revision} of {of}
          {got && `, from ${clock(got.at)}, ${from(got.by)}`}
        </div>
        <p className="nd">{says}</p>
        {!narrow && acts}
      </div>
      {(failed || refused) && (
        <p role="alert" className="err">
          {refused || failed}
        </p>
      )}
      {got && (
        <div className="rev">
          <span className="t2">
            <span>revision {got.revision}</span>
            <span>{clock(got.at)}</span>
            <span>{from(got.by)}</span>
          </span>
          {children}
        </div>
      )}
      {narrow && acts}
    </div>
  )
}
