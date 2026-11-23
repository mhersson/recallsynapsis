import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { nameOf, useAreas } from '../../api/areas.ts'
import { api, message } from '../../api/client.ts'
import Dialog from '../../components/Dialog.tsx'
import { RestoreIcon } from '../../components/icons.tsx'
import ListEnd from '../../components/ListEnd.tsx'
import PageHead from '../../components/PageHead.tsx'
import { ago } from '../../components/time.ts'
import { useClose } from '../../components/useClose.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { useMore, useTask, type Task } from './api.ts'
import { period } from './dates.ts'
import { project } from './line.tsx'
import TaskRow from './TaskRow.tsx'
import TaskView from './TaskView.tsx'
import './Tasks.css'

const tasks = (n: number) => `${n} ${n === 1 ? 'task' : 'tasks'}`

// What was trashed, done and open alike, the most recently trashed first, with the task open beside the list on desktop
// and as a page of its own on a phone, read only. Nothing empties the trash but the owner.
export default function Trashed() {
  const narrow = useNarrow()
  const navigate = useNavigate()
  const id = Number(useParams().id) || undefined
  const close = useClose('/tasks/trashed')
  const areas = useAreas(true)
  const alone = narrow && !!id
  const trash = useMore(alone ? '' : 'trashed=true', (t) => !!t.trashed_at)
  const opened = useTask(id)
  // The task last restored here, and whether one was restored or deleted: after either, none opens unasked.
  const [restored, setRestored] = useState<Task>()
  const [settled, setSettled] = useState(false)
  const [emptying, setEmptying] = useState(false)
  const [refused, setRefused] = useState('')
  const newest = trash.list?.tasks[0]?.id
  useEffect(() => {
    if (!narrow && !id && !settled && newest) void navigate(`/tasks/trashed/${newest}`, { replace: true })
  }, [narrow, id, settled, newest, navigate])

  // On a phone the task's page goes back to the list; on desktop the list stays, with no task open.
  const settle = (t?: Task) => {
    setSettled(true)
    setRestored(t)
    if (narrow) close()
    else void navigate('/tasks/trashed', { replace: true })
  }
  const act = async (method: string, path: string, done: (t?: Task) => void) => {
    try {
      const t = await api<Task | undefined>(method, path)
      // A task deleted is out of the trash, as one restored is.
      trash.took(t ?? { ...opened.task!, trashed_at: null })
      done(t)
      return ''
    } catch (e) {
      return message(e)
    }
  }
  const empty = () => {
    setEmptying(false)
    api('DELETE', '/tasks/trash').then(
      () => {
        setRefused('')
        trash.again()
        if (id) settle()
      },
      (e: unknown) => setRefused(message(e)),
    )
  }

  const now = trash.list?.now ?? opened.task?.now
  const task = opened.task && now && (
    <TaskView
      key={opened.task.id}
      task={opened.task}
      now={now}
      areas={areas}
      onRestore={() => act('POST', `/tasks/${id}/restore`, settle)}
      onDelete={() => act('DELETE', `/tasks/${id}`, () => settle())}
    />
  )
  const failed = opened.failed && (
    <p role="alert" className="err">
      {opened.failed}
    </p>
  )

  if (alone)
    return (
      <div className="page narrow">
        <div className="crumb">
          <Link to="/tasks">Tasks</Link> / <Link to="/tasks/trashed">Trashed</Link> /
        </div>
        {task || failed}
      </div>
    )

  const total = trash.list?.total ?? 0
  const emptyIt = total > 0 && (
    <button className="btn danger" onClick={() => setEmptying(true)}>
      Empty the trash
    </button>
  )
  return (
    <div className="page tasks">
      <div className="crumb">
        <Link to="/tasks">Tasks</Link> /
      </div>
      <PageHead title="Trashed">{!narrow && emptyIt}</PageHead>
      {trash.list && (
        <p className="sub">
          {total > 0 ? `${tasks(total)}, the most recently trashed first. Nothing empties the trash on its own.` : 'A trashed task waits here until you restore it or delete it for good.'}
        </p>
      )}
      {restored && (
        <div className="notice">
          <div className="nt">
            <RestoreIcon />
            {restored.title} is restored.
          </div>
          <p className="nd">
            It is back in {nameOf(areas, restored.area)}
            {restored.planned_from && `, planned for ${period(restored.planned_from, restored.planned_to)}`}, with its log.{' '}
            <Link to={`/tasks/${restored.id}`}>Open it</Link>
          </p>
        </div>
      )}
      {(trash.failed || refused) && (
        <p role="alert" className="err">
          {refused || trash.failed}
        </p>
      )}
      <div className={narrow ? undefined : 'panes'}>
        <div>
          {trash.list && total === 0 && (
            <div className="empty">
              <b>The trash is empty.</b>
            </div>
          )}
          {trash.list && total > 0 && (
            <div className="list">
              {trash.list.tasks.map((t) => (
                <TaskRow
                  key={t.id}
                  t={t}
                  to={`/tasks/trashed/${t.id}`}
                  on={t.id === id}
                  bits={[
                    project(t, areas),
                    t.planned_from && period(t.planned_from, t.planned_to),
                    t.list,
                    `trashed ${ago(t.trashed_at!)}`,
                  ].filter(Boolean)}
                />
              ))}
            </div>
          )}
          {trash.list && <ListEnd shown={trash.list.tasks.length} total={trash.list.total} what="trashed" more={trash.more} />}
        </div>
        {!narrow &&
          (id ? (
            <div className="box">{task || failed}</div>
          ) : (
            total > 0 && <div className="empty">Pick a task to read it here.</div>
          ))}
      </div>
      {narrow && emptyIt && <div className="acts stack task-foot">{emptyIt}</div>}
      {emptying && (
        <Dialog title="Empty the trash?" onClose={() => setEmptying(false)}>
          <p className="note">{total === 1 ? 'The task in it is' : `The ${total} tasks in it are`} deleted for good, with their logs. This cannot be undone.</p>
          <div className="acts">
            <button className="btn" onClick={() => setEmptying(false)}>
              Keep them
            </button>
            <button className="btn danger" onClick={empty}>
              Empty the trash
            </button>
          </div>
        </Dialog>
      )}
    </div>
  )
}
