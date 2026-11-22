import { useEffect, useRef, useState } from 'react'
import { api, message } from '../../api/client.ts'
import { Markdown } from '../../components/Markdown.tsx'
import { clock } from '../../components/time.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { count, from, useRead, type Note, type Revision, type Version } from './api.ts'
import { HistoryIcon } from './icons.tsx'
import Section from './Section.tsx'

// The note's revisions, the newest first, read again whenever the note moves on. fresh is the one a restore just wrote.
export default function History({ note, fresh, onRead }: { note: Note; fresh?: number; onRead: (revision: number) => void }) {
  const [list, setList] = useState<Revision[]>()
  const [failed, setFailed] = useState('')
  useEffect(() => {
    let stale = false
    api<{ revisions: Revision[] }>('GET', `/notes/${note.id}/revisions`).then(
      (r) => {
        if (stale) return
        setList(r.revisions)
        setFailed('')
      },
      (e: unknown) => !stale && setFailed(message(e)),
    )
    return () => {
      stale = true
    }
  }, [note.id, note.revision])

  return (
    <Section title="History" count={count(note.revision, 'revision')}>
      {failed && (
        <p role="alert" className="err">
          {failed}
        </p>
      )}
      {list?.map((r) => (
        <div key={r.revision} className={r.revision === fresh ? 'hrow fresh' : 'hrow'}>
          <span className="n">revision {r.revision}</span>
          <span>{clock(r.at)}</span>
          <span className="src">{from(r.source)}</span>
          {r.revision === note.revision ? (
            <span className="tag">current</span>
          ) : (
            <button className="btn link" onClick={() => onRead(r.revision)}>
              Read
            </button>
          )}
          {r.restored_from && <span className="src">restored revision {r.restored_from}</span>}
          {r.said && <p className="said">You said: “{r.said}”</p>}
        </div>
      ))}
    </Section>
  )
}

type ReadProps = { note: Note; revision: number; onRestore: (v: Version) => Promise<string>; onBack: () => void }

// An earlier revision, read in a frame that is clearly not the current text. Restoring it is a change of the note,
// which a note in the trash takes none of.
export function RevisionView({ note, revision, onRestore, onBack }: ReadProps) {
  const narrow = useNarrow()
  const { got: v, failed } = useRead<Version>(`/notes/${note.id}/revisions/${revision}`)
  const [sending, setSending] = useState(false)
  const [refused, setRefused] = useState('')
  // Read from the foot of the note, the revision opens where it begins.
  const top = useRef<HTMLDivElement>(null)
  useEffect(() => top.current?.scrollIntoView?.({ block: 'start' }), [])
  const restore = async (v: Version) => {
    setSending(true)
    setRefused(await onRestore(v))
    setSending(false)
  }
  const acts = (
    <div className={narrow ? 'acts stack note-foot' : 'acts'}>
      {v && !note.trashed_at && (
        <button className="btn" disabled={sending} onClick={() => void restore(v)}>
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
          Revision {revision} of {note.revision}
          {v && `, from ${clock(v.at)}, ${from(v.source)}`}
        </div>
        <p className="nd">
          This is how the note read then, not its current text. Restoring brings back its title, text, tags and links as revision {note.revision + 1}. The
          area and the kind stay.
        </p>
        {!narrow && acts}
      </div>
      {(failed || refused) && (
        <p role="alert" className="err">
          {refused || failed}
        </p>
      )}
      {v && (
        <div className="rev">
          <span className="t2">
            <span>revision {v.revision}</span>
            <span>{clock(v.at)}</span>
            <span>{from(v.source)}</span>
          </span>
          {v.title && <h2 className="rt">{v.title}</h2>}
          <Markdown text={v.body} />
          <div className="note-tags">
            {v.tags.map((t) => (
              <span key={t} className="tag">
                #{t}
              </span>
            ))}
          </div>
        </div>
      )}
      {narrow && acts}
    </div>
  )
}
