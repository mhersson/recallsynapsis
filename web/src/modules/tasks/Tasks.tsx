import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { api, message } from '../../api/client.ts'
import { nameOf, useAreas } from '../../api/areas.ts'
import { TrashIcon } from '../../components/icons.tsx'
import ListEnd from '../../components/ListEnd.tsx'
import PageHead from '../../components/PageHead.tsx'
import { back, useClose } from '../../components/useClose.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import AddLine from './AddLine.tsx'
import { useMore, useOpen, useTask, type Fields, type Now, type Row, type Task } from './api.ts'
import { add, clock, day, week } from './dates.ts'
import { line, project } from './line.tsx'
import TaskRow from './TaskRow.tsx'
import TaskView from './TaskView.tsx'
import TaskViews from './TaskViews.tsx'
import { groupNames, latest, order, sortInto, views, type View } from './views.ts'
import './Tasks.css'

// What the head of a view says of it.
function about(view: View, now: Now) {
  if (view === 'agenda' || view === 'today') return `Today is ${day(now.date)}.`
  if (view === 'week') return `The rest of this week, to ${day(week(now)[1])}, and what lies beyond it.`
  if (view === 'done') return 'The most recently done first, by the day each was done.'
  return 'By the first date each has, then those with none, the oldest first.'
}

// The heading of a day in Done.
const dayName = (date: string, now: Now) => (date === now.date ? 'Today' : date === add(now.date, -1) ? 'Yesterday' : day(date))

// Tasks: a row of views over the open tasks, read once and sorted into each view's groups by the server's today, and Done,
// a list of its own. The view, the area chosen and the task open live in the address, so a reload keeps them.
export default function Tasks() {
  const narrow = useNarrow()
  const navigate = useNavigate()
  const id = Number(useParams().id) || undefined
  const [params, setParams] = useSearchParams()
  const view = views.find(([v]) => v === params.get('view'))?.[0] ?? 'agenda'
  const area = params.get('area') ?? ''
  const search = params.toString() ? `?${params}` : ''
  const close = useClose('/tasks' + search)
  const areas = useAreas(true)
  // A task open as a page of its own on a phone shows no list, and gives the server's today itself.
  const alone = narrow && !!id
  const { list, failed, took } = useOpen(!alone)
  const opened = useTask(id)
  const [refused, setRefused] = useState('')
  const [added, setAdded] = useState<number>()
  const here = (t: Row) => !area || t.area === area
  const open = list?.tasks.filter(here) ?? []
  const done = useMore(alone ? '' : new URLSearchParams({ done: 'true', ...(area && { area }) }).toString(), (t) => t.state === 'done' && !t.trashed_at && here(t), view !== 'done')
  const shown = view === 'done' ? (done.list?.tasks ?? []).toSorted(latest) : open.sort(order)
  const groups = list ? sortInto(view, shown, list.now) : []
  // On desktop the view opens its first task by naming it in the address.
  const first = (view === 'done' ? shown : groups.flatMap(([, ts]) => ts))[0]?.id
  useEffect(() => {
    if (!narrow && !id && first) void navigate(`/tasks/${first}${search}`, { replace: true })
  }, [narrow, id, first, search, navigate])

  const to = (v: View, taskID?: number) => {
    const q = new URLSearchParams(Object.entries({ view: v === 'agenda' ? '' : v, area }).filter(([, x]) => x))
    return '/tasks' + (taskID ? `/${taskID}` : '') + (q.size ? `?${q}` : '')
  }

  // Until the list is read the task open gives the day, so a phone turned keeps the task in place.
  const now = (!alone && list?.now) || opened.task?.now
  if (!now) {
    const why = alone ? opened.failed : failed
    return (
      why && (
        <div className="page">
          <p role="alert" className="err">
            {why}
          </p>
        </div>
      )
    )
  }

  async function change(t: Row, fields: Fields): Promise<string> {
    try {
      const answer = await api<Task>('PATCH', `/tasks/${t.id}`, fields)
      took(answer)
      done.again()
      opened.show(answer)
      return ''
    } catch (e) {
      return message(e)
    }
  }
  // Moves a task to the trash at once, out of every list, and closes it: on a phone to where it was opened from, and on
  // desktop to the view, which opens its first task.
  async function trash(t: Task): Promise<string> {
    try {
      const answer = await api<Task>('POST', `/tasks/${t.id}/trash`)
      took(answer)
      done.again()
      if (narrow) close()
      else void navigate('/tasks' + search, { replace: true })
      return ''
    } catch (e) {
      return message(e)
    }
  }
  // A task trashed elsewhere while it is open here is restored here, back where it was.
  const restore = (t: Task) =>
    api<Task>('POST', `/tasks/${t.id}/restore`).then((answer) => {
      took(answer)
      done.again()
      opened.show(answer)
      return ''
    }, message)
  const tap = async (t: Row, fields: Fields) => {
    setRefused('')
    setRefused(await change(t, fields))
  }
  const row = (t: Row, bits: ReturnType<typeof line>) => (
    <TaskRow key={t.id} t={t} to={`/tasks/${t.id}${search}`} bits={bits} on={t.id === id} done={t.state === 'done'} added={t.id === added} onRing={() => void tap(t, { done: t.state !== 'done' })} />
  )

  let listed
  if (view === 'done') {
    // The days the tasks shown were done, the newest first, as the order of the done has them.
    const days: [string, Row[]][] = []
    for (const t of shown) {
      const date = clock(t.done_at!, now)[0]
      if (days.at(-1)?.[0] === date) days.at(-1)![1].push(t)
      else days.push([date, [t]])
    }
    listed = !done.list ? null : shown.length === 0 ? (
      <div className="empty">
        <b>Nothing done yet.</b>A task marked done shows here, by the day it was done.
      </div>
    ) : (
      <>
        {days.map(([date, ts]) => (
          <section key={date} data-day={date}>
            <h2 className="tgrp">{dayName(date, now)}</h2>
            <div className="list">{ts.map((t) => row(t, [!area && project(t, areas), `done ${clock(t.done_at!, now)[1]}`].filter(Boolean)))}</div>
          </section>
        ))}
        <ListEnd shown={done.list.tasks.length} total={done.list.total} what="done" more={done.more} />
      </>
    )
  } else if (!list) listed = null
  else if (open.length === 0)
    listed = (
      <div className="empty">
        <b>No open tasks.</b>
        {view === 'week' ? 'Add one on Today, Soon or Someday' : 'Type one above'}, or ask an AI client to make one.
      </div>
    )
  else
    listed = groups.map(([g, ts]) =>
      ts.length > 0 ? (
        <section key={g} data-group={g}>
          <h2 className="tgrp">
            {groupNames[g]} <span className="dim">{ts.length}</span>
          </h2>
          <div className="list">{ts.map((t) => row(t, line(t, g, now, areas, !!area)))}</div>
        </section>
      ) : (
        g === 'today' && (
          <section key={g} data-group={g}>
            <h2 className="tgrp">{groupNames[g]}</h2>
            <div className="empty">
              <b>Nothing planned for today.</b>
              <Link to={to('soon')}>Pick one from Soon</Link>
            </div>
          </section>
        )
      ),
    )

  const select = (
    <select className="task-area" aria-label="Area" value={area} onChange={(e) => setParams(Object.entries({ view: params.get('view') ?? '', area: e.target.value }).filter(([, x]) => x), { replace: true })}>
      <option value="">All areas</option>
      {areas
        .filter((a) => !a.archived || a.slug === area)
        .map((a) => (
          <option key={a.slug} value={a.slug}>
            {a.name}
          </option>
        ))}
    </select>
  )

  const isDone = opened.task?.state === 'done'
  const outside = !opened.task?.trashed_at && isDone !== (view === 'done') && !shown.some((t) => t.id === id)
  const matchView = (changed: boolean) => {
    if (!changed) void navigate(to(isDone ? 'done' : 'agenda', id), { replace: true })
    else if (!narrow) void navigate(to(view), { replace: true })
  }
  const task = opened.task ? (
    <TaskView
      key={opened.task.id} task={opened.task} now={now} areas={areas}
      onChange={(f) => change(opened.task!, f)} onOutsideView={outside ? matchView : undefined}
      onTrash={() => trash(opened.task!)} onRestore={() => restore(opened.task!)}
    />
  ) : (
    opened.failed && (
      <p role="alert" className="err">
        {opened.failed}
      </p>
    )
  )
  // The task keeps its place in the page on a phone and on desktop, so turning a phone never ends an edit.
  return (
    <div className={alone ? 'page narrow' : 'page tasks'}>
      {alone ? (
        <div className="crumb">
          <Link to={'/tasks' + search}>Tasks</Link> /
        </div>
      ) : (
        <>
          <PageHead title="Tasks">
            {select}
            <Link className="btn sm" to="/tasks/trashed" state={back}>
              <TrashIcon />
              Trashed
            </Link>
          </PageHead>
          <p className="sub">{about(view, now)}</p>
        </>
      )}
      <div className={narrow ? undefined : 'panes'}>
        {!alone && (
          <div>
            <TaskViews current={view} items={views.map(([v, label]) => {
              let count
              if (v === 'agenda') count = open.length
              else if (v === 'done') count = done.list?.total ?? 0
              else count = sortInto(v, open, now).reduce((sum, [, ts]) => sum + ts.length, 0)
              return { view: v, label, to: to(v), count }
            })} />
            {view !== 'week' && view !== 'done' && (
              <AddLine
                key={view}
                placeholder={`Add a task${view === 'agenda' ? '' : view === 'today' ? ' for today' : ` to ${views.find(([v]) => v === view)![1]}`}${area && `, in ${nameOf(areas, area)}`}`}
                fields={{ ...(view === 'today' && { planned_from: now.date }), ...((view === 'soon' || view === 'someday') && { list: view }), ...(area && { area }) }}
                onAdded={(t) => {
                  took(t)
                  setAdded(t.id)
                  // On desktop the task added opens beside the list; the line keeps the focus for the next.
                  if (!narrow) void navigate(`/tasks/${t.id}${search}`, { state: back })
                }}
              />
            )}
            {(refused || done.failed || failed) && (
              <p role="alert" className="err">
                {refused || done.failed || failed}
              </p>
            )}
            {listed}
          </div>
        )}
        {id && <div className={narrow ? undefined : 'box'}>{task}</div>}
      </div>
    </div>
  )
}
