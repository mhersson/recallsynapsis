import { useState, type FormEvent } from 'react'
import { api, message } from '../../api/client.ts'
import { useAreas } from '../../api/areas.ts'
import Seg from '../../components/Seg.tsx'
import { useModules } from '../../session.ts'
import { kinds } from './api.ts'
import './Capture.css'

type Props = { area?: number; dialog?: boolean; onClose: () => void }

// Saves a thought to the inbox, or files it in the area chosen. Opened from a project's page, that project is chosen. A
// kind another module adds goes to an area, the personal area unless another is chosen, and shows what it asks for more.
export default function Capture({ area, dialog, onClose }: Props) {
  const areas = useAreas()
  const added = useModules().flatMap((m) => m.capture ?? [])
  const [body, setBody] = useState('')
  const [kind, setKind] = useState('note')
  const other = added.find((k) => k.name === kind)
  const [more, setMore] = useState<Record<string, string>>({})
  // The slug of the area chosen, '' for the inbox; until a choice is made, the project of the page once it is read.
  const [chosen, setChosen] = useState<string>()
  const picked = chosen ?? areas.find((a) => a.id === area && a.kind === 'project')?.slug ?? ''
  const to = other && !picked ? (areas.find((a) => a.kind === 'personal')?.slug ?? '') : picked
  const [sending, setSending] = useState(false)
  const [refused, setRefused] = useState('')

  async function submit(e: FormEvent) {
    e.preventDefault()
    setRefused('')
    setSending(true)
    const text = body.trim()
    try {
      if (other) await other.save(text, to, more)
      else await api('POST', '/notes', to ? { body: text, kind, area: to } : { body: text, kind, inbox: true })
      onClose()
    } catch (err) {
      setRefused(message(err))
      setSending(false)
    }
  }

  const goes = (
    <select aria-label="Goes to" value={to} onChange={(e) => setChosen(e.target.value)}>
      {!other && <option value="">Inbox</option>}
      {areas.map((a) => (
        <option key={a.id} value={a.slug}>
          {a.name}
        </option>
      ))}
    </select>
  )
  const pick = <Seg label="Kind" options={[...kinds, ...added.map((k): [string, string] => [k.name, k.label])]} value={kind} onChange={setKind} />
  const asked = other && <other.more value={more} onChange={setMore} />
  const save = (
    <button className="btn primary" disabled={sending || !body.trim()}>
      Capture
    </button>
  )
  return (
    <form className="capture" onSubmit={(e) => void submit(e)}>
      <textarea aria-label="Text" placeholder="Capture a thought" autoFocus value={body} onChange={(e) => setBody(e.target.value)} />
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
      {dialog ? (
        <>
          {asked}
          <div className="acts">
            {goes}
            {pick}
            <span className="spacer" />
            <button type="button" className="btn" onClick={onClose}>
              Cancel
            </button>
            {save}
          </div>
        </>
      ) : (
        <>
          <div className="field">
            <span className="label">Goes to</span>
            {goes}
          </div>
          <div className="field">
            <span className="label">Kind</span>
            {pick}
          </div>
          {asked}
          <div className="acts stack">{save}</div>
        </>
      )}
    </form>
  )
}
