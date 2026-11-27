import { useEffect, useState, type FormEvent, type KeyboardEvent } from 'react'
import { ApiError, api, message } from '../../api/client.ts'
import Field from '../../components/Field.tsx'
import { cameIn } from '../../components/history.ts'
import { plain } from '../../components/markdown.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { useUnsaved } from '../../components/useUnsaved.tsx'
import type { Decision, Fields, Full } from './api.ts'
import './Knowledge.css'

type Props = { area: string; of?: Decision; start: Fields; decisions: Decision[]; onSaved: (d: Decision) => void; onCancel: () => void }

// The fields that differ from start, a text as the server keeps it, without the space around it.
const differ = (f: Fields, start: Fields) => Object.fromEntries(Object.entries(f).filter(([k, v]) => v.trim() !== start[k as keyof Fields])) as Partial<Fields>

// A line takes no line break, so Enter saves, as in a one-line field.
function line(e: KeyboardEvent<HTMLTextAreaElement>) {
  if (e.key !== 'Enter') return
  e.preventDefault()
  e.currentTarget.form?.requestSubmit()
}

// Adds a decision to the project area names, or mends of. start is held, so no read replaces what is entered, and a mend
// refused because the decision moved on reads it again for the revision a second Save sends from.
export default function DecisionForm({ area, of, start: given, decisions, onSaved, onCancel }: Props) {
  const narrow = useNarrow()
  const [start] = useState(given)
  const [f, setF] = useState(given)
  const [revision, setRevision] = useState(of?.revision)
  const [refused, setRefused] = useState('')
  const [sending, setSending] = useState(false)
  const change = differ(f, start)
  // The way out of a Cancel or a save, taken once the question before leaving is disarmed, so neither asks it.
  const [out, setOut] = useState<{ go: () => void }>()
  const leaving = useUnsaved(!out && Object.keys(change).length > 0, 'the decision')
  useEffect(() => {
    out?.go()
  }, [out])

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (of && Object.keys(change).length === 0) return onCancel()
    setRefused('')
    setSending(true)
    const entry = of ? { mend: [{ ref: of.ref, revision, ...change }] } : { add: [{ ...f, replaces: f.replaces || undefined }] }
    try {
      const { decisions: saved } = await api<{ decisions: Decision[] }>('POST', '/knowledge', { area, ...entry })
      return setOut({ go: () => onSaved(saved[0]) })
    } catch (err) {
      const now = of && err instanceof ApiError && err.status === 409 ? await api<Full>('GET', `/knowledge/${of.id}`).catch(() => undefined) : undefined
      if (now && now.revision !== revision) {
        setRevision(now.revision)
        setRefused(`Revision ${now.revision} of the decision came in ${cameIn(now.changed_at, now.changed_by)}, while you were editing. Save again to save yours over it.`)
      } else setRefused(message(err))
    }
    setSending(false)
  }

  const set = (p: Partial<Fields>) => setF({ ...f, ...p })
  // Replaces offers the decisions that stand, but the one edited, and the one it replaced as the form opened.
  const offered = decisions.filter((x) => x.ref !== of?.ref && (x.state === 'stands' || x.ref === start.replaces))
  const save = (
    <button className="btn primary" disabled={sending}>
      Save
    </button>
  )
  return (
    <form className="kform" onSubmit={(e) => void submit(e)}>
      <Field label="Decision" hint="Markdown">
        <textarea className="one kline" rows={1} autoFocus placeholder="What was decided, in one line" value={f.decision} onKeyDown={line} onChange={(e) => set({ decision: e.target.value })} />
      </Field>
      <Field label="Why" hint="Markdown">
        <textarea className="ta" placeholder={of ? undefined : 'Why this, and what was turned down, if anything was'} value={f.why} onChange={(e) => set({ why: e.target.value })} />
      </Field>
      <Field label="Source">
        <textarea className="one" rows={1} placeholder="Where it was said, in words or as an address" value={f.source} onKeyDown={line} onChange={(e) => set({ source: e.target.value })} />
      </Field>
      <Field label="Decided">
        <input type="date" value={f.decided} onChange={(e) => set({ decided: e.target.value })} />
      </Field>
      <Field label="Replaces">
        <select value={f.replaces} onChange={(e) => set({ replaces: e.target.value })}>
          <option value="">Nothing</option>
          {offered.map((x) => (
            <option key={x.ref} value={x.ref}>
              {plain(x.decision)}
            </option>
          ))}
        </select>
      </Field>
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
      {/* Save is last on desktop and first on a phone in the DOM too, so the focus moves in the order shown. */}
      <div className={narrow ? 'acts stack' : 'acts end'}>
        {narrow && save}
        <button type="button" className="btn" onClick={() => setOut({ go: onCancel })}>
          Cancel
        </button>
        {!narrow && save}
      </div>
      {leaving}
    </form>
  )
}
