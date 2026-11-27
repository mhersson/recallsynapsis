import { useEffect, useRef, useState } from 'react'
import { ApiError, api, message } from '../../api/client.ts'
import { useStream } from '../../api/stream.ts'
import type { Linked } from '../../api/types.ts'
import type { Named } from '../registry.ts'

// A task's state: done, or unsorted while it is open with no list, no plan and no deadline, or else open.
export type State = 'open' | 'unsorted' | 'done'

// A task as GET /api/tasks lists it. The dates are YYYY-MM-DD and "" when it has none, a plan of one day has its to equal
// to its from, planned_time is HH:MM, area is the slug, source that of the last change, web or the name of a token, and
// trashed_at null unless it is in the trash.
export type Row = {
  id: number
  ref: string
  area: string
  title: string
  planned_from: string
  planned_from_day: string
  planned_to: string
  planned_to_day: string
  planned_time: string
  deadline: string
  deadline_day: string
  list: '' | 'soon' | 'someday'
  state: State
  done_at: string | null
  has_notes: boolean
  source: string
  created_at: string
  changed_at: string
  trashed_at: string | null
}

// An entry of a task's log, the newest first: what each field was and became, as strings, an area as its slug and done as
// "false" or "true"; the notes and the links give neither. The oldest is the making, each field it was given from "".
export type Entry = { at: string; source: string; said: string; changes: { field: string; from?: string; to?: string }[] }

// A task as GET /api/tasks/{id} gives it, in full: with its notes, the owner's own words when a client gave them, its links
// and the items that link to it, as the core resolves them, its log, and the server's today.
export type Task = Row & { notes: string; said: string; links: Linked[]; linked_from: Linked[]; log: Entry[]; now: Now }

// The server's today, in its time zone, and the first day of its week, a weekday's name in lower case.
export type Now = { date: string; weekday: string; time: string; timezone: string; week_start: string }

export type List = { tasks: Row[]; total: number; now: Now }

// The kinds of notification, as GET /api/tasks/notify gives them: each on or off, and the hour, HH:MM, of each that comes
// once a day. They hold for every browser of the user.
export type Kind = 'start' | 'morning' | 'deadline' | 'untimed'
export type Hour = 'morning_at' | 'deadline_at' | 'untimed_at'
export type Kinds = Record<Kind, boolean> & Record<Hour, string>

// What a change of a task may send; a change sends only what it changes, and links whole, as references.
export type Fields = Partial<Pick<Task, 'title' | 'notes' | 'area' | 'planned_from' | 'planned_to' | 'planned_time' | 'deadline' | 'list'>> & { done?: boolean; links?: string[] }

// Makes one task, with the owner's words when given, and gives it as a list gives it; a refusal throws.
export const make = (t: Fields, said = '') => api<{ tasks: Row[] }>('POST', '/tasks', { tasks: [t], ...(said && { said }) }).then((r) => r.tasks[0])

// The server's today, read once with a list of one task, for a form that reads no task.
export function useNow() {
  const [now, setNow] = useState<Now>()
  useEffect(() => {
    api<List>('GET', '/tasks?limit=1').then(
      (l) => setNow(l.now),
      () => {},
    )
  }, [])
  return now
}

// A task as a link names it.
export const made = (t: Row): Named => ({ ref: t.ref, kind: 'task', state: t.state, title: t.title })

// A text as a task takes it: its first line the title, and what follows the notes.
export function split(text: string): { title: string; notes?: string } {
  const [title, ...rest] = text.trim().split('\n')
  const notes = rest.join('\n').trim()
  return { title: title.trim(), ...(notes && { notes }) }
}

// Reads when the page mounts, when the stream opens again, on any change of a task and on any change of an area, whose
// slug names the tasks in it.
export function useTasks(read: () => void) {
  useStream((changes) => {
    if (!changes || changes.some((c) => c.module === 'tasks' || (c.module === 'core' && c.kind === 'area'))) read()
  })
}

async function openTasks(signal: AbortSignal): Promise<List> {
  const tasks = new Map<number, Row>()
  let offset = 0
  for (;;) {
    const page = await api<List>('GET', '/tasks?limit=500' + (offset ? `&offset=${offset}` : ''), undefined, signal)
    for (const t of page.tasks) tasks.set(t.id, t)
    offset += page.tasks.length
    if (offset >= page.total || !page.tasks.length) return { ...page, tasks: [...tasks.values()] }
  }
}

// A local write takes precedence over any earlier read until the stream refreshes the list.
export function useOpen(on = true) {
  const [list, setList] = useState<List>()
  const [failed, setFailed] = useState('')
  const [tick, setTick] = useState(0)
  const asked = useRef(0)
  useTasks(() => setTick((n) => n + 1))
  useEffect(() => {
    if (!on || !tick) return
    const n = ++asked.current
    const controller = new AbortController()
    openTasks(controller.signal).then(
      (l) => {
        if (n !== asked.current) return
        setList(l)
        setFailed('')
      },
      (e: unknown) => !controller.signal.aborted && n === asked.current && setFailed(message(e)),
    )
    return () => controller.abort()
  }, [on, tick])
  const took = (t: Row) => {
    asked.current++
    setList((l) => {
      if (!l) return l
      const rest = l.tasks.filter((x) => x.id !== t.id)
      const tasks = t.state === 'done' || t.trashed_at ? rest : [...rest, t]
      return { ...l, tasks, total: tasks.length }
    })
  }
  return { list, failed, took }
}

const step = 50

// One row carries the total for a badge. A list starts at 50 and can grow to 500.
export function useMore(asked: string, holds: (t: Row) => boolean, countOnly = false) {
  const [shown, setShown] = useState({ asked, limit: step })
  const limit = countOnly ? 1 : shown.asked === asked ? shown.limit : step
  const [got, setGot] = useState<{ asked: string; list: List }>()
  const [failed, setFailed] = useState('')
  const [tick, setTick] = useState(0)
  const n = useRef(0)
  useTasks(() => setTick((x) => x + 1))
  useEffect(() => {
    if (!tick || !asked) return
    const at = ++n.current
    api<List>('GET', `/tasks?${asked}&limit=${limit}`).then(
      (list) => {
        if (at !== n.current) return
        setGot({ asked, list })
        setFailed('')
      },
      (e: unknown) => at === n.current && setFailed(message(e)),
    )
  }, [asked, limit, tick])
  const took = (t: Row) => {
    n.current++
    setGot((g) => {
      if (!g) return g
      const rest = g.list.tasks.filter((x) => x.id !== t.id)
      const tasks = holds(t) ? [t, ...rest] : rest
      return { ...g, list: { ...g.list, tasks, total: g.list.total + tasks.length - g.list.tasks.length } }
    })
  }
  const more = limit < 500 ? () => setShown({ asked, limit: limit + step }) : undefined
  return { list: got?.asked === asked ? got.list : undefined, failed, more, took, again: () => setTick((x) => x + 1) }
}

// The task id names, in full, read when the page mounts, on every change the stream tells of, since what it links to may
// be any module's item and the core tells only of areas, and when id changes. show puts a change's answer in its place
// while that task is the one named, and a read asked before it is dropped.
export function useTask(id?: number) {
  const [task, setTask] = useState<Task>()
  const [failed, setFailed] = useState('')
  const [tick, setTick] = useState(0)
  const asked = useRef(0)
  const named = useRef(id)
  useEffect(() => {
    named.current = id
  })
  useStream(() => setTick((n) => n + 1))
  useEffect(() => {
    if (!tick || !id) return
    const n = ++asked.current
    api<Task>('GET', `/tasks/${id}`).then(
      (t) => {
        if (n !== asked.current) return
        setTask(t)
        setFailed('')
      },
      (e: unknown) => {
        if (n !== asked.current) return
        // A task deleted meanwhile, as by emptying the trash elsewhere, closes to say so.
        if (e instanceof ApiError && e.status === 404) setTask(undefined)
        setFailed(message(e))
      },
    )
  }, [id, tick])
  const show = (t: Task) => {
    if (t.id !== named.current) return
    asked.current++
    setTask(t)
  }
  return { task: task?.id === id ? task : undefined, failed, show }
}

// The class of a task's mark after mk, a ring: open, filled once done, and in the yellow of what waits while it is to sort.
export const ring = (state: State) => 'm-task' + (state === 'done' ? ' t-done' : state === 'unsorted' ? ' t-sort' : '')
