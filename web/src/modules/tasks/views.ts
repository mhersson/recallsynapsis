import type { Now, Row } from './api.ts'
import { week } from './dates.ts'

export type View = 'agenda' | 'today' | 'week' | 'soon' | 'someday' | 'done'

export type Group = 'scheduled' | 'unscheduled' | 'sort' | 'left' | 'late' | 'today' | 'week' | 'later' | 'soon' | 'someday'

export const groupNames: Record<Group, string> = {
  scheduled: 'Scheduled',
  unscheduled: 'Unscheduled',
  sort: 'To sort',
  left: 'Left from earlier',
  late: 'Overdue',
  today: 'Today',
  week: 'This week',
  later: 'Later',
  soon: 'Soon',
  someday: 'Someday',
}

// The views in the order of their row, each with its name and its groups in order.
export const views: [View, string, Group[]][] = [
  ['agenda', 'Agenda', ['scheduled', 'unscheduled']],
  ['today', 'Today', ['sort', 'left', 'late', 'today']],
  ['week', 'Week', ['week', 'later']],
  ['soon', 'Soon', ['soon']],
  ['someday', 'Someday', ['someday']],
  ['done', 'Done', []],
]

// Whether a task belongs in a group on the server's today, end being the last day of its week.
function holds(g: Group, t: Row, today: string, end: string) {
  const from = t.planned_from
  const to = t.planned_to
  const due = t.deadline
  switch (g) {
    case 'scheduled':
      return !!(from || due)
    case 'unscheduled':
      return !from && !due
    case 'sort':
      return t.state === 'unsorted'
    case 'left':
      return !!from && to < today
    case 'late':
      return !!due && due < today
    case 'today':
      return (!!from && from <= today && today <= to) || due === today
    case 'week':
      return (from > today && from <= end) || (due > today && due <= end)
    case 'later':
      return from > end || due > end
    default:
      return t.list === g
  }
}

// An area's page uses the detailed groups. Each task belongs to its first matching group.
export function sortInto(view: View | 'all', tasks: Row[], now: Now): [Group, Row[]][] {
  const end = week(now)[1]
  const placed = new Set<number>()
  const groups = view === 'all' ? views.flatMap(([v, , gs]) => (v === 'agenda' ? [] : gs)) : views.find(([v]) => v === view)![2]
  return groups.map((g) => {
    const held = tasks.filter((t) => !placed.has(t.id) && holds(g, t, now.date, end))
    held.forEach((t) => placed.add(t.id))
    return [g, held]
  })
}

// The order of the server's lists of open tasks: by the first day that matters, the planned from or else the deadline,
// those with a time first on it, those with no date last, and the oldest first among equals, which the id tells, as
// tasks made in one call share their time of making.
const key = (t: Row) => `${t.planned_from || t.deadline || '~'} ${t.planned_time || '~'}`
export const order = (a: Row, b: Row) => (key(a) < key(b) ? -1 : key(a) > key(b) ? 1 : a.id - b.id)

// The order of the done: the most recently done first.
export const latest = (a: Row, b: Row) => Date.parse(b.done_at!) - Date.parse(a.done_at!)
