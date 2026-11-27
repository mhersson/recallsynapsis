import { useState, type FormEvent } from 'react'
import { ApiError, api, message } from '../../api/client.ts'
import type { Area } from '../../api/types.ts'
import Field from '../../components/Field.tsx'

type Props = { area?: Area; dialog?: boolean; onSaved: (a: Area) => void; onCancel: () => void }

// Makes a project, or changes the area it is given. The personal area keeps its name.
export default function AreaForm({ area: given, dialog, onSaved, onCancel }: Props) {
  // The page may read the area again while the form is open; the form keeps the one it opened with.
  const [area] = useState(given)
  const [name, setName] = useState(area?.name ?? '')
  const [brief, setBrief] = useState(area?.brief ?? '')
  const [refused, setRefused] = useState('')
  const [sending, setSending] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    const n = name.trim()
    // Only what was changed here is sent, so a change made elsewhere meanwhile stands and the personal area sends no name. The
    // brief names the revision the form opened with, so the server refuses it once the brief has moved on.
    const body = area
      ? { ...(n !== area.name && { name: n }), ...(brief.trim() !== area.brief && { brief, brief_revision: area.brief_revision }) }
      : { name: n, brief }
    if (Object.keys(body).length === 0) return onCancel()
    setRefused('')
    setSending(true)
    try {
      onSaved(await api<Area>(area ? 'PATCH' : 'POST', area ? `/areas/${area.id}` : '/areas', body))
    } catch (err) {
      // A 409 is the name taken unless the brief was sent too, which may have moved on.
      setRefused(err instanceof ApiError && err.status === 409 && !('brief_revision' in body) ? `A project named ${n} exists already.` : message(err))
      setSending(false)
    }
  }

  const cancel = (
    <button type="button" className="btn" onClick={onCancel}>
      Cancel
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
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
      <Field label="Brief">
        <textarea placeholder="A line or two on what it is" value={brief} onChange={(e) => setBrief(e.target.value)} />
      </Field>
      {!area && <p className="note">AI clients see the name and the brief when they choose where to file.</p>}
      <div className={dialog ? 'acts' : 'acts stack'}>
        {dialog && cancel}
        <button className="btn primary" disabled={sending || !name.trim()}>
          {area ? 'Save' : 'Create'}
        </button>
        {!dialog && cancel}
      </div>
    </form>
  )
}
