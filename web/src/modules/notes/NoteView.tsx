import { useEffect, useEffectEvent, useRef, useState, type DragEvent } from 'react'
import { useBlocker } from 'react-router'
import { ApiError, api, message } from '../../api/client.ts'
import type { Area } from '../../api/types.ts'
import Dialog from '../../components/Dialog.tsx'
import Field from '../../components/Field.tsx'
import { Markdown } from '../../components/Markdown.tsx'
import { ago, when } from '../../components/time.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { from, mark, nameOf, useNotes, type Kind, type Note, type Version } from './api.ts'
import { Place } from './Filing.tsx'
import Files from './Files.tsx'
import History, { RevisionView } from './History.tsx'
import { AttachIcon, RestoreIcon } from './icons.tsx'
import Links from './Links.tsx'
import TagField from './TagField.tsx'
import { size, useUploads } from './uploads.ts'

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
  // The earlier revision being read, and the one a restore just brought back, as which revision and from when.
  const [reading, setReading] = useState<number>()
  const [restored, setRestored] = useState<{ from: number; at: string; as: number }>()
  // A note read, or a change's answer, that left the server before the note shown would show the note as it was. A file
  // changes no revision, so of the reads only the last one asked is shown.
  const show = (n: Note) => setNote((x) => (x && x.revision > n.revision ? x : n))
  const asked = useRef(0)
  const read = () => {
    const n = ++asked.current
    return api<Note>('GET', `/notes/${id}`).then(
      (got) => {
        if (n === asked.current) show(got)
        return got
      },
      (e: unknown) => void setFailed(message(e)),
    )
  }
  useNotes(() => void read())
  // At the note's level, so an upload goes on while the note is edited.
  const up = useUploads(id, read)
  // On desktop a file dropped on the note, or an image pasted anywhere, is attached to the note being read.
  const attachable = !narrow && !draft && !reading && !!note && !note.trashed_at
  const [over, setOver] = useState(false)
  const pasted = useEffectEvent((e: ClipboardEvent) => {
    // A paste in a dialog, such as Capture, is the dialog's. Cells copied from a spreadsheet come as text and as a picture
    // of them, and in a field the text is what was meant.
    const at = e.target instanceof Element ? e.target : null
    if (at?.closest('dialog') || (at?.closest('input, textarea, [contenteditable]') && e.clipboardData?.types.includes('text/plain'))) return
    const files = [...(e.clipboardData?.files ?? [])].filter((f) => f.type.startsWith('image/'))
    if (files.length === 0) return
    e.preventDefault()
    up.send(files)
  })
  useEffect(() => {
    if (!attachable) return
    document.addEventListener('paste', pasted)
    return () => document.removeEventListener('paste', pasted)
  }, [attachable])
  // A file dropped beside the note would have the browser leave the app to show it.
  useEffect(() => {
    const keep = (e: globalThis.DragEvent) => {
      if (e.defaultPrevented || !e.dataTransfer?.types.includes('Files')) return
      e.preventDefault()
      e.dataTransfer.dropEffect = 'none'
    }
    document.addEventListener('dragover', keep)
    document.addEventListener('drop', keep)
    return () => {
      document.removeEventListener('dragover', keep)
      document.removeEventListener('drop', keep)
    }
  }, [])
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

  // Sends a change, shows the note it answers and gives it to done. A refusal reads the note again and gives the server's
  // line, which the page shows where the change was asked for.
  async function send(method: string, path: string, body?: unknown, done?: (n: Note) => void) {
    setRefused('')
    setSending(true)
    try {
      const n = await api<Note>(method, path, body)
      show(n)
      onChanged?.()
      done?.(n)
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

  if (reading)
    return (
      <RevisionView
        note={note}
        revision={reading}
        onBack={() => setReading(undefined)}
        onRestore={(v: Version) =>
          send('POST', `/notes/${id}/revisions/${v.revision}/restore`, undefined, (n) => {
            setReading(undefined)
            setRestored({ from: v.revision, at: v.at, as: n.revision })
          })
        }
      />
    )

  const trashed = !!note.trashed_at
  const acts = !trashed && (
    <>
      <button className="btn" disabled={sending} onClick={() => setDraft({ from: note, title: note.title, body: note.body, area: note.area, kind: note.kind, tags: note.tags })}>
        Edit
      </button>
      <button className="btn danger" disabled={sending} onClick={() => void send('POST', `/notes/${id}/trash`, undefined, onGone).then(setRefused)}>
        Trash
      </button>
    </>
  )
  const files = (e: DragEvent) => e.dataTransfer.types.includes('Files')
  const drop = attachable && {
    onDragOver: (e: DragEvent) => {
      if (!files(e)) return
      e.preventDefault()
      setOver(true)
    },
    onDragLeave: (e: DragEvent) => !e.currentTarget.contains(e.relatedTarget as Node) && setOver(false),
    onDrop: (e: DragEvent) => {
      if (!files(e)) return
      e.preventDefault()
      setOver(false)
      up.send([...e.dataTransfer.files])
    },
  }
  return (
    <div className={over ? 'note-read dropping' : 'note-read'} {...drop}>
      {over && (
        <div className="drop">
          <div>
            <AttachIcon />
            Drop to attach it to this note
            <span>Any type{up.cap > 0 && `, at most ${size(up.cap)}`}.</span>
          </div>
        </div>
      )}
      {restored?.as === note.revision && (
        <div className="notice restored">
          <div className="nt">
            <RestoreIcon />
            Restored revision {restored.from} as revision {restored.as}.
          </div>
          <p className="nd">The note reads as it did {when(restored.at)}. Every revision is kept, so this can be undone the same way.</p>
        </div>
      )}
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
        <Files note={note} trashed={trashed} up={up} onRemoved={() => void read()} />
        <History note={note} fresh={restored?.as} onRead={setReading} />
      </div>
      {narrow && acts && <div className="acts stack note-foot">{acts}</div>}
    </div>
  )
}
