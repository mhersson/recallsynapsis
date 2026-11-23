import { useEffect, useId, useRef, useState } from 'react'
import type { Area } from '../../api/types.ts'
import Field from '../../components/Field.tsx'
import { Markdown } from '../../components/Markdown.tsx'
import Section from '../../components/Section.tsx'
import Seg from '../../components/Seg.tsx'
import { useNarrow } from '../../components/useNarrow.ts'
import { useUnsaved } from '../../components/useUnsaved.tsx'
import { from, nameOf, ring, type Fields, type Now, type Task } from './api.ts'
import Check from './Check.tsx'
import { period, shortcuts, stamp, whole } from './dates.ts'
import { words } from './log.ts'

type Props = { task: Task; now: Now; areas: Area[]; onChange: (f: Fields) => Promise<string> }

// A task, read first. Done, the plan, the deadline, the list and the area save as they change; the title and the notes
// through Edit, into a draft that no read of the task replaces. onChange sends a change and gives the server's refusal.
export default function TaskView({ task, now, areas, onChange }: Props) {
  const narrow = useNarrow()
  const [draft, setDraft] = useState<{ from: Task; title: string; notes: string }>()
  const [sending, setSending] = useState(false)
  const [refused, setRefused] = useState('')
  // Counts refusals, so a field that sent what was refused shows the task's value again.
  const [refusals, setRefusals] = useState(0)
  const typed = !!draft && (draft.title !== draft.from.title || draft.notes !== draft.from.notes)
  const leaving = useUnsaved(typed, 'the task')
  const ids = { planned: useId(), deadline: useId(), area: useId() }

  async function send(f: Fields) {
    setRefused('')
    setSending(true)
    const why = await onChange(f)
    setSending(false)
    if (why) {
      setRefused(why)
      setRefusals((n) => n + 1)
    }
    return !why
  }
  // Sends what differs from the task as the editing began, so a change made elsewhere meanwhile stands.
  async function save(d: NonNullable<typeof draft>) {
    const f = { ...(d.title !== d.from.title && { title: d.title }), ...(d.notes !== d.from.notes && { notes: d.notes }) }
    if (Object.keys(f).length === 0 || (await send(f))) setDraft(undefined)
  }

  const done = task.state === 'done'
  const edit = (
    <button className="btn" onClick={() => setDraft({ from: task, title: task.title, notes: task.notes })}>
      Edit
    </button>
  )

  return (
    <div className="task-read">
      <div className="page-head task-head">
        <Check big done={done} sort={task.state === 'unsorted'} onClick={() => void send({ done: !done })} />
        {draft ? <input className="ttl" aria-label="Title" autoComplete="off" value={draft.title} onChange={(e) => setDraft({ ...draft, title: e.target.value })} /> : <h1>{task.title}</h1>}
        {!narrow && !draft && edit}
      </div>
      <span className="t2 task-meta">
        <i className={ring(task.state)} />
        <span>task</span>
        <span>{nameOf(areas, task.area)}</span>
        {task.state === 'unsorted' && <span>to sort</span>}
        <span>{done ? `done ${stamp(task.done_at!, now)}` : `changed ${stamp(task.changed_at, now)}`}</span>
        <span>{from(task.source)}</span>
      </span>
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
      {draft ? (
        <div className="task-edit">
          <Field label="Notes">
            <textarea className="ta" autoFocus value={draft.notes} onChange={(e) => setDraft({ ...draft, notes: e.target.value })} />
          </Field>
          <div className={narrow ? 'acts stack' : 'acts'}>
            <button className="btn primary" disabled={sending} onClick={() => void save(draft)}>
              Save
            </button>
            <button className="btn" onClick={() => setDraft(undefined)}>
              Cancel
            </button>
            {!narrow && (
              <>
                <span className="spacer" />
                <span className="t2">Markdown</span>
              </>
            )}
          </div>
          {leaving}
        </div>
      ) : task.notes ? (
        <Markdown text={task.notes} />
      ) : (
        <p className="task-none">No notes</p>
      )}
      <div className="task-fields">
        <div className="tf">
          <span className="lb" id={ids.planned}>
            Planned
          </span>
          <div role="group" aria-labelledby={ids.planned}>
            <div className="scs">
              {shortcuts(now).map(([label, a, b]) => {
                const on = task.planned_from === a && task.planned_to === b
                return (
                  <button key={label} type="button" className={on ? 'sc on' : 'sc'} aria-current={on || undefined} onClick={() => !on && void send(a === b ? { planned_from: a } : { planned_from: a, planned_to: b })}>
                    <b>{label}</b>
                    <span>{period(a, b)}</span>
                  </button>
                )
              })}
            </div>
            <div className="dts">
              <label>
                From
                {/* A span keeps its to while the from stays on or before it; else the from plans that one day. */}
                <Moment
                  type="date"
                  value={task.planned_from}
                  reset={refusals}
                  onSave={(v) => send(task.planned_to > task.planned_from && task.planned_to >= v ? { planned_from: v, planned_to: task.planned_to } : { planned_from: v })}
                />
              </label>
              <label>
                To
                <Moment type="date" value={task.planned_to} reset={refusals} min={task.planned_from} disabled={!task.planned_from} onSave={(v) => send({ planned_to: v })} />
              </label>
              <label>
                Starts at
                <Moment type="time" value={task.planned_time} reset={refusals} disabled={!task.planned_from} onSave={(v) => send({ planned_time: v })} />
              </label>
              {task.planned_from && (
                <button className="btn link" onClick={() => void send({ planned_from: '' })}>
                  Clear the plan
                </button>
              )}
            </div>
          </div>
        </div>
        <div className="tf">
          <label className="lb" htmlFor={ids.deadline}>
            Deadline
          </label>
          <div className="acts">
            <Moment id={ids.deadline} type="date" value={task.deadline} reset={refusals} onSave={(v) => send({ deadline: v })} />
            {task.deadline && (
              <button className="btn link" aria-label="Clear the deadline" onClick={() => void send({ deadline: '' })}>
                Clear
              </button>
            )}
          </div>
        </div>
        <div className="tf">
          <span className="lb">List</span>
          <Seg
            label="List"
            options={[
              ['', 'None'],
              ['next', 'Next'],
              ['someday', 'Someday'],
            ]}
            value={task.list}
            onChange={(list) => list !== task.list && void send({ list })}
          />
        </div>
        <div className="tf">
          <label className="lb" htmlFor={ids.area}>
            Area
          </label>
          <div>
            <select id={ids.area} value={task.area} onChange={(e) => void send({ area: e.target.value })}>
              {areas
                .filter((a) => !a.archived || a.slug === task.area)
                .map((a) => (
                  <option key={a.slug} value={a.slug}>
                    {a.name}
                  </option>
                ))}
            </select>
          </div>
        </div>
      </div>
      <div className="sects">
        <Section title="Log" count={`${task.log.length} ${task.log.length === 1 ? 'change' : 'changes'}`}>
          {task.log.map((e, i) => (
            <div key={i} className="lg">
              <span className="w">{stamp(e.at, now)}</span>
              <span className="src">{from(e.source)}</span>
              <span className="c">{words(e, i === task.log.length - 1, areas)}</span>
              {e.said && <p className="said">You said: “{e.said}”</p>}
            </div>
          ))}
        </Section>
      </div>
      {narrow && !draft && <div className="acts stack task-foot">{edit}</div>}
    </div>
  )
}

type MomentProps = { id?: string; type: 'date' | 'time'; value: string; reset: number; min?: string; disabled?: boolean; onSave: (v: string) => void }

// A date or a time field of the browser's own. Typed into, it holds a whole but wrong date at each segment before the one
// meant, and its own calendar or clock may set each step it passes while it is open, so it saves on leaving the field or
// on Enter, and only a whole value; a time is cleared by leaving it empty. It shows the task's value again when that
// changes, when a change is refused, and when it is left holding what was not sent.
function Moment({ id, type, value, reset, min, disabled, onSave }: MomentProps) {
  const field = useRef<HTMLInputElement>(null)
  // What was last sent, so Enter and then leaving the field send it once.
  const sent = useRef<string>(undefined)
  useEffect(() => {
    field.current!.value = value
    sent.current = undefined
  }, [value, reset])
  const save = (v: string) => {
    if (v === sent.current) return
    if (v !== value && (type === 'date' ? whole(v) : /^\d\d:\d\d$/.test(v) || v === '')) {
      sent.current = v
      onSave(v)
    } else field.current!.value = value
  }
  return (
    <input
      ref={field}
      id={id}
      type={type}
      defaultValue={value}
      min={min}
      disabled={disabled}
      onKeyDown={(e) => e.key === 'Enter' && save(e.currentTarget.value)}
      onBlur={(e) => save(e.target.value)}
    />
  )
}
