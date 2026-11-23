import { useEffect, useId, useState, type ComponentProps } from 'react'
import { message } from '../../api/client.ts'
import Field from '../../components/Field.tsx'
import { useNarrow } from '../../components/useNarrow.ts'
import { useItem } from '../../session.ts'
import type { Maker } from '../registry.ts'
import { made, make, split, useNow, type Fields } from './api.ts'
import { AreaField, PlanFields, type Plan } from './fields.tsx'

type Props = ComponentProps<Maker['form']>

// The form of Make a task: the title, Area, Planned, Deadline, List and Notes, and the link back to the item it is made
// from, which Save sends as the task's links. It starts with the item's title, or the first line of its text.
export default function MakeTask({ of, text, area, areas, onEntered, onClose }: Props) {
  const narrow = useNarrow()
  const now = useNow()
  const item = useItem()(of.ref, of.kind, of.state)
  const notesId = useId()
  const named = of.title || split(text).title
  const [start] = useState<Plan & { title: string; notes: string; area: string }>({ title: named, notes: '', area, planned_from: '', planned_to: '', planned_time: '', deadline: '', list: '' })
  const [draft, setDraft] = useState(start)
  const set = (d: Partial<typeof draft>) => setDraft({ ...draft, ...d })
  const entered = Object.entries(start).some(([k, v]) => draft[k as keyof typeof draft] !== v)
  useEffect(() => onEntered(entered), [entered, onEntered])
  const [sending, setSending] = useState(false)
  const [refused, setRefused] = useState('')

  // Only what holds a value is sent.
  async function save() {
    setRefused('')
    setSending(true)
    try {
      onClose(made(await make({ ...(Object.fromEntries(Object.entries(draft).filter(([, v]) => v)) as Fields), title: draft.title.trim(), links: [of.ref] })))
    } catch (e) {
      setRefused(message(e))
      setSending(false)
    }
  }

  const saveButton = (
    <button className="btn primary" disabled={sending || !draft.title.trim()} onClick={() => void save()}>
      Save
    </button>
  )
  return (
    <div className="task-edit task-make">
      <Field label="Title">
        <input className="title" autoComplete="off" autoFocus value={draft.title} onChange={(e) => set({ title: e.target.value })} />
      </Field>
      <div className="task-fields">
        <AreaField value={draft.area} areas={areas.filter((a) => !a.archived || a.slug === area)} onChange={(a) => set({ area: a })} />
        {now && <PlanFields value={draft} now={now} onChange={set} />}
        <div className="tf">
          <label className="lb" htmlFor={notesId}>
            Notes
          </label>
          <textarea id={notesId} className="ta" placeholder="Markdown, if you want" value={draft.notes} onChange={(e) => set({ notes: e.target.value })} />
        </div>
        <div className="tf">
          <span className="lb">Links to</span>
          <div className="lrow">
            <i className={item ? `mk ${item.mark}` : 'mk'} />
            <span>{named}</span>
          </div>
        </div>
      </div>
      {!draft.planned_from && !draft.deadline && !draft.list && <p className="note">With no plan, deadline or list, it waits on Tasks to be sorted.</p>}
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
      {/* Save is last on desktop and first on a phone in the DOM too, so the focus moves in the order shown. */}
      <div className={narrow ? 'acts stack' : 'acts end'}>
        {narrow && saveButton}
        <button className="btn" onClick={() => onClose()}>
          Cancel
        </button>
        {!narrow && saveButton}
      </div>
    </div>
  )
}
