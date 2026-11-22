import { useEffect, useState } from 'react'
import { useBlocker } from 'react-router'
import { ApiError, api, message } from '../../api/client.ts'
import type { Area } from '../../api/types.ts'
import Dialog from '../../components/Dialog.tsx'
import Field from '../../components/Field.tsx'
import { Markdown } from '../../components/Markdown.tsx'
import { ago } from '../../components/time.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { from, mark, nameOf, useNotes, type Kind, type Note } from './api.ts'
import { Place } from './Filing.tsx'
import Links from './Links.tsx'
import TagField from './TagField.tsx'

type Props = { id: number; areas: Area[]; onGone: () => void; onChanged?: () => void }

// What is typed, and the note as the editing began; moved once a save was refused because the note moved on, and shown
// while the note as it now is shows above the text typed.
type Draft = { from: Note; title: string; body: string; area: string; kind: Kind; tags: string[]; moved?: boolean; shown?: boolean }

// What differs from the note as the editing began: only that is saved, so a change made elsewhere meanwhile stands,
// and a capture is filed only by a change of its area, its kind or its tags.
const changeOf = (d: Draft) => ({
  ...(d.title !== d.from.title && { title: d.title }),
  ...(d.body !== d.from.body && { body: d.body }),
  ...(d.area !== d.from.area && { area: d.area }),
  ...(d.kind !== d.from.kind && { kind: d.kind }),
  ...(d.tags.join(' ') !== d.from.tags.join(' ') && { tags: d.tags }),
})

// A note, read first. Edit swaps the text for a Markdown text area, and the note read again meanwhile never replaces
// what is typed. onChanged tells of a change made here; onGone closes the note once it is trashed.
export default function NoteView({ id, areas, onGone, onChanged }: Props) {
  const narrow = useNarrow()
  const [note, setNote] = useState<Note>()
  const [failed, setFailed] = useState('')
  const [draft, setDraft] = useState<Draft>()
  const [sending, setSending] = useState(false)
  const [refused, setRefused] = useState('')
  // A note read, or a change's answer, that left the server before the note shown would show the note as it was.
  const show = (n: Note) => setNote((x) => (x && x.revision > n.revision ? x : n))
  const read = () =>
    api<Note>('GET', `/notes/${id}`).then(
      (n) => {
        show(n)
        return n
      },
      (e: unknown) => void setFailed(message(e)),
    )
  useNotes(() => void read())
  // Leaving the note with what is typed unsaved asks first. A change of the filters keeps the note open, and asks nothing.
  const typed = !!draft && Object.keys(changeOf(draft)).length > 0
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

  if (!note)
    return (
      failed && (
        <p role="alert" className="err">
          {failed}
        </p>
      )
    )

  // Sends a change and shows the note it answers. A refusal reads the note again and gives the server's line, which the
  // page shows where the change was asked for.
  async function send(method: string, path: string, body?: unknown) {
    setRefused('')
    setSending(true)
    try {
      show(await api<Note>(method, path, body))
      onChanged?.()
      return ''
    } catch (e) {
      void read()
      return message(e)
    } finally {
      setSending(false)
    }
  }

  // A save refused because the note moved on keeps what was typed and reads the note again.
  async function save(d: Draft, revision: number) {
    const change = changeOf(d)
    if (Object.keys(change).length === 0) return setDraft(undefined)
    setRefused('')
    setSending(true)
    try {
      show(await api<Note>('PATCH', `/notes/${id}`, { revision, ...change }))
      setDraft(undefined)
      onChanged?.()
    } catch (e) {
      const now = e instanceof ApiError && e.status === 409 ? await read() : undefined
      if (now) setDraft((x) => x && { ...x, moved: !now.trashed_at })
      if (!now || now.trashed_at) setRefused(now ? 'This note was trashed while you were editing, so your text was not saved.' : message(e))
    }
    setSending(false)
  }

  const alert = refused && (
    <p role="alert" className="err">
      {refused}
    </p>
  )

  if (draft) {
    const set = (d: Partial<Draft>) => setDraft({ ...draft, ...d })
    return (
      <div className="note-edit">
        {draft.moved && (
          <div className="notice">
            <div className="nt">
              <i className="mk wait" />
              This note changed while you were editing
            </div>
            <p className="nd">
              Revision {note.revision} was saved {ago(note.changed_at)}, {from(note.source)}. Your text was not saved, and it is kept below.
            </p>
            <div className={narrow ? 'acts stack' : 'acts'}>
              <button className="btn" onClick={() => set({ shown: !draft.shown })}>
                {draft.shown ? 'Hide it' : 'Show it as it is now'}
              </button>
            </div>
          </div>
        )}
        {alert}
        <Field label="Title">
          <input className="title" autoComplete="off" value={draft.title} onChange={(e) => set({ title: e.target.value })} />
        </Field>
        {draft.moved && draft.shown && (
          <div className="field">
            <span className="label">
              As it is now{' '}
              <span className="t2">
                <span>revision {note.revision}</span>
                <span>{ago(note.changed_at)}</span>
                <span>{from(note.source)}</span>
              </span>
            </span>
            <pre className="ta now">{note.body}</pre>
          </div>
        )}
        <Field
          label={
            draft.moved ? (
              <>
                Yours{' '}
                <span className="t2">
                  <span>from revision {draft.from.revision}</span>
                </span>
              </>
            ) : (
              'Text'
            )
          }
        >
          <textarea className="ta" autoFocus value={draft.body} onChange={(e) => set({ body: e.target.value })} />
        </Field>
        <Place areas={areas.filter((a) => !a.archived || a.slug === draft.from.area)} narrow={narrow} area={draft.area} kind={draft.kind} onChange={set} />
        <div className="field">
          <span className="label">Tags</span>
          <TagField tags={draft.tags} onChange={(tags) => set({ tags })} />
        </div>
        <div className={narrow ? 'acts stack' : 'acts'}>
          {draft.moved ? (
            <>
              <button className="btn" disabled={sending} onClick={() => void save(draft, note.revision)}>
                Save mine over it
              </button>
              <button className="btn danger" onClick={() => setDraft(undefined)}>
                Give up mine
              </button>
            </>
          ) : (
            <>
              <button className="btn primary" disabled={sending} onClick={() => void save(draft, draft.from.revision)}>
                Save
              </button>
              <button className="btn" onClick={() => setDraft(undefined)}>
                Cancel
              </button>
            </>
          )}
          {!narrow && (
            <>
              <span className="spacer" />
              <span className="t2">{draft.moved ? 'Markdown' : `Markdown · from revision ${draft.from.revision}`}</span>
            </>
          )}
        </div>
        {leaving.state === 'blocked' && (
          <Dialog title="Give up what you typed?" onClose={() => leaving.reset()}>
            <p className="note">It is not saved, and leaving the note loses it.</p>
            <div className="acts">
              <button className="btn" onClick={() => leaving.reset()}>
                Keep editing
              </button>
              <button className="btn danger" onClick={() => leaving.proceed()}>
                Give up mine
              </button>
            </div>
          </Dialog>
        )}
      </div>
    )
  }

  const trashed = !!note.trashed_at
  const acts = !trashed && (
    <>
      <button className="btn" disabled={sending} onClick={() => setDraft({ from: note, title: note.title, body: note.body, area: note.area, kind: note.kind, tags: note.tags })}>
        Edit
      </button>
      <button className="btn danger" disabled={sending} onClick={() => void send('POST', `/notes/${id}/trash`).then((line) => (line ? setRefused(line) : onGone()))}>
        Trash
      </button>
    </>
  )
  return (
    <div className="note-read">
      <div className="page-head">
        {note.title && <h1>{note.title}</h1>}
        {!narrow && acts && (
          <>
            <span className="spacer" />
            {acts}
          </>
        )}
      </div>
      <span className="t2 note-meta">
        <i className={`mk ${mark(note.kind, note.inbox)}`} />
        <span>{note.kind}</span>
        <span>{nameOf(areas, note.area)}</span>
        {note.inbox && <span>in the inbox</span>}
        <span>changed {ago(note.changed_at)}</span>
        <span>{from(note.source)}</span>
        <span>revision {note.revision}</span>
      </span>
      {trashed && (
        <div className="notice">
          <div className="nt">In the trash</div>
          <p className="nd">It cannot be changed, and no list, search or link finds it until it is restored.</p>
        </div>
      )}
      {alert}
      <Markdown text={note.body} />
      <div className="note-tags">
        {trashed ? (
          note.tags.map((t) => (
            <span key={t} className="tag">
              #{t}
            </span>
          ))
        ) : (
          // A tag added or removed here is a change of the note, which files a capture.
          <TagField tags={note.tags} waiting={sending} onChange={(tags) => void send('PATCH', `/notes/${id}`, { revision: note.revision, tags }).then(setRefused)} />
        )}
      </div>
      <div className="sects">
        <Links note={note} areas={areas} trashed={trashed} sending={sending} onLinks={(links) => send('PATCH', `/notes/${id}`, { revision: note.revision, links })} />
      </div>
      {narrow && acts && <div className="acts stack note-foot">{acts}</div>}
    </div>
  )
}
