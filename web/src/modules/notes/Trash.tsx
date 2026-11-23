import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { api, message } from '../../api/client.ts'
import { nameOf, useAreas } from '../../api/areas.ts'
import Dialog from '../../components/Dialog.tsx'
import { RestoreIcon } from '../../components/icons.tsx'
import ListEnd from '../../components/ListEnd.tsx'
import PageHead from '../../components/PageHead.tsx'
import { useClose } from '../../components/useClose.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { count, useList, type Note } from './api.ts'
import NoteView from './NoteView.tsx'
import { Rows } from './Rows.tsx'
import './Notes.css'

// What was trashed, the most recently trashed first, with the note open beside the list on desktop and as a page of
// its own on a phone. Nothing empties the trash but the owner.
export default function Trash() {
  const narrow = useNarrow()
  const navigate = useNavigate()
  const id = Number(useParams().id) || undefined
  const close = useClose('/trash')
  const areas = useAreas(true)
  const alone = narrow && !!id
  const { list, failed, more, reread } = useList(alone ? '' : 'trashed=true')
  // The note last restored here, and whether one was restored or deleted: after either, none opens unasked.
  const [restored, setRestored] = useState<Note>()
  const [settled, setSettled] = useState(false)
  const [emptying, setEmptying] = useState(false)
  const [refused, setRefused] = useState('')
  const newest = list?.notes[0]?.id
  useEffect(() => {
    if (!narrow && !id && !settled && newest) void navigate(`/trash/${newest}`, { replace: true })
  }, [narrow, id, settled, newest, navigate])

  // On a phone the note's page goes back to the list; on desktop the list stays, with no note open.
  const settle = (n?: Note) => {
    setSettled(true)
    setRestored(n)
    if (narrow) close()
    else void navigate('/trash', { replace: true })
  }
  const empty = () => {
    setEmptying(false)
    api('DELETE', '/notes/trash').then(
      () => {
        setRefused('')
        reread()
        if (id) settle()
      },
      (e: unknown) => setRefused(message(e)),
    )
  }

  if (alone)
    return (
      <div className="page narrow">
        <div className="crumb">
          <Link to="/trash">Trash</Link> /
        </div>
        <NoteView key={id} id={id} areas={areas} onGone={() => settle()} onRestored={settle} onChanged={reread} />
      </div>
    )

  const total = list?.total ?? 0
  const emptyIt = total > 0 && (
    <button className="btn danger" onClick={() => setEmptying(true)}>
      Empty the trash
    </button>
  )
  return (
    <div className="page notes">
      {narrow && (
        <div className="crumb">
          <Link to="/more">More</Link> /
        </div>
      )}
      <PageHead title="Trash">{!narrow && emptyIt}</PageHead>
      {list && (
        <p className="sub">
          {total > 0
            ? `${count(total, 'note')}, the most recently trashed first. Nothing empties the trash on its own.`
            : 'A trashed note waits here until you restore it or empty the trash.'}
        </p>
      )}
      {restored && (
        <div className="notice">
          <div className="nt">
            <RestoreIcon />
            {restored.title || restored.excerpt} is restored to {nameOf(areas, restored.area)}.
          </div>
          <p className="nd">
            It is back where it was, with its history and its attachments. <Link to={`/notes/${restored.id}`}>Open it</Link>
          </p>
        </div>
      )}
      {(failed || refused) && (
        <p role="alert" className="err">
          {refused || failed}
        </p>
      )}
      <div className={narrow ? undefined : 'panes'}>
        <div>
          {list && total === 0 && (
            <div className="empty">
              <b>The trash is empty.</b>
            </div>
          )}
          {list && <Rows notes={list.notes} areas={areas} to={(r) => `/trash/${r.id}`} open={id} trashed />}
          {list && <ListEnd shown={list.notes.length} total={list.total} what="trashed" more={more} />}
        </div>
        {!narrow &&
          (id ? (
            <div className="box">
              <NoteView key={id} id={id} areas={areas} onGone={() => settle()} onRestored={settle} onChanged={reread} />
            </div>
          ) : (
            total > 0 && <div className="empty">Pick a note to read it here.</div>
          ))}
      </div>
      {narrow && emptyIt && <div className="acts stack note-foot">{emptyIt}</div>}
      {emptying && (
        <Dialog title="Empty the trash?" onClose={() => setEmptying(false)}>
          <p className="note">Emptying it deletes the {count(total, 'note')} in it for good, with their history and their attachments. This cannot be undone.</p>
          <div className="acts">
            <button className="btn" onClick={() => setEmptying(false)}>
              Keep them
            </button>
            <button className="btn danger" onClick={empty}>
              Empty the trash
            </button>
          </div>
        </Dialog>
      )}
    </div>
  )
}
