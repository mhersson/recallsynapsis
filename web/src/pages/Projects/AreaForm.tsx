import { useState, type FormEvent } from 'react'
import { areaOf } from '../../api/areas.ts'
import { ApiError, api, message } from '../../api/client.ts'
import type { Area } from '../../api/types.ts'
import Field from '../../components/Field.tsx'
import { from } from '../../components/history.ts'
import { ago } from '../../components/time.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { useUnsaved } from '../../components/useUnsaved.tsx'

type Props = { area?: Area; dialog?: boolean; onSaved: (a: Area) => void; onCancel: () => void }

// Makes a project, or changes the area it is given. The personal area keeps its name.
export default function AreaForm({ area: given, dialog, onSaved, onCancel }: Props) {
  const narrow = useNarrow()
  // The page may read the area again while the form is open; the form keeps the one it opened with.
  const [area] = useState(given)
  const [name, setName] = useState(area?.name ?? '')
  const [brief, setBrief] = useState(area?.brief ?? '')
  // The revision the brief is sent from: the one the form opened with, or one that came in since and refused a save.
  const [revision, setRevision] = useState(area?.brief_revision ?? 0)
  const [refused, setRefused] = useState('')
  const [sending, setSending] = useState(false)
  const n = name.trim()
  // Only what was changed here is sent, so a change made elsewhere meanwhile stands and the personal area sends no name.
  const change = area && { ...(n !== area.name && { name: n }), ...(brief.trim() !== area.brief && { brief, brief_revision: revision }) }
  const leaving = useUnsaved(!!change && Object.keys(change).length > 0, area?.kind === 'personal' ? 'the personal area' : 'the project')

  async function submit(e: FormEvent) {
    e.preventDefault()
    const body = change ?? { name: n, brief }
    if (Object.keys(body).length === 0) return onCancel()
    setRefused('')
    setSending(true)
    try {
      onSaved(await api<Area>(area ? 'PATCH' : 'POST', area ? `/areas/${area.id}` : '/areas', body))
    } catch (err) {
      const conflict = err instanceof ApiError && err.status === 409
      // A 409 is the brief moved on when another revision came in, and else the name taken.
      const now = conflict && 'brief_revision' in body ? await areaOf(area!.id).catch(() => undefined) : undefined
      if (now && now.brief_revision !== revision) {
        setRevision(now.brief_revision)
        setRefused(`Revision ${now.brief_revision} of the brief came in ${ago(now.brief_changed_at!)}, ${from(now.brief_changed_by)}, while you were editing. Save again to save yours over it.`)
      } else setRefused(conflict && 'name' in body ? `A project named ${n} exists already.` : message(err))
      setSending(false)
    }
  }

  const save = (
    <button className="btn primary" disabled={sending || !n}>
      {area ? 'Save' : 'Create'}
    </button>
  )
  return (
    <form className={dialog ? 'area-form' : 'area-form page-form'} onSubmit={(e) => void submit(e)}>
      {area?.kind === 'personal' ? (
        <div className="field">
          <span className="label">Name</span>
          <p className="fixed">{area.name}</p>
          <p className="note">The personal area keeps its name and is never archived.</p>
        </div>
      ) : (
        <Field label="Name">
          <input autoFocus autoComplete="off" value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
      )}
      <Field label="Brief" hint="Markdown">
        <textarea className="ta" autoFocus={area?.kind === 'personal'} placeholder="What it is and why" value={brief} onChange={(e) => setBrief(e.target.value)} />
      </Field>
      {!area && <p className="note">AI clients see the name and the brief when they choose where to file.</p>}
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
      {/* Save is last on desktop and first on a phone in the DOM too, so the focus moves in the order shown. */}
      <div className={narrow ? 'acts stack' : 'acts end'}>
        {narrow && save}
        <button type="button" className="btn" onClick={onCancel}>
          Cancel
        </button>
        {!narrow && save}
      </div>
      {leaving}
    </form>
  )
}
