import { useEffect, useState } from 'react'
import { api } from '../../api/client.ts'
import { useAreas } from '../../api/areas.ts'
import PageHead from '../../components/PageHead.tsx'
import { ago } from '../../components/time.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import type { Named } from '../registry.ts'
import { from, useNotes, type List, type Note, type Row } from './api.ts'
import Filing from './Filing.tsx'
import MadeNotice from './MadeNotice.tsx'
import './Inbox.css'

function count(n: number, narrow: boolean) {
  if (n === 0) return 'Captures wait here until you file them.'
  if (n === 1) return 'One capture waits to be sorted.'
  return `${n} captures wait to be sorted${narrow ? '' : ', the most recent first'}.`
}

// The captures that wait, the most recently changed first, each filed in place: beside the list on desktop, within it
// on a phone.
export default function Inbox() {
  const narrow = useNarrow()
  const areas = useAreas()
  const [list, setList] = useState<List | null>(null)
  // The capture open for filing stays open while it waits, however the list is sorted again; else the newest opens.
  const [chosen, setChosen] = useState(0)
  const open = list?.notes.find((r) => r.id === chosen)
  // It is read in full again whenever the list shows it at a new revision.
  const [note, setNote] = useState<Note>()
  // What was last made of a capture here, until the page is left.
  const [made, setMade] = useState<Named>()
  const [id, revision] = [open?.id, open?.revision]
  useEffect(() => {
    if (!id) return
    let stale = false
    api<Note>('GET', `/notes/${id}`).then((n) => !stale && setNote(n), () => {})
    return () => {
      stale = true
    }
  }, [id, revision])

  const read = () =>
    void api<List>('GET', '/notes?inbox=true').then(
      (l) => {
        setList(l)
        setChosen((c) => (l.notes.some((r) => r.id === c) ? c : (l.notes[0]?.id ?? 0)))
      },
      () => {},
    )
  useNotes(read)

  if (!list) return null
  const filing = note && note.id === id && <Filing key={note.id} note={note} areas={areas} narrow={narrow} onDone={read} onMade={setMade} />
  const text = (r: Row) => r.title || r.excerpt
  const meta = (r: Row) => (
    <span className="t2">
      <span>{ago(r.changed_at)}</span>
      <span>{from(r.source)}</span>
    </span>
  )
  const empty = list.notes.length === 0 && (
    <div className="empty">
      <b>Nothing waits in the inbox.</b>
      What you capture, and what an AI client captures without saying where, lands here.
    </div>
  )

  return (
    <div className="page inbox">
      <PageHead title="Inbox" />
      <p className="sub">{count(list.total, narrow)}</p>
      {made && <MadeNotice made={made}>The capture is in the trash.</MadeNotice>}
      {narrow ? (
        <>
          {empty}
          {list.notes.map((r) => (
            <div key={r.id} className="strip">
              {r === open && filing ? (
                filing
              ) : (
                <>
                  <p className="cap">{text(r)}</p>
                  <div className="fold">
                    {meta(r)}
                    <button className="btn" onClick={() => setChosen(r.id)}>
                      File
                    </button>
                  </div>
                </>
              )}
            </div>
          ))}
        </>
      ) : (
        <div className="panes">
          {empty || (
            <div className="list">
              {list.notes.map((r) => (
                <button key={r.id} className="pick" aria-current={r === open || undefined} onClick={() => setChosen(r.id)}>
                  <i className="mk wait" />
                  <span>
                    <span className="t1 wrap">{text(r)}</span>
                    {meta(r)}
                  </span>
                </button>
              ))}
            </div>
          )}
          {filing && <div className="box">{filing}</div>}
        </div>
      )}
    </div>
  )
}
