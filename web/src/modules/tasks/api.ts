import { useEffect, useRef, useState } from 'react'
import { api, message } from '../../api/client.ts'
import { useStream } from '../../api/stream.ts'

// A task's state: done, or unsorted while it is open with no list, no plan and no deadline, or else open.
export type State = 'open' | 'unsorted' | 'done'

// A task as GET /api/tasks lists it. The dates are YYYY-MM-DD and "" when it has none, a plan of one day has its to equal
// to its from, planned_time is HH:MM, area is the slug, and source that of the last change, web or the name of a token.
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
  list: '' | 'next' | 'someday'
  state: State
  done_at: string | null
  has_notes: boolean
  source: string
  created_at: string
  changed_at: string
}

// An entry of a task's log, the newest first: what each field was and became, as strings, an area as its slug and done as
// "false" or "true"; the notes and the links give neither. The oldest is the making, each field it was given from "".
export type Entry = { at: string; source: string; said: string; changes: { field: string; from?: string; to?: string }[] }

// A task as GET /api/tasks/{id} gives it, in full: with its notes, the owner's own words when a client gave them, and its log.
export type Task = Row & { notes: string; said: string; log: Entry[] }

// The server's today, in its time zone, and the first day of its week, a weekday's name in lower case.
export type Now = { date: string; weekday: string; time: string; timezone: string; week_start: string }

export type List = { tasks: Row[]; total: number; now: Now }

// What a change of a task may send; a change sends only what it changes.
export type Fields = Partial<Pick<Task, 'title' | 'notes' | 'area' | 'planned_from' | 'planned_to' | 'planned_time' | 'deadline' | 'list'>> & { done?: boolean }

// Reads when the page mounts, when the stream opens again, on any change of a task and on any change of an area, whose
// slug names the tasks in it.
export function useTasks(read: () => void) {
  useStream((changes) => {
    if (!changes || changes.some((c) => c.module === 'tasks' || (c.module === 'core' && c.kind === 'area'))) read()
  })
}

// The open tasks, 500 at most, as useTasks reads them. took places a task as a change answered it, at once and before
// the stream tells of it, and takes it out once done; a read asked before it is dropped. A page sorts what it shows.
export function useOpen() {
  const [list, setList] = useState<List>()
  const [failed, setFailed] = useState('')
  const asked = useRef(0)
  useTasks(() => {
    const n = ++asked.current
    api<List>('GET', '/tasks?limit=500').then(
      (l) => {
        if (n !== asked.current) return
        setList(l)
        setFailed('')
      },
      (e: unknown) => n === asked.current && setFailed(message(e)),
    )
  })
  const took = (t: Row) => {
    asked.current++
    setList((l) => {
      if (!l) return l
      const rest = l.tasks.filter((x) => x.id !== t.id)
      return { ...l, tasks: t.state === 'done' ? rest : [...rest, t] }
    })
  }
  return { list, failed, took }
}

// The task id names, in full, read as useTasks reads and when id changes. show puts a change's answer in its place while
// that task is the one named, and a read asked before it is dropped.
export function useTask(id?: number) {
  const [task, setTask] = useState<Task>()
  const [failed, setFailed] = useState('')
  const [tick, setTick] = useState(0)
  const asked = useRef(0)
  const named = useRef(id)
  useEffect(() => {
    named.current = id
  })
  useTasks(() => setTick((n) => n + 1))
  useEffect(() => {
    if (!tick || !id) return
    const n = ++asked.current
    api<Task>('GET', `/tasks/${id}`).then(
      (t) => {
        if (n !== asked.current) return
        setTask(t)
        setFailed('')
      },
      (e: unknown) => n === asked.current && setFailed(message(e)),
    )
  }, [id, tick])
  const show = (t: Task) => {
    if (t.id !== named.current) return
    asked.current++
    setTask(t)
  }
  return { task: task?.id === id ? task : undefined, failed, show }
}

// The name of the area a task names by its slug.
export const nameOf = (areas: { slug: string; name: string }[], slug: string) => areas.find((a) => a.slug === slug)?.name ?? slug

// Where a change came from, as a list shows it.
export const from = (source: string) => (source === 'web' ? 'web' : `via ${source}`)

// The class of a task's mark, a ring: open, filled once done, and in the yellow of what waits while it is to sort.
export const ring = (state: State) => 'mk m-task' + (state === 'done' ? ' t-done' : state === 'unsorted' ? ' t-sort' : '')
