import { useEffect, useState, type FormEvent } from 'react'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router'
import { api, message } from '../../api/client.ts'
import { useAreas } from '../../api/areas.ts'
import { AddIcon } from '../../components/icons.tsx'
import PageHead from '../../components/PageHead.tsx'
import { ago } from '../../components/time.ts'
import { back } from '../../components/useClose.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { from, nameOf, useOpen, useTask, type Fields, type Now, type Row, type Task } from './api.ts'
import Check from './Check.tsx'
import { day, period, week } from './dates.ts'
import TaskView from './TaskView.tsx'
import { order, sortInto, views, type Group, type View } from './views.ts'
import './Tasks.css'

const groupNames: Record<Group, string> = {
  sort: 'To sort',
  left: 'Left from earlier',
  late: 'Overdue',
  today: 'Today',
  week: 'This week',
  later: 'Later',
  soon: 'Soon',
  someday: 'Someday',
}

// What the head of a view says of it.
function about(view: View, now: Now) {
  if (view === 'today') return `Today is ${day(now.date)}.`
  if (view === 'week') return `The rest of this week, to ${day(week(now)[1])}, and what lies beyond it.`
  return 'By the first date each has, then those with none, the oldest first.'
}

// Tasks: a row of views over the open tasks, read once and sorted into each view's groups by the server's today. The view,
// the area chosen and the task open live in the address, so a reload keeps them.
export default function Tasks() {
  const narrow = useNarrow()
  const navigate = useNavigate()
  const id = Number(useParams().id) || undefined
  const { state } = useLocation()
  const [params, setParams] = useSearchParams()
  const view = views.find(([v]) => v === params.get('view'))?.[0] ?? 'today'
  const area = params.get('area') ?? ''
  const search = params.toString() ? `?${params}` : ''
  const areas = useAreas(true)
  const { list, failed, took } = useOpen()
  const opened = useTask(id)
  const [refused, setRefused] = useState('')
  const [added, setAdded] = useState<number>()
  // The tasks marked done in this view, as they were, so each stays where it stood, struck through, until the view is left.
  const [struck, setStruck] = useState<{ view: View; rows: Row[] }>({ view, rows: [] })
  if (struck.view !== view) setStruck({ view, rows: [] })
  const done = struck.rows
  // A task open as a page of its own on a phone shows no list; the list still gives the server's today.
  const alone = narrow && !!id

  const here = (t: Row) => !area || t.area === area
  const open = list?.tasks.filter(here) ?? []
  const shown = [...open, ...done.filter((t) => here(t) && !open.some((x) => x.id === t.id))].sort(order)
  const groups = list ? sortInto(view, shown, list.now) : []
  // On desktop the view opens its first task by naming it in the address.
  const first = groups.flatMap(([, ts]) => ts)[0]?.id
  useEffect(() => {
    if (!narrow && !id && first) void navigate(`/tasks/${first}${search}`, { replace: true })
  }, [narrow, id, first, search, navigate])

  const to = (v: View) => {
    const q = new URLSearchParams(Object.entries({ view: v === 'today' ? '' : v, area }).filter(([, x]) => x))
    return q.size ? `/tasks?${q}` : '/tasks'
  }

  if (!list)
    return (
      failed && (
        <div className="page">
          <p role="alert" className="err">
            {failed}
          </p>
        </div>
      )
    )
  const now = list.now
  const isDone = (t: Row) => done.some((x) => x.id === t.id) && !open.some((x) => x.id === t.id)

  // Sends a change of a task and places the task as the answer has it, a task marked done staying where it is shown;
  // gives the server's line when it is refused.
  async function change(t: Row, fields: Fields): Promise<string> {
    const as = shown.find((x) => x.id === t.id)
    try {
      const answer = await api<Task>('PATCH', `/tasks/${t.id}`, fields)
      took(answer)
      opened.show(answer)
      if (fields.done !== undefined) setStruck((s) => ({ ...s, rows: [...s.rows.filter((x) => x.id !== t.id), ...(fields.done && as ? [as] : [])] }))
      return ''
    } catch (e) {
      return message(e)
    }
  }
  const tap = async (t: Row, fields: Fields) => {
    setRefused('')
    setRefused(await change(t, fields))
  }

  const row = (t: Row, g: Group) => {
    const struckOut = isDone(t)
    const one = t.planned_to === t.planned_from
    const bits = [
      !area && areas.find((a) => a.slug === t.area)?.kind === 'project' && nameOf(areas, t.area),
      g === 'sort' && from(t.source),
      g === 'sort' && ago(t.created_at),
      t.planned_from && (g === 'today' && one ? t.planned_time : `${period(t.planned_from, t.planned_to)}${t.planned_time && ' ' + t.planned_time}`),
      t.deadline && <span className={t.deadline < now.date ? 'late' : undefined}>due {day(t.deadline)}</span>,
      t.list && t.list !== g && t.list,
    ].filter(Boolean)
    const quick: [string, Fields][] =
      g === 'sort'
        ? [
            ['Today', { planned_from: now.date }],
            ['Soon', { list: 'soon' }],
            ['Someday', { list: 'someday' }],
          ]
        : g === 'left'
          ? [['Move to today', { planned_from: now.date }]]
          : []
    return (
      <div key={t.id} className={'task-row' + (t.id === id ? ' on' : '') + (struckOut ? ' done' : '') + (t.id === added ? ' new' : '')}>
        <Check done={struckOut} sort={t.state === 'unsorted'} onClick={() => void tap(t, { done: !struckOut })} />
        <div className="bd">
          <Link className="tl" to={`/tasks/${t.id}${search}`} state={t.id === id ? state : back} aria-current={t.id === id || undefined}>
            <span className="t1">{t.title}</span>
            {bits.length > 0 && (
              <span className="t2">
                {bits.map((b, i) => (
                  <span key={i}>{b}</span>
                ))}
              </span>
            )}
          </Link>
          {!struckOut && quick.length > 0 && (
            <div className="qa">
              {quick.map(([label, fields]) => (
                <button key={label} className="btn sm" onClick={() => void tap(t, fields)}>
                  {label}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    )
  }

  let listed
  if (list.tasks.length === 0 && done.length === 0)
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
          <div className="list">{ts.map((t) => row(t, g))}</div>
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

  const task = opened.task ? (
    <TaskView key={opened.task.id} task={opened.task} now={now} areas={areas} onChange={(f) => change(opened.task!, f)} />
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
          <PageHead title="Tasks">{select}</PageHead>
          <p className="sub">{about(view, now)}</p>
        </>
      )}
      <div className={narrow ? undefined : 'panes'}>
        {!alone && (
          <div>
            <div className="views">
              {views.map(([v, label]) => {
                const n = sortInto(v, v === view ? shown : open, now).reduce((sum, [, ts]) => sum + ts.length, 0)
                return (
                  <Link key={v} to={to(v)} aria-current={v === view ? 'page' : undefined}>
                    {label}
                    {n > 0 && <span className="dim"> {n}</span>}
                  </Link>
                )
              })}
            </div>
            {view !== 'week' && (
              <AddLine
                key={view}
                view={view}
                area={area && nameOf(areas, area)}
                fields={{ ...(view === 'today' ? { planned_from: now.date } : { list: view }), ...(area && { area }) }}
                onAdded={(t) => {
                  took(t)
                  setAdded(t.id)
                  // On desktop the task added opens beside the list; the line keeps the focus for the next.
                  if (!narrow) void navigate(`/tasks/${t.id}${search}`, { state: back })
                }}
              />
            )}
            {refused && (
              <p role="alert" className="err">
                {refused}
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

// The line that adds a task where one stands, given what that puts on it: planned for today, on the list, in the area.
function AddLine({ view, area, fields, onAdded }: { view: View; area: string; fields: Fields; onAdded: (t: Row) => void }) {
  const [title, setTitle] = useState('')
  const [sending, setSending] = useState(false)
  const [refused, setRefused] = useState('')
  async function add(e: FormEvent) {
    e.preventDefault()
    setRefused('')
    setSending(true)
    try {
      const r = await api<{ tasks: Row[] }>('POST', '/tasks', { tasks: [{ title: title.trim(), ...fields }] })
      setTitle('')
      onAdded(r.tasks[0])
    } catch (err) {
      setRefused(message(err))
    }
    setSending(false)
  }
  const where = view === 'today' ? 'for today' : `to ${views.find(([v]) => v === view)![1]}`
  return (
    <form className="addl" onSubmit={(e) => void add(e)}>
      <label className="addl-field">
        <AddIcon />
        <input aria-label="New task" placeholder={`Add a task ${where}${area && `, in ${area}`}`} autoComplete="off" value={title} onChange={(e) => setTitle(e.target.value)} />
      </label>
      <button className="btn" disabled={sending || !title.trim()}>
        Add
      </button>
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
    </form>
  )
}
