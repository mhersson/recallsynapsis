import { useState } from 'react'
import { Link } from 'react-router'
import { api, message } from '../../api/client.ts'
import type { Area } from '../../api/types.ts'
import { back } from '../../components/useClose.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import AddLine from './AddLine.tsx'
import { useOpen, type Row, type Task } from './api.ts'
import { line } from './line.tsx'
import TaskRow from './TaskRow.tsx'
import { groupNames, order, sortInto } from './views.ts'
import './Tasks.css'

// An area's open tasks on its page, each once, in the groups of the views, under the line that adds a task to sort in the
// area, with Done in the area at the foot. On a phone it folds, open when it is the page's first section. A row marked
// done stays where it stood, struck through, until the page is left.
export default function AreaTasks({ area, first }: { area: Area; first: boolean }) {
  const narrow = useNarrow()
  const { list, failed, took } = useOpen()
  const [kept, setKept] = useState<Row[]>([])
  const [refused, setRefused] = useState('')
  const [added, setAdded] = useState<number>()
  if (!list)
    return (
      failed && (
        <p role="alert" className="err">
          {failed}
        </p>
      )
    )

  const open = list.tasks.filter((t) => t.area === area.slug)
  const shown = [...open, ...kept.filter((k) => !open.some((x) => x.id === k.id))].sort(order)
  const isDone = (t: Row) => !open.some((x) => x.id === t.id)
  const groups = sortInto('all', shown, list.now).filter(([, ts]) => ts.length > 0)
  async function tap(t: Row) {
    setRefused('')
    try {
      took(await api<Task>('PATCH', `/tasks/${t.id}`, { done: !isDone(t) }))
      setKept((k) => [...k.filter((x) => x.id !== t.id), t])
    } catch (e) {
      setRefused(message(e))
    }
  }

  const head = (
    <>
      Tasks <span className="dim">{open.length}</span>
    </>
  )
  const body = (
    <>
      {!area.archived && (
        <AddLine
          placeholder={`Add a task to ${area.name}`}
          fields={{ area: area.slug }}
          onAdded={(t) => {
            took(t)
            setAdded(t.id)
          }}
        />
      )}
      {(refused || failed) && (
        <p role="alert" className="err">
          {refused || failed}
        </p>
      )}
      {groups.map(([g, ts]) => (
        <section key={g} data-group={g}>
          <h3 className="tgrp">
            {groupNames[g]} <span className="dim">{ts.length}</span>
          </h3>
          <div className="list">
            {ts.map((t) => (
              <TaskRow
                key={t.id}
                t={t}
                to={`/tasks/${t.id}?area=${area.slug}`}
                bits={line(t, g, list.now, [area], true)}
                on={false}
                done={isDone(t)}
                added={t.id === added}
                onRing={() => void tap(t)}
              />
            ))}
          </div>
        </section>
      ))}
      <p className="afoot">
        <Link to={`/tasks?view=done&area=${area.slug}`} state={back}>
          Done in {area.name}
        </Link>
      </p>
    </>
  )
  return narrow ? (
    <details className="area-tasks" open={first}>
      <summary>{head}</summary>
      {body}
    </details>
  ) : (
    <section className="area-tasks">
      <h2 className="part">{head}</h2>
      {body}
    </section>
  )
}
