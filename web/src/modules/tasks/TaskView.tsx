import { useState } from 'react'
import { nameOf } from '../../api/areas.ts'
import type { Area } from '../../api/types.ts'
import Dialog from '../../components/Dialog.tsx'
import Field from '../../components/Field.tsx'
import Links from '../../components/Links.tsx'
import { TrashIcon } from '../../components/icons.tsx'
import { Markdown } from '../../components/Markdown.tsx'
import Section from '../../components/Section.tsx'
import { useNarrow } from '../../components/useNarrow.ts'
import { useUnsaved } from '../../components/useUnsaved.tsx'
import { from, ring, type Fields, type Now, type Task } from './api.ts'
import Check from './Check.tsx'
import { day, period, stamp } from './dates.ts'
import { AreaField, PlanFields } from './fields.tsx'
import { words } from './log.ts'

// Each callback sends what its button asks and gives the server's refusal; a button shows only when its callback is given.
type Props = {
  task: Task
  now: Now
  areas: Area[]
  onChange?: (f: Fields) => Promise<string>
  onTrash?: () => Promise<string>
  onRestore?: () => Promise<string>
  onDelete?: () => Promise<string>
}

// What the form holds, and the task as the editing began.
type Draft = Pick<Task, 'title' | 'notes' | 'planned_from' | 'planned_to' | 'planned_time' | 'deadline' | 'list' | 'area'> & { from: Task }

const onList = { '': 'on no list', soon: 'on Soon', someday: 'on Someday' }

// A plan as the server keeps it: none, or a from, a to that is the from for one day, and a time.
const plan = (x: Draft | Task): [string, string, string] => (x.planned_from ? [x.planned_from, x.planned_to || x.planned_from, x.planned_time] : ['', '', ''])

// The change that makes the task what the form shows, of only what differs from the task as the editing began, so a
// change made elsewhere meanwhile stands. A from without a to plans one day, a from of "" clears the plan whole, and a to
// alone keeps the from.
function changeOf(d: Draft): Fields {
  const [f, t, h] = plan(d)
  const [f0, t0, h0] = plan(d.from)
  return {
    ...(d.title !== d.from.title && { title: d.title }),
    ...(d.notes !== d.from.notes && { notes: d.notes }),
    ...(f !== f0 ? { planned_from: f, ...(!!f && t !== f && { planned_to: t }) } : t !== t0 && { planned_to: t }),
    ...(!!f && h !== h0 && { planned_time: h }),
    ...(d.deadline !== d.from.deadline && { deadline: d.deadline }),
    ...(d.list !== d.from.list && { list: d.list }),
    ...(d.area !== d.from.area && { area: d.area }),
  }
}

// A task, read first, and edited in place in one form that sends nothing before Save; no read of the task replaces the
// form. Only the ring, Trash and a link act at once. In the trash it is read only, with Restore and Delete for good.
export default function TaskView({ task, now, areas, onChange, onTrash, onRestore, onDelete }: Props) {
  const narrow = useNarrow()
  const [draft, setDraft] = useState<Draft>()
  const [sending, setSending] = useState(false)
  const [refused, setRefused] = useState('')
  const [deleting, setDeleting] = useState(false)
  const leaving = useUnsaved(!!draft && Object.keys(changeOf(draft)).length > 0, 'the task')

  async function send(act: () => Promise<string>) {
    setRefused('')
    setSending(true)
    const why = await act()
    setSending(false)
    setRefused(why)
    return !why
  }
  const change = (f: Fields) => send(() => onChange!(f))
  // A link added or removed is a change of the task, whose refusal the links section shows.
  async function link(links: string[]) {
    setSending(true)
    const why = await onChange!({ links })
    setSending(false)
    return why
  }
  const editing = (d?: Draft) => {
    setDraft(d)
    setRefused('')
  }
  async function save(d: Draft) {
    const f = changeOf(d)
    if (Object.keys(f).length === 0 || (await change(f))) editing()
  }

  const done = task.state === 'done'
  const trashed = !!task.trashed_at
  const alert = refused && (
    <p role="alert" className="err">
      {refused}
    </p>
  )
  const acts = trashed ? (
    <>
      {onRestore && (
        <button className="btn" disabled={sending} onClick={() => void send(onRestore)}>
          Restore
        </button>
      )}
      {onDelete && (
        <button className="btn danger" disabled={sending} onClick={() => setDeleting(true)}>
          Delete for good
        </button>
      )}
    </>
  ) : (
    <>
      {onChange && (
        <button className="btn" onClick={() => editing({ ...task, from: task })}>
          Edit
        </button>
      )}
      {onTrash && (
        <button className="btn danger" disabled={sending} onClick={() => void send(onTrash)}>
          Trash
        </button>
      )}
    </>
  )

  let body
  if (draft) {
    const set = (d: Partial<Draft>) => setDraft({ ...draft, ...d })
    const saveButton = (
      <button className="btn primary" disabled={sending} onClick={() => void save(draft)}>
        Save
      </button>
    )
    body = (
      <div className="task-edit">
        <Field label="Notes" hint="Markdown">
          <textarea className="ta" value={draft.notes} onChange={(e) => set({ notes: e.target.value })} />
        </Field>
        <div className="task-fields">
          <PlanFields value={draft} now={now} onChange={set} />
          <AreaField value={draft.area} areas={areas.filter((a) => !a.archived || a.slug === draft.from.area)} onChange={(area) => set({ area })} />
        </div>
        {alert}
        {/* Save is last on desktop and first on a phone in the DOM too, so the focus moves in the order shown. */}
        <div className={narrow ? 'acts stack' : 'acts end'}>
          {narrow && saveButton}
          <button className="btn" onClick={() => editing()}>
            Cancel
          </button>
          {!narrow && saveButton}
        </div>
      </div>
    )
  } else
    body = (
      <>
        {alert}
        <div className="task-lines">
          <p>{task.planned_from ? `Planned for ${period(task.planned_from, task.planned_to)}${task.planned_time && ` at ${task.planned_time}`}` : 'Not planned'}</p>
          <p>
            {task.deadline ? (
              <>
                Due <span className={task.deadline < now.date && !done ? 'late' : undefined}>{day(task.deadline)}</span>
              </>
            ) : (
              'No deadline'
            )}
            , {onList[task.list]}
          </p>
        </div>
        {task.notes ? <Markdown text={task.notes} /> : <p className="task-none">No notes</p>}
      </>
    )

  return (
    <div className="task-read">
      <div className="page-head task-head">
        {onChange && !trashed && <Check big done={done} sort={task.state === 'unsorted'} onClick={() => void change({ done: !done })} />}
        {draft ? (
          <input className="ttl" aria-label="Title" autoComplete="off" autoFocus value={draft.title} onChange={(e) => setDraft({ ...draft, title: e.target.value })} />
        ) : (
          <h1>{task.title}</h1>
        )}
        {!narrow && !draft && acts}
      </div>
      <span className="t2 task-meta">
        <i className={`mk ${ring(task.state)}`} />
        <span>task</span>
        <span>{nameOf(areas, task.area)}</span>
        {task.state === 'unsorted' && <span>to sort</span>}
        <span>{trashed ? `trashed ${stamp(task.trashed_at!, now)}` : done ? `done ${stamp(task.done_at!, now)}` : `changed ${stamp(task.changed_at, now)}`}</span>
        <span>{from(task.source)}</span>
      </span>
      {trashed && (
        <div className="notice">
          <div className="nt">
            <TrashIcon />
            In the trash
          </div>
          <p className="nd">It cannot be changed, and no list, search or link finds it until it is restored.</p>
        </div>
      )}
      {body}
      <div className="sects">
        <Links of={task} areas={areas} trashed={trashed || !onChange} sending={sending} onLinks={link} />
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
      {narrow && !draft && <div className="acts stack task-foot">{acts}</div>}
      {deleting && (
        <Dialog title={`Delete “${task.title}” for good?`} onClose={() => setDeleting(false)}>
          <p className="note">The task and its log are deleted. This cannot be undone.</p>
          <div className="acts">
            <button className="btn" onClick={() => setDeleting(false)}>
              Keep it
            </button>
            <button
              className="btn danger"
              onClick={() => {
                setDeleting(false)
                void send(onDelete!)
              }}
            >
              Delete for good
            </button>
          </div>
        </Dialog>
      )}
      {leaving}
    </div>
  )
}
