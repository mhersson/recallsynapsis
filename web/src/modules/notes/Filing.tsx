import { useState } from 'react'
import { Link } from 'react-router'
import { api, message } from '../../api/client.ts'
import type { Area } from '../../api/types.ts'
import { Markdown } from '../../components/Markdown.tsx'
import Seg from '../../components/Seg.tsx'
import { ago } from '../../components/time.ts'
import { back } from '../../components/useClose.ts'
import { useModules } from '../../session.ts'
import type { Maker, Named } from '../registry.ts'
import { from, kinds, type Kind, type Note } from './api.ts'
import TagField from './TagField.tsx'

type Props = { note: Note; areas: Area[]; narrow: boolean; onDone: () => void; onMade: (made: Named) => void }

// Files a capture where it stands, or makes another module's item of it in the area chosen, with what the owner said on
// it, and trashes it. Read again at a new revision, it shows as it now is but for what was chosen here.
export default function Filing({ note, areas, narrow, onDone, onMade }: Props) {
  const makers = useModules().flatMap((m) => (m.make ? [m.make] : []))
  const [chosen, setChosen] = useState<{ area?: string; kind?: Kind; tags?: string[] }>({})
  const { area = note.area, kind = note.kind, tags = note.tags } = chosen
  const choose = (c: typeof chosen) => setChosen({ ...chosen, ...c })
  const [sending, setSending] = useState(false)
  const [refused, setRefused] = useState('')

  async function send(method: string, path: string, body?: unknown) {
    setRefused('')
    setSending(true)
    try {
      await api(method, path, body)
    } catch (e) {
      setRefused(message(e))
      setSending(false)
    }
    // Read again after a refusal too, so a capture that has moved on shows as it now is.
    onDone()
  }

  // The capture goes to the trash only once what is made of it is made, so a refusal leaves it as it was; what is made is
  // told of at once, so a capture that then stays is not made again unawares.
  async function make(m: Maker) {
    setRefused('')
    setSending(true)
    try {
      onMade(await m.atOnce(note.title ? `${note.title}\n${note.body}` : note.body, area, note.said))
      await api('POST', `/notes/${note.id}/trash`)
    } catch (e) {
      setRefused(message(e))
      setSending(false)
    }
    onDone()
  }

  return (
    <div className="filing">
      {note.title && <h2 className="cap-title">{note.title}</h2>}
      <div className="cap">
        <Markdown text={note.body} />
      </div>
      <span className="t2">
        <span>{ago(note.changed_at)}</span>
        <span>{from(note.source)}</span>
      </span>
      {note.said && (
        <p className="said">
          <span className="label">You said</span>
          {note.said}
        </p>
      )}
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
      <Place areas={areas} narrow={narrow} area={area} kind={kind} onChange={choose} />
      <div className="field">
        <span className="label">Tags</span>
        <TagField tags={tags} onChange={(v) => choose({ tags: v })} />
      </div>
      <div className={narrow ? 'acts stack' : 'acts'}>
        <button className="btn primary" disabled={sending} onClick={() => void send('PATCH', `/notes/${note.id}`, { revision: note.revision, area, kind, tags, inbox: false })}>
          File
        </button>
        {makers.map((m) => (
          <button key={m.label} className="btn" disabled={sending} onClick={() => void make(m)}>
            {m.label}
          </button>
        ))}
        {!narrow && (
          <>
            <Link className="btn" to={`/notes/${note.id}`} state={back}>
              Open
            </Link>
            <span className="spacer" />
          </>
        )}
        <button className="btn danger" disabled={sending} onClick={() => void send('POST', `/notes/${note.id}/trash`)}>
          Trash
        </button>
      </div>
      {makers.map((m) => (
        <p key={m.label} className="note">
          {m.label} takes the text and the area, and moves the capture to the trash. Kind and tags are for filing.
        </p>
      ))}
    </div>
  )
}

type PlaceProps = { areas: Area[]; narrow: boolean; area: string; kind: Kind; onChange: (c: { area?: string; kind?: Kind }) => void }

export function Place({ areas, narrow, area, kind, onChange }: PlaceProps) {
  return (
    <>
      <div className="field">
        <span className="label">Area</span>
        {narrow ? (
          <select aria-label="Area" value={area} onChange={(e) => onChange({ area: e.target.value })}>
            {areas.map((a) => (
              <option key={a.id} value={a.slug}>
                {a.name}
              </option>
            ))}
          </select>
        ) : (
          <Seg label="Area" value={area} onChange={(v) => onChange({ area: v })} options={areas.map((a) => [a.slug, a.kind === 'project' ? <span className="mono">{a.name}</span> : a.name])} />
        )}
      </div>
      <div className="field">
        <span className="label">Kind</span>
        <Seg label="Kind" value={kind} onChange={(v) => onChange({ kind: v })} options={kinds} />
      </div>
    </>
  )
}
