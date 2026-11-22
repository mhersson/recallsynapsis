import { useState } from 'react'
import { ApiError, api, message } from '../../api/client.ts'
import type { Area } from '../../api/types.ts'
import Field from '../../components/Field.tsx'
import { Markdown } from '../../components/Markdown.tsx'
import { ago } from '../../components/time.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { from, mark, useNotes, type Note } from './api.ts'
import TagField from './TagField.tsx'

type Props = { id: number; areas: Area[]; onGone: () => void; onChanged?: () => void }

// What is typed, and the note as the editing began; moved once a save was refused because the note moved on, and shown
// while the note as it now is shows above the text typed.
type Draft = { from: Note; title: string; body: string; tags: string[]; moved?: boolean; shown?: boolean }

// A note, read first. Edit swaps the text for a Markdown text area, and the note read again meanwhile never replaces
// what is typed. onChanged tells of a change made here; onGone closes the note once it is trashed.
export default function NoteView({ id, areas, onGone, onChanged }: Props) {
  const narrow = useNarrow()
  const [note, setNote] = useState<Note>()
  const [failed, setFailed] = useState('')
  const [draft, setDraft] = useState<Draft>()
  const [sending, setSending] = useState(false)
  const [refused, setRefused] = useState('')
  const read = () =>
    api<Note>('GET', `/notes/${id}`).then(
      (n) => {
        // An answer that left the server before a change made here would show the note as it was.
        setNote((x) => (x && x.revision > n.revision ? x : n))
        return n
      },
      (e: unknown) => void setFailed(message(e)),
    )
  useNotes(() => void read())

  if (!note)
    return (
      failed && (
        <p role="alert" className="err">
          {failed}
        </p>
      )
    )

  async function send(method: string, path: string, body?: unknown) {
    setRefused('')
    setSending(true)
    try {
      setNote(await api<Note>(method, path, body))
      onChanged?.()
      return true
    } catch (e) {
      setRefused(message(e))
      void read()
      return false
    } finally {
      setSending(false)
    }
  }

  // Only what was changed here is sent, so a change made elsewhere meanwhile stands, and a capture is filed only by a
  // change of its tags. A save refused because the note moved on keeps what was typed and reads the note again.
  async function save(d: Draft, revision: number) {
    const change = {
      ...(d.title !== d.from.title && { title: d.title }),
      ...(d.body !== d.from.body && { body: d.body }),
      ...(d.tags.join(' ') !== d.from.tags.join(' ') && { tags: d.tags }),
    }
    if (Object.keys(change).length === 0) return setDraft(undefined)
    setRefused('')
    setSending(true)
    try {
      setNote(await api<Note>('PATCH', `/notes/${id}`, { revision, ...change }))
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
      </div>
    )
  }

  const trashed = !!note.trashed_at
  const acts = !trashed && (
    <>
      <button className="btn" disabled={sending} onClick={() => setDraft({ from: note, title: note.title, body: note.body, tags: note.tags })}>
        Edit
      </button>
      <button className="btn danger" disabled={sending} onClick={() => void send('POST', `/notes/${id}/trash`).then((ok) => ok && onGone())}>
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
        <span>{areas.find((a) => a.slug === note.area)?.name ?? note.area}</span>
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
          <TagField tags={note.tags} waiting={sending} onChange={(tags) => void send('PATCH', `/notes/${id}`, { revision: note.revision, tags })} />
        )}
      </div>
      {narrow && acts && <div className="acts stack note-foot">{acts}</div>}
    </div>
  )
}
